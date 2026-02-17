# StitchMap - Crochet Pattern Tracker

## Overview

StitchMap is a web application that allows users to create, manage, and track progress through crochet patterns. Users define patterns using standard or custom stitches, organize them into rows or rounds, and track their progress stitch-by-stitch during a work session.

## Tech Stack

- **Language:** Go (1.22+)
- **CSS Framework:** Bulma
- **Reactivity:** Datastar (server-sent events for reactive UI updates)
- **Templating:** Templ (type-safe Go HTML templating via `.templ` files)
- **Database:** SQLite via `modernc.org/sqlite` (pure Go, no CGO)
- **Router:** `net/http` (Go 1.22 enhanced routing with method+path patterns)
- **Migrations:** Embedded SQL files via `embed` package, applied at startup
- **Password Hashing:** `golang.org/x/crypto/bcrypt`
- **Session Store:** SQLite-backed; session ID stored in a secure, HttpOnly cookie

## Crochet Concepts

### Rows vs Rounds

Patterns are worked in one of two modes:

- **Rows** — Flat, back-and-forth work (scarves, blankets). Each row ends with a turn, and the next row begins with a **turning chain** to reach the height of the stitch being used.
- **Rounds** — Circular work (hats, amigurumi). Rounds can be:
  - **Joined** — Each round ends with a slip stitch to join, then begins with a chain.
  - **Continuous (spiral)** — No joining; the next round begins immediately in the first stitch of the previous round. A stitch marker tracks the start of each round.

### Turning Chains

When working in rows (or joined rounds), a turning chain is made at the beginning. The number of chains depends on the stitch height:

| Stitch              | Turning Chain | Counts as Stitch? |
|---------------------|---------------|--------------------|
| Single Crochet (sc) | ch 1          | Usually no         |
| Half Double (hdc)   | ch 2          | Varies             |
| Double Crochet (dc) | ch 3          | Usually yes        |
| Treble Crochet (tr) | ch 4          | Usually yes        |

Whether the turning chain counts as a stitch affects the total stitch count and where the first "real" stitch is placed. Patterns must specify this.

### Stitch Count

Every row/round has an **expected stitch count** — the total number of stitches after completing it. This is the primary way crafters verify they haven't accidentally skipped or added stitches. The app should display this prominently.

### Stitch Groups and Repeats

Patterns use grouping notation:

- **Parentheses for groups into one stitch/space:** `(sc, ch 2, sc) in next st` — multiple stitches worked into a single stitch or space.
- **Brackets or parentheses for repeats:** `[sc 3, inc] x 6` — repeat the bracketed instructions 6 times.
- **Nested repeats** are possible: `[(sc 2, inc) x 3, sc 3] x 2`

### Special Techniques

- **Magic Ring (MR)** — Starting technique for working in the round that creates a tight, closed center. Most amigurumi and circular patterns begin this way.
- **Front Loop Only (FLO)** / **Back Loop Only (BLO)** — Working into only one loop of the stitch instead of both, creating texture or ridges.
- **Front Post (FP)** / **Back Post (BP)** — Working around the post of a stitch from the previous row/round for textured effects like ribbing.
- **Fasten Off (FO)** — Cutting the yarn and securing the last stitch. May include "leave a long tail" for sewing pieces together.
- **Color Changes** — Switching yarn color mid-row/round.

### Pattern Sections

Complex patterns (especially amigurumi) are composed of multiple **sections** or **parts** — e.g., Head, Body, Arms, Legs — each with their own sequence of rows/rounds. Sections may include assembly instructions for joining pieces.

## Data Model

### User
- `id` (integer, primary key, autoincrement)
- `email` (text, unique, not null)
- `password_hash` (text, not null)
- `created_at` (text, ISO 8601, not null, default current_timestamp)

### Session (auth)
- `id` (text, primary key) — random token (32 bytes, hex-encoded)
- `user_id` (integer, foreign key → User, not null)
- `created_at` (text, not null, default current_timestamp)
- `expires_at` (text, not null) — 30-day expiry, rolling on activity

### Stitch
- `id` (integer, primary key, autoincrement)
- `user_id` (integer, nullable, foreign key → User) — null for built-in stitches
- `name` (text, not null) — e.g. "Single Crochet"
- `abbreviation` (text, not null) — e.g. "sc"
- `description` (text, not null, default "")
- `is_builtin` (boolean, not null, default false)

Unique constraint on `(user_id, abbreviation)` — a user cannot have two custom stitches with the same abbreviation. Built-in stitches (user_id IS NULL) must also have unique abbreviations.

