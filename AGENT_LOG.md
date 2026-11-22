# Agent log

One entry per agent session, newest at the bottom. Be honest: what went wrong is the useful part.

<!-- Copy this block for each session:

## YYYY-MM-DD · <short title>
- Goal:
- Delegated:
- Checkpoints:
- Went wrong:
- Changed by hand:

-->

## 2026-11-03 · Build 0 and 1 slices
- Goal: implement 0 and 1 slices
- Delegated: plan, database setup, sign in and lab list page
- Checkpoints: try sign in, try open labs without signing in, open labs table
- Went wrong: cookie bug
- Changed by hand: none

## 2026-11-03 · Exercise 3 plan
Goal: a written plan for the staff adding lab feature, no code
Autonomy: the agent works alone within one step of the plan; I read the diff before every commit
Checkpoints: plan review before any code, a permission prompt for everything in "Always ask before"

## 2026-11-17 · Slices 3–3e: booking, owners, sessions, cancelling, dates, CSRF
- Goal: booking (Slice 3), then the re-planned Slices 3a–3e: lab owners and "My labs", sessions, cancelling sessions and deleting labs, browsing by date, CSRF tokens
- Delegated: planning each slice, migrations 000003–000004, store/service/handler code, tests (in-memory and PostgreSQL), SPEC.md traceability and results
- Checkpoints: plan approval in plan mode before each slice; one commit per step after reading the report; every lock and constraint test broken on purpose to prove it catches the bug; hand checks in the browser
- Went wrong: the agent overwrote internal/store/sessions.go (sign-in sessions) and restored it from git; a RESTRICT foreign key reports SQLSTATE 23001, not 23503; re-seeding duplicated cancelled sessions; a stale SESSION_SECRET stayed in the PowerShell session; the real secret was pasted into .env.example and printed by git diff (never committed; restored and rotated)
- Changed by hand: pasted the secret into .env.example by mistake; asked for a "back to home" button on /staff/labs/new

## 2026-11-18 · Slice 4: cancelling bookings and "My bookings"
- Goal: students cancel their own bookings before the session starts, see them on /bookings (with "Cancelled by staff"), and see "Booked" on sessions they booked
- Delegated: store/service/handler code, the /bookings page, tests (in-memory and PostgreSQL), SPEC.md; afterwards a "Labs" link in the header
- Checkpoints: plan approval; a commit per step; the cancel lock and the "Booked" button broken on purpose; hand check in the browser
- Went wrong: the cancel form shared the book-form class, so booking.js turned a cancel into a second "Booked" button with "undefined of undefined places free" (HTTP tests don't run the script; fixed with its own class and a test); the demo data had almost no bookings that could still be cancelled; test placeholders left in the first draft; shell heredocs broke on quoting
- Changed by hand: found the cancel bug in the browser; asked for a "Labs" header link

## 2026-11-18 · Slice 5: MCP tools
- Goal: the nine MCP tools from SPEC at /mcp, going through the same services as the web pages
- Delegated: the Go MCP SDK, internal/mcpserver (four reading and five changing tools), shared confirmation sentences, cmd/mcptoken, .vscode/mcp.json, AGENTS.md, SPEC.md
- Checkpoints: plan approval (I chose HTTP at /mcp and a sign-in session token from a CLI); a commit per step; break-on-purpose checks (a missing token acting as staff, get_lab returning booking lists to non-owners, book_lab with its own wording, add_lab treating everyone as staff); tools called over JSON-RPC against the dev database; tried by hand from Claude Code, including a separate Haiku chat
- Went wrong: mcptoken looked frozen because the hidden password prompt didn't say so (and Git Bash showed no prompt at all); `claude mcp add` failed because only the VS Code extension is installed; three DB tests used seeded() for tomorrow's session; heredoc quoting again
- Changed by hand: added .mcp.json; stored MCP_TOKEN as a user variable and restarted VS Code; tested the tools as a student and as staff

## 2026-11-22 · Slice 6: Google sign-in
- Goal: Sign in with Google that links only existing accounts with a verified email, matched on Google's id afterwards
- Delegated: migration 000005, linking rules in the service, the OAuth client with PKCE, callback routes, tests, docs, SPEC.md
- Checkpoints: plan approval (refuse unknown addresses; I create the OAuth client); a commit per step; state check and verified check broken on purpose; hand check with a real Google client (link, re-sign-in, cancel, changed email)
- Went wrong: the dev database needed migration 000005 before anyone could sign in; a test used a package name as a type; heredoc quoting again
- Changed by hand: created the Google OAuth client and put its id and secret in .env; renamed a demo account to my Gmail for the check

## 2026-11-22 · Slice 7: smart search
- Goal: a free-text search on /labs that Claude Haiku 5.5 turns into filters, shown as editable chips, with a fallback whenever the model can't answer
- Delegated: the service rules (filters, strict checking of the model's answer, 10 searches a minute), internal/model/anthropic.go with the Anthropic Go SDK (v2.7.0, older than the newest days-old releases), the search box and chips, tests against a fake model and a fake API, AGENTS.md, SPEC.md; afterwards more demo labs to test with
- Checkpoints: plan approval (I chose: no date means the next 7 days, hide the box without a key, a hand check with a real key); a commit per step; break-on-purpose checks (no length check, unknown fields accepted, no rate limit, waiting past the deadline for retry-after, a search that writes); hand check with the real API, by keyboard and at 375 px
- Went wrong: every real search fell back at first, because the API refused the tool schema (an enum on a "string or null" type) and the fake API in the tests never checks schemas; fixed with anyOf and a test of the schema shape. The first search after a restart can still hit the 5 second deadline. The SDK sends the key as a bearer token, not X-Api-Key, as a test assumed; one test expected the wrong number of free places; a coverage gap came from an unreachable check
- Changed by hand: added ANTHROPIC_API_KEY to .env; reported the failing search; asked for more demo labs to test every case