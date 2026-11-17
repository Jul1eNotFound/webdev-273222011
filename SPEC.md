# Labwiz

## Project option
lab booking

## Goals
- A student signs in, sees free labs and books one that has free places.
- A staff adds labs with a limit on places and sees who has booked it.

## Non-goals
- No multilingual support: only English is supported
- No dark/light theme options: only light generic theme
- No mobile app: one web UI that also works on phone screen
- No animations such as transitions, hover effects or animated loading indicators (a static "Loading…" text is allowed)
- No booking notifications or reminders for both users and lab owners
- No auto-refresh of pages

## Constraints
- Golang with Chi, PostgreSQL, plain HTML and JavaScript in the UI.
- Passwords are stored only as a hash, never as readable text.

## The model feature
- Model: Claude Haiku 5.5 (`claude-haiku-5-5`), text in and text out, called from the Go backend through the Anthropic Messages API. The API key is read from an environment variable and never reaches the browser. Feature: natural-language lab search, where a student types something like "a quiet lab tomorrow afternoon with 5 free places" and the model turns it into filters for the existing `/labs` list.
- Input: the student's free-text query (max 200 characters) and today's date and timezone. No user data, no emails, and no booking data are sent.
- Output: a JSON object with optional fields `date_from`, `date_to`, `time_of_day` (morning, afternoon or evening), `min_free_places` and `keywords`. It is requested through a tool schema so the shape is enforced. The server validates it, runs the normal lab-list query with those filters, and shows them as editable chips above the results.
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

## MCP tools
| Tool | What it does | Who may call it |
| --- | --- | --- |
| list_labs | Lists labs available on a given date (default today), each with title, description, availability time period, place limit and free places. | Any signed-in user |
| get_lab | Returns one lab by id with its details and free places. For staff it also lists the students who booked it. | Any signed-in user; the booking list is for staff only |
| book_lab | Books a place in a lab for the calling student. Refused if the lab is full or the student already booked it. | Student |
| cancel_booking | Cancels the calling student's booking for a lab and frees the place. | Student, own bookings only |
| my_bookings | Lists the calling student's current bookings with lab title and time period. | Student |
| add_lab | Creates a lab with title, description, time period and a place limit of 1 or more. Refused if the title already exists. | Staff |
| list_lab_bookings | Lists the students who booked a given lab. | Staff |

## UI
- Stack: plain HTML, CSS and JavaScript served by the Go backend. There is no framework and no build step. One stylesheet, with spacing, type scale and colours defined as CSS custom properties. Pages are server-rendered, and small scripts handle the booking button and the smart search.
- Screens:
  1. **Login** (`/login`): email and password, a "Sign in with <provider>" button, and an error line. Visitors who are not signed in land here from any protected page.
  2. **Lab list** (`/labs`): today's available labs as cards, each with title, description, time period, and free places. It has a smart-search box with editable filter chips and a "Book" button on each card, which becomes "Full" and disabled when no places are left. The empty state reads "No labs available today", or "No labs match these filters" when search filters are applied.
  3. **Lab detail and my bookings** (`/labs/{id}`, `/bookings`): a lab's details, with "Book" or "Cancel booking". The bookings page lists the student's own bookings. When a student books a full lab, they see "This lab is full" with links back to the lab list and the home page (AC-1).
  4. **Staff: add lab and bookings** (`/staff/labs/new`, `/staff/labs/{id}`): a form with title, description, time period and place limit, and a table of students who booked each lab. These pages return 403 for students.
- Layout: a single column that works at 380 px and widens to a centred max width on desktop. The header has the app name, the signed-in user and a Sign out button. Light theme only.
- States, on every screen: loading (static "Loading…" text), empty, error (one readable sentence) and success (a message at the top of the page, no reload needed).

## Architecture
<!-- One request traced through every layer, naming the file for each one,
     plus one place where your own code does not match this layering. -->
Layers: handler → service → store → database. HTTP and MCP both go through the same service.

