---
description: "Task list for 001-serialize-e2e-runs"
---

# Tasks: Serialize Local E2E Runs Behind a Single Lock

**Input**: Design documents from `/specs/001-serialize-e2e-runs/`

**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), [research.md](research.md),
[data-model.md](data-model.md), [contracts/](contracts/), [quickstart.md](quickstart.md)

**Tests**: **Included, and not optional on this project.** The constitution's
*Development Workflow & Quality Gates* requires ≥80% statement coverage on lock, queue
and release logic **and** names one specific mandatory test (kill a Lock holder without
letting it release). `CLAUDE.md` → Testing Philosophy adds that no logic merges
untested. So test tasks are first-class here, and within each story the **acceptance
test comes before the implementation and is watched failing** — that is the ordering
`CLAUDE.md` requires, because only the acceptance test can be written from the spec
alone.

**Organization**: Grouped by user story so each is independently implementable,
testable and shippable.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: US1 / US2 / US3, from [spec.md](spec.md)
- Exact file paths are in every task

## Path Conventions

Single Go module at the repository root — no `src/`, no `internal/`, no `cmd/`
(`plan.md` → Project Structure). Tests are `_test.go` files beside the code they name;
the acceptance suite is `e2e_test.go` at the root.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Make the repository a buildable Go module with a dispatch skeleton, so
every later task has somewhere to land and the gates can run.

- [X] T001 Create `go.mod` at repository root: module `github.com/exustash/trainsty`, `go 1.20`, and **no `require` block** — the floor is set by `http.NewResponseController` (research R1) and zero dependencies is Principle I
- [X] T002 Create `main.go` with subcommand dispatch only — `wrap`, `start`, `stop`, `status`, `ui`, `help`, plus hidden `serve` — each returning a "not implemented" stub and exit code 1; no logic in this file, ever
- [X] T003 [P] Verify the four gates run clean on the skeleton: `gofmt -l .` silent, `go vet ./...`, `go build ./...`, `go test -race ./...`. Record the commands in the commit body — they are the merge gate (`RULES.md` §3.3)
- [X] T004 [P] Add `doc.go` at repository root with a package comment naming what trainsty is and pointing at `knowledge/index.md`

**Checkpoint**: `go build` produces a binary that prints help-ish stubs for six commands.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The three packages every user story needs. **Nothing in Phase 3+ can
start until these are done**, because `scheduler/` holds the Lock every story touches
and `process/` owns every syscall two of them need.

⚠️ **`scheduler/` must not import `net/http`, `syscall` or `os/exec`.** That constraint
is what makes T007's tests fast and the coverage gate reachable.

