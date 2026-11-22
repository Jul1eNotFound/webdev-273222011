package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go/v2/option"

	"github.com/Jul1eNotFound/webdev-273222011/internal/handler"
	"github.com/Jul1eNotFound/webdev-273222011/internal/model"
	"github.com/Jul1eNotFound/webdev-273222011/internal/service"
	"github.com/Jul1eNotFound/webdev-273222011/internal/service/servicetest"
)

const (
	msgSearchUnavailable = "Smart search is unavailable, showing all labs."
	msgSearchTooLong     = "Please keep your search to 200 characters or fewer."
	msgNoMatch           = "No labs match these filters"
	searchBox            = `<input id="q" name="q" type="search" maxlength="200"`
	tomorrowOnly         = "/labs?from=2026-10-23&to=2026-10-23"
)

// searchModel is a fake model for the handler tests.
type searchModel struct {
	mu    sync.Mutex
	out   string
	err   error
	block bool
	calls int
}

func (m *searchModel) SearchFilters(ctx context.Context, _ string, _ time.Time, _ *time.Location) (json.RawMessage, error) {
	m.mu.Lock()
	m.calls++
	m.mu.Unlock()
	if m.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return json.RawMessage(m.out), m.err
}

func (m *searchModel) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

func withSearch(m service.SearchModel, timeout time.Duration) func(*handler.Deps) {
	return func(d *handler.Deps) {
		d.Labs.WithSearchModel(m)
		if timeout > 0 {
			d.Labs.WithSearchTimeout(timeout)
		}
	}
}

// newSearchApp is newStaffApp's data with a fake model. At 08:30: Chemistry basics today 09:00–12:00,
// Circuits workshop today 13:00–17:00, Robotics intro tomorrow 09:00–12:00.
func newSearchApp(t *testing.T, out string) (string, *servicetest.Store, *searchModel) {
	t.Helper()
	_, st := newStaffApp(t)
	m := &searchModel{out: out}
	return newServer(t, st, st, withSearch(m, 0)).URL, st, m
}

func searchFor(t *testing.T, c *http.Client, base, query string) response {
	t.Helper()
	return do(t, c, http.MethodGet, base+"/labs?"+url.Values{"q": {query}}.Encode(), nil)
}

func TestAC31_LabsTomorrowShowsOnlyTomorrowWithADateChip(t *testing.T) {
	base, _, m := newSearchApp(t, `{"date_from":"2026-10-23","date_to":"2026-10-23","time_of_day":null,"min_free_places":null,"keywords":null}`)
	c := signedIn(t, base, studentEmail, studentPass)
	wantStatus(t, do(t, c, http.MethodGet, base+"/labs", nil), http.StatusOK, searchBox)
	wantRedirect(t, searchFor(t, c, base, "labs tomorrow"), http.StatusSeeOther, tomorrowOnly)

	r := do(t, c, http.MethodGet, base+tomorrowOnly, nil)
	for _, want := range []string{"<title>Search results · Labwiz</title>", "<h1>Search results</h1>", "Robotics intro", "Fri 23 Oct",
		`<input id="from" name="from" type="date" value="2026-10-23">`, `<input id="to" name="to" type="date" value="2026-10-23">`} {
		wantStatus(t, r, http.StatusOK, want)
	}
	if strings.Contains(r.body, "Chemistry basics") || strings.Contains(r.body, "Circuits workshop") {
		t.Error("today's labs are shown")
	}
	// A reload carries the filters in the URL: the model is asked once.
	do(t, c, http.MethodGet, base+tomorrowOnly, nil)
	if m.count() != 1 {
		t.Fatalf("model calls = %d, want 1", m.count())
	}
}

func TestAC55_RemovingOrEditingAChipUpdatesTheList(t *testing.T) {
	base, _, m := newSearchApp(t, `{}`)
	c := signedIn(t, base, studentEmail, studentPass)
	// The week, afternoons only: Circuits workshop.
	r := do(t, c, http.MethodGet, base+"/labs?from=2026-10-22&to=2026-10-28&time=afternoon", nil)
	wantStatus(t, r, http.StatusOK, "Circuits workshop")
	if strings.Contains(r.body, "Robotics intro") {
		t.Fatal("a morning lab is shown")
	}
	// Removing the time-of-day chip widens the list.
	remove := `<a href="/labs?from=2026-10-22&amp;to=2026-10-28">Remove time of day</a>`
	wantStatus(t, r, http.StatusOK, remove)
	wide := do(t, c, http.MethodGet, base+"/labs?from=2026-10-22&to=2026-10-28", nil)
	for _, want := range []string{"Chemistry basics", "Circuits workshop", "Robotics intro"} {
		wantStatus(t, wide, http.StatusOK, want)
	}
	// Editing chips, as the form submits them: mornings with at least 8 free places are Chemistry basics
	// (10 places, 2 booked) and Robotics intro (8 places); at least 9 is none of them.
	edit := func(min string) response {
		return do(t, c, http.MethodGet, base+"/labs?"+url.Values{"from": {"2026-10-22"}, "to": {"2026-10-28"}, "time": {"morning"}, "min": {min}, "kw": {""}}.Encode(), nil)
	}
	eight := edit("8")
	for _, want := range []string{"Chemistry basics", "Robotics intro", `<option value="morning" selected>Morning</option>`, `<input id="min" name="min" type="number" min="1" inputmode="numeric" value="8">`} {
		wantStatus(t, eight, http.StatusOK, want)
	}
	if strings.Contains(eight.body, "Circuits workshop") {
		t.Error("an afternoon lab with 1 free place is shown")
	}
	wantStatus(t, edit("9"), http.StatusOK, msgNoMatch)
	// Removing the dates goes back to the next seven days.
	wantStatus(t, wide, http.StatusOK, `<a href="/labs?from=&amp;to=">Remove dates</a>`)
	week := do(t, c, http.MethodGet, base+"/labs?from=&to=", nil)
	wantStatus(t, week, http.StatusOK, `value="2026-10-28"`)
	if m.count() != 0 {
		t.Fatalf("editing chips called the model %d times", m.count())
	}
}