### Pattern
- `id` (integer, primary key, autoincrement)
- `user_id` (integer, foreign key → User, not null)
- `name` (text, not null)
- `description` (text, not null, default "")
- `created_at` (text, not null, default current_timestamp)
- `updated_at` (text, not null, default current_timestamp)

### PatternSection
- `id` (integer, primary key, autoincrement)
- `pattern_id` (integer, foreign key → Pattern, not null, on delete cascade)
- `position` (integer, not null) — ordering within the pattern
- `name` (text, not null) — e.g. "Head", "Body", "Main Panel"
- `notes` (text, not null, default "") — assembly or finishing instructions for this section

Unique constraint on `(pattern_id, position)`.

### Row
Represents a single row or round within a section.
- `id` (integer, primary key, autoincrement)
- `section_id` (integer, foreign key → PatternSection, not null, on delete cascade)
- `position` (integer, not null) — ordering within the section
- `label` (text, not null, default "") — e.g. "Rnd 1", "Row 5"; auto-generated if left empty
- `type` (text, not null) — `row`, `joined_round`, or `continuous_round`
- `expected_stitch_count` (integer, not null) — total stitches at end of this row/round
- `turning_chain_count` (integer, not null, default 0) — number of chains at start
- `turning_chain_counts_as_stitch` (boolean, not null, default false)
- `repeat_count` (integer, not null, default 1) — for repeated identical rows/rounds (e.g. "Rnds 5-10: sc in each st around")
- `notes` (text, not null, default "") — e.g. "do not join", "change to Color B"

Unique constraint on `(section_id, position)`.

### RowInstruction
A single instruction step within a row/round. Instructions are executed in order and may themselves repeat.
- `id` (integer, primary key, autoincrement)
- `row_id` (integer, foreign key → Row, not null, on delete cascade)
- `position` (integer, not null) — ordering within the row
- `stitch_id` (integer, foreign key → Stitch, nullable) — null for non-stitch instructions like "turn" or "fasten off"
- `count` (integer, not null, default 1) — how many times to work this stitch consecutively (e.g. "sc 6" = 6)
- `into` (text, not null, default "") — placement modifier: "next st", "same st", "ch-sp", "FLO", "BLO"
- `is_group` (boolean, not null, default false) — if true, this is a group header; child instructions follow
- `parent_id` (integer, nullable, foreign key → RowInstruction, on delete cascade) — for stitches inside a group
- `group_repeat` (integer, not null, default 1) — how many times to repeat this group
- `note` (text, not null, default "") — e.g. "in corner space", "skip next 2 sts"

Unique constraint on `(row_id, parent_id, position)`. `parent_id` is null for top-level instructions.

### WorkSession
- `id` (integer, primary key, autoincrement)
- `user_id` (integer, foreign key → User, not null)
- `pattern_id` (integer, foreign key → Pattern, not null)
- `started_at` (text, not null, default current_timestamp)
- `last_active_at` (text, not null, default current_timestamp)
- `completed_at` (text, nullable)

Only one active (non-completed) session per user per pattern. Unique partial index on `(user_id, pattern_id) WHERE completed_at IS NULL`.

### WorkProgress
Tracks the current position in the pattern. A single row — updated in place on each advance/undo rather than appended.
- `id` (integer, primary key, autoincrement)
- `session_id` (integer, foreign key → WorkSession, unique, not null, on delete cascade)
- `section_id` (integer, foreign key → PatternSection, not null)
- `row_id` (integer, foreign key → Row, not null)
- `row_repeat_index` (integer, not null, default 0) — which repetition of a repeated row (0-based)
- `instruction_id` (integer, foreign key → RowInstruction, not null)
- `stitch_index` (integer, not null, default 0) — which repetition within the instruction's count (0-based)
- `group_repeat_index` (integer, not null, default 0) — which repetition of a group
- `stitches_completed_in_row` (integer, not null, default 0) — running count for current row/round
- `updated_at` (text, not null, default current_timestamp)

## Project Structure