A staff member adds a lab (`POST /staff/labs/new`):
1. **Entry**: `cmd/server/main.go` loads config, opens the pool and wires store, services and router together.
2. **Routing and session**: `internal/handler/router.go` matches the route inside the `RequirePage` group. `internal/auth/session.go` reads the cookie, gets the caller from `service.Auth.CurrentUser` (`internal/service/auth.go`), and puts it in the request context. With no valid session, the visitor is redirected to `/login`.
3. **Handler**: `addLab` in `internal/handler/staff.go` caps the body size, parses the form into `service.NewLab` and calls `Labs.Add`.
4. **Service**: `Labs.Add` in `internal/service/labs.go` checks the role (`RequireStaff`, AC-15), validates the title, time period and place limit (AC-13), and maps `store.ErrDuplicate` to `ErrDuplicateTitle` (AC-2). The typed errors are in `internal/service/errors.go`.
5. **Store**: `CreateLab` in `internal/store/labs.go` runs the only SQL, an `INSERT … RETURNING id`, and turns SQLSTATE `23505` into `ErrDuplicate`.
6. **Database**: the `labs` table and its unique title come from `migrations/000001_init.up.sql`. The unique constraint, not a prior lookup, decides a duplicate title, so two staff adding the same title at once cannot both succeed.
7. **Back out**: on success the handler redirects to `/staff/labs/{id}?created=1`. A validation or conflict error re-renders `web/templates/lab_new.html` with the message and the typed values (400/409). Any other error goes through `fail` → `statusFor` in `internal/handler/render.go`, which shows one readable sentence (AC-43).

**Where the code does not match the layering:** `newLabForm` and `addLab` in `internal/handler/staff.go` call `service.RequireStaff` themselves before calling the service. AGENTS.md says handlers never check roles. The rule itself still lives in the service, and `Labs.Add` checks it again, so the result is right. But the handler now decides when authorization runs: it does so to answer 403 before reading the form. `newLabForm` has no service call, so the role check is its only guard. An MCP `add_lab` tool will not get this early check, so the two paths differ in when they refuse, though not in what they refuse. The fix is a service method such as `Labs.NewLabForm(caller)` that the GET handler calls, and dropping the handler's own check from `addLab`.

## Acceptance criteria
Each criterion is one behaviour, written as Given / When / Then, and tagged with how it is checked: `[http]` request and response, `[db]` database state, `[ui]` browser, `[mcp]` MCP tool call. IDs are stable once a test names them: new criteria get new numbers, and removed ones are listed at the end, not reused.

Decisions behind the wording:
- `/` redirects to `/labs`, so "home page" means `/labs`.
- Accounts are seeded; there is no sign-up screen. Staff and students are told apart by a role on the `users` row.
- `/labs` defaults to labs whose time period includes today. A smart-search date filter may widen this to other dates.
- Lab editing is out of scope: staff can only add labs.
- Tests that depend on "today" or "tomorrow" run with a fixed clock. The fixed clock drives server logic only. Anything the client checks against its own clock, such as cookie expiry, is relative (`Max-Age`, not `Expires`), so tests keep passing as the real date moves on.
- "Today" is the calendar day in the server process's local timezone. A deployment sets that timezone explicitly.
- A time period is shown as `HH:MM–HH:MM` when it starts and ends on the same day, and as `D Mon HH:MM – D Mon HH:MM` otherwise, both in that timezone.
- Free places are `place_limit` minus the lab's rows in `bookings`. This holds from Slice 1 on, even though bookings can only be made from Slice 3.
- `[ui]` criteria are checked over HTTP, by asserting on the rendered HTML and headers, unless the behaviour exists only in a browser (layout at a viewport width, keyboard use, script-driven updates). Those are checked by hand in a browser until a browser test tool is added.
- Error pages (400, 401, 403, 404, 405, 409, 500) count as one page and share one title.

### Definition of done (every slice)
Each criterion here covers only the pages that exist in the slice being checked: `/staff/labs/new` is checked from Slice 2 on.

