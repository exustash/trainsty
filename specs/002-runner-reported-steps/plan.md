# Implementation Plan: Runner-reported Steps on the Dashboard

**Branch**: `docs/runner-reported-steps-spec` (feature directory `002-runner-reported-steps`) | **Date**: 2026-11-05 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/002-runner-reported-steps/spec.md`

**Note**: This replaces the pre-spec sketch committed in `a3f9ee7`. Its findings
and client-side changes are carried into [research.md](research.md).

## Summary

The Runner announces the start and end of each named Step. The Daemon keeps the
Job's running Steps (at most 16) and reports them in `/status`, and the Dashboard
shows each one with its own elapsed time. Steps live on the Job, so all five
release paths clear them with no change to any path. Announcements are matched to
the Job by exact pid, so a stale run cannot label its successor. The shipped Runner
passes its pid to the suite as `TRAINSTY_PID`, and a new `trainsty step` command
reads it and always exits 0, so a progress label can never fail CI.

## Technical Context

**Language/Version**: Go, the floor set by `go.mod` (1.20; `ResponseController`)

**Primary Dependencies**: standard library only (`net/http`, `encoding/json`, `embed`)

**Storage**: none. In memory, behind the one mutex (DDR-001)

**Testing**: `testing` and `httptest`, `go test -race ./...`, and the acceptance suite via `scripts/ci-local.sh --everything`

**Target Platform**: Linux and macOS (ADR-002)

**Project Type**: single static binary: Daemon, CLI and embedded Dashboard

**Performance Goals**: each announcement adds at most 1 s with no Daemon answering, and at most 100 ms against a responsive one (SC-015)

**Constraints**: no new dependency, mutex, persistence, configuration or push channel; the Daemon never reads the Runner's output (FR-050)

**Scale/Scope**: ≤16 running Steps per Job, names ≤128 bytes, so at most ~2 KB of new state. One endpoint, one subcommand, one Dashboard column

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle / constraint | Verdict | Why |
| --- | --- | --- |
| I. Single binary, stdlib first | ✅ | No new import beyond what `httpapi/` and `daemonctl/` already use |
| II. The Daemon is a traffic light | ✅ | Steps are announced, never inferred from output (R1). The Daemon spawns, captures and reformats nothing; `wrap` only gains an environment variable |
| III. Every Lock has a guaranteed release | ✅ | No path acquires or holds the Lock. Steps are owned by the Job, so every release path clears them; no release path changes |
| IV. Unix-only, process-group discipline | ✅ | No syscall. `/step` compares a pid and signals nothing (R3) |
| V. One state, one mutex, one truth | ✅ | Steps sit on the Job under `s.mu`; `/status` reports them verbatim; the Dashboard derives them from the latest poll; nothing is persisted |
| API surface is the complete contract | ⚠️ **amendment** | `POST /step` is new: **MINOR, 1.0.0 → 1.1.0**, carried in the same change. Not a violation, but the gate requires it |
| Timings are named constants | ✅ | `stepTimeout` in `daemonctl/` |
| Footprint: no unbounded buffer | ✅ | 16-Step cap and 128-byte names (FR-043, FR-044) |
| Inputs validated before use | ✅ | `pid`, `state` and `name` validated in `httpapi/` before the scheduler is called (R5) |
| Simplicity review | ✅ | No interface, no knob: the cap is a constant, not configuration |

**Post-design re-check (after Phase 1):** unchanged, all ✅ plus the MINOR
amendment. The design added nothing the table above did not anticipate.

## Project Structure

### Documentation (this feature)

```text
specs/002-runner-reported-steps/
├── spec.md
├── plan.md               # this file
├── research.md           # R1–R10, including the client changes
├── data-model.md         # Step, Job, Snapshot
├── quickstart.md         # seven validation scenarios
├── contracts/
│   ├── http-api.md       # POST /step, and the /status change
│   └── cli.md            # trainsty step, and wrap's TRAINSTY_PID
├── checklists/requirements.md
└── tasks.md              # /speckit-tasks — not created here
```

### Source Code (repository root)

```text
scheduler/
├── scheduler.go          # Step type, Job.steps, StartStep/EndStep, ErrTooManySteps, JobView.Steps
└── scheduler_test.go     # identity, cap, restart, Release clears, clock
httpapi/
├── step.go               # new: handleStep, validation, refusal logging
├── step_test.go          # new: guard, codes, no-ops, logging
├── guard.go              # three new error codes
├── server.go             # route /step behind guardMutating
└── status_test.go        # steps [] and populated
runner/
├── wrap.go               # acquire reports granted; set TRAINSTY_PID
└── wrap_test.go          # child sees it only when granted
daemonctl/
├── step.go               # new: Step client, stepTimeout, always exit 0
└── step_test.go          # new: silent, posts pid, not-recorded line
main.go                   # "step" in the commands table
dashboard/
├── index.html            # "Running now" column
└── embed_test.go         # the column uses textContent and a list
```

**Structure Decision**: the existing package layout, unchanged. The scheduling
rule (identity and cap) is pure and lives in `scheduler/`; the wire format and
validation in `httpapi/`; the client in `daemonctl/`, which keeps `os/exec` out of
the new command.

## Documentation carried in the same change

| File | Change |
| --- | --- |
| `knowledge/domains/ubiquitous-language.md` | **First:** the Step term. Banned names: stage, phase, task, job |
| `.specify/memory/constitution.md` | MINOR 1.1.0: `POST /step` in the API surface; version line; Sync Impact Report |
| `knowledge/conventions/api.md` | Endpoint table and a `/step` section |
| `knowledge/architecture/adr.md` | ADR-014: the Runner announces Steps (R1's rejected alternatives) |
| `knowledge/data/state.md`, `knowledge/data/ddr.md` | Steps held in memory; DDR-003 records that nothing new is written |
| `knowledge/security/sdr.md` | SDR-001 note: `/step` changes only a label, so the no-token reasoning holds |
| `knowledge/product/decisions.md` | Register rows for FR-040–FR-052, **after** the vault note gains the requirement |
| `CLAUDE.md` | CLI table: eight subcommands, with `step`'s always-0 exception |
| `README.md` | "Reporting Steps", with the `curl` form for hand-written Runners |

## Sequencing and the busy-machine constraint

- **Any time:** tasks, code and unit and `httpapi/` tests; all docs.
- **Needs no active Job on the machine:** the acceptance suite (it binds 45678),
  installing the new binary, restarting the Daemon (ADR-009), and the quickstart.
- **After a release (a `RULES.md` §1.3 approval step):** the `teneerai.web`
  `ci-local.sh` change (research R10), made while no run of it is active.

## Complexity Tracking

No violations. The one item the gate flags is the constitution's own amendment
procedure for a new endpoint, not a deviation from a principle.