func TestAC59_NoMatchKeepsTheChips(t *testing.T) {
	base, _, _ := newSearchApp(t, `{}`)
	c := signedIn(t, base, studentEmail, studentPass)
	r := do(t, c, http.MethodGet, base+"/labs?from=2026-10-23&to=2026-10-23&kw=chemistry", nil)
	for _, want := range []string{msgNoMatch, `<input id="kw" name="kw" type="text" value="chemistry">`, "Remove keywords", "Apply filters"} {
		wantStatus(t, r, http.StatusOK, want)
	}
	if strings.Contains(r.body, "No labs available today") {
		t.Error("the plain empty state is shown")
	}
}

func TestAC27_AFailingModelShowsTodaysListWithASentence(t *testing.T) {
	for name, m := range map[string]*searchModel{
		"error":    {err: errors.New("429 rate limited")},
		"bad JSON": {out: `{"lab_id": 1}`}, // AC-32 over HTTP
		"too slow": {block: true},          // AC-56 over HTTP
	} {
		_, st := newStaffApp(t)
		base := newServer(t, st, st, withSearch(m, 50*time.Millisecond)).URL
		c := signedIn(t, base, studentEmail, studentPass)
		r := searchFor(t, c, base, "labs tomorrow")
		for _, want := range []string{msgSearchUnavailable, "Labs available today", "Chemistry basics", "Circuits workshop",
			`action="/sessions/1/book"`, `href="/bookings">My bookings</a>`, `value="labs tomorrow"`} {
			if r.status != http.StatusOK || !strings.Contains(r.body, want) {
				t.Errorf("%s: status %d, body lacks %q", name, r.status, want)
			}
		}
		// The rest of the page still works: booking from it.
		wantRedirect(t, bookByForm(t, c, base, "/sessions/1/book"), http.StatusSeeOther, "/labs/1?booked=1")
	}
}

func TestAC27_RateLimitedAPIFallsBackThroughTheRealClient_AC57(t *testing.T) {
	var requests int
	var mu sync.Mutex
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("retry-after", "30")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"type":"error","error":{"type":"rate_limit_error","message":"Rate limited"}}`)
	}))
	t.Cleanup(api.Close)
	_, st := newStaffApp(t)
	base := newServer(t, st, st, withSearch(model.NewAnthropic("sk-ant-test", "", option.WithBaseURL(api.URL)), 0)).URL
	c := signedIn(t, base, studentEmail, studentPass)
	start := time.Now()
	wantStatus(t, searchFor(t, c, base, "labs tomorrow"), http.StatusOK, msgSearchUnavailable)
	if requests != 1 || time.Since(start) > 2*time.Second {
		t.Fatalf("requests %d, took %v; want one request and no 30 second wait", requests, time.Since(start))
	}
}

func TestAC33_ALongQueryIsRefusedBeforeAnyCall(t *testing.T) {
	base, _, m := newSearchApp(t, `{}`)
	c := signedIn(t, base, studentEmail, studentPass)
	long := strings.Repeat("a", 201)
	r := searchFor(t, c, base, long)
	wantStatus(t, r, http.StatusBadRequest, msgSearchTooLong)
	wantStatus(t, r, http.StatusBadRequest, "Labs available today")
	wantStatus(t, searchFor(t, c, base, "   "), http.StatusBadRequest, "Please type what you are looking for.")
	if m.count() != 0 {
		t.Fatalf("model calls = %d, want 0", m.count())
	}
}

func TestAC58_TheEleventhSearchInAMinuteFallsBack(t *testing.T) {
	base, _, m := newSearchApp(t, `{}`)
	c := signedIn(t, base, studentEmail, studentPass)
	for i := range 10 {
		if r := searchFor(t, c, base, "labs"); r.status != http.StatusSeeOther {
			t.Fatalf("search %d: status %d", i+1, r.status)
		}
	}
	wantStatus(t, searchFor(t, c, base, "labs"), http.StatusOK, msgSearchUnavailable)
	if m.count() != 10 {
		t.Fatalf("model calls = %d, want 10", m.count())
	}
}

func TestAC34_TheAPIKeyNeverReachesTheBrowser(t *testing.T) {
	const key = "sk-ant-api03-SECRET-never-in-a-page"
	answers := []string{
		`{"id":"m","type":"message","role":"assistant","model":"claude-haiku-5-5","content":[{"type":"tool_use","id":"t","name":"set_filters","input":{"date_from":"2026-10-23","date_to":null,"time_of_day":null,"min_free_places":null,"keywords":null}}],"stop_reason":"tool_use","stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1}}`,
		// An error whose text would carry the key if anything echoed it.
		`{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key ` + key + `"}}`,
	}
	var n int
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if n > 0 {
			w.WriteHeader(http.StatusUnauthorized)
		}
		_, _ = io.WriteString(w, answers[min(n, 1)])
		n++
	}))
	t.Cleanup(api.Close)
	_, st := newStaffApp(t)
	base := newServer(t, st, st, withSearch(model.NewAnthropic(key, "", option.WithBaseURL(api.URL)), 0)).URL
	c := signedIn(t, base, studentEmail, studentPass)
	var seen []response
	seen = append(seen, searchFor(t, c, base, "labs tomorrow"), do(t, c, http.MethodGet, base+tomorrowOnly, nil),
		searchFor(t, c, base, "labs again"), do(t, c, http.MethodGet, base+"/labs", nil),
		do(t, c, http.MethodGet, base+"/static/booking.js", nil), do(t, c, http.MethodGet, base+"/static/style.css", nil))
	if seen[2].status != http.StatusOK || !strings.Contains(seen[2].body, msgSearchUnavailable) {
		t.Fatalf("a rejected key: status %d", seen[2].status)
	}
	for i, r := range seen {
		head := ""
		for k, v := range r.header {
			head += k + ": " + strings.Join(v, ",") + "\n"
		}
		if strings.Contains(r.body, "SECRET") || strings.Contains(head, "SECRET") {
			t.Errorf("response %d contains the key", i)
		}
	}
}

