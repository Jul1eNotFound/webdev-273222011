# Instructions for the agent

Read this file at the start of every session. Keep it short and correct; update it when the project changes.

## Project
Web application that allows staff members to add labs and students to book them.
See `SPEC.md` for goals, non-goals and acceptance criteria.

## Commands
- Install: `go mod download`, then `cp .env.example .env` and fill in the values
- Load `.env`: the Go code only reads real environment variables and never opens `.env`. In a terminal, load it once per session before the commands below: `. .\scripts\load-env.ps1` (PowerShell; it keeps values already set in the session and lists them, so after editing `.env` reload with `-Force`) or `set -a; . ./.env; set +a` (bash). Tests run from VS Code get it through `go.testEnvFile` in `.vscode/settings.json`. In Codespaces and on the host, the variables are set by the platform instead.
- Run: `go run ./cmd/server` (serves on `$PORT`, default 3000; needs `DATABASE_URL` pointing at a running PostgreSQL)
- Migrate and seed: `go run ./cmd/migrate up && go run ./cmd/seed` (golang-migrate, used as a library; `down` reverts). Seed needs `SEED_STUDENT_PASSWORD` and `SEED_STAFF_PASSWORD`, creates `student@labwiz.test` and `staff@labwiz.test`, and re-dates the demo labs to today on every run. It also adds demo staff, students, labs and bookings (`internal/seed/demo.go`); their random passwords go to `seed-credentials.txt`, which git ignores and the next run reuses. Tests use only `seed.Run`, never the demo data.
- Test: `go test ./...` (needs `TEST_DATABASE_URL`, a separate PostgreSQL database; never point it at the development database)
- Coverage: `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out`. This only reports numbers; the gate (100% of `internal/service`, 80% overall) is enforced in CI.
- MCP: the tools are served by the app itself at `/mcp` (Streamable HTTP), so `go run ./cmd/server` is all it takes. A client signs in with `Authorization: Bearer <token>`; get a token with `go run ./cmd/mcptoken -email <account>` (asks for the password; prints only the token, so `MCP_TOKEN=$(go run ./cmd/mcptoken -email …)` works). Claude Code (CLI and VS Code extension) connects through `.mcp.json`, and VS Code's own MCP client through `.vscode/mcp.json`; both read `${MCP_TOKEN}` from the environment and hold no secret. The extension only sees variables set before VS Code started, so on Windows store the token with `[Environment]::SetEnvironmentVariable("MCP_TOKEN", $t, "User")` and restart VS Code; tokens last 7 days. With the CLI installed, `claude mcp add --transport http labwiz http://localhost:3000/mcp --header "Authorization: Bearer $MCP_TOKEN"` works too. MCP Inspector: `npx @modelcontextprotocol/inspector`, transport Streamable HTTP, the same URL and header.

## Conventions
- Layout:
  - `cmd/server/` is the entry point; it only wires things together.
  - `internal/handler/` holds Chi route handlers.
  - `internal/service/` holds business rules.
  - `internal/store/` holds the data layer and is the only place that writes SQL.
  - `internal/model/` holds the one file that names the AI provider.
  - `internal/auth/` holds sessions, bcrypt and the Google OAuth flow.
  - `mcp-server/` holds the MCP tools.
  - `migrations/` holds the numbered SQL files.
  - `web/templates/` and `web/static/` hold the server-rendered HTML, the single stylesheet and the small scripts.