```
cmd/
  server/
    main.go              — entry point: parse flags, open DB, run migrations, start server
internal/
  database/
    db.go                — open SQLite, configure WAL/journal, pragma setup
    migrate.go           — read and apply embedded SQL migrations in order
    migrations/
      001_users.sql
      002_stitches.sql
      003_patterns.sql
      004_work_sessions.sql
  model/
    user.go              — User struct and queries
    stitch.go            — Stitch struct and queries
    pattern.go           — Pattern, PatternSection, Row, RowInstruction structs and queries
    session.go           — WorkSession, WorkProgress structs and queries
    auth.go              — Session (auth) struct and queries
  handler/
    middleware.go        — auth middleware, request logging
    auth.go              — login, register, logout handlers
    dashboard.go         — dashboard handler
    stitch.go            — stitch CRUD handlers
    pattern.go           — pattern/section/row/instruction CRUD handlers
    work.go              — work session and advance/undo handlers
  view/
    layout.templ         — base HTML layout (head, nav, bulma CSS, datastar JS)
    auth.templ           — login and register forms
    dashboard.templ      — dashboard page
    stitch.templ         — stitch management page
    pattern.templ        — pattern builder views
    work.templ           — work mode view
    components.templ     — shared UI components (buttons, modals, flash messages)
static/
  css/
    app.css              — custom styles beyond Bulma
  js/                    — minimal; Datastar handles most interactivity
migrations/              — symlink or copy; embedded via go:embed
go.mod
go.sum
```

## Pages & Routes

### Public
| Method | Path        | Handler           | Description                |
|--------|-------------|-------------------|----------------------------|
| GET    | /login      | `auth.LoginPage`  | Login form                 |
| POST   | /login      | `auth.Login`      | Authenticate, set cookie   |
| GET    | /register   | `auth.RegisterPage` | Registration form        |
| POST   | /register   | `auth.Register`   | Create account, set cookie |
| POST   | /logout     | `auth.Logout`     | Clear session and cookie   |

### Authenticated (behind auth middleware)
| Method | Path                             | Handler                    | Description                        |
|--------|----------------------------------|----------------------------|------------------------------------|
| GET    | /                                | `dashboard.Index`          | Pattern list + active sessions     |
| GET    | /stitches                        | `stitch.Index`             | List and manage custom stitches    |
| POST   | /stitches                        | `stitch.Create`            | Create custom stitch               |
| PUT    | /stitches/{id}                   | `stitch.Update`            | Update custom stitch               |
| DELETE | /stitches/{id}                   | `stitch.Delete`            | Delete custom stitch               |
| GET    | /patterns/new                    | `pattern.New`              | Empty pattern builder form         |
| POST   | /patterns                        | `pattern.Create`           | Create pattern with initial data   |
| GET    | /patterns/{id}                   | `pattern.Show`             | View/edit pattern                  |
| PUT    | /patterns/{id}                   | `pattern.Update`           | Update pattern metadata            |
| DELETE | /patterns/{id}                   | `pattern.Delete`           | Delete pattern and all children    |
| POST   | /patterns/{id}/sections          | `pattern.CreateSection`    | Add section                        |
| PUT    | /sections/{id}                   | `pattern.UpdateSection`    | Update section                     |
| DELETE | /sections/{id}                   | `pattern.DeleteSection`    | Delete section                     |
| POST   | /sections/{id}/rows              | `pattern.CreateRow`        | Add row/round to section           |
| PUT    | /rows/{id}                       | `pattern.UpdateRow`        | Update row/round                   |
| DELETE | /rows/{id}                       | `pattern.DeleteRow`        | Delete row/round                   |
| POST   | /rows/{id}/instructions          | `pattern.CreateInstruction`| Add instruction to row             |
| PUT    | /instructions/{id}               | `pattern.UpdateInstruction`| Update instruction                 |
| DELETE | /instructions/{id}               | `pattern.DeleteInstruction`| Remove instruction                 |
| GET    | /patterns/{id}/work              | `work.Start`               | Start or resume work session       |
| POST   | /sessions/{id}/advance           | `work.Advance`             | Mark next stitch complete          |
| POST   | /sessions/{id}/undo              | `work.Undo`                | Undo last stitch                   |

All authenticated routes return 302 → `/login` if no valid session. All mutation routes (POST/PUT/DELETE) that modify pattern data verify the authenticated user owns the resource; return 403 otherwise.

## Feature Details

### Built-in Stitches

The app ships with a seed set of common US-terminology stitches, inserted during the stitches migration if not already present:

