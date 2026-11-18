# Labwiz

## Project option
lab booking

## Goals
- A student signs in, browses lab sessions by date and books a session that has free places.
- A staff member adds labs they own, schedules sessions for them, sees who booked each session, and cancels sessions or deletes labs nobody has booked.

## Non-goals
- No multilingual support: only English is supported
- No dark/light theme options: only light generic theme
- No mobile app: one web UI that also works on phone screen
- No animations such as transitions, hover effects or animated loading indicators (a static "Loading…" text is allowed)
- No booking notifications or reminders for both users and lab owners
- No auto-refresh of pages
- No calendar grid: sessions are scheduled and shown as lists grouped by date
- No editing of a lab or a session after it is created: a session is cancelled and a new one added instead
- No transfer of a lab to another owner

## Constraints
- Golang with Chi, PostgreSQL, plain HTML and JavaScript in the UI.
- Passwords are stored only as a hash, never as readable text.

## The model feature
- Model: Claude Haiku 5.5 (`claude-haiku-5-5`), text in and text out, called from the Go backend through the Anthropic Messages API. The API key is read from an environment variable and never reaches the browser. Feature: natural-language lab search, where a student types something like "a quiet lab tomorrow afternoon with 5 free places" and the model turns it into filters for the existing `/labs` list of sessions.
- Input: the student's free-text query (max 200 characters) and today's date and timezone. No user data, no emails, and no booking data are sent.
- Output: a JSON object with optional fields `date_from`, `date_to`, `time_of_day` (morning, afternoon or evening), `min_free_places` and `keywords`. It is requested through a tool schema so the shape is enforced. The server validates it, runs the normal session-list query with those filters, and shows them as editable chips above the results.
- When it fails:
  - Slow: the call has a 5 second timeout. On timeout, the student sees "Smart search is unavailable, showing all labs" and the unfiltered list.
  - Rate-limited (429) or API error: same fallback message and list. The server retries at most once, honouring `retry-after` if it is under the timeout, and limits each student to 10 searches per minute.
  - Wrong: if the JSON fails validation (unknown fields, a date range in the past, `min_free_places` that is not a number), it is discarded and treated as a failure. If the JSON is valid but misreads the query, the chips show exactly what was applied, and the student can remove or edit them. The model never books, cancels or writes anything.
  - Prompt injection: the query is passed as data, and the model has no tools except returning the filter object. The worst outcome is a wrong filter.

## Auth
- Local: e-mail and password, stored as a bcrypt hash.
- Provider: Sign in with Google, the authorization-code flow, on the server.
- Linking: matched on Google's account id. An address Google has not marked as verified is
  never linked to an existing account.
- Forms: every form that changes something carries a CSRF token tied to the sign-in session: the HMAC-SHA256 of the session's cookie token under `SESSION_SECRET`, which must be at least 32 characters or the server does not start. The sign-in form is exempt, since there is no session yet.
- MCP: each tool call carries `Authorization: Bearer <token>`, where the token is an ordinary sign-in session token printed by `go run ./cmd/mcptoken -email <account>` (it asks for the password and signs in through the same `Auth.Login`). It lasts 7 days like any session and grants exactly what that account may do. `/mcp` never reads the session cookie, so it needs no CSRF token.

## MCP tools
| Tool | What it does | Who may call it |
| --- | --- | --- |
| list_labs | Lists the sessions on a given date (default today), each with lab title, description, owner, time period, place limit, free places and whether it is cancelled. | Any signed-in user |
| get_lab | Returns one lab by id with its owner, place limit and upcoming sessions with their free places. For the lab's owner it also lists who booked each session. | Any signed-in user; the booking lists are for the owner only |
| book_lab | Books a place in one session for the calling student. Refused if the session is full, cancelled or already started, or the student already booked it. | Student |
| cancel_booking | Cancels the calling student's booking for a session before it starts and frees the place. | Student, own bookings only |
| my_bookings | Lists the calling student's bookings with lab title, session time and whether the session was cancelled. | Student |
| add_lab | Creates a lab owned by the caller, with title, description and a place limit of 1 or more. Refused if the title already exists. | Staff |
| list_lab_bookings | Lists the students who booked each session of a given lab. | Staff who own the lab |
| schedule_session | Adds a session to a lab: a start and an end, and optionally a date to repeat it weekly until. Refused if it starts in the past or overlaps another session of the lab, with the same messages as the web page. | Staff who own the lab |
| cancel_session | Cancels an upcoming session of a lab. Its bookings are kept and shown to the students as cancelled. Refused once the session has started. | Staff who own the lab |

## UI
- Stack: plain HTML, CSS and JavaScript served by the Go backend. There is no framework and no build step. One stylesheet, with spacing, type scale and colours defined as CSS custom properties. Pages are server-rendered, and small scripts handle the booking button and the smart search.
- Screens:
  1. **Login** (`/login`): email and password, a "Sign in with <provider>" button, and an error line. Visitors who are not signed in land here from any protected page.
  2. **Lab list** (`/labs`, `/labs?date=YYYY-MM-DD`): the sessions on one day (today by default) as cards, each with lab title, description, owner ("Run by …"), time period and free places. Links to the previous and next day and a labelled date field change the day. A smart-search box with editable filter chips. Each card has a "Book" button, which is disabled and reads "Full" when no places are left, "Closed" once the session has started, "Cancelled" when staff cancelled it, and "Booked" when the student has booked it. The empty state reads "No labs available today", "No labs on this day" for another day, or "No labs match these filters" when search filters are applied.
  3. **Lab detail and my bookings** (`/labs/{id}`, `/bookings`): a lab's details and owner, and its upcoming sessions, each with "Book" or "Cancel booking". The bookings page lists the student's own bookings, marking those whose session was cancelled as "Cancelled by staff". When a student books a full session, they see "This lab is full" with links back to the lab list and the home page (AC-1).
  4. **Staff: my labs, add lab** (`/staff/labs`, `/staff/labs/new`): the labs the signed-in staff member owns, each with its next session, and a form with title, description and place limit. Students get 403.
  5. **Staff: one owned lab** (`/staff/labs/{id}`): the lab's details, an "Add a session" form (Starts and Ends as dates with times, so a session may run past midnight, and an optional "Repeat weekly until" date that adds the same session every week, at most 26), and its sessions grouped by date, upcoming first, each with the places booked, the students who booked it and a "Cancel session" link. A "Delete lab" link while nobody has booked it. Only the owner may open it; everyone else gets 403.
  6. **Confirmations** (`/staff/labs/{id}/delete`, `/staff/sessions/{id}/cancel`): one sentence naming what will change, a button that confirms it (POST), and a link back that changes nothing.
- Layout: a single column that works at 380 px and widens to a centred max width on desktop. The header has the app name (a link to `/labs`), a "Labs" link to the lab list, "My bookings" for students or "My labs" and "Add a lab" for staff, the signed-in user and a Sign out button. The link of the page the user is on is marked `aria-current="page"` and underlined. Light theme only.
- States, on every screen: loading (static "Loading…" text), empty, error (one readable sentence) and success (a message at the top of the page, no reload needed).

## Architecture
<!-- One request traced through every layer, naming the file for each one,
     plus one place where your own code does not match this layering. -->
Layers: handler → service → store → database. HTTP and MCP both go through the same service.

A staff member adds a lab (`POST /staff/labs/new`):
1. **Entry**: `cmd/server/main.go` loads config, opens the pool and wires store, services and router together.
2. **Routing and session**: `internal/handler/router.go` matches the route inside the `RequirePage` group. `internal/auth/session.go` reads the cookie, gets the caller from `service.Auth.CurrentUser` (`internal/service/auth.go`), and puts it in the request context. With no valid session, the visitor is redirected to `/login`.
3. **Handler**: `addLab` in `internal/handler/staff.go` caps the body size, parses the form (title, description, place limit) into `service.NewLab` and calls `Labs.Add`. It does not check the role: no handler does.
4. **Service**: `Labs.Add` in `internal/service/labs.go` checks the role (`RequireStaff`, AC-15), validates the title and place limit (AC-13), makes the caller the owner (AC-60), and maps `store.ErrDuplicate` to `ErrDuplicateTitle` (AC-2). The role and ownership checks (`RequireStaff`, `RequireOwner`) are in `internal/service/auth.go`, the typed errors in `internal/service/errors.go`.
5. **Store**: `CreateLab` in `internal/store/labs.go` runs the only SQL, an `INSERT … RETURNING id` that includes `owner_id`, and turns SQLSTATE `23505` into `ErrDuplicate`.
6. **Database**: the `labs` table and its unique title come from `migrations/000001_init.up.sql`, and its required `owner_id`, a foreign key to `users`, from `migrations/000003_lab_owners.up.sql`. The unique constraint, not a prior lookup, decides a duplicate title, so two staff adding the same title at once cannot both succeed.
7. **Back out**: on success the handler redirects to `/staff/labs/{id}?created=1`. A validation or conflict error re-renders `web/templates/lab_new.html` with the message and the typed values (400/409). Any other error goes through `fail` → `statusFor` in `internal/handler/render.go`, which shows one readable sentence (AC-43).

**Where the code does not match the layering:** `seed.Demo` in `internal/seed/demo.go` books the demo students straight through `store.BookLab`, with a check of its own that only skips duplicates and full labs. It never goes through `Labs.Book`, so the service's booking rules do not run: no student role check, and no "not open today" rule. That is why the demo lab "Water quality testing", which ended yesterday, has a booking that no request could make. The store still enforces what the database enforces (unique booking, the lab lock), so the data is consistent, but it is not data the app's rules would produce. It is acceptable for a development seed, and tests use `seed.Run`, which books nothing. The fix, if the demo data should obey the rules, is to book through `Labs.Book` with each demo student as the caller, and to give past labs no bookings.