- [AC-29] [ui] Given a 375px viewport, when `/labs`, `/login` and `/staff/labs/new` are opened, then there is no horizontal scroll and every button and input is visible and can be activated.
- [AC-30] [http] Given the served CSS, JS and HTML, when grepped, then they contain no language switcher, no theme toggle, and no `transition`, `animation` or `@keyframes` rule.
- [AC-41] [ui] Given `/login`, `/labs` and `/staff/labs/new`, when inspected, then every input has an associated label, and sign-in, booking and add-lab can be completed with the keyboard alone.
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
- [AC-22] [ui] Given a seeded lab whose time period starts tomorrow, when `/labs` is opened without a search, then that lab is not shown.
- [AC-23] [ui] Given no lab is available today, when `/labs` is opened without a search, then it shows "No labs available today".

### Slice 2: staff adds labs
- [AC-12] [ui][db] Given a signed-in staff member, when they submit a title, description, time period and a place limit of 1 or more, then `labs` gains one row and the lab appears on `/labs`.
- [AC-13] [http][db] Given a staff member, when they submit a place limit of 0, a negative number or non-numeric text, then a validation message is shown and no row is created.
- [AC-2] [http][db] Given a lab titled "X" exists, when staff add another lab titled "X", then the response is 409 with "Please choose a different title" and no row is created.
- [AC-14] [ui][db] Given a lab with bookings, when a staff member opens `/staff/labs/{id}`, then the table lists the names of exactly the students in the `bookings` rows for that lab.
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
- [AC-48] [http][db] Given a lab whose time period does not include today, when a student POSTs a booking for it, then the response is 409 with "This lab is not open for booking today." and no row is created.

### Slice 4: cancelling
- [AC-7] [ui][db] Given a student with a booking, when they cancel it, then the free-place count goes up by 1 and the `bookings` row is gone.
- [AC-49] [http][db] Given a student with no booking for a lab, when they POST a cancel for it, then the response is 404 and `bookings` is unchanged.
- [AC-50] [db] Given a full lab, when one student cancels and another then books, then the second booking succeeds and the bookings count equals the limit.

### Slice 5: MCP tools
- [AC-24] [mcp] Given the server, when `tools/list` is called, then all seven tools in the MCP table appear, each with a description of at least 20 characters.
- [AC-39] [mcp][db] Given a caller with no valid token, when any of the seven tools is called, then an authentication error is returned and the database is unchanged.
- [AC-37] [mcp][db] Given a student and a lab with a free place, when `book_lab` succeeds, then its result matches the HTTP booking, the free-place count drops by 1, and `bookings` has one new row for the caller.
- [AC-25] [mcp][db] Given a student and a full lab, when `book_lab` is called, then it returns the same "full" error as the HTTP API and no row is created.
- [AC-51] [mcp][db] Given a staff caller, when `book_lab` is called, then it is refused and no row is created.
- [AC-38] [mcp][db] Given a lab titled "X" exists, when `add_lab` is called with title "X", then it is refused with the same message as the HTTP API and no row is created.
- [AC-26] [mcp][db] Given a student caller, when `add_lab` is called, then it is refused and no row is created.
- [AC-35] [mcp][db] Given another student's booking, when a student calls `cancel_booking` for it, then it is refused and that row is unchanged.
- [AC-36] [mcp] Given a student caller, when `list_lab_bookings` is called it is refused, and when `get_lab` is called it returns the lab details without the booking list.
- [AC-40] [mcp] Given bookings by several students, when a student calls `my_bookings`, then only that student's bookings are returned.

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

