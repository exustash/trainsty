# Implementation Plan: Serialize Local E2E Runs Behind a Single Lock

**Branch**: `001-serialize-e2e-runs` | **Date**: 2026-09-13 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/001-serialize-e2e-runs/spec.md`

## Summary

Build the whole of trainsty: a background daemon holding one FIFO lock of depth one,
a `wrap` command that runs a developer's suite under it, a polled dashboard, and
forced termination of a hung run's process group. The design is already settled by
twelve ADRs, one DDR pair and one SDR; this plan resolves the four implementation
unknowns those records deliberately left open, fixes the package layout, and defines
the contracts the acceptance scenarios assert against.

**The technical approach in one line:** six packages with a one-way dependency graph,
standard library only, where the only shared mutable state is one struct behind one
mutex and the only syscalls live in one package.

**What research changed** (see [research.md](research.md)): two findings alter
guidance that was already written down, and both are corrections rather than
additions.

1. **`http.NewResponseController(w).SetWriteDeadline(time.Time{})`** disables the
   write deadline **per request**, so `/register` can hold a 30-minute stream open
   *without* globally zeroing `Server.WriteTimeout` and losing timeouts on every other
   route. `knowledge/conventions/api.md` currently prescribes the global version.
2. **`Ctrl+C` reaches only the terminal's foreground process group.** Putting the
   suite in its own group — which FR-035 requires so it can be terminated as a unit —
   means the suite stops receiving the developer's `Ctrl+C`. `wrap` must forward it.
   Nothing in the knowledge base had noticed this, and it is the difference between
   `Ctrl+C` working and appearing to do nothing.

## Technical Context

**Language/Version**: Go, `go 1.20` in `go.mod`. **1.20 is the floor and it is
load-bearing**: `http.NewResponseController` arrived in 1.20 and is what makes
finding 1 above possible. `embed` (1.16) and everything else needed predates it.
Verified against the installed toolchain, go1.27.1 darwin/arm64. A low floor is
deliberate — the product's value is installing anywhere with no setup.

**Primary Dependencies**: **None.** `go.mod` requires nothing. `net/http`,
`os/exec`, `os/signal`, `syscall`, `encoding/json`, `embed`, `log`, `sync`, `time`,
`testing`, `net/http/httptest`. Enforced by the constitution's Principle I.

**Storage**: No datastore. One append-only log file per user
(`~/Library/Logs/trainsty.log`, or `${XDG_STATE_HOME:-~/.local/state}/trainsty/trainsty.log`),
mode `0600`, **never read back** — DDR-002, and FR-013b makes the never-read half a
requirement. All scheduling state is in memory (DDR-001).

**Testing**: `go test -race ./...` with `testing`, `httptest`, and real short-lived
child processes for `process/`. No test framework, no matcher library, no mocks of
the OS. `-race` is a gate, not an option.

**Target Platform**: Linux and macOS (Unix only, ADR-002). `arm64` and `amd64` — no
platform-specific code beyond the log path branch and Unix-only syscalls.

**Project Type**: A single Go module producing one static binary that is three things
at once: a **daemon** (`start`), a **client** (`wrap`, `status`, `stop`, `ui`), and an
**embedded web UI** server.

**Performance Goals**: Grant the Lock within **100 ms** of it becoming free.
Release within **2 s** of a Registration dropping (FR-006) and within **5 s** of a
holder's process disappearing (FR-007). Dashboard reflects reality within one 2 s
poll. Idle CPU indistinguishable from zero — no busy waiting anywhere.

**Constraints**: Resident set under **20 MB** idle. No goroutine that outlives its
Job or request. A wait of **30+ minutes** must not be severed by any timeout
(FR-003). `wrap` must add no measurable latency to a suite and must not alter a
single byte of its output (FR-037).

**Scale/Scope**: One machine, one developer, realistically **≤10** concurrent
waiters. Roughly 1,500–2,000 lines of Go including tests. 42 functional
requirements across three independently shippable stories.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Gates derived from `.specify/memory/constitution.md` v1.0.0.

| Principle | Gate | Pre-Phase 0 | Post-Phase 1 |
| --------- | ---- | ----------- | ------------ |
| **I. Single Static Binary, Stdlib First** | `go.mod` requires nothing; no web framework, router library, SSE library or test framework | ✅ PASS | ✅ PASS — `http.ServeMux`, hand-written SSE, `embed` |
| **II. The Daemon Is a Traffic Light** | The Daemon does not spawn, supervise, capture or reformat a suite | ⚠️ **CHECKED — see below** | ✅ PASS — `runner/` is client-side; `scheduler/` and `httpapi/` never spawn |
| **III. Every Lock Has a Guaranteed Release** | All five release paths exist, are idempotent, and each has a test | ✅ PASS | ✅ PASS — one `release` func, five callers, identity-compared |
| **IV. Unix-Only, Process-Group Discipline** | No Windows shims, no syscall abstraction; termination targets the group | ✅ PASS | ✅ PASS — `process/` is the only syscall site |
| **V. One State, One Mutex, One Truth** | One struct, one mutex, nothing persisted, FIFO depth one | ⚠️ **CHECKED — see below** | ✅ PASS — `Scheduler` + `sync.Mutex`; log is write-only |

**Principle II — `wrap` spawns a process, and this is not a violation.** The
principle constrains the **Daemon**: *"The daemon owns lock and queue state and
nothing else. It MUST NOT spawn, supervise, proxy, capture, buffer, or reformat the
E2E test process or its output."* `trainsty wrap` is a **separate process the
developer starts** — a client that happens to ship in the same binary. The Daemon
started by `trainsty start` spawns nothing and reads no output. Recorded as
**ADR-012** before any code exists, precisely so that `exec.Command` in the `wrap`
path is not read as a breach.

**Enforcement — and there are two spawn sites, not one.** Research R3 adds a second:
`trainsty start` re-executes the binary to detach the Daemon. Both are **client-side**
— `start` is the developer's command, and the process it spawns *becomes* the Daemon
rather than being spawned *by* one. So the review check is:

> `os/exec` may be imported by **`runner/` and `daemonctl/` only.** An import of
> `os/exec` in `scheduler/`, `httpapi/`, `process/`, `dashboard/` or `logpath/` is a
> defect.

That is grep-able, which a prose claim about intent is not.

**Principle V — the log file is not persisted state.** The principle forbids
persisting state because *"a stale on-disk lock is a deadlock waiting to be
inherited."* The log is **append-only and never opened for reading** (FR-013b), so it
can never be inherited as state. The test is not *does trainsty write* but **does
trainsty read anything back** — DDR-002 records it, and DDR-001 is unchanged.

**No violations. Complexity Tracking is empty.**

Two additional gates this feature must satisfy, from the constitution's *Development
Workflow & Quality Gates*:

- **≥80% statement coverage** on lock, queue and release logic → `scheduler/` is pure
  precisely so this is reachable.
- **The mandatory abrupt-exit test** — kill a holder without letting it call
  `/release`, assert the next waiter is promoted. Planned as
  `httpapi/register_test.go`, and it is the single most important test in the suite.

## Project Structure

### Documentation (this feature)

```text
specs/001-serialize-e2e-runs/
├── plan.md              # This file
├── research.md          # Phase 0 output — the four unknowns, resolved
├── data-model.md        # Phase 1 output — state, invariants, transitions
├── quickstart.md        # Phase 1 output — how to prove it works
├── contracts/
│   ├── http-api.md      #   the five endpoints, exactly
│   └── cli.md           #   the six subcommands, their output and exit codes
├── checklists/
│   └── requirements.md  # Spec quality checklist — 16/16
└── tasks.md             # Phase 2 output (/speckit-tasks — NOT created here)
```

### Source Code (repository root)

```text
trainsty/
├── go.mod                      # module git-hosted path; requires nothing
├── main.go                     # subcommand dispatch ONLY — no logic
├── scheduler/                  # US1 core. NO net/http, NO syscall, NO os/exec
│   ├── scheduler.go            #   Lock, Queue, Register, Release, Grant
│   └── scheduler_test.go       #   pure, fast, -race, ≥80% statements
├── process/                    # the ONLY syscall site
│   ├── group.go                #   GroupAlive, KillGroup, IsGroupLeader
│   ├── group_darwin_linux.go   #   (only if a real divergence appears)
│   └── group_test.go           #   real children, always reaped
├── httpapi/                    # the five endpoints
│   ├── server.go               #   ServeMux, timeouts, the loopback bind
│   ├── register.go             #   SSE: ResponseController, per-request deadline
│   ├── status.go               #   snapshot under the mutex, then marshal
│   ├── control.go              #   /release /stop /shutdown + the CSRF floor
│   └── *_test.go               #   httptest, incl. the mandatory abrupt-exit test
├── runner/                     # `trainsty wrap` — client-side. Spawns the suite
│   ├── wrap.go                 #   Setpgid child, signal forwarding, exit passthrough
│   └── wrap_test.go
├── daemonctl/                  # `start` / `stop` / `status` / `ui` — client-side
│   ├── start.go                #   re-exec with Setsid, then VERIFY the bind (R3)
│   ├── stop.go                 #   the confirmation prompt (ADR-009)
│   └── *_test.go
├── dashboard/                  # US2
│   ├── embed.go                #   go:embed index.html
│   └── index.html              #   inline CSS + JS, no build step
├── logpath/                    # DDR-002's one platform branch, isolated
│   ├── path.go
│   └── path_test.go
└── e2e_test.go                 # drives the built binary; skips if 45678 is taken
```

**Structure Decision**: Six packages plus `main.go`, chosen to make the dependency
direction in `knowledge/architecture/overview.md` mechanically true rather than
aspirational:

- **`scheduler/` is pure** and imports none of `net/http`, `syscall` or `os/exec`.
  This is the constraint that makes the ≥80% coverage gate and the FIFO tests cheap —
  no server, no children, no sleeping.
- **`process/` isolates every syscall**, so the `-pid` negation is written once with
  the comment that explains it, and so a reviewer grepping for `syscall.Kill` finds
  exactly one call site.
- **`runner/` and `daemonctl/` are separate packages** from everything the Daemon
  uses, which turns ADR-012's "`wrap` is a client" from a claim into a structural fact
  — and makes *"only these two import `os/exec`"* a one-line check.
- **`daemonctl/` holds the client subcommands** so `main.go` stays dispatch. Its
  `start.go` carries R3's non-obvious part: **verify the bind before exiting `0`**,
  because otherwise `start` reports success for a daemon that died on a taken port and
  ADR-011's careful error message lands in a log nobody is watching.
- **`logpath/` exists to contain the project's first platform branch.** Two lines of
  `runtime.GOOS` in their own package with their own test, rather than a conditional
  in `main.go` that grows.
- **No `internal/`, no `cmd/`.** One binary and one module; the extra path segments
  buy nothing (`knowledge/architecture/structure.md`).

## Complexity Tracking

> Fill ONLY if Constitution Check has violations that must be justified.

**No violations.** Both ⚠️ rows in the Constitution Check are *clarifications with
existing decision records* (ADR-012, DDR-002), not deviations — neither adds a
dependency, a persisted store, a second mutex, or configuration, and each was
recorded before this plan rather than to excuse it.

## Post-Design Constitution Re-Check

*Required by the gate. Re-evaluated after Phase 1, against the artifacts actually
produced rather than against the intent.*

**All five principles pass.** The `Post-Phase 1` column above is the result. Three
things were checked specifically because Phase 0 changed the approach:

- **R1's per-request write deadline** uses `net/http` only — no new dependency, and it
  *strengthens* the design by keeping timeouts on the other four routes. Principle I
  unaffected.
- **R2's signal forwarding** happens in `runner/`, in the developer's own process.
  The Daemon still neither spawns nor signals anything it did not learn from a
  validated Registration. Principle II holds.
- **R3's re-exec** introduced a second `os/exec` site and therefore a **correction to
  this plan's own enforcement rule** — see Principle II above. Caught here rather than
  in review, which is what the re-check is for.

**Complexity Tracking remains empty.** No violation was found, and nothing needed
justifying: the design adds no dependency, no second mutex, no persisted state that is
read back, no configuration, and no concurrency above one.

**Two follow-ups are owed to the knowledge base**, recorded in
[research.md](research.md) → *Follow-ups*: `conventions/api.md` (per-request deadline)
and `conventions/go.md` (signal forwarding, `SIGTTIN`). `RULES.md` §6.6 requires them
to land **with the code that proves them**, not now — a convention doc that describes
unwritten code is the drift that rule exists to prevent.