- [X] T005 Implement `process/group.go`: `IsGroupLeader(pid int) (bool, error)` via `syscall.Getpgid`, and `GroupAlive(pid int) (bool, error)` via `syscall.Kill(pid, 0)` classifying **`nil` → alive, `ESRCH` → dead, `EPERM` → alive *and* a loud defect, anything else → alive + logged** (research R4). Write the `// SAFETY`-style comment explaining the classification
- [X] T006 Implement `process/kill.go`: `KillGroup(pid int) error` as the **single** `syscall.Kill(-pid, SIGKILL)` call site in the codebase, with the doc comment stating the caller MUST pass a group leader and citing ADR-002
- [X] T007 [P] Write `process/group_test.go`: start real children with `SysProcAttr{Setpgid: true}`, assert alive → killed → `ESRCH`; assert `IsGroupLeader` true for a `Setpgid` child and false for a non-leader; **`t.Cleanup` group-kills every child**; no hardcoded PIDs, no magic sleeps (poll with a deadline)
- [X] T008 [P] Write `process/kill_test.go`: a child that spawns three children of its own (`sh -c 'sleep 30 & sleep 30 & sleep 30 & wait'`), assert **all four** are gone after `KillGroup` — this is the test that catches signalling a bare PID
- [X] T009 Implement `scheduler/scheduler.go` per [data-model.md](data-model.md): `Scheduler{mu, job, queue}`, `Job`, `Waiter`, and `Register`, `Grant`, `Release`. **Release compares Job identity by pointer before clearing** (invariant 3) and is guarded by `sync.Once` (FR-008). Grant removes from the queue and installs the Job **in one critical section** (invariant 1)
- [X] T010 Write `scheduler/scheduler_test.go` covering, as table-driven cases where the data is all that varies: immediate grant on a free Lock; FIFO across three waiters; **a re-registration keeping its queue position** (ADR-007); a re-registration by the current holder getting an immediate grant; **a double Release being a no-op**; and **a late Release from a finished holder NOT releasing its successor** (invariant 3 — the plausible-but-wrong case)
- [X] T011 [P] Implement `logpath/path.go`: `Resolve() (string, error)` returning `~/Library/Logs/trainsty.log` on darwin and `${XDG_STATE_HOME:-~/.local/state}/trainsty/trainsty.log` elsewhere, creating parent directories with `0700`. **This package exists to contain the project's only platform branch** (DDR-002)
- [X] T012 [P] Write `logpath/path_test.go`: asserts the darwin and non-darwin shapes and that `XDG_STATE_HOME` is honoured when set
- [X] T013 Implement `httpapi/server.go`: `http.ServeMux`, bind **`127.0.0.1:45678`** only, `ReadHeaderTimeout: 5s`, `WriteTimeout: 30s` **left set** (R1 removes it per-request), and a bind failure that distinguishes *already running* from *port taken by something else* and prints `lsof -nP -iTCP:45678` (ADR-011). The successful bind is the single-instance mechanism — **no PID file**
- [X] T014 Implement `httpapi/guard.go`: the SDR-001 floor as middleware for mutating routes — `POST` only (`405`), `Content-Type: application/json` required (`415`), foreign `Origin` refused (`403`) — plus the `writeJSONError(w, status, code)` helper that is the only way this package emits an error body
- [X] T015 [P] Write `httpapi/guard_test.go`: `GET` on a mutating route is `405`; a form content type is `415`; `Origin: https://evil.example` is `403`; **an absent `Origin` is allowed** (a shell client sends none); and no response body ever contains a filesystem path or an errno string
- [X] T016 Implement `httpapi/log.go`: a write-only `*log.Logger` over the `logpath` file opened `O_APPEND|O_CREATE|O_WRONLY` mode `0600`, logging grants, releases with cause, refusals and non-`ESRCH` probe errors. **A log that cannot be opened must not stop the Daemon** — degrade to stderr and continue (DDR-002)

**Checkpoint**: `go test -race ./process/... ./scheduler/... ./logpath/... ./httpapi/...` is green, and `go test -cover ./scheduler/` reports **≥80%**. No HTTP handler exists yet.

---

## Phase 3: User Story 1 — Queue my suite behind whoever is already running (Priority: P1) 🎯 MVP

**Goal**: A developer wraps two suites in two clones; the second waits and then runs.
The Lock is released on all five paths.

**Independent test**: [quickstart.md](quickstart.md) scenarios 1–5 and 8–10 pass with
no dashboard and no Stop control built. That is what makes this story a shippable MVP.

### Acceptance tests for User Story 1 (written FIRST, watched failing)

⚠️ **These are written before the implementation tasks below and must be seen failing
for the right reason.** They name no function or type — only commands, output and exit
codes from [contracts/cli.md](contracts/cli.md) — which is why they can exist first.

- [X] T017 [US1] Create `e2e_test.go` harness: build the binary into `t.TempDir()`, **skip with a message naming `trainsty status` if port 45678 is already bound**, and provide helpers to start the daemon, wrap a command, and read `/status`. The skip is required — the acceptance suite contends for the same machine-wide port the product arbitrates
- [X] T018 [P] [US1] Write the scenario-1 acceptance test in `e2e_test.go`: two wrapped `sleep` suites started a second apart run **strictly in order**, and `/status` during the overlap shows one Job and one Waiter (FR-001, FR-002, FR-003, FR-005)
- [X] T019 [P] [US1] Write **the mandatory acceptance test** in `e2e_test.go`: hold the Lock, `kill -9` the holder **without letting it call `/release`**, assert the next Waiter is granted within 5 s (FR-007, quickstart scenario 3). The constitution names this test specifically; it is the product working when everything else has gone wrong
- [X] T020 [P] [US1] Write the scenario-2 acceptance test in `e2e_test.go`: send `SIGINT` to a wrapped run, assert **the suite's own process dies** and the next Waiter is granted within 2 s (FR-006, FR-036, research R2 — this fails if signal forwarding is missing)
- [X] T021 [P] [US1] Write the scenario-9 acceptance test in `e2e_test.go`: with no daemon, a wrapped suite **still runs** and exits with the suite's status, and `trainsty status` exits **3** rather than 1 (FR-018, FR-038)