| Name                      | Abbreviation | Description                                      |
|---------------------------|--------------|--------------------------------------------------|
| Chain                     | ch           | Foundation stitch; yarn over, pull through loop   |
| Slip Stitch               | sl st        | Join or move yarn without adding height           |
| Single Crochet            | sc           | Short stitch; insert, yarn over, pull through twice |
| Half Double Crochet       | hdc          | Medium height; yarn over before inserting         |
| Double Crochet            | dc           | Tall stitch; yarn over, insert, three pull-throughs |
| Treble Crochet            | tr           | Extra tall; yarn over twice before inserting      |
| Magic Ring                | MR           | Adjustable starting loop for working in the round |
| Increase                  | inc          | Two single crochets worked into the same stitch   |
| Decrease                  | dec          | Single crochet two together (sc2tog)              |
| Front Post Double Crochet | FPdc         | dc worked around the front of previous row's post |
| Back Post Double Crochet  | BPdc         | dc worked around the back of previous row's post  |

Users can create custom stitches with their own names and abbreviations. Custom stitches are scoped to the user — they do not affect other users. Built-in stitches cannot be edited or deleted.

### Pattern Builder

- User creates a pattern with a name and optional description
- A default section ("Main") is created automatically; user can rename or add more
- Within each section, adds **rows/rounds** in order, specifying:
  - Type: row, joined round, or continuous round
  - Expected stitch count at the end
  - Turning chain details (if applicable)
  - Whether the row/round repeats (e.g. "Rnds 5-10: repeat")
- Within each row/round, adds **instructions** in order:
  - A stitch with a count (e.g. "sc 6")
  - A placement modifier (e.g. "in FLO", "in ch-sp")
  - A group of stitches worked into one stitch (e.g. "(sc, ch 2, sc) in next st")
  - A repeat group (e.g. "[sc 3, inc] x 6")
- Rows/rounds can be reordered via up/down controls
- Stitch selection uses a dropdown populated with all built-in + user's custom stitches
- Pattern displays a **read-only summary** rendering the full written pattern in standard crochet notation, e.g.:
  ```
  Head
    Rnd 1: MR, 6 sc in ring [6]
    Rnd 2: inc in each st around [12]
    Rnd 3: (sc, inc) x 6 [18]
    Rnd 4: (sc 2, inc) x 6 [24]
    Rnds 5-8: sc in each st around [24]
    Rnd 9: (sc 2, dec) x 6 [18]
  ```
- The expected stitch count is auto-calculated when possible (simple flat sequences of known stitches) and manually overridden otherwise

### Work Mode

- User clicks "Start" on a pattern from the dashboard (or "Resume" for an existing session)
- If an active session exists for that pattern, it resumes; otherwise a new session is created
- The work view shows:
  - **Current section name** (e.g. "Head")
  - **Current row/round label** and overall progress (e.g. "Rnd 3 of 12")
  - **Full row/round instruction text** with the current stitch highlighted/bolded
  - **Current stitch detail** (e.g. "sc — 4 of 6")
  - **Running stitch count** for the current row/round vs expected count (e.g. "12 / 24")
  - A **large tap/click target** (primary action) to advance to the next stitch
  - An **undo button** to step back one stitch
  - A **row/round complete indicator** that appears when stitch count is reached
- Flow:
  - Advancing past the last stitch in an instruction moves to the next instruction
  - Advancing past the last instruction in a row/round moves to the next row/round (or next repeat of the current one)
  - Advancing past the last row/round in a section moves to the next section
  - Completing the final section marks the session as complete and shows a completion screen
- **Persistence:** The WorkProgress row is updated on every advance/undo via a POST. Datastar sends the request and merges the HTML fragment response into the page without a full reload.
- **Undo logic:** The server walks backward through the instruction/row/section chain. One undo step = one stitch. Undo at the very first stitch of the session is a no-op.

### Dashboard

- Lists the user's patterns as cards with name, total section count, total row/round count, and last modified date
- Shows any in-progress work sessions with a "Resume" button and a textual progress summary (e.g. "Head — Rnd 3 of 12, 14/24 stitches")
- Quick actions: "New Pattern" button, "Manage Stitches" link
- Empty state messaging for new users with no patterns yet

## Non-Functional Requirements

- Mobile-first layout — work mode must be comfortable on a phone screen held in one hand
- The advance button in work mode should be large (minimum 64x64px tap target) and easy to tap repeatedly
- SQLite database stored as a single file for simple deployment; WAL mode enabled for concurrent reads
- No external service dependencies beyond the Go binary and SQLite file
- US crochet terminology by default (standard per the Craft Yarn Council)
- All timestamps stored as ISO 8601 strings in UTC
- Passwords hashed with bcrypt (cost 12)
- Session cookies: `Secure`, `HttpOnly`, `SameSite=Lax`, 30-day expiry

