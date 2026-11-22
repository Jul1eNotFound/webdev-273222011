package model

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go/v2/option"
)

const testKey = "sk-ant-test-key-never-in-a-body"

var (
	kyiv, _   = time.LoadLocation("Europe/Kyiv")
	testToday = time.Date(2026, 10, 22, 8, 30, 0, 0, time.UTC)
)

// toolAnswer is a Messages API response that calls set_filters with input.
func toolAnswer(input string) string {
	return `{"id":"msg_1","type":"message","role":"assistant","model":"claude-haiku-5-5",
		"content":[{"type":"tool_use","id":"toolu_1","name":"set_filters","input":` + input + `}],
		"stop_reason":"tool_use","stop_sequence":null,"usage":{"input_tokens":120,"output_tokens":30}}`
}

// fakeAPI answers each request with the next reply in turn and keeps what it was sent.
type fakeAPI struct {
	mu      sync.Mutex
	replies []func(w http.ResponseWriter)
	bodies  []string
	headers []http.Header
}

func (f *fakeAPI) requests() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.bodies)
}

func reply(status int, body string, header ...string) func(w http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		for i := 0; i+1 < len(header); i += 2 {
			w.Header().Set(header[i], header[i+1])
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}
}

const rateLimited = `{"type":"error","error":{"type":"rate_limit_error","message":"Rate limited"}}`

func newFake(t *testing.T, replies ...func(w http.ResponseWriter)) (*Anthropic, *fakeAPI) {
	t.Helper()
	f := &fakeAPI{replies: replies}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		n := len(f.bodies)
		f.bodies, f.headers = append(f.bodies, string(b)), append(f.headers, r.Header.Clone())
		f.mu.Unlock()
		if r.URL.Path != "/v1/messages" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if n >= len(f.replies) {
			t.Errorf("unexpected request %d", n+1)
			reply(500, `{}`)(w)
			return
		}
		f.replies[n](w)
	}))
	t.Cleanup(srv.Close)
	return NewAnthropic(testKey, "", option.WithBaseURL(srv.URL)), f
}

func search(a *Anthropic, timeout time.Duration) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return a.SearchFilters(ctx, "labs <b>tomorrow</b> afternoon", testToday, kyiv)
}