### Implementation for User Story 1

- [X] T022 [US1] Implement `httpapi/register.go` per [contracts/http-api.md](contracts/http-api.md): validate `pid` and `repo` **before queueing** (FR-012), refuse a non-leader PID with `not_group_leader` (FR-010), then `http.NewResponseController(w)` → `SetWriteDeadline(time.Time{})` → write SSE headers → `Flush()` → `select` on the grant channel **and** `r.Context().Done()` → on grant write `event: grant` and `Flush()` again. **Both flushes and the per-request deadline are load-bearing** (research R1)
- [X] T023 [US1] Write `httpapi/register_test.go`: the SSE handshake headers; the grant event shape; each of the four `400` refusals; **`EPERM` refused loudly rather than treated as dead**; and cancelling the request context mid-wait releasing the Lock (FR-006 at the integration level)
- [X] T024 [US1] Implement the Liveness Probe in `httpapi/probe.go`: a 3–5 s ticker **owned by the Job** and stopped by its Release, calling `process.GroupAlive` and releasing only on `ESRCH` (ADR-008, FR-007). A probe outliving its Job would release a healthy successor
- [X] T025 [US1] Write `httpapi/probe_test.go`: a dead holder released within the interval; an `EPERM` holder **not** released; and the probe stopping when its Job is released
- [X] T026 [US1] Implement `POST /release` in `httpapi/control.go`: release if held, grant to the head, and return `{"released":false}` with **`200`** when nothing held it — a no-op is a success, because a Runner's `trap` fires on paths where the Lock is already free (FR-005, FR-008)
- [X] T027 [US1] **Done in Phase 2, not here** — `Listen()` classifies *already running* vs *port taken by something else* (ADR-011) by probing `/status`, so the bind cannot be classified without it. Implement `GET /status` in `httpapi/status.go`: take a snapshot **under the mutex**, marshal **after releasing it**, compute `elapsedSeconds`/`waitingSeconds` server-side, and emit `job: null` with `queue: []` — **never `null`** for the queue (contracts/http-api.md)
- [X] T028 [US1] Write `httpapi/status_test.go`: the idle shape; one Job and two ordered Waiters; **no PID appearing twice** (invariant 2); and `queue` serialising as `[]` when empty
- [X] T029 [US1] Implement `POST /shutdown` in `httpapi/control.go`: release the Lock, close every Registration, return `jobWasActive` so the CLI can warn, then exit — **without signalling the Job** (ADR-009)
- [X] T030 [US1] Write `httpapi/shutdown_test.go`: asserts the Lock is dropped **and the Job's process is still alive** afterwards. ADR-009 is a behaviour, so it is asserted rather than assumed
- [X] T031 [US1] Implement `runner/wrap.go` per [contracts/cli.md](contracts/cli.md): resolve the repo label from the git toplevel; start the suite with `SysProcAttr{Setpgid: true}` so **the child leads its group**; register the **child's** PID; **forward `SIGINT`/`SIGTERM` to `-childPid`** (research R2); inherit `Stdout`/`Stderr` with no pipe; release on every path; exit with the child's status
- [X] T032 [US1] Write `runner/wrap_test.go`: output passed through byte-for-byte; the child's non-zero exit status propagated; the registered PID **is** a group leader; and with no daemon reachable the suite still runs with exactly one `trainsty:` notice on **stderr**
- [X] T033 [US1] Implement `daemonctl/start.go`: re-exec `os.Executable()` with the hidden `serve` subcommand and `SysProcAttr{Setsid: true}`, redirect stdin to `/dev/null` and both outputs to the log file, `Start()` **without** `Wait()`, then **poll `/status` for up to ~1 s to verify the bind before exiting 0** and print the address and the log path (research R3, FR-013a)
- [X] T034 [US1] Implement the hidden `serve` subcommand in `main.go` + `daemonctl/serve.go`: run the Daemon in the foreground. **Excluded from `help`** — it is an implementation detail of `start`
- [X] T035 [US1] Implement `daemonctl/stop.go`: `POST /shutdown`, and **when `jobWasActive` is true prompt for confirmation naming the repo** and stating that the suite keeps running and becomes invisible to scheduling (FR-014, ADR-009). `--force` skips it; **refuse rather than assume yes** when not a TTY
- [X] T036 [US1] Implement `daemonctl/status.go`: print the Job and the ordered queue per contracts/cli.md, `idle:` when free, and **exit 3** when unreachable so a script can tell *no scheduler* from *scheduler said no* (FR-015, FR-018)
- [X] T037 [P] [US1] Implement `daemonctl/ui.go`: `open` on darwin, `xdg-open` elsewhere, and **print the URL and exit 0 if the opener fails** — a failed browser launch is not a failed command (FR-016)
- [X] T038 [P] [US1] Implement `help` in `main.go`: the six public subcommands, **one line saying what trainsty is for** (the name gives nothing away — ADR-010), and one line stating that interactive/watch-mode suites are out of scope because a background process group reading the terminal gets `SIGTTIN` (research R2)
- [X] T039 [US1] Wire every release path through the one `scheduler.Release` and confirm all five are live: `/release`, the dropped Registration, the probe's `ESRCH`, `/stop` (stubbed until US3), `/shutdown`. Cross-check against `CLAUDE.md` → The Release Paths