---

## Implementation Phases

### Phase 1: Foundation

Establish the project skeleton, database layer, and authentication. No crochet-specific features yet — the goal is a running server with working login/register/logout and a protected dashboard shell.

**Deliverables:**
- `go.mod` initialized with dependencies (templ, modernc sqlite, bcrypt, datastar)
- SQLite database setup with WAL mode and foreign keys enabled
- Migration system: embedded SQL files applied in order at startup
- Migration `001_users.sql`: `users` and `sessions` tables
- User model: create, find by email, find by ID
- Auth model: create session, find session, delete session, delete expired sessions
- Password hashing (bcrypt cost 12) and verification
- Auth handlers: GET/POST `/login`, GET/POST `/register`, POST `/logout`
- Auth middleware: extract session cookie, look up session, attach user to request context, redirect to `/login` if missing/expired
- Base Templ layout: HTML shell, Bulma CSS (CDN), Datastar JS (CDN), nav bar with login/logout state
- Login and register Templ pages with Bulma form styling
- Dashboard handler: `GET /` behind auth middleware, renders a placeholder "Welcome, {email}" page
- Server entry point: parse `--addr` and `--db` flags, wire everything together

**Acceptance criteria:**
- `go run ./cmd/server` starts on `:8080`, serves the login page
- A user can register, be redirected to `/`, see their email, log out, and log back in
- Visiting `/` without a session redirects to `/login`
- Database file is created automatically on first run

### Phase 2: Stitch Management

Add the stitch table with built-in seed data, and a CRUD page for managing custom stitches.

**Deliverables:**
- Migration `002_stitches.sql`: `stitches` table with seed data for all 11 built-in stitches
- Stitch model: list built-in, list user custom, create, update, delete (with ownership check)
- Stitch handlers: GET `/stitches`, POST `/stitches`, PUT `/stitches/{id}`, DELETE `/stitches/{id}`
- Stitches Templ page: table of all available stitches (built-in shown as read-only, custom shown with edit/delete), inline form to add new stitch
- Datastar integration for the stitch page: add/edit/delete without full page reloads (SSE fragment responses)
- Validation: abbreviation required, unique per user, name required, cannot modify built-in stitches

**Acceptance criteria:**
- Built-in stitches appear on first load with no user action
- User can add a custom stitch, see it in the list, edit it, and delete it
- Attempting to delete a built-in stitch is rejected
- Two users can each have a custom stitch with the same abbreviation without conflict

### Phase 3: Pattern Builder (Structure)

Add pattern, section, and row/round CRUD. No instruction-level editing yet — just the structural skeleton of a pattern.

**Deliverables:**
- Migration `003_patterns.sql`: `patterns`, `pattern_sections`, `rows` tables
- Pattern model: CRUD with ownership checks
- Section model: CRUD, reorder positions
- Row model: CRUD, reorder positions; includes type, stitch count, turning chain fields, repeat count
- Handlers for all pattern/section/row routes
- Pattern list on dashboard: cards showing each pattern with name, section count, row count, last modified
- `GET /patterns/new`: form to enter pattern name/description, creates pattern with a default "Main" section
- `GET /patterns/{id}`: pattern detail page showing sections and their rows in order
- Section and row management: add, edit, delete, reorder — using Datastar for inline updates
- Ownership enforcement: users can only see and edit their own patterns
- Cascade deletes: deleting a pattern removes sections, rows; deleting a section removes rows

**Acceptance criteria:**
- User can create a pattern, see it on the dashboard
- User can add sections, add rows/rounds to sections with type and stitch count
- Rows display in order; reordering works
- Deleting a section removes its rows
- Another user cannot access the pattern

### Phase 4: Pattern Builder (Instructions)

Add the instruction layer within rows/rounds — stitch selection, counts, groups, repeats, and the read-only pattern summary view.

**Deliverables:**
- Migration `004_row_instructions.sql`: `row_instructions` table
- RowInstruction model: CRUD with parent/child support for groups
- Instruction handlers: POST/PUT/DELETE for instructions within a row
- Instruction editor UI within each row: ordered list of instructions, each showing stitch abbreviation and count
  - Stitch dropdown (built-in + user custom)
  - Count input
  - Placement modifier dropdown (none, FLO, BLO, ch-sp, same st)
  - "Add group" button: creates a group header, then child instructions can be added within it
  - Group repeat count input
