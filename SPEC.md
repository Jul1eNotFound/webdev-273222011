# Labwiz

<!-- Exercise 2. Write the non-goals first. Every acceptance criterion must be checkable in under a minute.
     Keep these headings exactly as they are: the self-checks look for them by name.
     The full target is on the course site under Resources → The course project. -->

## Project option
<!-- One line: lab booking, study groups, club events, or the own project the lecturer approved. -->
lab booking

## Goals

- A student signs in, sees free labs and books one that has free places.
- A staff adds labs with a limit on places and sees who has booked it.

## Non-goals

- No multilingual support: only English is supported
- No dark/light theme options: only light generic theme
- No mobile app: one web UI that also works on phone screen
- No animations such as transitions, hover effects or loading indicators
- No booking notifications or reminders for both users and lab owners
- No auto-refresh of pages

## Constraints

- Golang with Chi, PostgreSQL, plain HTML and JavaScript in the UI.
- Passwords are stored only as a hash, never as readable text.

## The model feature
<!-- Which model, which modality, and what it does for a user.
     Name three things or the check fails: the input it receives, the output it produces,
     and what happens when the model is slow, rate-limited or wrong. -->
- Model:
- Input:
- Output:
- When it fails:

## Auth
<!-- Local sign-in plus at least one provider. Say how an account signing in both ways stays one account. -->
- Local:
- Provider:
- Linking:

## MCP tools
<!-- At least three tools your own MCP server exposes over this project's data.
     Each tool calls the same service functions the HTTP API calls, so the same rules apply.
     A description shorter than 20 characters is not a description. -->
| Tool | What it does | Who may call it |
| --- | --- | --- |
|  |  |  |

## UI
<!-- Stack, and the three or more screens. The graded checklist lives in docs/UI.md. -->

## Architecture
<!-- Exercise 3. One request traced through every layer, naming the file for each one,
     plus one place where your own code does not match this layering. -->

## Acceptance criteria
<!-- Every criterion carries an id. A test proves it by naming that id in its title, for example
     test('AC-3 a booking that overlaps another is refused with 409', ...).
     Say who acts, what they see, and what must be true in the database. -->
- [AC-1] A student who books a lab with no free places sees "This lab is full", and promted to go back to lab list or home page.
- [AC-2] A staff who adds a lab with existing title is promted to choose a different title
- [AC-3] A student signs in and they see the home page with available labs for today.

### Slice 1: sign-in and lab list

- [AC-9] A visitor who opens `/labs` without signing in lands on `/login` instead.
- [AC-10] After signing in with a seeded account, `/labs` shows the three of seeded available labs for current day, each with its
  title, description, availability time period and number of free places.
- [AC-11] Deleting one lab row in the database and reloading `/labs` removes that lab from the screen.

## Traceability
<!-- Filled in from Exercise 3 onward: each AC id, and the test that names it. -->
| Criterion | Test |
| --- | --- |
| AC-1 |  |

## Spec vs result
<!-- After the agent builds the slice: which criteria passed, which it missed or reinterpreted, and what you changed in this spec because of it. -->
