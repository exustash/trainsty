<!--
Sync Impact Report
Version change: (unfilled template) -> 1.0.0
Bump rationale: Initial ratification. All placeholder tokens replaced with concrete,
testable governance derived from the Trainsty local E2E scheduler requirements note.

Modified principles (placeholder -> concrete):
- [PRINCIPLE_1_NAME] -> I. Single Static Binary, Stdlib First
- [PRINCIPLE_2_NAME] -> II. The Daemon Is a Traffic Light (NON-NEGOTIABLE)
- [PRINCIPLE_3_NAME] -> III. Every Lock Has a Guaranteed Release
- [PRINCIPLE_4_NAME] -> IV. Unix-Only, Process-Group Discipline
- [PRINCIPLE_5_NAME] -> V. One State, One Mutex, One Truth

Added sections:
- Additional Constraints (was [SECTION_2_NAME])
- Development Workflow & Quality Gates (was [SECTION_3_NAME])
- Governance (rules filled)

Removed sections: none

Templates / docs status:
- .specify/templates/plan-template.md .. OK (Constitution Check gate is generic and
  resolves against this file; no hardcoded principles to update)
- .specify/templates/spec-template.md .. OK (no constitution-specific sections required)
- .specify/templates/tasks-template.md . OK (phase categories already cover setup,
  foundational, per-story, and polish work implied by these principles)
- .specify/templates/checklist-template.md OK (no constitution references)
- .claude/skills/speckit-*/SKILL.md .... OK (generic agent-neutral wording, no stale
  CLAUDE-only references requiring change)
- README.md ............................ ABSENT (no runtime guidance doc to sync)

Deferred items: none. RATIFICATION_DATE set to the date of this initial adoption.
-->

# Trainsty Constitution

Trainsty is a local E2E test scheduler: a single-binary Unix daemon that serializes
end-to-end test runs across multiple repository clones on one developer machine.

## Core Principles

### I. Single Static Binary, Stdlib First

The project ships as one statically compiled Go binary with zero runtime dependencies;
installation MUST never require a language runtime, package manager, or service manager.
The Go standard library is the default toolbox: `net/http` for the API and dashboard,
`os/exec` and `syscall` for process control, `encoding/json` for the wire format. A third
party dependency MUST NOT be added unless the standard library genuinely cannot do the job,
and each one MUST be justified in the plan's Complexity Tracking table before use.

Rationale: the tool exists to remove friction from a developer's machine. A dependency tree
or a runtime prerequisite reintroduces the very setup cost the scheduler is meant to save.

### II. The Daemon Is a Traffic Light (NON-NEGOTIABLE)

The daemon owns lock and queue state and nothing else. It MUST NOT spawn, supervise, proxy,
capture, buffer, or reformat the E2E test process or its output. The CI wrapper script keeps
ownership of execution so that native `stdout`/`stderr` reach the developer's terminal
unchanged. The single permitted exception is termination: `/stop` MAY signal a registered
process group it never started. Any change that moves test execution into the daemon is a
redefinition of the product and requires a MAJOR amendment.

Rationale: preserving the terminal workflow is the feature. A daemon that owns execution
becomes a CI server, inherits log plumbing, and breaks the workflow it was built to protect.

### III. Every Lock Has a Guaranteed Release

The lock MUST be released on every exit path, expected or not. Four release paths are
mandatory and MUST all remain live: explicit `POST /release`, SSE stream disconnect,
death of the tracked PID (detected by an OS liveness probe equivalent to `kill -0`, where
`ESRCH` triggers immediate release), and operator action via `POST /stop` or
`POST /shutdown`. No code path may acquire or hold the lock without a corresponding release
path, and every such path MUST ship with a test that terminates the client abruptly rather
than cleanly. Errors in a release path MUST be logged and MUST still free the lock; a
release MUST NOT be skipped because a cleanup step failed.

Rationale: a queue that deadlocks is worse than no queue. A developer whose terminal was
killed with `Ctrl+C` must never need to restart the daemon, and silent cleanup failures are
how indefinite deadlocks are born.

### IV. Unix-Only, Process-Group Discipline

Linux and macOS are the only supported targets. The project MUST NOT carry Windows
compatibility shims, abstraction layers over `syscall`, or portability indirection for
platforms it does not support. Every tracked job MUST be identified by the process group
leader's PID, and forced termination MUST target the whole group
(`syscall.Kill(-pid, syscall.SIGKILL)`) so orphaned headless browsers and child processes
die with it. Killing a bare PID and leaving descendants behind is a defect, not a partial
success.

Rationale: whole-group termination is the only reliable way to reclaim the resources an
aborted E2E suite holds, and it is exactly what a cross-platform abstraction would forfeit.