- Pattern summary rendering: a function that walks sections → rows → instructions and produces standard crochet notation text (e.g. `Rnd 3: (sc, inc) x 6 [18]`)
- Summary displayed as a read-only block on the pattern detail page
- Auto-label rows if label is empty: "Row N" for rows, "Rnd N" for rounds, "Rnds N-M" for repeated rows

**Acceptance criteria:**
- User can add stitches to a row, see them listed in order
- User can create a group with child stitches and a repeat count
- Pattern summary renders correctly for a sample amigurumi-style pattern
- Instructions cascade-delete with their parent row

### Phase 5: Work Mode

The core tracking experience — starting a session, advancing stitch by stitch, undoing, persisting progress, and resuming.

**Deliverables:**
- Migration `005_work_sessions.sql`: `work_sessions` and `work_progress` tables
- WorkSession model: create, find active by user+pattern, mark completed
- WorkProgress model: initialize to first stitch, advance, undo, read current state
- Advance logic: given current progress position, compute the next position by walking the instruction tree:
  1. Increment `stitch_index`; if < instruction count, done
  2. Else move to next instruction (reset stitch_index to 0)
  3. If instruction was in a group, check `group_repeat_index`; increment if more repeats remain
  4. If all instructions in row exhausted, check `row_repeat_index`; increment if more repeats remain
  5. If row complete, advance to next row in section (reset all sub-indices)
  6. If section complete, advance to next section
  7. If all sections complete, mark session as completed
- Undo logic: reverse of advance; walk backward through the same tree. No-op at the very start.
- `GET /patterns/{id}/work` handler:
  - Find or create active session
  - Initialize progress to first stitch if new session
  - Render work mode page
- `POST /sessions/{id}/advance` handler:
  - Verify ownership
  - Call advance logic
  - Update `work_progress` and `last_active_at`
  - Return Datastar SSE fragment with updated work mode UI
- `POST /sessions/{id}/undo` handler: same pattern, calls undo logic
- Work mode Templ page:
  - Section name, row/round label, overall round progress
  - Full instruction text for current row with current stitch highlighted
  - Current stitch detail (abbreviation, index within count)
  - Running stitch counter vs expected (e.g. "12 / 24")
  - Large advance button (full-width, tall, primary color)
  - Smaller undo button
  - Row/round completion indicator
  - Session completion screen with link back to dashboard
- Dashboard updates: show active sessions with "Resume" and progress summary

**Acceptance criteria:**
- User can start a work session from a pattern
- Tapping advance progresses through every stitch in order across instructions, groups, rows, sections
- Stitch counter matches expected count at row/round completion
- Undo steps backward correctly, including across row/group boundaries
- Closing the browser and returning to `/patterns/{id}/work` resumes at the saved position
- Completing the last stitch of the pattern shows a completion screen
- Dashboard shows in-progress sessions with accurate progress text

### Phase 6: Polish and UX

Refinements after core features are working.

**Deliverables:**
- Mobile layout tuning: work mode optimized for one-handed phone use, advance button in thumb zone
- Haptic feedback on advance (via `navigator.vibrate` if available)
- Keyboard support: spacebar or Enter to advance, Backspace to undo
- Empty states: no patterns yet, no stitches yet, no sessions yet — friendly messaging with CTAs
- Flash messages for success/error on mutations (create, delete, validation failures)
- Confirmation dialogs before destructive actions (delete pattern, delete section, abandon session)
- Pattern duplication: copy an existing pattern as a starting point
- Loading states for Datastar requests (button disabled during flight)
- Basic input validation with server-side error messages rendered inline

**Acceptance criteria:**
- Work mode is comfortable to use on a phone
- Keyboard shortcuts work in work mode on desktop
- All destructive actions have confirmation
- Validation errors display next to the relevant field

---

## References

- [Craft Yarn Council — How to Read a Crochet Pattern](https://www.craftyarncouncil.com/standards/how-to-read-crochet-pattern)
- [How to Read Amigurumi Patterns — All About Ami](https://www.allaboutami.com/howtoreadamigurumipatterns/)
- [Crochet in the Round: Spiral vs Joining — Look At What I Made](https://lookatwhatimade.net/crafts/yarn/crochet/crochet-tutorials/how-to-crochet-in-the-round-spiral-vs-joining/)
- [Does the Turning Chain Count as a Stitch? — Sigoni Macaroni](https://www.sigonimacaroni.com/does-the-turning-chain-count-as-a-stitch/)
- [Written Pattern Conventions — Every Trick on the Hook](https://everytrickonthehook.com/patterns-2/written-pattern-conventions/)