**Checkpoint**: T018–T021 are green. Quickstart scenarios 1–5 and 8–10 pass by hand. **This is a shippable MVP** — a developer changes one line in a repository and collisions stop.

---

## Phase 4: User Story 2 — See what is holding the lock (Priority: P2)

**Goal**: A developer glances at a browser and knows what is running, for how long, and
who is next.

**Independent test**: quickstart scenario 6 — with one running and two waiting, the page
shows an advancing elapsed time and both waiters in grant order; stopping the daemon
shows *cannot reach the scheduler*, distinct from an empty queue.

- [X] T040 [P] [US2] Write the scenario-6 acceptance test in `e2e_test.go`: `GET /` returns the page; with a Job and two Waiters `/status` carries an advancing `elapsedSeconds` and the queue in grant order (FR-021, FR-022, FR-026)
- [X] T041 [US2] Implement `dashboard/index.html`: one file, inline CSS and JS, **no framework, no build step, no CDN** — the daemon must work with no network. Poll `/status` every 2000 ms, render the Job and the queue as a **table** with headers, and show an **advancing elapsed time** rather than a static badge (FR-022, ADR-005)
- [X] T042 [US2] In `dashboard/index.html`, render every value from `/status` with **`textContent`, never `innerHTML`** — `repo` is developer-supplied, arrives as a query parameter, and lands on a page that can terminate process groups (FR-025)
- [X] T043 [US2] In `dashboard/index.html`, distinguish ***cannot reach the scheduler*** from ***nothing is running*** as separate visible states, recover from the former without a reload, and hold **no client-side state**: no cache, no `localStorage`, no optimistic update (FR-023, FR-024)
- [X] T044 [US2] Implement `dashboard/embed.go` with `//go:embed index.html` and serve it at `GET /`; every other path is `404` with an empty body. `go:embed` is what lets one binary serve a UI (ADR-001)
- [X] T045 [P] [US2] Write `dashboard/embed_test.go`: `GET /` serves the page with `text/html`; an unknown path is `404`; and the embedded bytes contain **no `innerHTML`** — a grep-able guard for T042 that survives future edits

**Checkpoint**: The Lock is observable. US1 still passes unchanged.

---

## Phase 5: User Story 3 — Kill a run that must not finish (Priority: P3)

**Goal**: A developer ends a hung suite and every process it started, and the queue moves
on.

**Independent test**: quickstart scenario 7 — a suite with three children, Stop pressed,
**zero** survivors, next waiter granted.