- Naming: Go standard style. Packages are lowercase single words. Files are `snake_case.go`. Tests are `*_test.go` beside the code. DB tables are plural `snake_case` (`users`, `labs`, `bookings`). Routes are lowercase, as in SPEC (`/labs`, `/staff/labs/new`).
- Business rules live in `internal/service` only. This covers place limits, duplicate booking, "lab is full", unique lab titles and "limit cannot drop below current bookings". Handlers and MCP tools never re-implement them.
- Authorization is decided in the service from the caller's role (student or staff). Handlers and MCP tools pass the caller in and never check roles themselves. HTTP and MCP therefore enforce the same rules.
- Concurrency: booking the last place must be atomic, using a transaction and a row lock or a conditional update. A `bookings` unique constraint on (user, lab) backs up the duplicate check (AC-5, AC-6).
- Errors: services return typed errors (not found, full, already booked, duplicate title, validation, forbidden, unauthenticated). The handler maps each to a status: 400, 401, 403, 404 or 409. The same error text is used for HTTP and MCP (AC-25, AC-38).
- Errors to the client: always one readable sentence, never raw JSON, a stack trace or an SQL error (AC-43). Full detail goes to the server log, with no secrets and no personal data.
- Login errors never reveal whether the email exists (AC-17).
- Passwords: bcrypt hash only; the plain text is never stored or logged (AC-18).
- Templates: every page has a distinct `<title>` and `<html lang="en">`, and every input has a label (AC-41, AC-42).
- CSS: one stylesheet, with spacing, type and colours as custom properties. No `transition` or `animation` rules, no theme toggle and no language switcher (AC-30).
- Config: read from environment variables in one place at startup. Never hard-code a secret, and keep `.env.example` in sync when adding a variable.

## Layers
Requests arrive two ways and both end in the same place:

- HTTP route handler (`internal/handler/`, routes in `router.go`; session check in `internal/auth/session.go`) → service (`internal/service/`) → data access (`internal/store/`) → database (`migrations/`)
- MCP tool (`internal/mcpserver/`, mounted at `/mcp` in `router.go`) → **the same service** → data access → database

Tests without a database use the in-memory store in `internal/service/servicetest`. Tests with a database call `storetest.Open`, which skips when `TEST_DATABASE_URL` is unset.

A route handler reads the request and turns a result into a status code. A service holds the
rules and knows nothing about HTTP or about MCP. Only the data layer writes SQL. An MCP tool
that talks to the database directly is a second, unguarded way into your data: never add one.

## The model feature
<!-- Provider, env var, and the call site. -->
- The provider is named in exactly one file: 
- The key comes from an environment variable and is never committed or logged.
- Every call has a timeout and a deterministic fallback, so the app still works when the provider is down or rate-limited.
- The model's output is re-checked against real data before anything is saved or shown.
- Tests never call the provider: they pass in a fake client.

## The MCP server
<!-- Where it lives and how it authenticates. -->
- Lives in `internal/mcpserver/`, served by the app at `/mcp`. The Node.js server in `mcp-server/` is the Exercise 1 coursework (the lecture tools) and is not part of the app.
- Each tool call is authenticated by its own `Authorization: Bearer <token>`, a sign-in session token from `cmd/mcptoken`; the session cookie is never read, so `/mcp` needs no CSRF token. A missing or expired token gives the empty caller, which every service method refuses.
- Every tool calls a service function; none touches the data layer or the database directly.
- It carries a user, and enforces exactly the same authorization rules as the HTTP API.
- Adding a tool means adding a row to the table in `SPEC.md` too.

## Tests and coverage
- Every acceptance criterion in `SPEC.md` has an id like `AC-3`, and a test whose title contains that id.
- The coverage gate runs in CI: 100% of the service layer, 80% overall.
- A test is evidence. Changing a test so the code passes destroys the evidence.

## Always ask before
- deleting files or folders
- editing a migration that has already run
- installing or removing dependencies
- pushing, tagging or changing CI
- anything that touches secrets or `.env`
- changing an authentication or authorization rule
- adding, renaming or removing an MCP tool
- changing a coverage threshold
- sending user data to a third-party model

## Never
- commit secrets, passwords, API keys or an OAuth client secret
- weaken or delete a test to make it pass
- lower a coverage threshold or switch off a CI job to make the build green
- add an MCP tool that reaches past the service layer
- log a prompt that contains somebody's personal data