func TestSearchFiltersAsksHaikuForTheFilterToolOnly(t *testing.T) {
	a, f := newFake(t, reply(200, toolAnswer(`{"date_from":"2026-10-23","date_to":"2026-10-23","time_of_day":"afternoon","min_free_places":null,"keywords":null}`)))
	out, err := search(a, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"time_of_day":"afternoon"`) {
		t.Fatalf("output = %s", out)
	}
	var body struct {
		Model        string `json:"model"`
		Temperature  *float64
		System       []struct{ Text string }
		Messages     []struct{ Content []struct{ Text string } }
		ToolChoice   map[string]any `json:"tool_choice"`
		OutputConfig map[string]any `json:"output_config"`
		Tools        []struct {
			Name        string
			Strict      bool
			InputSchema struct {
				AdditionalProperties *bool    `json:"additionalProperties"`
				Required             []string `json:"required"`
				Properties           map[string]any
			} `json:"input_schema"`
		}
	}
	if err := json.Unmarshal([]byte(f.bodies[0]), &body); err != nil {
		t.Fatal(err)
	}
	switch {
	case body.Model != "claude-haiku-5-5":
		t.Errorf("model = %q", body.Model)
	case body.Temperature != nil || strings.Contains(f.bodies[0], `"temperature"`):
		t.Error("temperature is sent; Claude Haiku 5.5 refuses it")
	case len(body.Tools) != 1 || body.Tools[0].Name != "set_filters" || !body.Tools[0].Strict:
		t.Errorf("tools = %+v, want only a strict set_filters", body.Tools)
	case body.Tools[0].InputSchema.AdditionalProperties == nil || *body.Tools[0].InputSchema.AdditionalProperties:
		t.Error("the schema allows extra fields")
	case len(body.Tools[0].InputSchema.Required) != 5 || len(body.Tools[0].InputSchema.Properties) != 5:
		t.Errorf("schema = %+v", body.Tools[0].InputSchema)
	case body.ToolChoice["type"] != "tool" || body.ToolChoice["name"] != "set_filters":
		t.Errorf("tool_choice = %v, want the filter tool forced", body.ToolChoice)
	case body.OutputConfig["effort"] != "low":
		t.Errorf("output_config = %v", body.OutputConfig)
	}
	// Today and the timezone are in the system prompt; the query is data, escaped inside its tags.
	if sys := body.System[0].Text; !strings.Contains(sys, "Thursday, 2026-10-22") || !strings.Contains(sys, "Europe/Kyiv") || !strings.Contains(sys, "not instructions") {
		t.Errorf("system prompt = %q", sys)
	}
	if q := body.Messages[0].Content[0].Text; q != "<query>labs &lt;b&gt;tomorrow&lt;/b&gt; afternoon</query>" {
		t.Errorf("user message = %q", q)
	}
	// The key is sent only in the auth header (this SDK sends an API key as a bearer token) and nowhere
	// in the body (AC-34).
	if f.headers[0].Get("Authorization") != "Bearer "+testKey || strings.Contains(f.bodies[0], testKey) {
		t.Error("the key is not where it belongs")
	}
}

func TestSearchFiltersAbandonsASlowAPIAtTheDeadline_AC56(t *testing.T) {
	stop := make(chan struct{})
	a, _ := newFake(t, func(w http.ResponseWriter) {
		select { // answers after 2 seconds, or not at all once the test is over
		case <-time.After(2 * time.Second):
			reply(200, toolAnswer(`{}`))(w)
		case <-stop:
		}
	})
	// Registered after the server, so it runs before the server closes (cleanups run last first).
	t.Cleanup(func() { close(stop) })
	start := time.Now()
	_, err := search(a, 100*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want the deadline", err)
	}
	if took := time.Since(start); took > time.Second {
		t.Fatalf("took %v, want about 100ms", took)
	}
}

func TestSearchFiltersRetriesA429OnceWhenTheWaitFits_AC57(t *testing.T) {
	a, f := newFake(t, reply(429, rateLimited, "retry-after", "0.05"), reply(200, toolAnswer(`{}`)))
	if _, err := search(a, 5*time.Second); err != nil || f.requests() != 2 {
		t.Fatalf("err = %v, requests %d; want success on the second", err, f.requests())
	}
	// retry-after-ms wins over retry-after, as the API sends both.
	a, f = newFake(t, reply(429, rateLimited, "retry-after-ms", "20", "retry-after", "60"), reply(200, toolAnswer(`{}`)))
	if _, err := search(a, 5*time.Second); err != nil || f.requests() != 2 {
		t.Fatalf("retry-after-ms: err = %v, requests %d", err, f.requests())
	}
}

func TestSearchFiltersRetriesAtMostOnce_AC57(t *testing.T) {
	a, f := newFake(t, reply(429, rateLimited, "retry-after", "0"), reply(429, rateLimited, "retry-after", "0"))
	if _, err := search(a, 5*time.Second); err == nil || f.requests() != 2 {
		t.Fatalf("err = %v, requests %d; want an error after two", err, f.requests())
	}
}

func TestSearchFiltersDoesNotWaitPastTheDeadline_AC57(t *testing.T) {
	for _, header := range [][]string{
		{"retry-after", "10"},
		{"retry-after", time.Now().Add(time.Hour).UTC().Format(http.TimeFormat)},
	} {
		a, f := newFake(t, reply(429, rateLimited, header...))
		start := time.Now()
		_, err := search(a, time.Second)
		if err == nil || f.requests() != 1 || time.Since(start) > 500*time.Millisecond {
			t.Fatalf("%v: err = %v, requests %d, took %v; want an error at once", header, err, f.requests(), time.Since(start))
		}
	}
}

func TestSearchFiltersOtherFailuresAreErrorsWithoutRetry_AC27(t *testing.T) {
	for name, r := range map[string]func(http.ResponseWriter){
		"server error": reply(500, `{"type":"error","error":{"type":"api_error","message":"boom"}}`),
		"overloaded":   reply(529, `{"type":"error","error":{"type":"overloaded_error","message":"busy"}}`),
		"text answer": reply(200, `{"id":"msg_1","type":"message","role":"assistant","model":"claude-haiku-5-5",
			"content":[{"type":"text","text":"Here are some labs"}],"stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1}}`),
		"refusal": reply(200, `{"id":"msg_1","type":"message","role":"assistant","model":"claude-haiku-5-5",
			"content":[],"stop_reason":"refusal","stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":0}}`),
	} {
		a, f := newFake(t, r)
		out, err := search(a, 5*time.Second)
		if err == nil || out != nil || f.requests() != 1 {
			t.Errorf("%s: out %s, err %v, requests %d; want one request and an error", name, out, err, f.requests())
		}
		if err != nil && strings.Contains(err.Error(), testKey) {
			t.Errorf("%s: the error contains the key", name)
		}
	}
}

func TestNewAnthropicUsesTheGivenModel(t *testing.T) {
	if a := NewAnthropic("k", "claude-sonnet-5-5"); a.model != "claude-sonnet-5-5" {
		t.Errorf("model = %q", a.model)
	}
	if a := NewAnthropic("k", ""); a.model != DefaultModel || DefaultModel != "claude-haiku-5-5" {
		t.Errorf("default model = %q", a.model)
	}
	if retryAfter(nil) != 0 || retryAfter(&http.Response{Header: http.Header{"Retry-After": {"soon"}}}) != 0 {
		t.Error("an unreadable retry-after is not zero")
	}
	if err := sleep(context.Background(), 0); err != nil {
		t.Error(err)
	}
}