Slice 3a removed the earlier mismatch: `internal/handler/staff.go` used to call `service.RequireStaff` itself. The GET handler now asks `Labs.NewLabForm(caller)`, and `addLab` leaves the role check to `Labs.Add`, so HTTP and MCP refuse a student at the same point.

This section describes the code as it is after Slice 5. A new lab has no time period and no sessions: its owner then adds sessions on `/staff/labs/{id}`, which follow the same layers (`addSessions` in `staff.go` → `Labs.AddSessions` in `internal/service/sessions.go` → `AddSessions` in `internal/store/lab_sessions.go`, where the `lab_sessions_no_overlap` exclusion constraint from `migrations/000004_lab_sessions.up.sql` decides an overlap). A booking is made on a session, through `POST /sessions/{id}/book` → `Labs.Book` → `BookSession`, which locks the session's row. Cancelling a session (`Labs.CancelSession` → `CancelSession` in `internal/store/cancel_delete.go`) takes the same lock, so no booking gets in after a cancel. A student cancelling their own booking (`POST /sessions/{id}/cancel-booking` → `Labs.CancelBooking` → `CancelBooking` in `internal/store/my_bookings.go`) takes it too and deletes only the caller’s row, so a booking that waited for it gets the freed place. Deleting a lab is one `DELETE`: its sessions cascade, and the `ON DELETE RESTRICT` key on `bookings.session_id` from `000004_lab_sessions` refuses a lab that has any booking. The database decides, not a prior lookup, so a booking made while the delete runs is never lost. Every request that changes something passes the CSRF check in `require` in `internal/auth/session.go`, after the session and before any handler, so no route can forget it.