### Retired
- AC-16 (lowering a lab's limit): there is no lab-editing feature.
- AC-44 (booking confirmation without reload): merged into AC-4.

## Traceability
<!-- Filled in from Exercise 3 onward: each AC id, and the test that names it. -->
| Criterion | Test |
| --- | --- |
| AC-10 | `TestListTodayFreePlacesAreLimitMinusBookings_AC10`, `TestAC10_LabCardsShowTitleDescriptionPeriodAndFreePlaces`, `TestAC10_DB_LabsShowFreePlacesFromBookingsRows` |
| AC-23 | `TestAC23_NoLabTodayShowsEmptyState`, `TestAC23_DB_NoLabTodayShowsEmptyState` |
| AC-12 | `TestAddLabCreatesOneLab_AC12`, `TestAddLabAcceptsAPlaceLimitOfOne_AC12`, `TestAC12_StaffAddsLabAndItAppearsOnLabs`, `TestAC12_DB_StaffAddsLabAndLabsGainsOneRow` |
| AC-13 | `TestAddLabRejectsBadPlaceLimit_AC13`, `TestAC13_BadPlaceLimitShowsMessageAndCreatesNothing`, `TestAC13_DB_BadPlaceLimitCreatesNoRow` |
| AC-2 | `TestAddLabDuplicateTitleIsConflict_AC2`, `TestAC2_DuplicateTitleIs409`, `TestAC2_DB_DuplicateTitleIs409AndNoRow` |
| AC-14 | `TestLabBookingsListsExactlyTheBookers_AC14`, `TestAC14_StaffSeesBookersTable`, `TestAC14_DB_StaffSeesExactlyTheStudentsWhoBooked` |
| AC-15 | `TestAddLabStudentIsForbidden_AC15`, `TestRequireStaff_AC15`, `TestAC15_StudentGetAndPostNewLabAre403`, `TestAC15_DB_StudentGetsAndPosts403AndNoRow` |
| AC-45 | `TestLabBookingsStudentIsForbidden_AC45`, `TestAC45_StudentGetsStaffLabPage403`, `TestAC45_DB_StudentGetsStaffLabPage403` |
| AC-4 | `TestBookTakesOnePlace_AC4`, `TestAC4_ScriptBookingConfirmsAndReturnsTheNewCount`, `TestAC4_FormBookingRedirectsToTheLabWithConfirmation`, `TestAC4_BookingScriptIsServedAndLoaded`, `TestAC4_DB_BookingAddsExactlyOneRowAndLowersTheCount` |
| AC-5 | `TestBookTwiceIsAlreadyBooked_AC5`, `TestAC5_SecondBookingIs409`, `TestAC5_DB_SecondBookingIs409AndLeavesOneRow`, `TestBookLabReportsAMissingLabAndADuplicate_AC47_AC5` |
| AC-1 | `TestBookFullLabIsRefused_AC1`, `TestAC1_FullLabIs409WithLinksToTheListAndHome`, `TestAC1_DB_FullLabIs409AndCreatesNoRow` |
| AC-8 | `TestAC8_FullLabShowsADisabledFullButtonAndRefusesAPost`, `TestAC8_DB_FullLabShowsFullAndRefusesAPost` |
| AC-6 | `TestBookTheLastPlaceThenFull_AC6`, `TestAC6_DB_BookLabHoldsTheLabLockWhileTheCheckRuns`, `TestAC6_DB_ConcurrentBookingsOfTheLastPlaceLetExactlyOneThrough` |
| AC-46 | `TestRequireStudent_AC46`, `TestBookByStaffIsForbidden_AC46`, `TestAC46_StaffBookingIs403`, `TestAC46_DB_StaffBookingIs403AndCreatesNoRow` |
| AC-47 | `TestBookUnknownLabIsNotFound_AC47`, `TestGetLabShowsFreePlaces_AC47`, `TestAC47_UnknownLabIs404`, `TestAC47_DB_UnknownLabIs404AndCreatesNoRow`, `TestBookLabReportsAMissingLabAndADuplicate_AC47_AC5` |
| AC-48 | `TestBookLabNotAvailableTodayIsRefused_AC48`, `TestAC48_LabNotAvailableTodayIsRefused`, `TestAC48_DB_LabNotAvailableTodayIsRefusedAndCreatesNoRow` |
| AC-19 | `TestBookWithoutCallerIsUnauthenticated_AC19`, `TestAC19_BookingWithoutSessionIs401`, `TestAC19_ScriptBookingWithoutSessionIs401`, `TestAC19_DB_BookingWithoutSessionIs401AndBookingsUnchanged` |

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
- No CSRF token: the session cookie is `SameSite=Lax`, so a cross-site POST carries no session.
- `/bookings` and the Cancel button move to Slice 4, with cancelling. After a booking, the script turns the button into a disabled "Booked". A reload shows "Book" again until Slice 4 knows the caller's own bookings; pressing it then gives "You have already booked this lab".

What changed in this spec because of it:
- AC-48 gives the status and the exact sentence.