- [X] T046 [P] [US3] Write the scenario-7 acceptance test in `e2e_test.go`: wrap a suite that spawns three children, assert they exist, `POST /stop`, then assert **all of them are gone** and the next Waiter is granted (FR-027, FR-028, FR-029)
- [X] T047 [US3] Implement `POST /stop` in `httpapi/control.go`: take **no target parameter** so a stale page cannot name a suite that started after it rendered (FR-032), call `process.KillGroup` **outside the mutex** so `/status` stays answerable while the kill is in flight, then release and grant. Return `{"stopped":false}` with `200` when there is no Job (FR-031)
- [X] T048 [US3] Write `httpapi/stop_test.go`: a real child tree fully terminated and the Lock freed; `stopped:false` with `200` and no error when idle; and **a Stop arriving after the Job ended does not terminate its successor** (the stale-click case, FR-032)
- [X] T049 [US3] Add the Stop control to `dashboard/index.html`: a real `<button>` on the **Job's row only** — a Waiter has nothing to stop — that **confirms first**, naming the repo and stating the suite ends immediately, and posts with `Content-Type: application/json` and no body (FR-030, FR-033, SDR-001)
- [X] T050 [P] [US3] Extend `dashboard/embed_test.go`: the embedded page's Stop path sends the JSON content type and **no `pid`**, asserted against the embedded bytes

**Checkpoint**: All three stories complete. All ten quickstart scenarios pass.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: The documentation corrections research created, and the gates the
constitution requires. **The first two are obligations, not tidying** — `RULES.md` §6.6
requires a restatement to move with the code that proves it, and both of these were
deliberately deferred from the planning commit until the code existed.

- [X] T051 Update `knowledge/conventions/api.md`: replace the *"`WriteTimeout` and `IdleTimeout` must be zero"* guidance with the **per-request** `http.NewResponseController(w).SetWriteDeadline(time.Time{})` pattern, keeping both traps (the deadline and the missing `Flush()`) and citing research R1
- [X] T052 Update `knowledge/conventions/go.md` process rules: add that a child in its own process group **does not receive the terminal's `SIGINT`**, so a parent that puts it there MUST forward signals, and note the `SIGTTIN` consequence for suites that read the terminal (research R2)
- [X] T053 [P] Update `knowledge/architecture/structure.md`: the planned tree gained `daemonctl/` and `logpath/`, and `os/exec` is importable by `runner/` and `daemonctl/` **only** — record that as the grep-able rule it is
- [X] T054 [P] Update `README.md` Status table: Go code exists; state what is built and what the gate is. **Assert only what is in the tree** (`RULES.md` §6.6)
- [X] T055 Verify `go test -race -cover ./scheduler/` reports **≥80% statements** and that all five release paths have a test, per `knowledge/conventions/testing.md` → *Every release path has a test*. If a path lacks one, that test is the fix
- [X] T056 Run `scripts`-free gate sweep by hand and record it: `gofmt -l .` silent, `go vet ./...`, `go build ./...`, `go test -race ./...`. Confirm `go.mod` still has **no `require` block**
- [X] T057 Walk every scenario in `specs/001-serialize-e2e-runs/quickstart.md` end to end. **All ten walked.** 1, 2, 3, 6, 7 and 9 are automated in `e2e_test.go`; 5, 8 and 10 were run by hand, including scenario 10's *the daemon must not read its own log* check — a fabricated line was appended and the next `start` still reported idle. **Scenario 4 was verified by the shortened equivalent quickstart.md itself proposes**, not by a real 32-minute wait: with `WriteTimeout` cut to 2s, a 7-second wait still succeeded, which is what proves the per-request deadline removal (R1). The full-length form remains unrun.
- [x] T058 [P] ~~Add a hook under `.githooks/` running the gates~~ **DONE 2026-09-14, ahead of the phase.** `scripts/ci-local.sh` + `.githooks/pre-push` (running `--quick`) + `knowledge/playbooks/local-ci.md`. Landed early because the gates it enforces — `go.mod` declaring nothing, `os/exec` confined to `runner/` and `daemonctl/` — are cheaper to have before the code than to retrofit after. Wire it with `git config core.hooksPath .githooks`

---

## Dependencies & Execution Order

### Phase dependencies

- **Setup (Phase 1)**: no dependencies
- **Foundational (Phase 2)**: needs Setup. **Blocks all three stories** — every story
  reads or writes the Lock, and two of them need `process/`
- **US1 (Phase 3)**: needs Foundational
- **US2 (Phase 4)**: needs Foundational, and **needs `GET /status` from T027** — it is a
  view over it. This is the one real cross-story dependency, and it is why US2 is P2
- **US3 (Phase 5)**: needs Foundational, `process.KillGroup` (T006), and the Dashboard
  from US2 for its control surface
- **Polish (Phase 6)**: needs the stories whose documents it corrects — T051 needs T022,
  T052 needs T031