An MCP tool call takes the same path from the service down. `POST /mcp` is mounted in `router.go` outside the cookie and CSRF groups and handled by `internal/mcpserver` (the SDK's stateless Streamable HTTP handler, capped at 1 MB). For `book_lab`, `addTool` in `server.go` reads `Authorization: Bearer <token>` and asks `Auth.CurrentUser` for the caller; the tool in `write.go` then calls `Labs.Book`, the same method as `POST /sessions/{id}/book`, and returns `service.MsgBooked` with the free places, the fields the booking script gets. A `*service.Error` becomes a tool error with its sentence. The tools never check a role and never reach the store.

## Decisions
How the criteria are written:
- Each criterion is one behaviour, written as Given / When / Then, and tagged with how it is checked: `[http]` request and response, `[db]` database state, `[ui]` browser, `[mcp]` MCP tool call.
- IDs are stable once a test names them: new criteria get new numbers, and removed ones are listed under "Retired criteria", not reused.
- The definition of done covers only the pages that exist in the slice being checked: `/staff/labs/new` is checked from Slice 2 on, `/staff/labs` and `/staff/labs/{id}` from Slice 3a on.
- `[ui]` criteria are checked over HTTP, by asserting on the rendered HTML and headers, unless the behaviour exists only in a browser (layout at a viewport width, keyboard use, script-driven updates). Those are checked by hand in a browser until a browser test tool is added.
- Tests that depend on "today" or "tomorrow" run with a fixed clock. The fixed clock drives server logic only. Anything the client checks against its own clock, such as cookie expiry, is relative (`Max-Age`, not `Expires`), so tests keep passing as the real date moves on.

The domain:
- A **lab** is a title, a description, a place limit and an owner. A **session** is one scheduled run of a lab, with a start and an end (table `lab_sessions`; the sign-in records stay in `sessions`). From Slice 3b on, a criterion that books, fills or counts the free places of "a lab" means one session of it, and "its time period" means that session's.
- The place limit belongs to the lab: every session of it has that many places. Free places of a session are the lab's `place_limit` minus that session's rows in `bookings`. This holds from Slice 1 on, even though bookings can only be made from Slice 3.
- The owner is the staff member who added the lab. Only the owner adds and cancels its sessions, deletes it, and sees who booked it. Other staff see the same lab pages as students, without a Book button.
- Two sessions of one lab cannot overlap, unless one is cancelled. A PostgreSQL exclusion constraint (`btree_gist`) decides it, not a prior lookup, so two staff adding sessions at once cannot both succeed.
- A session can be booked until it starts. A cancelled session cannot be booked, and its bookings are kept so the student can see the cancellation; a cancelled session frees no places and cannot be un-cancelled.
- A lab can be deleted only while it has no bookings at all, past ones included. Otherwise its owner cancels its sessions. Deleting and cancelling go through a server-rendered confirmation page, so they work without JavaScript; a script may turn it into a dialog.
- Lab and session editing is out of scope: staff add labs and sessions, and cancel sessions.

The pages:
- `/` redirects to `/labs`, so "home page" means `/labs`.
- Accounts are seeded; there is no sign-up screen. Staff and students are told apart by a role on the `users` row.
- `/labs` defaults to the sessions whose time period includes part of today. `?date=YYYY-MM-DD` shows another day (any day, past ones too; their sessions show as "Closed"), and a smart-search date filter may widen it to a range.
- "Today" is the calendar day in the server process's local timezone. A deployment sets that timezone explicitly.
- A time period is shown as `HH:MM–HH:MM` when it starts and ends on the same day, and as `D Mon HH:MM – D Mon HH:MM` otherwise, both in that timezone.
- Error pages (400, 401, 403, 404, 405, 409, 500) count as one page and share one title.
- From Slice 3a on, every role and ownership check runs in the service. Handlers and MCP tools pass the caller in and never check roles themselves.

MCP (Slice 5):
- The tools are served by the app at `/mcp` over Streamable HTTP, statelessly, with the official Go SDK (`github.com/modelcontextprotocol/go-sdk`). One process, so the deployed app has them too.
- `initialize` and `tools/list` need no token; every tool call is checked. A missing, unknown or expired token gives the empty caller, which every service method refuses with "Please sign in to continue." before the store is touched.
- A refusal is a tool error (`isError`) whose text is the service's sentence, the same as the web page shows. Any other error is logged and reported as "Something went wrong on our side. Please try again."
- Times are RFC 3339 in the server's timezone; `schedule_session` takes `starts` and `ends` as `YYYY-MM-DDTHH:MM`, like the web form, so both get the same messages.
- `get_lab` asks the service for the booking lists too, and leaves them out when the service refuses (anyone but the owner). The tool never decides who owns a lab.

## Acceptance criteria

### Definition of done (every slice)
- [AC-29] [ui] Given a 375px viewport, when `/labs`, `/login` and the staff pages are opened, then there is no horizontal scroll and every button and input is visible and can be activated.
- [AC-30] [http] Given the served CSS, JS and HTML, when grepped, then they contain no language switcher, no theme toggle, and no `transition`, `animation` or `@keyframes` rule.
- [AC-41] [ui] Given `/login`, `/labs` and the staff pages, when inspected, then every input has an associated label, and sign-in, booking, add-lab, add-session, cancelling and deleting can be completed with the keyboard alone.
- [AC-42] [http] Given any page, when fetched, then it has `<html lang="en">` and a `<title>` that no other page uses. All error pages count as one page and share one title.
- [AC-43] [http] Given any failing request (validation, 403, 404, 409, 500), when the response is shown, then it is one readable sentence, with no raw JSON, stack trace or SQL.

### Slice 0: skeleton and local sign-in
- [AC-3] [ui] Given a seeded student, when they sign in with the right email and password, then they land on `/labs` and see the labs available today.
- [AC-9] [http] Given a visitor who is not signed in, when they open `/labs` or any protected page, then they are redirected to `/login`.
- [AC-17] [http] Given a wrong password for a known email, or any password for an unknown email, when the visitor signs in, then both show exactly the same sentence, `Invalid email or password.` (with the full stop).
- [AC-18] [db] Given a seeded account, when its `users` row is read, then `password` holds a bcrypt or argon2 hash and is not equal to the plain password.
- [AC-19] [http][db] Given a visitor who is not signed in, when they POST to a booking endpoint, then the response is 401 and `bookings` is unchanged. Until Slice 3 the booking endpoint is a stub, so this is checked again when the real endpoint replaces it.
- [AC-21] [http][db] Given a signed-in user, when they sign out, then every protected page they were shown was sent with `Cache-Control: no-store`, their `sessions` row is gone, and reloading `/labs`, with or without the old cookie, redirects to `/login`.

### Slice 1: lab list
- [AC-10] [ui] Given the three seeded labs available today, when a signed-in user opens `/labs`, then each lab is shown with title, description, time period and free places (`place_limit` minus its bookings).
- [AC-11] [ui][db] Given a lab shown on `/labs`, when its row is deleted from `labs` and the page is reloaded, then the lab is gone.
- [AC-22] [ui] Given a seeded lab whose time period starts tomorrow, when `/labs` is opened without a date or a search, then that lab is not shown.
- [AC-23] [ui] Given no lab is available today, when `/labs` is opened without a date or a search, then it shows "No labs available today".

### Slice 2: staff adds labs
- [AC-12] [ui][db] Given a signed-in staff member, when they submit a title, description and a place limit of 1 or more, then `labs` gains one row and they land on that lab's staff page. The lab appears on `/labs` once it has a session that day (AC-67).
- [AC-13] [http][db] Given a staff member, when they submit a place limit of 0, a negative number or non-numeric text, then a validation message is shown and no row is created.
- [AC-2] [http][db] Given a lab titled "X" exists, when staff add another lab titled "X", then the response is 409 with "Please choose a different title" and no row is created.
- [AC-14] [ui][db] Given a lab with bookings, when its owner opens `/staff/labs/{id}`, then each of its sessions lists the names of exactly the students in that session's `bookings` rows.
- [AC-15] [http][db] Given a signed-in student, when they GET or POST `/staff/labs/new`, then the response is 403 and no row is created.
- [AC-45] [http] Given a signed-in student, when they GET `/staff/labs/{id}`, then the response is 403 and no booking list is returned.

### Slice 3: booking
- [AC-4] [ui][db] Given a signed-in student and a lab with at least one free place, when they book it, then they see "Booking confirmed" and the new free-place count without a manual reload, the count has dropped by 1, and `bookings` has exactly one row for that student and lab.
- [AC-5] [http][db] Given the student already booked the lab, when they book it again, then the response is 409, they see "You have already booked this lab", and `bookings` still has one row for the pair.
- [AC-1] [http][ui] Given a lab with no free places, when a student books it, then the response is 409, they see "This lab is full" with links to the lab list and the home page, and no row is created.
- [AC-8] [ui][http] Given a full lab, when `/labs` is shown, then its "Book" button is disabled or shows "Full", and a direct POST to book it is refused with 409.
- [AC-6] [db] Given a lab with one free place, when two students book it at the same time, then exactly one succeeds, the other sees "This lab is full", and the number of bookings never exceeds the lab's limit.
- [AC-46] [http][db] Given a signed-in staff member, when they POST a booking, then the response is 403 and no row is created.
- [AC-47] [http] Given an id that matches no lab, when a booking or lab page is requested, then the response is 404 with a readable sentence.

### Slice 3a: lab owners and "My labs"
- [AC-60] [db] Given a signed-in staff member, when they add a lab, then its `labs` row has them as its owner.
- [AC-61] [ui] Given labs owned by different staff, when `/labs` or `/labs/{id}` is opened, then each lab shows "Run by" and its owner's name.
- [AC-62] [ui] Given labs owned by two staff members, when one of them opens `/staff/labs`, then it lists exactly the labs they own, including labs with no session today, each linking to its `/staff/labs/{id}`.
- [AC-63] [ui] Given a staff member who owns no lab, when they open `/staff/labs`, then it shows "You have no labs yet." with a link to `/staff/labs/new`.
- [AC-64] [http] Given a lab owned by another staff member, when a staff member GETs its `/staff/labs/{id}`, then the response is 403 and no booking list is returned.
- [AC-65] [http] Given a signed-in student, when they GET `/staff/labs`, then the response is 403.

### Slice 3b: sessions
- [AC-66] [db] Given a database with labs, time periods and bookings from before Slice 3b, when the migration runs, then each lab has one session with its old time period, each booking belongs to that session, each lab is owned by a staff account, and no lab or booking is lost.
- [AC-67] [ui][db] Given a lab's owner, when they add a session with a start and an end, then `lab_sessions` gains one row for that lab, and it is listed on the lab's staff page and on `/labs/{id}`.
- [AC-68] [http][db] Given a lab's owner, when they add a session whose end is not after its start, which starts in the past, or whose date or time cannot be read, then the response is 400 with a validation message and no row is created.
- [AC-69] [http][db] Given a session of a lab, when its owner adds another session of that lab that overlaps it, then the response is 409 with "This lab already has a session at that time." and no row is created. When two overlapping sessions are added at the same time, exactly one is created.
- [AC-70] [db] Given a lab's owner, when they add a session with "Repeat weekly until" a date, then there is one session per week up to and including that date, at most 26. If any of them would overlap an existing session, none is created and the response is 409.
- [AC-71] [db] Given two sessions of one lab, when a student books one of them, then only that session's free places drop by 1, and the student can also book the other one.
- [AC-72] [http][db] Given a lab, when a student or a staff member who does not own it POSTs a new session for it, then the response is 403 and no row is created.
- [AC-83] [http][db] Given a session that has started, when a student POSTs a booking for it, then the response is 409 with "Booking is closed: this session has already started." and no row is created, and `/labs` shows its button disabled as "Closed".
- [AC-84] [http][db] Given a session tomorrow with a free place, when a student books it, then the booking succeeds and `bookings` has one row for it.

### Slice 3c: cancelling sessions and deleting labs
- [AC-73] [ui][db] Given an upcoming session with bookings, when its lab's owner confirms "Cancel session", then the session is marked cancelled, its `bookings` rows are kept, and `/labs` and `/labs/{id}` show it as "Cancelled" with no Book button.
- [AC-74] [http][db] Given a cancelled session, when a student POSTs a booking for it, then the response is 409 with "This session has been cancelled." and no row is created.
- [AC-75] [http][db] Given a session that has started, when its owner POSTs a cancel for it, then the response is 409 with "This session has already started." and the session is unchanged.
- [AC-76] [ui][http] Given a lab's owner, when they open the delete page of the lab or the cancel page of a session, then it names the lab and, for a session, its time period, offers a confirm button and a link back, and nothing changes until the confirm button is used.
- [AC-77] [http][db] Given a lab with no bookings, when its owner confirms "Delete lab", then the lab and its sessions are gone and they land on `/staff/labs` with "Lab deleted.". Given a lab with any booking, the response is 409 with "This lab has bookings. Cancel its sessions instead." and nothing is deleted.
- [AC-78] [db] Given a lab with no bookings, when its owner deletes it while a student books one of its sessions, then either the delete is refused with 409 and the booking stays, or the booking fails with 404 and the lab is gone. A booking is never deleted with the lab.
- [AC-79] [http][db] Given a lab or a session, when a student or a staff member who does not own the lab POSTs a delete or a cancel, then the response is 403 and nothing changes.

### Slice 3d: browsing by date
- [AC-80] [ui] Given sessions on several days, when `/labs?date=YYYY-MM-DD` is opened, then it shows the sessions on that day, a heading naming the day, links to the previous and next day, and a labelled date field that shows another day.
- [AC-81] [http] Given a `date` that is not a valid `YYYY-MM-DD` date, when `/labs` is opened with it, then the response is 400 with "Please choose a valid date.".
- [AC-82] [ui] Given a day other than today with no session, when `/labs?date=` shows it, then it reads "No labs on this day".

### Slice 3e: CSRF protection
- [AC-85] [http][db] Given a signed-in user, when they POST to book, add a lab, add a session, cancel, delete or sign out without the CSRF token or with a wrong one, then the response is 403 with "This page has expired. Please reload it and try again." and nothing changes.
- [AC-86] [http] Given two signed-in users, when one POSTs a form with the other's CSRF token, then the response is 403 and nothing changes.
- [AC-87] [http] Given any page with a form that changes something, when it is rendered, then the form carries the CSRF token, and the booking script sends it in a request header.

### Slice 4: cancelling
- [AC-7] [ui][db] Given a student with a booking, when they cancel it, then the free-place count goes up by 1 and the `bookings` row is gone.
- [AC-49] [http][db] Given a student with no booking for a lab, when they POST a cancel for it, then the response is 404 and `bookings` is unchanged.
- [AC-50] [db] Given a full lab, when one student cancels and another then books, then the second booking succeeds and the bookings count equals the limit.
- [AC-88] [http][db] Given a booking for a session that has started, when the student POSTs a cancel for it, then the response is 409 and the booking is kept.
- [AC-89] [ui] Given a student who booked a session, when `/labs` or `/labs/{id}` is reloaded, then that session's button is disabled and reads "Booked".
- [AC-90] [ui] Given a student's bookings, one of them for a cancelled session, when they open `/bookings`, then each booking shows its lab title and time period, and the cancelled one reads "Cancelled by staff".

### Slice 5: MCP tools
- [AC-24] [mcp] Given the server, when `tools/list` is called, then all nine tools in the MCP table appear, each with a description of at least 20 characters.
- [AC-39] [mcp][db] Given a caller with no valid token, when any of the nine tools is called, then an authentication error is returned and the database is unchanged.
- [AC-37] [mcp][db] Given a student and a lab with a free place, when `book_lab` succeeds, then its result matches the HTTP booking, the free-place count drops by 1, and `bookings` has one new row for the caller.
- [AC-25] [mcp][db] Given a student and a full lab, when `book_lab` is called, then it returns the same "full" error as the HTTP API and no row is created.
- [AC-51] [mcp][db] Given a staff caller, when `book_lab` is called, then it is refused and no row is created.
- [AC-38] [mcp][db] Given a lab titled "X" exists, when `add_lab` is called with title "X", then it is refused with the same message as the HTTP API and no row is created.
- [AC-26] [mcp][db] Given a student caller, when `add_lab` is called, then it is refused and no row is created.
- [AC-35] [mcp][db] Given another student's booking, when a student calls `cancel_booking` for it, then it is refused and that row is unchanged.
- [AC-36] [mcp] Given a student caller, when `list_lab_bookings` is called it is refused, and when `get_lab` is called it returns the lab details without the booking list.
- [AC-40] [mcp] Given bookings by several students, when a student calls `my_bookings`, then only that student's bookings are returned.
- [AC-91] [mcp] Given a staff caller who does not own a lab, when `list_lab_bookings` is called for it, then it is refused, and `get_lab` returns the lab without the booking lists.
- [AC-92] [mcp][db] Given a lab's owner, when `schedule_session` is called with a valid start and end, then `lab_sessions` gains the same rows as adding the session on the web page, and when it overlaps an existing session it is refused with the same message as the web page and no row is created.
- [AC-93] [mcp][db] Given a lab's owner and an upcoming session with bookings, when `cancel_session` is called, then the session is marked cancelled and its `bookings` rows are kept, matching the web page.
- [AC-94] [mcp][db] Given a student or a staff caller who does not own the lab, when `schedule_session` or `cancel_session` is called, then it is refused and nothing changes.

### Slice 6: Google sign-in
- [AC-20] [http][db] Given a local account whose email Google marks as verified, when its owner first signs in with Google, then there is still one `users` row and the Google account id is attached to it.
- [AC-52] [http][db] Given a local account, when someone signs in with Google using the same email but Google does not mark it as verified, then they are not linked to that account, no Google id is attached to it, and they see a readable error.
- [AC-53] [http][db] Given an account already linked to a Google account id, when that id signs in with a changed email, then the same `users` row is used and no new row is created.
- [AC-54] [http] Given a Google sign-in that is cancelled by the user or returns a wrong `state`, when the callback is reached, then no session is created and the user lands on `/login` with a readable error.

### Slice 7: smart search
- [AC-31] [ui] Given the fixed clock, when a student searches "labs tomorrow", then only labs available tomorrow are shown and a chip shows the date that was applied.
- [AC-55] [ui] Given applied filter chips, when the student removes or edits a chip, then the list updates to match the remaining filters.
- [AC-33] [http] Given a query of more than 200 characters, when it is submitted, then it is refused before any API call is made.
- [AC-34] [http] Given any response, HTML or JavaScript sent to the browser, when searched, then the value of `ANTHROPIC_API_KEY` does not appear.
- [AC-27] [http][ui] Given the model call times out or is rate-limited, when a student searches, then they see "Smart search is unavailable, showing all labs" with the unfiltered list, the rest of the page still works, and there is no 500.
- [AC-56] [http] Given a model call that takes longer than 5 seconds, when a student searches, then it is abandoned at 5 seconds and AC-27 applies.
- [AC-57] [http] Given a 429 from the API, when a student searches, then the server retries at most once, waiting for `retry-after` only if it is under the 5 second timeout.
- [AC-58] [http] Given a student who has made 10 searches in the last minute, when they make an 11th, then no API call is made and AC-27 applies.
- [AC-32] [http][ui] Given the model returns invalid JSON, or JSON with unknown fields, a date range in the past, or a non-numeric `min_free_places`, when a student searches, then the output is discarded, AC-27 applies, and there is no 500.
- [AC-28] [db] Given any smart search, when it runs, then no row in any table is inserted, updated or deleted.
- [AC-59] [ui] Given applied search filters that match no lab, when the results are shown, then they read "No labs match these filters" and the filter chips are still shown so they can be edited.

## Retired criteria
- AC-16 (lowering a lab's limit): there is no lab-editing feature.
- AC-44 (booking confirmation without reload): merged into AC-4.
- AC-48 (a lab whose time period does not include today cannot be booked): replaced in Slice 3b by AC-83 and AC-84. Booking is no longer limited to today; it closes when the session starts. Its tests were replaced by AC-83 and AC-84 tests in Slice 3b.

## Traceability
<!-- Filled in from Exercise 3 onward: each AC id, and the test that names it. -->
| Criterion | Test |
| --- | --- |
| AC-10 | `TestAC10_DB_LabsShowFreePlacesFromBookingsRows`, `TestAC10_LabCardsShowTitleDescriptionPeriodAndFreePlaces`, `TestListTodayFreePlacesAreLimitMinusBookings_AC10` |
| AC-23 | `TestAC23_DB_NoLabTodayShowsEmptyState`, `TestAC23_NoLabTodayShowsEmptyState` |
| AC-12 | `TestAC12_DB_StaffAddsLabAndLabsGainsOneRow`, `TestAC12_StaffAddsLabAndItAppearsOnLabs`, `TestAddLabAcceptsAPlaceLimitOfOne_AC12`, `TestAddLabCreatesOneLab_AC12` |
| AC-13 | `TestAddLabRejectsBadPlaceLimit_AC13`, `TestAC13_BadPlaceLimitShowsMessageAndCreatesNothing`, `TestAC13_DB_BadPlaceLimitCreatesNoRow` |
| AC-2 | `TestAddLabDuplicateTitleIsConflict_AC2`, `TestAC2_DuplicateTitleIs409`, `TestAC2_DB_DuplicateTitleIs409AndNoRow` |
| AC-14 | `TestOwnerGetsBookersFromGetLabAndListLabBookings_AC14`, `TestAC14_DB_StaffSeesAStudentWhoBookedThroughTheApp`, `TestAC14_DB_StaffSeesExactlyTheStudentsWhoBooked`, `TestAC14_StaffSeesBookersTable`, `TestLabBookingsListsExactlyTheBookers_AC14`, `TestLabBookingsListsPastSessionsMostRecentFirst_AC14` |
| AC-15 | `TestAddLabStudentIsForbidden_AC15`, `TestRequireStaff_AC15`, `TestNewLabFormIsForStaffOnly_AC15`, `TestAC15_StudentGetAndPostNewLabAre403`, `TestAC15_StudentPostOfNewLabIs403WithoutHandlerRoleCheck`, `TestAC15_DB_StudentGetsAndPosts403AndNoRow` |
| AC-45 | `TestLabBookingsStudentIsForbidden_AC45`, `TestAC45_StudentGetsStaffLabPage403`, `TestAC45_DB_StudentGetsStaffLabPage403` |
| AC-4 | `TestAC4_BookingScriptIsServedAndLoaded`, `TestAC4_DB_BookingAddsExactlyOneRowAndLowersTheCount`, `TestAC4_FormBookingRedirectsToTheLabWithConfirmation`, `TestAC4_ScriptBookingConfirmsAndReturnsTheNewCount`, `TestBookTakesOnePlace_AC4` |
| AC-5 | `TestAC5_DB_SecondBookingIs409AndLeavesOneRow`, `TestAC5_SecondBookingIs409`, `TestBookSessionReportsAMissingSessionAndADuplicate_AC47_AC5`, `TestBookTwiceIsAlreadyBooked_AC5` |
| AC-1 | `TestAC1_DB_FullLabIs409AndCreatesNoRow`, `TestAC1_FullLabIs409WithLinksToTheListAndHome`, `TestBookFullLabIsRefused_AC1` |
| AC-8 | `TestAC8_DB_FullLabShowsFullAndRefusesAPost`, `TestAC8_FullLabShowsADisabledFullButtonAndRefusesAPost` |
| AC-6 | `TestAC6_DB_BookSessionHoldsTheSessionLockWhileTheCheckRuns`, `TestAC6_DB_ConcurrentBookingsOfTheLastPlaceLetExactlyOneThrough`, `TestBookTheLastPlaceThenFull_AC6` |
| AC-46 | `TestAC46_DB_StaffBookingIs403AndCreatesNoRow`, `TestAC46_StaffBookingIs403`, `TestBookByStaffIsForbidden_AC46`, `TestRequireStudent_AC46` |
| AC-47 | `TestAC47_DB_UnknownLabIs404AndCreatesNoRow`, `TestAC47_UnknownLabIs404`, `TestBookSessionReportsAMissingSessionAndADuplicate_AC47_AC5`, `TestBookUnknownSessionIsNotFound_AC47`, `TestGetLabShowsFreePlaces_AC47` |
| AC-48 (retired) | Replaced by AC-83 and AC-84 in Slice 3b; its three tests were removed with the "today only" rule. |
| AC-19 | `TestAC19_BookingWithoutSessionIs401`, `TestAC19_DB_BookingWithoutSessionIs401AndBookingsUnchanged`, `TestAC19_ScriptBookingWithoutSessionIs401`, `TestBookWithoutCallerIsUnauthenticated_AC19` |
| AC-60 | `TestAddLabStoresTheCallerAsOwner_AC60`, `TestAC60_DB_AddedLabIsOwnedByTheStaffMember`, `TestAC60_DB_MigrationGivesExistingLabsTheFirstStaffAccount`, `TestAC60_DB_LabsMustHaveAnExistingOwner`, `TestDemoLabsAreOwnedByStaffFromThreeAccounts_AC60` |
| AC-61 | `TestLabsShowTheirOwnersName_AC61`, `TestAC61_LabsShowTheirOwner` |
| AC-62 | `TestAC62_CardTitlesLinkStaffToTheirOwnLabsOnly`, `TestAC62_DB_EachStaffMemberSeesOnlyTheirOwnLabs`, `TestAC62_HeaderLinksStaffToMyLabs`, `TestAC62_MyLabsListsExactlyTheCallersLabs`, `TestMyLabsListsOnlyTheCallersLabs_AC62`, `TestMyLabsShowsTheNextSessionOrNone_AC62`, `TestSessionsBetweenAndMyLabsNextSession_AC62` |
| AC-63 | `TestMyLabsIsEmptyForStaffWhoOwnNothing_AC63`, `TestAC63_MyLabsWithoutLabsShowsTheEmptyState` |
| AC-64 | `TestRequireOwner_AC64`, `TestLabBookingsAreForTheOwnerOnly_AC64`, `TestAC64_AnotherStaffMembersLabIs403`, `TestAC64_DB_AnotherStaffMembersLabIs403` |
| AC-65 | `TestMyLabsIsForStaffOnly_AC65`, `TestAC65_StudentGetsMyLabs403` |
| AC-66 | `TestAC66_DB_MigrationDownRefusesALabWithTwoSessions`, `TestAC66_DB_MigrationMakesEachLabsPeriodItsOneSession` |
| AC-67 | `TestAC67_DB_AddSessionsAddsRowsForTheLab`, `TestAC67_DB_OwnerAddsASessionAndAStudentBooksIt`, `TestAC67_OwnerAddsASessionAndItIsListed`, `TestAC67_StaffPageGroupsSessionsByDayWithTheirOwnBookers`, `TestAddSessionAddsOneSession_AC67`, `TestAddSessionMayRunPastMidnight_AC67`, `TestDemoHasALabThatRunsWeekly_AC67` |
| AC-68 | `TestAC68_BadSessionFormIsShownAgainWithItsMessage`, `TestAddSessionRejectsABadForm_AC68` |
| AC-69 | `TestAC69_DB_OverlappingSessionIs409AndAddsNothing`, `TestAC69_DB_OverlappingSessionIsRefused`, `TestAC69_DB_TwoOverlappingSessionsAddedAtOnceGiveExactlyOne`, `TestAC69_OverlappingSessionIs409`, `TestAddSessionOverlappingAnotherIsConflict_AC69` |
| AC-70 | `TestAC70_DB_AddSessionsIsAllOrNone`, `TestAC70_WeeklyRepeatAddsOneSessionPerWeek`, `TestAddSessionRepeatKeepsTheLocalTimeAcrossADaylightSavingChange_AC70`, `TestAddSessionRepeatThatClashesAddsNone_AC70`, `TestAddSessionRepeatsWeeklyUpToTheDate_AC70` |
| AC-71 | `TestAC71_DB_EachSessionCountsItsOwnBookings`, `TestBookTwoSessionsOfOneLabCountSeparately_AC71` |
| AC-72 | `TestAC72_DB_AnotherStaffMemberCannotAddSessions`, `TestAC72_OnlyTheOwnerMayAddSessions`, `TestAddSessionIsForTheLabsOwnerOnly_AC72` |
| AC-83 | `TestAC83_DB_StartedSessionIsRefusedAndCreatesNoRow`, `TestAC83_StartedSessionShowsClosedAndRefusesABooking`, `TestBookSessionThatStartedIsRefused_AC83`, `TestListTodayMarksStartedSessions_AC83` |
| AC-84 | `TestAC84_DB_SessionTomorrowCanBeBooked`, `TestAC84_SessionTomorrowCanBeBooked`, `TestBookSessionTomorrowSucceeds_AC84` |
| AC-73 | `TestAC73_DB_CancelSessionKeepsItsBookings`, `TestAC73_DB_CancelThroughTheAppKeepsTheBookings`, `TestAC73_OwnerCancelsASessionAndStudentsSeeItCancelled`, `TestCancelSessionKeepsItsBookings_AC73`, `TestDemoHasACancelledSessionThatKeptItsBooking_AC73` |
| AC-74 | `TestAC74_BookingACancelledSessionIs409`, `TestAC74_DB_BookingThatWaitedForACancelIsRefused`, `TestBookCancelledSessionIsRefused_AC74` |
| AC-75 | `TestAC75_CancellingAStartedSessionIs409`, `TestAC75_DB_StartedSessionCannotBeCancelled`, `TestCancelRefusals_AC75`, `TestStaffPageSaysWhatMayBeCancelledOrDeleted_AC75_AC77` |
| AC-76 | `TestAC76_CancelPageNamesTheSessionAndChangesNothing`, `TestAC76_DeletePageNamesTheLabAndChangesNothing`, `TestCancelPageNamesTheSessionAndChangesNothing_AC76` |
| AC-77 | `TestAC77_DB_DeleteAnUnbookedLabAndRefuseABookedOne`, `TestAC77_DB_DeleteLabRemovesItAndItsSessions`, `TestAC77_DB_DeleteLabWithAnyBookingIsRefused`, `TestAC77_DeletingABookedLabIs409`, `TestAC77_OwnerDeletesALabNobodyBooked`, `TestDeleteLabNobodyBooked_AC77`, `TestDeleteLabWithBookingsIsRefused_AC77`, `TestStaffPageSaysWhatMayBeCancelledOrDeleted_AC75_AC77` |
| AC-78 | `TestAC78_DB_BookingAfterTheLabWasDeletedIsNotFound`, `TestAC78_DB_DeleteAndBookAtTheSameTimeNeverLoseABooking`, `TestAC78_DB_DeleteWhileABookingHoldsTheLockIsRefused` |
| AC-79 | `TestAC79_DB_AnotherStaffMemberCannotCancelOrDelete`, `TestAC79_OnlyTheOwnerMayCancelOrDelete`, `TestCancelIsForTheLabsOwnerOnly_AC79`, `TestDeleteIsForTheLabsOwnerOnly_AC79` |
| AC-80 | `TestAC80_DB_TomorrowsListShowsTomorrowsSessionOnly`, `TestAC80_DayPageMeetsTheDefinitionOfDone_AC30_AC41`, `TestAC80_LabListShowsTheChosenDay`, `TestAC80_TodayKeepsItsHeadingWithOrWithoutADate`, `TestListDayKeepsADaylightSavingDayWhole_AC80`, `TestListDayShowsTheChosenDay_AC80`, `TestListDayWithoutADateIsToday_AC80` |
| AC-81 | `TestAC81_InvalidDateIs400WithASentence`, `TestListDayRefusesAnInvalidDate_AC81` |
| AC-82 | `TestAC82_AnEmptyDayThatIsNotTodaySaysSo`, `TestAC82_DB_AnEmptyDayFarAheadSaysSo` |
| AC-85 | `TestAC85_ChangesWithoutTheRightTokenAre403AndChangeNothing`, `TestAC85_DB_ChangesWithoutTheTokenLeaveTheDatabaseAsItWas`, `TestAC85_ScriptBookingWithoutTheHeaderIs403AsJSON`, `TestCSRFValidAcceptsOnlyTheSessionsOwnToken_AC85_AC86` |
| AC-86 | `TestAC86_AnotherUsersTokenIsRefused`, `TestAC86_DB_AnotherAccountsTokenIsRefused`, `TestCSRFTokenIsPerSession_AC86`, `TestCSRFValidAcceptsOnlyTheSessionsOwnToken_AC85_AC86` |
| AC-87 | `TestAC87_BookingScriptSendsTheTokenInAHeader`, `TestAC87_EveryChangingFormCarriesTheSessionsToken`, `TestMyBookingsPageMeetsTheDefinitionOfDone_AC30_AC42_AC87` |
| AC-7 | `TestAC7_CancelBookingFormsAreOnTheLabPageAndMyBookings`, `TestAC7_DB_CancelBookingRemovesTheRowAndFreesThePlace`, `TestAC7_DB_CancelRemovesTheRowAndFreesThePlace`, `TestAC7_StudentCancelsABookingAndThePlaceComesBack`, `TestCancelBookingFreesThePlace_AC7` |
| AC-49 | `TestAC49_CancellingWithoutABookingIs404`, `TestAC49_DB_CancellingABookingYouDoNotHave`, `TestCancelBookingYouDoNotHaveIsNotFound_AC49` |
| AC-50 | `TestAC50_DB_ABookingThatWaitedForACancelGetsTheFreedPlace`, `TestAC50_DB_AfterACancelAnotherStudentFillsTheLastPlace` |
| AC-88 | `TestAC88_CancellingAfterTheStartIs409`, `TestAC88_DB_CancelAfterTheStartKeepsTheBooking`, `TestCancelBookingAfterTheStartIsRefused_AC88`, `TestCancelBookingRefusedByTheCheckKeepsTheRow_AC88` |
| AC-89 | `TestAC89_BookedSessionShowsBookedAfterAReload`, `TestBookingsOfAndSessionsBookedByAreTheUsersOwn_AC89_AC90`, `TestListsMarkTheStudentsOwnSessions_AC89` |
| AC-24 | `TestAC24_ToolsListShowsTheNineToolsWithDescriptions` |
| AC-39 | `TestAC39_ACookieIsNotATokenForMCP`, `TestAC39_DB_ToolsWithoutAValidTokenChangeNothing`, `TestAC39_ToolsWithoutAValidTokenAreRefusedAndChangeNothing` |
| AC-37 | `TestAC37_BookLabAndCancelBookingSayWhatTheWebPagesSay`, `TestAC37_DB_BookLabMatchesTheHTTPBooking` |
| AC-25 | `TestAC25_BookLabOnAFullSessionIsRefused`, `TestAC25_DB_FullSessionIsRefusedWithTheHTTPSentence` |
| AC-51 | `TestAC26_AC51_ToolsRefuseTheWrongRole`, `TestAC51_AC26_DB_TheWrongRoleChangesNothing` |
| AC-38 | `TestAC38_AddLabWithATakenTitleIsRefused`, `TestAC38_DB_TakenTitleIsRefusedWithTheHTTPSentence` |
| AC-26 | `TestAC26_AC51_ToolsRefuseTheWrongRole`, `TestAC51_AC26_DB_TheWrongRoleChangesNothing` |
| AC-35 | `TestAC35_CancelBookingOfAnotherStudentsBookingIsRefused`, `TestAC35_DB_AnotherStudentsBookingIsKept` |
| AC-36 | `TestAC36_StudentGetsTheLabWithoutBookersAndNoBookingList` |
| AC-40 | `TestAC40_MyBookingsReturnsOnlyTheCallersBookings` |
| AC-91 | `TestAC91_StaffWhoDoNotOwnTheLabGetNoBookingLists` |
| AC-92 | `TestAC92_DB_ScheduleSessionAddsTheSameRowsAsTheWebPage`, `TestAC92_ScheduleSessionAddsAndRefusesLikeTheWebPage` |
| AC-93 | `TestAC93_CancelSessionKeepsItsBookings`, `TestAC93_DB_CancelSessionKeepsItsBookingsLikeTheWebPage` |
| AC-94 | `TestAC94_DB_OnlyTheOwnerSchedulesAndCancels`, `TestAC94_OnlyTheOwnerSchedulesAndCancelsSessions` |
| AC-90 | `TestAC90_DB_AStaffCancelShowsOnTheStudentsBookings`, `TestAC90_MyBookingsShowsTitleTimeAndCancelledByStaff`, `TestAC90_NoBookingsYetSaysSo`, `TestBookingsOfAndSessionsBookedByAreTheUsersOwn_AC89_AC90`, `TestMyBookingsListsOwnBookingsAndMarksCancelled_AC90`, `TestMyBookingsPastAreMostRecentFirst_AC90` |

## Spec vs result
<!-- After the agent builds the slice: which criteria passed, which it missed or reinterpreted, and what you changed in this spec because of it. -->

### Slices 0 and 1 (checked 2026-11-03)
Checked three ways: `go test ./internal/...` against the in-memory store and a real `labwiz_test` database, the running server, and reading the code.

All failing handler tests (in-memory and DB) fail for one reason: the session cookie's `Expires` is the fixed test clock (2026-10-22) plus 7 days, so the test client's cookie jar, which uses the real date, drops it as expired. Every request after sign-in is then anonymous. The tests started failing on 2026-10-29 without any code change. The service-layer tests all pass. Fixed the same day: the cookie now uses `Max-Age` (`internal/auth/session.go`, guarded by `TestSetCookieUsesMaxAgeNotExpires`), and every test passes, including the DB tests.

| Criterion | Result | Notes |
| --- | --- | --- |
| AC-3 | Passed after the cookie fix | Sign-in lands on `/labs` with today's three labs. |
| AC-9 | Passed | A forged cookie is also redirected. |
| AC-17 | Passed | The code's text is "Invalid email or password." (with a full stop). Timing is equalised with a dummy bcrypt compare. |
| AC-18 | Passed | Unit and DB tests. |
| AC-19 | Passed, but trivially | The booking endpoint is a stub, so `bookings` cannot change anyway. Must be re-checked in Slice 3. |
| AC-21 | Passed after the cookie fix | `Cache-Control: no-store` and the session row is deleted on sign-out. Reinterpreted: checked over HTTP, not with a real browser's Back button (the criterion now says so). |
| AC-10 | Passed | Cards show all lab information |
| AC-11 | Untested | No test names it. The list is queried on every request with no cache, so it should pass. |
| AC-22 | Passed | Covered inside the AC-3 tests; no test title contains AC-22. |
| AC-23 | Passed | Shows "No labs available today". |
| AC-29 | Untested | No browser test. The CSS is single-column with 44px touch targets. `/staff/labs/new` does not exist yet. |
| AC-30 | Passed after the cookie fix | No forbidden rules in the served CSS or HTML. |
| AC-41 | Partly passed | Login inputs have labels. Keyboard-only use is untested; `/staff/labs/new` does not exist yet. |
| AC-42 | Passed after the cookie fix | `/login`, `/labs` and the error page have distinct titles; all error statuses share "Error · Labwiz". |
| AC-43 | Passed after the cookie fix | 404, 405 and 500 each show one sentence. Unknown errors map to one generic sentence and a panic shows the 500 page. 400, 403 and 409 come with later slices. |

What changed in this spec because of it:
- Decisions: the fixed clock drives server logic only, and cookie expiry is relative (`Max-Age`). This is the cookie bug above.
- Decisions: "today" is the server's local timezone, the time-period display format is fixed, and free places are defined from Slice 1 on (for AC-10).
- Decisions: `[ui]` criteria are checked over HTTP unless the behaviour exists only in a browser. AC-21 is rewritten as the HTTP and DB checks it was really tested with.
- AC-17 gives the exact sentence, with its full stop.
- AC-19 notes that it is checked again once Slice 3 replaces the stub.
- AC-23 applies without a search; new AC-59 covers the empty state with search filters.
- The definition of done covers only the pages that exist in the slice.
- AC-42: all error pages count as one page and share one title.

### Slice 1 follow-up and Slice 3 (checked 2026-11-17)
Checked with `go test ./...` against the in-memory store and a real `labwiz_test` database (no test skipped), `go test -race` on the booking tests, `node --check` on `booking.js`, and by reading the code. AC-10 and AC-23, missed on 2026-11-03, now pass: the cards show description, time period and free places, and an empty day shows "No labs available today".

| Criterion | Result | Notes |
| --- | --- | --- |
| AC-4 | Passed over HTTP | The script gets JSON (`message`, `free_places`) and rewrites the card; without JS the form redirects to `/labs/{id}?booked=1`. The script's behaviour in a real browser is not yet checked by hand. |
| AC-5 | Passed | Refused in the service from a check made under the lab lock; the `UNIQUE (user_id, lab_id)` constraint is the backstop. |
| AC-1 | Passed | The error page now links to `/labs` and `/` on every error, and the script shows the same two links. |
| AC-8 | Passed | Server-rendered disabled "Full" button; a direct POST is 409. |
| AC-6 | Passed | `labs` row lock (`SELECT … FOR UPDATE`) in one transaction. The first test version passed even with the lock removed, because a cold connection pool kept the goroutines from overlapping. The tests now warm the pool, and a store test forces the race with a slow check. Both fail without the lock and pass with it. |
| AC-46 | Passed | `RequireStudent` in the service; no role check in the handler. |
| AC-47 | Passed | `POST /labs/{id}/book` and the new `GET /labs/{id}`, including a non-numeric id. |
| AC-48 | Passed, reinterpreted | "Refused" was not specific: it is now 409 with "This lab is not open for booking today.", using the same "available today" rule as the lab list. |
| AC-19 | Passed, re-checked | Against the real endpoint. A script request without a session gets the 401 sentence as JSON. |
| AC-29, AC-41 | Untested | `/labs/{id}` and the Book button are new. 375 px layout and keyboard-only booking still need a check by hand. |
| AC-30, AC-42, AC-43 | Passed | Extended to `booking.js` and `/labs/{id}`. Booking errors show one sentence; the JSON is read only by the script. |

Decisions taken while building it:
- One endpoint for both clients: a request with `Accept: application/json` gets JSON, a plain form post gets a redirect or the error page.
- The booking rules (already booked, not open today, full) live in `Labs.Book` and are passed to `store.BookLab`, which runs them inside its transaction while the lab is locked. The rules stay in the service, and atomicity stays in the store.
- No CSRF token: the session cookie is `SameSite=Lax`, so a cross-site POST carries no session. (Replaced in Slice 3e by CSRF tokens.)
- `/bookings` and the Cancel button move to Slice 4, with cancelling. After a booking, the script turns the button into a disabled "Booked". A reload shows "Book" again until Slice 4 knows the caller's own bookings; pressing it then gives "You have already booked this lab". (Closed in Slice 4: a reload shows "Booked".)

What changed in this spec because of it:
- AC-48 gives the status and the exact sentence.

### Re-plan before Slice 4 (2026-11-17, no code yet)
New features were discussed and added as Slices 3a–3e, before Slices 4–7 build on the data model:
- Labs get an owner, and owner-only staff pages: "My labs" and one owned lab (AC-60–65). Until now every staff member could see every lab's booking list, and a lab outside today could only be found again through the redirect after adding it.
- A lab and its schedule are separate: a lab has many sessions, added as a list with an optional weekly repeat, never overlapping (AC-66–72). A calendar grid was rejected: too much plain JavaScript for 375 px and keyboard use.
- Staff cancel sessions instead of deleting booked labs, so a student can see the cancellation without notifications; a lab can be deleted only while nobody has booked it (AC-73–79).
- Booking is no longer limited to today: students browse by date (AC-80–82), and booking closes when the session starts (AC-83, AC-84; moved into Slice 3b when it was planned, since 3b rewrites the booking rule anyway). AC-48 is retired.
- CSRF tokens on every form that changes something, now that staff can delete and cancel (AC-85–87). This reverses the Slice 3 decision to rely on `SameSite=Lax` alone.
- Gaps closed in Slice 4: the "Booked" state after a reload, cancelled sessions on `/bookings`, and no cancelling after a session starts (AC-88–90). MCP: owner-only booking lists (AC-91), and two new tools, `schedule_session` and `cancel_session`, so staff can manage sessions through MCP as on the web pages (AC-92–94; AC-24 and AC-39 now count nine tools). Lab editing stays a non-goal.
- The decisions and the retired criteria moved out of the acceptance-criteria section, which the spec self-check read as criteria without an id.

### Slice 3a (checked 2026-11-17)
Checked with `go test ./...` against the in-memory store and a real `labwiz_test` database (no test skipped), by migrating the development database, which held Slice 3 data, and by reading the code. Service coverage stays at 100%.

| Criterion | Result | Notes |
| --- | --- | --- |
| AC-60 | Passed | `Labs.Add` makes the caller the owner. Migration `000003_lab_owners` gives labs from before owners existed to the lowest-id staff account and stops with a readable error if there is none. A store test runs it from schema version 2 over two labs and a booking; on the development database, 15 labs went to Stella and all 13 bookings were kept. The database refuses a lab with no owner or an unknown one. |
| AC-61 | Passed | "Run by …" on the lab cards and the lab page, from one shared template part. |
| AC-62 | Passed | `/staff/labs` lists the caller's labs on every day, not only today, with "N of M places booked". Added: a "My labs" header button for staff, and card titles that link staff to the staff page of their own labs only. |
| AC-63 | Passed | "You have no labs yet." with a link to `/staff/labs/new`. |
| AC-64 | Passed | `RequireOwner` in the service, after the lookup; a student is still refused before it (AC-45). On another owner's public lab page, staff see neither the booking-list link nor a Book button. |
| AC-65 | Passed | `MyLabs` calls `RequireStaff`. |
| AC-14 | Passed, reworded | The owner sees the booking list; other staff get AC-64's 403. Test fixtures now give their labs an owner. |
| AC-15 | Passed, re-checked | The handler no longer checks the role; `Labs.NewLabForm` and `Labs.Add` refuse a student with 403 and no row. |
| AC-29, AC-41 | Untested | `/staff/labs` at 375 px and by keyboard still need a check by hand. The page has no inputs. |
| AC-30, AC-42 | Passed | `/staff/labs` has no forbidden rules and its own title, "My labs · Labwiz". |

Decisions taken while building it:
- `RequireOwner` is exported next to `RequireStaff`, so Slices 3b and 3c and the MCP tools reuse one ownership rule.
- A lab's owner must be staff. The database cannot check another table's column, so the service does: only staff can add labs.
- The demo seed gives its labs to Dana and Omar, so the demo has three owners and "My labs" differs per account.

What changed in this spec because of it:
- Architecture: the trace names the owner and `RequireOwner`. The fixed handler-side role check is replaced, as the one place where the code does not match the layering, by `seed.Demo` booking through the store and skipping the service's rules.

### Slice 3b (checked 2026-11-17)
Checked with `go test ./...` against the in-memory store and a real `labwiz_test` database (no test skipped), `go test -race` on the booking and overlap tests, by breaking the lock and the overlap constraint on purpose, by migrating the development database, and by reading the code. Service coverage stays at 100%.

| Criterion | Result | Notes |
| --- | --- | --- |
| AC-66 | Passed | Migration `000004_lab_sessions` makes each lab's period its one session and moves every booking onto it. A store test runs it from version 3 with two labs and three bookings and back down; the development database went from 15 labs and 13 bookings to 15 sessions with all 13 bookings. The down migration refuses a lab with other than one session; that test runs the real down file in a rolled-back transaction, so the shared test database is not left dirty. |
| AC-67 | Passed | The owner's staff page has the add-session form. Sessions may run past midnight. A new lab appears on `/labs` only once it has a session that day. |
| AC-68 | Passed | Nine kinds of bad form in the service; over HTTP the page comes back with the message, the typed values and its booking lists. |
| AC-69 | Passed | The exclusion constraint decides. Four overlapping sessions added at once leave exactly one. Dropping the constraint on the test database made the AC-69 and AC-70 tests fail; restoring it made them pass. A cancelled session no longer blocks its time (for Slice 3c). |
| AC-70 | Passed | One session per week up to and including the date, at most 26, all or none. Weekly copies keep their local time across a daylight saving change (Europe/Kyiv, 25 Oct 2026). |
| AC-71 | Passed | Free places and the unique booking are per session: one student can book two sessions of one lab. |
| AC-72 | Passed | `AddSessions` runs the same owner check as the booking list. |
| AC-83, AC-84 | Passed, moved from Slice 3d | Any future session can be booked until it starts; a started session shows a disabled "Closed" and a POST is 409. |
| AC-6 | Passed, re-checked | The lock moved from the lab's row to the session's. Removing `FOR UPDATE OF s` made the store race test fail 2 of 2 (5 bookings for 1 place). |
| AC-12, AC-14 | Passed, reworded | AC-12 no longer has a time period; AC-14 lists bookers per session. |
| AC-29, AC-41 | Untested by hand | The staff page's new form and the session lists at 375 px and by keyboard. AC-41's test now also checks the session form's labels. |

Decisions taken while building it:
- `cancelled_at` is in the table from the start, so the overlap constraint is written once; Slice 3c only sets it.
- `bookings.session_id` is `ON DELETE RESTRICT`: a session with bookings cannot disappear, which Slice 3c relies on to refuse deleting a booked lab.
- The handler tests' fixed clock moved from 10:00 to 08:30, so the seeded 09:00 session is still open and the 08:00–20:00 one has started. The service tests keep 10:00.
- Database tests ask explicitly for a seeded lab's id or its session's id: in a fresh test database they happen to be equal, and tests that mix them up would still pass.
- An unknown session gives "This session does not exist." (404); the booking sentences still say "lab", which is what a student books.

What went wrong:
- Step 1 created `internal/store/sessions.go` over the existing file of the same name, which held the sign-in session queries. The store tests did not use them, so the step looked green while the server did not build. It was found in step 3, restored from git unchanged, and the lab-session code now lives in `lab_sessions.go`. Nothing broken was committed, because the steps were not committed one by one.

What changed in this spec because of it:
- AC-83 and AC-84 moved from Slice 3d to 3b; 3d is only browsing by date.
- The UI line for the add-session form, AC-12, AC-14 and AC-67 match what was built.
- The Architecture section traces adding a session and booking a session.

### Slice 3c (checked 2026-11-17)
Checked with `go test ./...` against the in-memory store and a real `labwiz_test` database (no test skipped), `go test -race` on the cancel, delete and booking tests, by breaking the booking lock, the `RESTRICT` key and the cancelled Book button on purpose, by re-seeding the development database, and by reading the code. Service coverage stays at 100%.

| Criterion | Result | Notes |
| --- | --- | --- |
| AC-73 | Passed | `cancelled_at` is set and the bookings stay; `/labs` and `/labs/{id}` show "Cancelled" with no Book form, and the owner's page keeps the "Booked by" table. Making the template show the Book button again made the test fail. |
| AC-74 | Passed | Refused in the booking check, before "already booked". A store test holds the cancel's lock while a booking waits: the booking then sees the session cancelled. Without `FOR UPDATE OF s` in `BookSession` that test failed 2 of 2. |
| AC-75 | Passed | Also refused on the confirmation page, so it never offers a cancel the POST would refuse; the staff page shows no Cancel link on a started or cancelled session. |
| AC-76 | Passed | One template for both pages; a GET changes nothing; "Keep it" goes back to the lab's staff page. |
| AC-77 | Passed | Deleting removes the lab and its sessions; any booking (past, upcoming or on a cancelled session) refuses it. With the key changed to `ON DELETE CASCADE` on the test database, the delete tests failed: the bookings were deleted with the lab. |
| AC-78 | Passed, with a limit | Both orders are forced deterministically in store tests: a booking holding the lock makes the delete fail with 409 and stays; a booking after the delete gets 404. The 20-round race through the service never lost a booking, but the booking won every round, because the delete looks the lab up first, so that test alone does not show the "delete first" outcome. |
| AC-79 | Passed | Owner only for both pages and both actions; a student is refused before the lookup. |
| AC-29, AC-41 | Untested by hand | The confirmation pages and the staff page's links at 375 px and by keyboard. The pages have no inputs, only a button and a link. |
| AC-30, AC-42 | Passed | The two new pages have their own titles, "Cancel a session · Labwiz" and "Delete a lab · Labwiz". |

Decisions taken while building it:
- A cancelled session shows "Cancelled" instead of the Book form, not a disabled button: there is nothing to book.
- The confirmation pages are plain forms. SPEC's "a script may turn it into a dialog" is left for later.
- A `RESTRICT` key reports SQLSTATE `23001`, not `23503` as planned; the store maps both to `ErrInUse`. The first test run showed the raw database error.

What went wrong:
- `EnsureSession`, written in Slice 3b, relied on the overlap constraint to find an existing session. The constraint ignores cancelled sessions, so once the demo cancelled a session, a second seed run made a live copy of it and booked it again. The demo seed test caught it (bookings 11 → 12 on the second run). `EnsureSession` now looks for an identical session first, cancelled or not; its store test and a demo test that seeds twice cover it.

What changed in this spec because of it:
- The Architecture section says how cancelling and deleting keep bookings safe.

### Slice 3d (checked 2026-11-17)
Checked with `go test ./...` against the in-memory store and a real `labwiz_test` database (no test skipped), by breaking the empty state on purpose, by reading the code, and by hand in the browser.

| Criterion | Result | Notes |
| --- | --- | --- |
| AC-80 | Passed | `/labs?date=` shows that day's sessions under "Labs on Friday 23 October 2026", with previous/next day links, a Today link off today, and a labelled date field in a plain GET form. Today, with or without a date, keeps "Labs available today". A daylight saving day (25 Oct 2026 in Kyiv, 25 hours) stays whole. |
| AC-81 | Passed | Seven invalid forms in the service and five over HTTP give 400 with "Please choose a valid date."; no parsing error text reaches the page. |
| AC-82 | Passed | Changing the text back to the today empty state made both AC-82 tests fail. |
| AC-22, AC-23 | Passed, re-checked | `/labs` without a date still shows only today, with its own empty state. |
| AC-29, AC-41 | Checked by hand | The navigation row and the date field work in the browser; the field has its label (tested). |
| AC-30 | Passed | The new CSS rules have no transitions. |

Decisions taken while building it:
- An empty `?date=` is today: the handler cannot tell it from no date, and today is the sensible answer.
- The heading always includes the year. Leaving it out for the current year would need the real clock in the renderer, which the tests' fixed clock would not match.

What changed in this spec because of it:
- The `?date=` decision says any day may be shown, past ones too.

### Slice 3e (checked 2026-11-17)
Checked with `go test ./...` against the in-memory store and a real `labwiz_test` database (no test skipped), `go test -race` on the CSRF tests, by switching the token check off on purpose, by starting the server with a real and with a placeholder secret, and by reading the code.

| Criterion | Result | Notes |
| --- | --- | --- |
| AC-85 | Passed | All six changing POSTs (book by form and by script, add lab, add session, cancel, delete, sign out) with no field, an empty token and a wrong one give 403 with the sentence; nothing changes in any table, and a refused sign-out leaves the user signed in. With the check switched off, all AC-85 and AC-86 tests failed. |
| AC-86 | Passed | The token is per sign-in session, so another user's token never matches; tested over HTTP, against the database, and on the token itself. |
| AC-87 | Passed | On eight pages, every POST form carries exactly the session's token, and a csrf-token meta tag feeds booking.js's X-CSRF-Token header. The sign-in page has no token. |
| AC-19 | Passed, re-checked | A visitor who is not signed in still gets 401 before any token check. |
| AC-29, AC-41 | Unchanged | The token is a hidden field; there is nothing to label or to see. |

Decisions taken while building it:
- The token is signed, not stored: `HMAC-SHA256(SESSION_SECRET, session cookie token)`. No schema change, and a token dies with its session.
- The check lives in the session middleware, so it covers every changing route, and sign-out moved behind it.
- A form too large to read is now refused in the middleware with one sentence, "The form could not be read. Please try again."; two tests' expected text changed to it.
- `.env.example` keeps the short placeholder `change-me`, so copying it unchanged makes the server refuse to start instead of running with a public secret.
- Tests read HTML: two button patterns now allow the hidden token field, and the "no unlabelled input" check ignores hidden inputs.

What went wrong:
- After `SESSION_SECRET` was set in `.env`, the server still refused to start: `load-env.ps1` keeps variables already set in the PowerShell session, so the old `change-me` stayed. The script now lists the variables it kept and takes `-Force` to reload them; AGENTS.md says so.
- The real secret was then found pasted into `.env.example`, which is tracked, and a `git diff` printed it in the agent session. It was never committed. `.env.example` was restored to the placeholder and the secret in `.env` was rotated; the old value is useless.

What changed in this spec because of it:
- The Auth section says how the token is made and that the server needs a 32-character `SESSION_SECRET`.
- The Slice 3 note that relied on `SameSite=Lax` alone is marked as replaced.
- The Architecture section says where the token check sits.

### Slice 4 (checked 2026-11-18)
Checked with `go vet ./...` and `go test ./...` against the in-memory store and a real `labwiz_test` database (no test skipped), `go test -race` on the cancel and booking tests, by breaking the cancel's lock and the "Booked" button on purpose, and by reading the code. Service coverage stays at 100%.

| Criterion | Result | Notes |
| --- | --- | --- |
| AC-7 | Passed | Cancelling from `/bookings` or `/labs/{id}` deletes the row, the count goes up by 1, and the session can be booked again. The POST redirects to `/bookings` with "Booking cancelled.". |
| AC-49 | Passed | A session the student did not book, including one another student booked, gives 404 "You have no booking for this session."; an unknown session gives the session 404. Nobody's booking changes. |
| AC-50 | Passed | Sequentially against PostgreSQL, as written, and as a store race: a cancel holds the session lock while another student's booking of the full session waits, then gets the freed place, and the count equals the limit. Without `FOR UPDATE OF s` in `CancelBooking` the race test failed. |
| AC-88 | Passed | A running or finished session gives 409 with the sentence and the booking is kept; `/bookings` offers no cancel for it. |
| AC-89 | Passed | `/labs` and `/labs/{id}` show a disabled "Booked" after a reload, for the student who booked only. Showing "Book" again in the template made the test fail. |
| AC-90 | Passed | `/bookings` lists title (linked to the lab), day and time, upcoming first and past most recent first; a cancelled session reads "Cancelled by staff" and has no cancel button. With no bookings it says so, with a link to `/labs`. |
| AC-87 | Passed, extended | Every POST form on `/bookings` carries the session's token, and a cancel without it is 403. |
| AC-30, AC-42 | Passed | `/bookings` has its own title and no animations. |
| AC-29, AC-41 | Not checked yet | `/bookings` at 375 px and by keyboard is still to be checked by hand. |

Decisions taken while building it:
- Cancelling deletes the row: free places are counted, so no schema change was needed.
- The delete is keyed on the caller's id, so another student's booking can only ever be "not yours" (404). The MCP tool `cancel_booking` will rely on it.
- A booking of a cancelled session that has not started can still be cancelled; it then leaves the list.
- "Booked" comes from one extra query per page (`SessionsBookedBy`), for students only, so the list queries did not change.
- `/labs` cards show only "Booked"; the "Cancel booking" button is on the lab page and on `/bookings`, to keep the cards compact.
- `/bookings` is for students: staff get 403 and no header link.

What went wrong:
- A shell heredoc broke on quoting while writing the test files; they were written again with the editor tools.
- The first version of the HTTP tests used two placeholder names that did not exist (`storeSessionView`, `omarlessStudent`); they were replaced with `store.SessionView` and an `addZoe` helper before anything was committed.
- Pressing "Cancel booking" in the browser cancelled the booking but then showed a second "Booked" button and "undefined of undefined places free" until a reload. The cancel form had the class `book-form`, so `booking.js` took it over, followed the redirect to `/bookings` and read the 200 as a booking. The HTTP tests post forms without the script, so they missed it. The cancel form now has its own class and posts normally; `TestAC7_OnlyBookFormsAreTakenOverByTheBookingScript` checks that only Book forms carry `book-form`, and failed with the old class.
- The handler tests now take about 50–60 s instead of about 22 s. The slowdown appeared with step 1, which did not touch the handlers; not investigated yet.

What changed in this spec because of it:
- The Architecture section says that cancelling a booking takes the session lock.
- The Slice 3 note about "Book" after a reload is marked as closed.

### Slice 5 (checked 2026-11-18)
Checked with `go vet ./...` and `go test ./...` against the in-memory store and a real `labwiz_test` database (no test skipped), `go test -race` on the MCP tests, by breaking the code on purpose four times, by calling the tools over plain JSON-RPC on a second server against the development database, and by hand from Claude Code in VS Code, as a student and as staff. Service coverage stays at 100%.

| Criterion | Result | Notes |
| --- | --- | --- |
| AC-24 | Passed | `tools/list` without a token gives exactly the nine tools; every description says who may call it and what refuses it. |
| AC-39 | Passed | All nine tools with no token, a made-up one and an expired one give "Please sign in to continue." and change nothing, in memory and in PostgreSQL. A signed-in browser cookie without the bearer header is refused too. Making a missing token act as a staff account made these tests fail. |
| AC-37 | Passed | `book_lab` returns `{message, free_places, place_limit}` with "Booking confirmed.", and adds one row for the caller; a second student booking the same session through the booking script gets the same sentence and one place fewer. Giving the tool its own wording made the test fail. |
| AC-25 | Passed | A full session gives the booking script's 409 sentence, "This lab is full.", and no row. |
| AC-51, AC-26 | Passed | Staff are refused by `book_lab` and `cancel_booking`, students by `add_lab`; no row changes. Making `add_lab` treat every caller as staff made the test fail. |
| AC-38 | Passed | A taken title gives the same sentence as `POST /staff/labs/new`, and no row; a place limit of 0 gives the form's sentence. |
| AC-35 | Passed | Cancelling another student's booking gives "You have no booking for this session." and the row stays. |
| AC-36 | Passed | A student is refused by `list_lab_bookings`, and `get_lab` returns the lab with no bookers field and no booker's name or email. |
| AC-40 | Passed | `my_bookings` returns only the caller's bookings; staff are refused. |
| AC-91 | Passed | Staff who do not own the lab: the same as AC-36. Their own lab comes with its booking list. Having `get_lab` ask for the lists as the owner made AC-36 and AC-91 fail. |
| AC-92 | Passed | `schedule_session` with a weekly repeat adds exactly the rows the web form adds for the same input. A bad time, a time in the past, an overlap and an unknown lab give the web page's sentences and add nothing. |
| AC-93 | Passed | `cancel_session` sets `cancelled_at` and keeps the bookings; the student's `/bookings` shows "Cancelled by staff". |
| AC-94 | Passed | A student and staff who do not own the lab are refused by `schedule_session` and `cancel_session`; nothing changes. |
| AC-14 | Passed, extended | The owner gets who booked from `get_lab` and `list_lab_bookings`. |

Decisions taken while building it:
- `/mcp` is part of the app, not a separate program, so the deployed app has the tools and tests reach them through the same router.
- The token is an ordinary sign-in session token from `cmd/mcptoken`, so no schema change. Signing out on the web does not end it; it expires after 7 days.
- "Booking confirmed." and "Booking cancelled." moved into the service (`MsgBooked`, `MsgBookingCancelled`), so the pages, the booking script and the tools cannot drift apart.
- `.vscode/mcp.json` now connects VS Code to `/mcp` instead of the Exercise 1 lecture server, and `.mcp.json` does the same for Claude Code. Both read `${MCP_TOKEN}` and hold no secret. The `mcp-server/` folder (Exercise 1) stays as coursework.

What went wrong:
- `cmd/mcptoken` seemed not to accept a password: in a console the typing is hidden and the prompt did not say so, and in Git Bash the program saw no console and waited without any prompt. Both prompts now say what to do.
- `claude mcp add` failed with "command not found": Claude Code runs as the VS Code extension here, with no `claude` command on the path. A project `.mcp.json` replaced it; the extension only sees `MCP_TOKEN` after VS Code is restarted.
- Three DB tests first used `seeded()`, which only finds today's sessions, for tomorrow's session; they now use `firstSessionOfView()`.
- Shell heredocs broke on quoting twice while writing test files and this section; they were written with the editor instead.

What changed in this spec because of it:
- The Auth section says how an MCP caller signs in.
- The Decisions section has an MCP part.
- The Architecture section traces a tool call.