func TestSmartSearchIsHiddenWithoutAModel(t *testing.T) {
	base, _ := newStaffApp(t)
	c := signedIn(t, base, studentEmail, studentPass)
	r := do(t, c, http.MethodGet, base+"/labs", nil)
	if strings.Contains(r.body, `id="q"`) {
		t.Error("the search box is shown without a model")
	}
	// A hand-made search still falls back, and chips still work without a model.
	wantStatus(t, searchFor(t, c, base, "labs"), http.StatusOK, msgSearchUnavailable)
	wantStatus(t, do(t, c, http.MethodGet, base+tomorrowOnly, nil), http.StatusOK, "Robotics intro")
}

func TestUnreadableChipsAreShownAgainWithASentence(t *testing.T) {
	base, _, _ := newSearchApp(t, `{}`)
	c := signedIn(t, base, studentEmail, studentPass)
	for q, want := range map[string]string{
		"from=2026-13-01":               "Please choose a valid date.",
		"from=2026-10-25&to=2026-10-23": "The end date must not be before the start date.",
		"time=night":                    "Please choose morning, afternoon or evening.",
		"min=zero":                      "The number of free places must be a whole number of 1 or more.",
	} {
		r := do(t, c, http.MethodGet, base+"/labs?"+q, nil)
		wantStatus(t, r, http.StatusBadRequest, want)
		wantStatus(t, r, http.StatusBadRequest, "Applied filters")
	}
}

func TestStaffCanSearchToo(t *testing.T) {
	base, _, _ := newSearchApp(t, `{"date_from":"2026-10-23"}`)
	c := signedIn(t, base, staffEmail, staffPass)
	wantRedirect(t, searchFor(t, c, base, "tomorrow"), http.StatusSeeOther, tomorrowOnly)
	r := do(t, c, http.MethodGet, base+tomorrowOnly, nil)
	wantStatus(t, r, http.StatusOK, "Robotics intro")
	if strings.Contains(r.body, `action="/sessions/3/book"`) {
		t.Error("staff get a Book button")
	}
}

func TestSearchPagesMeetTheDefinitionOfDone_AC30_AC41_AC42(t *testing.T) {
	base, _, _ := newSearchApp(t, `{}`)
	c := signedIn(t, base, studentEmail, studentPass)
	for _, path := range []string{"/labs", "/labs?from=2026-10-22&to=2026-10-28&time=morning&min=2&kw=robot", "/labs?time=night"} {
		r := do(t, c, http.MethodGet, base+path, nil)
		if m := regexp.MustCompile(`(?i)transition|animation|@keyframes`).FindString(r.body); m != "" {
			t.Errorf("%s contains %q", path, m)
		}
		for _, m := range regexp.MustCompile(`<(?:input|select) id="([^"]+)"`).FindAllStringSubmatch(r.body, -1) {
			if !strings.Contains(r.body, `<label for="`+m[1]+`">`) {
				t.Errorf("%s: %q has no label", path, m[1])
			}
		}
	}
	css := do(t, c, http.MethodGet, base+"/static/style.css", nil).body
	if m := regexp.MustCompile(`(?i)transition|animation|@keyframes`).FindString(css); m != "" {
		t.Errorf("style.css contains %q", m)
	}
}