### Story dependencies, stated honestly

The template's ideal is fully independent stories. Two of these are not, and pretending
otherwise would produce a false parallel plan:

- **US1 is genuinely independent** and is the MVP.
- **US2 depends on `/status`** (T027, inside US1). It could be unblocked by building
  `/status` in Foundational instead — rejected, because `trainsty status` in US1 needs it
  anyway and moving it would make Foundational bigger without making US2 shippable
  sooner.
- **US3 depends on US2** for the button, and on `process.KillGroup` from Foundational.
  Its server half (T047, T048) is independent of US2 and **can be built and tested via
  `curl` before any UI exists** — worth knowing if US3 gets pulled forward.

### Within each story

- **The acceptance test comes first** and is watched failing for the right reason
- `process/` and `scheduler/` before the handlers that use them
- Handlers before the CLI that calls them
- `/status` before the Dashboard that polls it

### Parallel opportunities

- **Phase 1**: T003, T004
- **Phase 2**: T007 and T008 (after T005/T006); T011 and T012 alongside anything; T015
  after T014
- **US1 acceptance tests**: T018, T019, T020, T021 — four different test functions, one
  file, written together then run together
- **US1 late CLI**: T037 and T038 touch different files
- **US2**: T040 and T045
- **Phase 6**: T053, T054, T058

**The genuine constraint on parallelism is `e2e_test.go`.** Every acceptance test binds
port 45678, so they are written in parallel and **must run serially** — add
`e2e_test.go` cases to one package and do not `t.Parallel()` them. This is trainsty's
own contention problem, in its own test suite.

---

## Parallel Example: User Story 1 acceptance tests

```bash
# Written together — four functions in e2e_test.go, from the spec and contracts alone:
Task: "Scenario 1: two wrapped suites run strictly in order"          # T018
Task: "Scenario 3: kill -9 the holder, next waiter promoted"          # T019  ← mandatory
Task: "Scenario 2: SIGINT kills the suite and frees the lock"         # T020
Task: "Scenario 9: no daemon — suite runs anyway, status exits 3"     # T021

# Then run them — SERIALLY, because they share port 45678:
go test -race -run 'TestAcceptance' .
```

All four must fail before T022 begins, and each must fail **because the feature is
absent**, not because the harness is broken.

---

## Implementation Strategy

### MVP first (User Story 1 only)

1. Phase 1 Setup → a buildable skeleton
2. Phase 2 Foundational → **the coverage gate must pass here**, before any handler
3. Phase 3 US1 → acceptance tests failing, then implementation until green
4. **STOP and VALIDATE**: quickstart scenarios 1–5 and 8–10 by hand
5. Usable: one line per repository and collisions stop

### Incremental delivery

1. Setup + Foundational → foundation
2. + US1 → **MVP**, genuinely shippable
3. + US2 → the Lock becomes observable
4. + US3 → hung runs become recoverable
5. + Phase 6 → the documentation matches the code, and a hook makes the gates real

### Parallel team strategy

One developer is the realistic case here, and the ordering above is the plan. With two:
after Foundational, one takes US1 while the other takes **T047/T048** (US3's server
half, testable with `curl`) and then US2's page — because US3's UI needs US2 but its
handler does not.

---

## Notes

- **`[P]` means different files and no incomplete dependency** — not "safe to run
  concurrently at runtime". The acceptance suite is the counter-example: parallel to
  write, serial to run
- **Commit per task or logical group**, Conventional Commits, scope from
  `CONTRIBUTING.md` (`scheduler`, `process`, `api`, `sse`, `dashboard`, `cli`, `docs`)
- **Watch every test fail before it passes.** A test that never ran red proves the code
  compiles, not that it works
- **`go test -race` is the gate**, never bare `go test` — a race here is a wrong Grant,
  which is two suites running at once
- **After fixing any bug**, append to `knowledge/ERRORS.md` with a `Prevention:` line
  that names a test (`RULES.md` §6.1)

---

## Phase 7: Convergence

**Appended 2026-09-14 by `/speckit-converge`.** Every finding below is a
**verification** gap, not a behavioural one: the feature does what the spec says, and
these are the parts that are trusted rather than proven. **No constitution principle is
violated** — all five were checked, including the Footprint MUST, which was measured
directly at 11.7 MB RSS and 0.0% CPU idle rather than assumed.

