# Tasks: Runner-reported Steps on the Dashboard

**Input**: Design documents from `specs/002-runner-reported-steps/`

**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), [research.md](research.md),
[data-model.md](data-model.md), [contracts/](contracts/), [quickstart.md](quickstart.md)

**Tests are mandatory here, not optional.** `CLAUDE.md` → Definition of Done: changed
behaviour ships with a changed test **in the same commit**, and `RULES.md` §2.1 asks
for the test to be seen failing first. Every test task below precedes the code it
covers, and the pair lands as one commit.

**Gate for every commit:** `scripts/ci-local.sh --quick` (gofmt, vet, build,
`go test -race ./...`, scheduler coverage ≥80%, no dependencies, `os/exec` only in
`runner/` and `daemonctl/`). Phase 6 adds `--everything`.

**Numbering:** ADR-014 is taken (wrap starts the Daemon), so this
feature's ADR is **ADR-015**. DDR-003 is free.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: can run in parallel (different files, no dependency on an open task)
- **[Story]**: US1, US2, US3 from [spec.md](spec.md)

---

## Phase 1: Setup (vocabulary and gates before any code)

**Purpose**: the glossary moves before the code (`CLAUDE.md` → Ubiquitous
Language), and the vault precondition is checked before the register is touched.

- [ ] T001 Add the **Step** term to `knowledge/domains/ubiquitous-language.md` (lock-domain table): definition "one named stage of a Job, announced by its Runner", lifecycle started → running → ended (or gone with its Job), relationship "0–16 per Job", banned names *stage, phase, task, job*; and under *Words this project does not use*, record the `ci-local.sh` "jobs" clash (`JOB_FUNC`, `JOB_LABEL`) from research.md → Findings
- [ ] T002 Confirm with the Developer that the Obsidian vault note `local-ci scheduler.md` has gained the Steps requirement and the mirror `knowledge/product/local-ci-scheduler-requirements.md` has been re-copied. **Writes outside the repository are the Developer's (`RULES.md` §1.3)** — do not edit the vault or the mirror. If not done, leave T040 open and continue; nothing else depends on it

---

## Phase 2: Foundational (the Step state every story needs)

**Purpose**: the pure scheduler model and the `/status` shape. US1, US2 and US3 all
read or write it.

**⚠️ No user-story work starts until this phase is green under `-race`.**

