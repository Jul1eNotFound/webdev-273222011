package handler_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Jul1eNotFound/webdev-273222011/internal/auth"
)

const msgSignIn = "Please sign in to continue."

// sessionToken is the sign-in session token in c's cookie jar: what cmd/mcptoken prints, and what an
// MCP client sends as its bearer token.
func sessionToken(t *testing.T, c *http.Client, base string) string {
	t.Helper()
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	for _, ck := range c.Jar.Cookies(u) {
		if ck.Name == auth.CookieName {
			return ck.Value
		}
	}
	t.Fatal("not signed in")
	return ""
}

// tokenOf signs in and returns the session token.
func tokenOf(t *testing.T, base, email, password string) string {
	t.Helper()
	return sessionToken(t, signedIn(t, base, email, password), base)
}

// bearer adds "Authorization: Bearer <token>" to every request, as an MCP client configured with a
// header does. An empty token sends no header.
type bearer struct{ token string }

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	if b.token != "" {
		r = r.Clone(r.Context())
		r.Header.Set("Authorization", "Bearer "+b.token)
	}
	return http.DefaultTransport.RoundTrip(r)
}

// mcpSession connects an MCP client to the app's /mcp with the given bearer token ("" for none).
func mcpSession(t *testing.T, base, token string) *mcp.ClientSession {
	t.Helper()
	return mcpSessionWith(t, base, &http.Client{Transport: bearer{token}})
}

// mcpSessionWith connects through the given HTTP client, which may carry a cookie jar.
func mcpSessionWith(t *testing.T, base string, hc *http.Client) *mcp.ClientSession {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "labwiz-test", Version: "0"}, nil)
	cs, err := client.Connect(t.Context(), &mcp.StreamableClientTransport{Endpoint: base + "/mcp", HTTPClient: hc}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

// callTool calls one tool and fails the test on a protocol error; a tool error comes back as IsError.
func callTool(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return res
}

func toolText(res *mcp.CallToolResult) string {
	var parts []string
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			parts = append(parts, tc.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// wantToolError fails unless the call was refused with exactly this sentence.
func wantToolError(t *testing.T, name string, res *mcp.CallToolResult, sentence string) {
	t.Helper()
	if !res.IsError || toolText(res) != sentence {
		t.Fatalf("%s: isError=%v text=%q, want the error %q", name, res.IsError, toolText(res), sentence)
	}
}

// wantNoBookerData fails if a result names who booked: no bookers field and none of the seeded bookers.
func wantNoBookerData(t *testing.T, name, text string) {
	t.Helper()
	for _, leak := range []string{"bookers", "ana@labwiz.test", "Ana Example"} {
		if strings.Contains(text, leak) {
			t.Errorf("%s result contains %q: %s", name, leak, text)
		}
	}
}

// toolOut decodes a successful call's structured result into out.
func toolOut(t *testing.T, name string, res *mcp.CallToolResult, out any) {
	t.Helper()
	if res.IsError {
		t.Fatalf("%s failed: %s", name, toolText(res))
	}
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, out); err != nil {
		t.Fatalf("%s: %v\n%s", name, err, b)
	}
}

// toolSession is a session as the tools return it, decoded by the tests.
type toolSession struct {
	SessionID  int64  `json:"session_id"`
	LabID      int64  `json:"lab_id"`
	Title      string `json:"title"`
	Owner      string `json:"owner"`
	StartsAt   string `json:"starts_at"`
	FreePlaces int    `json:"free_places"`
	PlaceLimit int    `json:"place_limit"`
	Cancelled  bool   `json:"cancelled"`
	BookedByMe bool   `json:"booked_by_me"`
	Bookers    *[]struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"bookers"`
}