Ordered by severity. Each task names the requirement it traces to and the gap type.

- [X] T059 Add `daemonctl/stop_test.go` and `daemonctl/status_test.go` covering the confirmation prompt, the **refusal when stdin is not a terminal** (`--force` absent), the idle/running/unreachable output shapes, and exit code 3 — per FR-013, FR-014, FR-015 (missing). `daemonctl/` is at **0.0% coverage** with real logic in it, and `CLAUDE.md` → Testing Philosophy admits no exemption; FR-014's failure mode orphans a running suite
- [X] T060 Add a test in `httpapi/server_test.go` that binds `127.0.0.1:45678` with a plain listener and asserts `Listen()` returns `ErrPortTaken`, then repeats against a real trainsty daemon and asserts `ErrAlreadyRunning` — per FR-019 and ADR-011 (missing). The distinction exists *because* the two remedies differ, and nothing currently proves it survives a refactor
- [X] T061 Tighten the acceptance bounds in `e2e_test.go` to the stated criteria: **2 s** for the interrupt case and **5 s** for the killed-holder case, replacing the current 4 s and 10 s — per SC-002 and SC-003 (partial). As written, a regression that doubled release latency passes both tests
- [X] T062 Add a test in `httpapi/log_test.go` asserting the log records each grant, release **with its cause**, and refusal, and asserting the absence half: no wrapped suite's output, no environment variable, no absolute path under `$HOME` — per FR-020 and `CLAUDE.md` → Logging (missing)
- [X] T063 Add an end-to-end assertion in `e2e_test.go` that a repository label containing markup (`<b>x`) reaches `/status` and is rendered as text — per US2/AC5 and FR-025 (partial). `dashboard/embed_test.go` greps the source for `.innerHTML`, which guards the implementation but never exercises the value
- [X] T064 Add a goroutine guard in `httpapi/register_test.go`: record `runtime.NumGoroutine()`, open and drop 50 registrations, then assert the count returns to its baseline within a bounded wait — per the constitution's Additional Constraints, which name a per-client goroutine leak as a defect (missing). On a daemon a leak is permanent, and nothing currently detects one
- [X] T065 Add `main_test.go` covering dispatch: no arguments, an unknown command, `wrap --` stripping versus `wrap` without the separator, and `--force`/`-f` parsing — per FR-017 and `contracts/cli.md` (missing). The root package is at **0.0% coverage** and the separator stripping is real logic
- [X] T066 Raise `runner/` coverage from **21.7%** by unit-testing `forwardSignals` re-entry behaviour and the release closure's idempotence against a stub server — per FR-034–FR-039 (partial). Signal forwarding is where two of this feature's defects already lived, and it is currently covered only at the acceptance layer
- [X] T067 **PASSED 2026-09-14** — a real 30m05s queue wait. The waiter was confirmed still queued and its process alive at five-minute checkpoints throughout, then was granted (`granted after 30m5s`) and ran to a clean exit. Result recorded in `quickstart.md` §4. Ran quickstart scenario 4 **at full length once** — a real 30-plus-minute queue wait against a release build — and record the result in `specs/001-serialize-e2e-runs/quickstart.md` — per FR-003 and SC-004 (partial). It is currently proven only by the shortened equivalent (a 7-second wait surviving a deliberately 2-second `WriteTimeout`), which validates the mechanism but not the stated duration
- [X] T068 Cover `logpath/` error branches — an unresolvable home directory and an unwritable parent — raising it from **57.1%** per the plan's decision to isolate the platform branch there (partial)

### Notes on what was deliberately NOT appended

- **`scheduler.SetClock`, `httpapi.DiscardLogger` and `dashboard.Page`** are exported
  test seams, each documented as such. Surfaced during the `unrequested` sweep and
  judged legitimate rather than filed as findings.
- **`govulncheck` reports "did not run" on every gate invocation** because it is not
  installed. It is part of `scripts/ci-local.sh`, not of this feature's spec or plan, so
  it is out of the convergence scope — but it is worth installing:
  `go install golang.org/x/vuln/cmd/govulncheck@latest`.
- **`OD-3` and `OD-4`** remain open decisions and block nothing. `OD-4` (distribution) is
  now the practical next question, since there is a binary worth installing.