### V. One State, One Mutex, One Truth

All lock and queue state lives in a single in-memory structure guarded by a single mutex.
`GET /status` MUST report that structure verbatim, and the web dashboard MUST be a thin
poller that derives every displayed value from the latest response, holding no state,
cache, or optimistic update of its own. State MUST NOT be persisted: the queue is
meaningless once the daemon and the processes it tracked are gone, and a stale on-disk lock
is a deadlock waiting to be inherited. All queueing is strict FIFO with exactly one holder
at a time; concurrency limits above one MUST NOT be introduced without a MAJOR amendment.

Rationale: a single guarded state and a dumb client make the whole system inspectable at a
glance, which is what lets a developer trust a lock they cannot see.

## Additional Constraints

- **Port**: the daemon binds the hardcoded port `45678` and MUST NOT fall back to another
  port. A bind failure MUST exit with a message naming the port and the likely cause
  (a daemon already running).
- **API surface**: `GET /register` (SSE), `POST /release`, `GET /status`, `POST /stop`, and
  `POST /shutdown` are the complete contract. Adding an endpoint is a MINOR amendment;
  changing or removing one is MAJOR.
- **Wait protocol**: queue waiting MUST use Server-Sent Events, never polling or a
  long-lived plain HTTP request, so a 20+ minute wait cannot be severed by a timeout.
- **Timings**: the dashboard polls `/status` every 2000 ms; the daemon probes the active
  PID's liveness every 3-5 seconds. Both intervals MUST be named constants, not literals
  scattered across call sites.
- **Footprint**: an idle daemon MUST hold negligible CPU and a small resident set. A busy
  wait, a per-client goroutine leak, or an unbounded buffer is a defect.
- **Inputs**: `pid` and `repo` query parameters are untrusted. Both MUST be validated
  before use, and a PID MUST be confirmed to exist and be signalable before it is tracked.
- **Secrets**: none are needed. No credential, token, or absolute user path may be
  committed; anything environment-specific is read from the environment.

## Development Workflow & Quality Gates

- **Constitution gate**: every `/speckit-plan` run MUST complete the Constitution Check
  before Phase 0 and re-check it after Phase 1. Any violation MUST appear in the plan's
  Complexity Tracking table with the simpler alternative that was rejected and why.
- **Tests**: lock acquisition, queue ordering, and release logic are business logic and MUST
  hold at least 80% statement coverage. Tests MUST be table-driven where the cases vary only
  in data, MUST follow Arrange-Act-Assert, and MUST be named as a behavioural statement
  ("releases the lock when the tracked process dies"). OS-level process control MUST be
  exercised against real short-lived child processes, not mocks.
- **Mandatory abrupt-exit test**: the suite MUST include a case that kills a lock holder
  without letting it call `/release` and asserts the next waiter is promoted.
- **Formatting**: `gofmt` output is authoritative. Code lines wrap at 100 characters;
  comments and commit message bodies wrap at 72. Files are UTF-8 with LF endings and 4-space
  indentation in Go; 2 spaces in any dashboard JavaScript.
- **Error handling**: every error is handled or returned with context. Silently discarded
  errors and bare `_ =` on a fallible call are rejected in review.
- **Commits**: Conventional Commits (`type(scope): description`), present tense, one-sentence
  summary followed by a detailed body when the change is not self-evident.
- **Simplicity review**: a change that adds an interface with one implementation, a
  configuration knob for a value that never varies, or scaffolding for an unrequested future
  need MUST be reduced before merge.

## Governance

This constitution supersedes all other development practices for this project. Where a
habit, template, or convenience conflicts with a principle here, the principle wins.

**Amendment procedure**: amendments are made by editing this file in the same change that
implements them. Each amendment MUST update the version line, MUST update the Sync Impact
Report comment at the top of this file, and MUST state the migration path for any code that
the amendment puts out of compliance. An amendment that leaves existing code non-compliant
MUST either fix that code in the same change or record the remediation as a tracked task.

**Versioning policy**: this document follows semantic versioning. MAJOR for a removed or
redefined principle or a backward-incompatible governance change; MINOR for a new principle
or materially expanded guidance; PATCH for clarifications, wording, and typo fixes that
change no obligation.

**Compliance review**: every plan passes the Constitution Check gate, and every review
verifies the change against these principles. Complexity MUST be justified in writing, not
assumed; "we might need it later" is not a justification. When no separate runtime guidance
file exists, this constitution is the authoritative guidance for agents and contributors
working in this repository.

**Version**: 1.0.0 | **Ratified**: 2026-09-13 | **Last Amended**: 2026-09-13
