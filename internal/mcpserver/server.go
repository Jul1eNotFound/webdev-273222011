// Package mcpserver serves Labwiz's MCP tools over Streamable HTTP at /mcp. It is an entry layer like
// internal/handler: each tool finds the caller from its bearer token, calls the same service method as
// the web pages, and turns the result into JSON. It never checks roles itself (SPEC decisions).
package mcpserver

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Jul1eNotFound/webdev-273222011/internal/service"
)

// maxBody caps one MCP request, like the forms' body limit.
const maxBody = 1 << 20

// msgInternal is what a caller sees for an error that is not a rule: nothing internal leaks.
const msgInternal = "Something went wrong on our side. Please try again."

type server struct {
	labs *service.Labs
	auth *service.Auth
	log  *slog.Logger
	mcp  *mcp.Server
}

// New returns the /mcp endpoint with every tool. It is stateless: each request stands alone, and each
// tool call is authenticated by its own Authorization header.
func New(labs *service.Labs, auth *service.Auth, log *slog.Logger) http.Handler {
	s := &server{labs: labs, auth: auth, log: log,
		mcp: mcp.NewServer(&mcp.Implementation{Name: "labwiz", Version: "0.5.0"}, nil)}
	s.addReadTools()
	s.addWriteTools()
	h := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s.mcp },
		&mcp.StreamableHTTPOptions{Stateless: true})
	return http.MaxBytesHandler(h, maxBody)
}

// caller is the user whose sign-in session token the request carries as "Authorization: Bearer <token>".
// The session cookie is never read, so a cross-site page cannot act through /mcp. A missing, unknown or
// expired token gives the empty caller, which every service method refuses with ErrUnauthenticated
// (AC-39), before it touches the store.
func (s *server) caller(ctx context.Context, req *mcp.CallToolRequest) (service.Caller, error) {
	if req.Extra == nil {
		return service.Caller{}, nil
	}
	token, ok := strings.CutPrefix(req.Extra.Header.Get("Authorization"), "Bearer ")
	if !ok {
		return service.Caller{}, nil
	}
	c, err := s.auth.CurrentUser(ctx, strings.TrimSpace(token))
	if errors.Is(err, service.ErrUnauthenticated) {
		return service.Caller{}, nil
	}
	return c, err
}

// fail turns a rule violation into a tool error with its one sentence, the same the web page shows. Any
// other error is logged and reported without its details.
func (s *server) fail(tool string, err error) error {
	var se *service.Error
	if errors.As(err, &se) {
		return se
	}
	s.log.Error("mcp tool failed", "tool", tool, "err", err)
	return errors.New(msgInternal)
}

// addTool registers one tool whose run gets the caller. The SDK turns a returned error into a tool
// error (isError) whose text is the sentence.
func addTool[In, Out any](s *server, name, description string, run func(context.Context, service.Caller, In) (Out, error)) {
	mcp.AddTool(s.mcp, &mcp.Tool{Name: name, Description: description},
		func(ctx context.Context, req *mcp.CallToolRequest, in In) (*mcp.CallToolResult, Out, error) {
			var zero Out
			c, err := s.caller(ctx, req)
			if err != nil {
				return nil, zero, s.fail(name, err)
			}
			out, err := run(ctx, c, in)
			if err != nil {
				return nil, zero, s.fail(name, err)
			}
			return nil, out, nil
		})
}