- [ ] T003 Write failing tests in `scheduler/scheduler_test.go`, table-driven, Arrange–Act–Assert, using `SetClock`: `TestStartStepRecordsForTheJobsPID`, `TestStartStepIgnoresAPIDThatIsNotTheJob` (no Job; a Waiter's pid; the previous Job's pid after `Release`) returning `recorded == false`, `TestEndStepRemovesARunningStep`, `TestEndStepOfAnUnknownNameIsANoop`, `TestReleaseDropsTheJobsSteps` (every release entry point: `Release`, `ReleaseByPID`, `Withdraw` of the Job's Waiter), `TestSuccessorStartsWithNoSteps` (SC-014), `TestReRegistrationKeepsTheJobsSteps` (ADR-007), `TestStepsNeverChangeGrantOrder` (FR-052, SC-013: the same sequence of Registrations and Releases, run with and without announcements, yields the same Grants in the same order)
- [ ] T004 Implement in `scheduler/scheduler.go`: unexported `step{name string; startedAt time.Time}`, `Job.steps []step` (oldest first, **read and written only under `s.mu`** — add a comment saying `PID`, `Repo`, `GrantedAt` stay immutable and `steps` does not), `func (s *Scheduler) StartStep(pid int, name string) (recorded bool, err error)` and `func (s *Scheduler) EndStep(pid int, name string) (recorded bool)`, each comparing `pid` with the current Job's under the mutex in one critical section (FR-041 hand-over). Doc comments on both. Make T003 pass
- [ ] T005 Write a failing race test `TestStepAnnouncementDuringHandOverReachesOneJobOrNone` in `scheduler/scheduler_test.go`: goroutines call `StartStep(oldPID, …)` while the Job is released and a Waiter is granted; assert the successor never holds a Step from `oldPID`. Run with `go test -race ./scheduler`; T004's single critical section should already make it pass — if it does not, fix T004, not the test
- [ ] T006 Write failing tests in `scheduler/scheduler_test.go` for the snapshot: `TestSnapshotReportsStepsOldestFirst`, `TestSnapshotReportsAnEmptyStepListNeverNil` (marshal to JSON and assert `"steps":[]`), `TestSnapshotStepElapsedUsesTheSameNowAsTheJob`
- [ ] T007 Implement in `scheduler/scheduler.go`: exported `StepView{Name string \`json:"name"\`; StartedAt time.Time \`json:"startedAt"\`; ElapsedSeconds int \`json:"elapsedSeconds"\`}` with a doc comment, `JobView.Steps []StepView \`json:"steps"\``, filled in `Snapshot()` under the mutex from the same `now` as the Job's elapsed time, initialised to an empty slice. Make T006 pass
- [ ] T008 [P] Add to `httpapi/status_test.go`: `TestStatusReportsAnEmptyStepsList` and `TestStatusReportsRunningSteps` (call `StartStep` on the server's scheduler, then `GET /status`, decode, assert names, order and `elapsedSeconds`). Additive: assert the existing `job` keys are unchanged (FR-045)
- [ ] T009 Run `go test -race -cover ./scheduler` and confirm ≥80% statements; commit T003–T008 as `feat(scheduler): hold the Job's running Steps`

**Checkpoint**: Steps exist in the model and `/status`, with nothing able to set them yet.

---

## Phase 3: User Story 1 — See which Step the Job is on, and for how long (P1) 🎯 MVP

**Goal**: a Runner under `trainsty wrap` announces Steps with `trainsty step`, and
the Job's row on the Dashboard names the running Step with its own ticking time.

**Independent test**: quickstart scenario 1 with a strictly sequential script —
each Step appears while it runs and disappears when it ends; a Job with no Steps
looks exactly as today.

### The endpoint

- [ ] T010 [US1] Write failing tests in a new `httpapi/step_test.go` (package comment, `httptest`, the `newTestServer` and `errorCode` helpers in `httpapi/guard_test.go`): `TestStepRecordsForTheJobsPID` (`200 {"recorded":true}`), `TestStepNoopsAnswerRecordedFalse` (table: no Job, stale pid, Waiter pid, ending an unknown name — all `200 {"recorded":false}`, FR-042), `TestStepRefusesGetWith405`, `TestStepRefusesAMissingJSONContentTypeWith415`, `TestStepRefusesAForeignOriginWith403` (FR-047)
- [ ] T011 [US1] Write failing validation tests in `httpapi/step_test.go`, table-driven: `invalid_pid` (missing, `0`, negative, non-numeric), `invalid_step_state` (missing, `pause`), `invalid_step_name` (empty, 129 bytes, `\n`, `\x00`, `\x7f`), the 128-byte name accepted, `Lint` and `Lint ` recorded as two Steps (no trimming), and **precedence**: a request bad in pid, state and name returns `invalid_pid`; bad in state and name returns `invalid_step_state` (FR-043)
- [ ] T012 [US1] Add `codeInvalidStepState = "invalid_step_state"`, `codeInvalidStepName = "invalid_step_name"` and `codeTooManySteps = "too_many_steps"` to the code block in `httpapi/guard.go`
- [ ] T013 [US1] Implement `handleStep` in a new `httpapi/step.go` (package comment): parse `pid`, `state`, `name` in that order; validate `name` with `isControl` and a `maxStepNameLen = 128` constant mirroring `maxRepoLen` in `httpapi/register.go`; delegate to `StartStep`/`EndStep`; encode `{"recorded":bool}` through `writeJSON`; return codes through `writeJSONError`, never an internal error string. Route it in `httpapi/server.go`: `s.mux.HandleFunc("/step", guardMutating(s.handleStep))`. Make T010 and T011 pass
- [ ] T014 [US1] Write a failing test `TestStepLogsRefusalsQuotedAndTruncated` and `TestStepDoesNotLogAcceptedOrStaleAnnouncements` in `httpapi/step_test.go`, using `recordingServer` from `httpapi/log_test.go`: a refused name of 300 bytes containing `\n` produces one line, `%q`-quoted, truncated to 128 bytes, with the code and pid; a recorded and a stale announcement produce no line (FR-051)
- [ ] T015 [US1] Implement refusal logging in `httpapi/step.go`: one `logger.Printf` per refusal with the operation name, code, pid and name **as received**, each truncated to 128 bytes before `%q`. Make T014 pass
- [ ] T016 [US1] Commit T010–T015 **with the constitution amendment** (API surface change, `CLAUDE.md` → Definition of Done): in `.specify/memory/constitution.md` add `POST /step` to *Additional Constraints → API surface*, bump **Version 1.0.0 → 1.1.0 (MINOR)**, set *Last Amended* to the commit date, and update the Sync Impact Report comment. Same commit: `knowledge/conventions/api.md` endpoint table plus a `/step` section linking [contracts/http-api.md](contracts/http-api.md). Message `feat(httpapi): add POST /step for Runner-reported Steps`

### The Runner and the command

- [ ] T017 [P] [US1] Write failing tests in `runner/wrap_test.go`: `TestWrapSetsTrainstyPIDOnlyOnceGranted` (fake Daemon that grants; the child prints `$TRAINSTY_PID`; assert it equals the registered pid) and `TestWrapLeavesTrainstyPIDUnsetWhenUnserialized` (no Daemon reachable, and a refused Registration; assert the child sees it empty, with the test's own `TRAINSTY_PID` cleared first) and `TestWrapKeepsInheritedTrainstyPIDWhenUnserialized` (no Daemon reachable, `TRAINSTY_PID=4242` inherited; assert the child sees `4242`, FR-048). Real children the test started, reaped via `t.Cleanup`; observe exit by waiting, never by probing (`knowledge/conventions/testing.md`)
- [ ] T018 [US1] Implement in `runner/wrap.go`: `acquire` returns `(release func(), granted bool)` (research R6); `Wrap` appends `TRAINSTY_PID=<pid>` to the command's environment only when `granted`, and never removes an inherited one (FR-048, clarification 2026-11-08). Make T017 pass
- [ ] T019 [P] [US1] Write failing tests in a new `daemonctl/step_test.go` using the `baseURL` seam (extend `fakeDaemon` in `daemonctl/stop_test.go`, or add a `/step` handler beside it): `TestStepIsSilentWhenTrainstyPIDIsUnset`, `TestStepPostsThePIDNameAndState` (assert method `POST`, `Content-Type: application/json`, and the three query parameters, with the name URL-escaped), `TestStepIsSilentWhenRecordedOrANoop`. Every case asserts exit code `0`
- [ ] T020 [US1] Implement `func Step(args []string, stderr io.Writer) int` in a new `daemonctl/step.go` (package doc unchanged; doc comment stating the always-0 exception and why): read `TRAINSTY_PID` via `os.Getenv`; `POST baseURL+"/step"` with an `http.Client{Timeout: stepTimeout}` and a named constant `stepTimeout = 1 * time.Second`; return `0` on every path. Make T019 pass
- [ ] T021 [US1] Add `{name: "step", summary: "announce a stage of the running suite to the dashboard", run: runStep}` to the commands table in `main.go` and a `runStep` that calls `daemonctl.Step`; update `TestHelpListsEveryVisibleCommandAndHidesServe` in `main_test.go` so it fails first, then passes
- [ ] T022 [US1] Commit T017–T021 as `feat(cli): add trainsty step and pass TRAINSTY_PID to the suite`, carrying `CLAUDE.md` → The CLI: **eight** subcommands, a `trainsty step` row, and a bullet recording that `step` always exits 0 as the one exception to "exit codes are the contract" (FR-049)

### The Dashboard

- [ ] T023 [US1] Write failing tests in `dashboard/embed_test.go`, in the style of `TestPageRendersAnAdvancingElapsedTime`: `TestJobRowHasARunningNowColumn` (the header text `Running now` is in the page), `TestStepsRenderAsAListWithTextContent` (the Step cell is built with `el("ul"` / `el("li"` and the existing `TestPageNeverUsesInnerHTML` still passes), `TestJobWithoutStepsRendersNoPlaceholder` (no placeholder string such as `no steps` in the page)
- [ ] T024 [US1] Implement in `dashboard/index.html`, inside `renderRunning`: a `Running now` column between *Elapsed* and the Stop button; when `job.steps` is non-empty, a `<ul>` with one `<li>` per Step holding `el("span", step.name)` and `el("span", humanSeconds(step.elapsedSeconds))` (mono); when empty, an empty cell. No new state, cache or timer (Principle V, ADR-005). Make T023 pass
- [ ] T025 [US1] Commit T023–T024 as `feat(dashboard): show the Job's running Steps`

**Checkpoint**: US1 complete. A sequential Runner's Steps appear on the Dashboard.

---

## Phase 4: User Story 2 — See every Step running at once (P2)

**Goal**: parallel stages each get their own line, oldest first; the 16-Step cap
holds; restarting a name restarts its time.

**Independent test**: start two Steps, end the first, end the second — the
Dashboard shows two lines, then one, then none.

- [ ] T026 [US2] Write failing tests in `scheduler/scheduler_test.go`: `TestTwoStepsRunAtOnceOldestFirst`, `TestRestartingARunningNameRestartsItsTimeAndMovesItLast` (data-model.md → Restart), `TestASeventeenthNewNameIsRefusedWithErrTooManySteps` (assert `errors.Is(err, scheduler.ErrTooManySteps)` and that the 16 already running are kept), `TestRestartingANameIsAllowedAtTheCap`
- [ ] T027 [US2] Implement in `scheduler/scheduler.go`: `const maxSteps = 16` with a comment naming the ceiling (footprint, FR-044), the exported sentinel `ErrTooManySteps` with a doc comment, and the restart rule in `StartStep` (remove then append). Make T026 pass
- [ ] T028 [US2] Write a failing test `TestStepRefusesASeventeenthNameWith400TooManySteps` in `httpapi/step_test.go`, including that the refusal is logged (FR-051), then map `ErrTooManySteps` to `codeTooManySteps` in `httpapi/step.go` with `errors.Is`
- [ ] T029 [P] [US2] Add `TestStatusReportsParallelStepsOldestFirst` to `httpapi/status_test.go`
- [ ] T030 [US2] Commit T026–T029 as `feat(scheduler): cap a Job at 16 running Steps and restart a running name`

**Checkpoint**: US1 and US2 both work; `teneerai.web`'s parallel stages display correctly.

---

## Phase 5: User Story 3 — Report Steps from any Runner, at no risk to the run (P3)

**Goal**: an announcement can never change a run's result, whatever the
scheduler's state, and a hand-written Runner can announce with one `curl`.

**Independent test**: the same Step-announcing script outside a Runner, under a
Runner with no scheduler, and against an older scheduler — identical exit status
each time (SC-013).

- [ ] T031 [US3] Write failing tests in `daemonctl/step_test.go`, table-driven, each asserting exit `0` and the stderr line from [contracts/cli.md](contracts/cli.md): `TRAINSTY_PID` set to `abc`, `0` and `-1` → `invalid TRAINSTY_PID`; a fake Daemon answering `400 {"error":"invalid_step_name"}` → that code; a fake Daemon answering `404` (an older Daemon) → the status; a `baseURL` nobody listens on → `no scheduler reachable`; missing, extra, or a state other than `start`/`end` → the usage line
- [ ] T032 [US3] Write a failing test `TestStepGivesUpWithinTheTimeout` in `daemonctl/step_test.go`: a fake Daemon whose handler blocks until the test ends; assert `Step` returns `0` with the `no scheduler reachable` line. **No timing assertion** (`knowledge/conventions/testing.md`): bound the test with a deadline channel well above `stepTimeout` and fail only if `Step` never returns
- [ ] T033 [US3] Implement the not-recorded paths in `daemonctl/step.go`: exactly one stderr line per failed announcement, `trainsty: step "<name>" not recorded (<reason>)`, with the name `%q`-quoted, and the usage line for bad arguments; never a non-zero exit. Make T031 and T032 pass
- [ ] T034 [P] [US3] Add a "Reporting Steps" section to `README.md`: the `trainsty step start|end` form, the `trainsty_step` shell helper from research.md → R10, the always-0 behaviour, and the `curl -X POST -H 'Content-Type: application/json' "http://127.0.0.1:45678/step?pid=$$&name=…&state=start"` form for a hand-written Runner (US3 scenario 3), naming that `pid` must be the one it registered with
- [ ] T035 [P] [US3] Add a hand-written-Runner Step example to `knowledge/playbooks/wrapper-integration.md` (*Writing a Runner by hand*), linking README's section rather than repeating it
- [ ] T036 [US3] Commit T031–T035 as `feat(cli): make trainsty step harmless when it cannot record`

**Checkpoint**: all three stories independently functional.

---

## Phase 6: Polish, records and end-to-end validation

- [ ] T037 [P] Add **ADR-015 — The Runner announces Steps; the Daemon infers nothing** to `knowledge/architecture/adr.md` (newest first, above ADR-014), with research R1's rejected alternatives; update the file's front-matter description and the "newest" sentence in `knowledge/index.md` (which still names ADR-013, missing ADR-014 as well)
- [ ] T038 [P] Update `knowledge/data/state.md` (the Job holds 0–16 running Steps, mutable under `s.mu`) and add **DDR-003 — Steps are held in memory and nothing new is written** to `knowledge/data/ddr.md` (`RULES.md` §6.2 requires the entry even for "write nothing")
- [ ] T039 [P] Add a note under SDR-001 in `knowledge/security/sdr.md`: `/step` changes only a label on the current Job, so the no-token reasoning holds; any future reach beyond a label reopens SDR-001
- [ ] T040 Add register rows for FR-040–FR-052 to `knowledge/product/decisions.md`, each naming the files that restate it. **Only after T002 is confirmed**; otherwise leave this open and say so in the PR
- [ ] T041 [P] Update `knowledge/architecture/structure.md` with the new files: `httpapi/step.go`, `httpapi/step_test.go`, `daemonctl/step.go`, `daemonctl/step_test.go`
- [ ] T042 Add an acceptance test `TestAcceptanceStepsAppearInStatusWhileWrapped` to `e2e_test.go` beside `TestAcceptanceNoDaemonStillRunsTheSuite`: the built binary runs `wrap -- sh -c 'trainsty step start A; sleep 2; trainsty step end A'`; poll `/status` with a deadline until `steps` names `A`, then until it is `[]`. It must **skip with a message naming `trainsty status`** when 45678 is taken, like the existing acceptance tests
- [ ] T043 **Machine-state gate (`RULES.md` §1.3).** Before running anything that binds 45678, run `trainsty status`. If a Job is active, **stop and tell the Developer which repo holds it**; do not run `trainsty stop`. Only with the port free: `scripts/ci-local.sh --everything`
- [ ] T044 With the port free, walk quickstart.md scenarios 1–7 against a build of this branch and record any mismatch as a defect, not a doc edit
- [ ] T045 Commit T037–T042 as `docs: record Runner-reported Steps` (or with T042 as `test(e2e): …` if it is its own commit), and confirm `knowledge/index.md` lists nothing that moved

---

## Dependencies & execution order

```text
Phase 1 (T001–T002) ─► Phase 2 (T003–T009) ─► US1 (T010–T025) ─► US2 (T026–T030)
                                                     │
                                                     └──────────► US3 (T031–T036)
                                                                        │
                                       Phase 6 (T037–T045) ◄────────────┘
```

- **Phase 2 blocks everything.** The scheduler is the contract the handlers assume
  (`RULES.md` §1.2: these packages are not as disjoint as they look).
- **US2 needs US1's endpoint** (T013) for its `httpapi` test, but its scheduler work
  (T026–T027) can start as soon as Phase 2 is done.
- **US3 needs US1's command** (T020).
- **T040 waits on T002**, a human step outside the repository.
- **T043–T044 wait on the machine**, not on the code: no active Job on 45678.

## Parallel opportunities

- Phase 2: T008 beside T006–T007 once T004 is in.
- US1: the Runner pair (T017–T018) and the command pair (T019–T020) touch different
  packages and can run together after T016; the Dashboard pair (T023–T024) needs
  only Phase 2.
- US3: T034 and T035 (docs) beside T031–T033.
- Phase 6: T037, T038, T039, T041 are independent files.

```text
# After T016, in parallel:
T017–T018  runner/wrap.go, runner/wrap_test.go
T019–T020  daemonctl/step.go, daemonctl/step_test.go
T023–T024  dashboard/index.html, dashboard/embed_test.go
```

## Implementation strategy

1. **MVP = Phases 1–3.** A sequential Runner's Steps on the Dashboard. Ship-worthy
   on its own for any repository whose stages run one at a time.
2. **US2 next**, before any `teneerai.web` adoption: its stages run in parallel by
   default, so US1 alone would mislead there.
3. **US3 before a release.** An announcement that can fail CI gets deleted from the
   script; this story is what makes the feature safe to adopt.
4. **Release, then integrate.** The `teneerai.web` `ci-local.sh` change (research
   R10) is out of scope here and is a `RULES.md` §1.3 step after a tagged release,
   made while no run of that script is active.
