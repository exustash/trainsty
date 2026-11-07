# Research: Runner-reported Steps

**Feature**: [spec.md](spec.md) · **Date**: 2026-11-05

The spec left nothing marked NEEDS CLARIFICATION. The decisions below settle what
the spec deliberately left to planning, and keep the findings from the pre-spec
sketch (commit `a3f9ee7`, `plan.md`).

## Findings from the client repositories

- **`teneerai.web`** wraps its whole CI: `trainsty wrap -- scripts/ci-local.sh`
  (its README). Stages run **in parallel by default** (`dispatch_parallel`;
  `--sequential` turns it off). Its main CI stage runs its own sequence inside it:
  lint, typecheck, test:coverage, coverage ratchet, deno check, build.
- **`teneerai.desk`** wraps only `npm run test:e2e` (`ci-local.sh`, via
  `run_step_supervised`), so its build and lint run outside the Lock. Out of scope.
- **Vocabulary clash:** `ci-local.sh` calls its stages "jobs" (`JOB_FUNC`,
  `JOB_LABEL`). In trainsty they are **Steps**; the **Job** is the whole locked run.

## R1 — The Runner announces Steps; nothing is inferred

- **Decision:** Steps reach the Daemon only through an explicit announcement.
- **Rationale:** the Daemon never sees output (ADR-003, FR-050), and `wrap` passes
  the suite the terminal itself (FR-037).
- **Alternatives rejected:**
  - `wrap` parses output for markers. That needs a pipe, which turns off colour
    and TTY-aware reporters: the workflow this product protects.
  - A step file the Daemon reads. A file trainsty reads is a stale state waiting
    to happen (DDR-001, DDR-002).
  - Pushing Steps to the Dashboard. ADR-005 settled polling; a 2 s lag is
    acceptable for a stage that lasts seconds to minutes.

## R2 — Endpoint shape: `POST /step?pid=&name=&state=start|end`

- **Decision:** one endpoint with query parameters, like `/release`. It sits behind
  the existing `guardMutating`: POST only, `Content-Type: application/json`, and a
  foreign `Origin` refused.
- **Rationale:** query parameters match every existing Runner call, so a
  hand-written Runner's `curl` line looks like its `/release` line. One endpoint
  with a `state` keeps the API surface to one addition (MINOR amendment).
- **Alternatives rejected:** a JSON body, which would make it the only endpoint
  with one; two endpoints, which doubles the amendment for no gain.

## R3 — Identity: exact pid match, no fallback, no syscall

- **Decision:** an announcement acts only if `pid == Job.PID`. No pid → `400
  invalid_pid`. There is no "whatever holds the Lock" form (FR-041).
- **No process validation.** `/register` probes the pid because the Daemon may
  later **signal** it. `/step` signals nothing; it compares against a pid that was
  already validated at Registration. So `process/` is not touched.
- **Known ceiling:** PID reuse. If the previous Job's leader died and the OS gave
  its pid to the next Runner, the previous run's leftover children could label the
  successor. This needs pid reuse inside one Lock hand-over. The cost is a wrong
  label, not a wrong Grant. Accepted.

## R4 — Where Steps live: a slice on the Job, under the one mutex

- **Decision:** `Job.steps []Step` (name, start time), at most 16, oldest first.
  Only `Scheduler` methods holding `s.mu` touch it; `Snapshot` copies it.
- **Rationale:** a slice keeps start order for free, and with at most 16 entries a
  linear scan is cheaper than a map plus a sort. The Job is the owner, so Release
  drops Steps with no extra code: no release path changes.
- **Race note:** `Current()` hands out `*Job` to handlers that read `PID` and
  `Repo` outside the mutex. Those stay immutable. `steps` is mutable and is
  **never** read outside `s.mu`; `go test -race` covers it.
- **Restart:** starting a name that is running removes it and appends it fresh, so
  its timer restarts and the order stays oldest first.

## R5 — Validation and error codes

| Input | Rule | Refusal |
| --- | --- | --- |
| `pid` | positive integer | `400 invalid_pid` (existing code) |
| `state` | `start` or `end` | `400 invalid_step_state` |
| `name` | 1–128 bytes, no control characters (same rule as `repo`) | `400 invalid_step_name` |
| 17th running Step | cap of 16 | `400 too_many_steps` |

No-ops are `200 {"recorded":false}`: a stale pid, no Job, ending an unknown name
(FR-042). Validation runs in that order. Three new codes are a contract addition,
reviewed as such (`RULES.md` §5.1).

## R6 — How a script learns the pid: `TRAINSTY_PID`

- **Decision:** after the Grant, `wrap` runs the suite with `TRAINSTY_PID=<wrap's
  own pid>` appended to its environment. It is not set on the three unserialized
  paths (no daemon, refused, lost stream).
- **Rationale:** that pid is the one registered, so identity holds with no
  configuration (FR-048). Leaving it unset off the Lock makes `trainsty step` a
  no-op there.
- **Implementation note:** `acquire` currently returns only a release function. It
  gains a `granted bool` so `Wrap` knows whether to set the variable.
- **Nested wraps:** an inner `wrap` that registers sets its own `TRAINSTY_PID` for
  its child, which is correct for that run. Pass-through patterns like
  `CAPTURE_E2E_QUEUED` inherit the outer value, which is also correct.

## R7 — The `trainsty step` command

- **Decision:** `trainsty step start|end <name>` in `daemonctl/` (an HTTP client,
  no `os/exec`), dispatched from `main.go`. It **always exits 0** (clarification
  Q2):
  - `TRAINSTY_PID` unset → silent.
  - Recorded, or a no-op → silent.
  - No answer within the named constant `stepTimeout = 1 * time.Second`, or a
    refusal → one stderr line: `trainsty: step "<name>" not recorded (<reason>)`.
- **Rationale:** a progress label is never a result a script branches on, and a
  `set -e` script must survive it (SC-012). A 1 s timeout bounds the worst case
  (SC-014).
- **Usage error:** a missing argument is a script-author bug, so it also prints one
  line and exits 0. Failing would break CI over a label.

## R8 — Logging (clarification Q3)

- Refused announcements are logged with the code and the pid, and the name is
  quoted with `%q` and truncated to 128 bytes, so no value can break a line.
- Accepted and stale announcements are not logged: up to dozens per run, in a log
  nothing rotates (DDR-002).

## R9 — Dashboard rendering

- A **"Running now"** column on the Job row: one line per Step, `name` and
  `humanSeconds(elapsedSeconds)`, built with `el()` and `textContent`.
- With no Steps the cell is empty (no placeholder), which keeps a Job without
  Steps looking as it does today (US1 scenario 3).
- Elapsed time is server-computed per poll, like the Job's own, so the page needs
  no clock (ADR-005).
- Accessibility: Steps form a list (`<ul>`) inside the cell, not line breaks, so a
  screen reader announces a count.

## R10 — Client integration (after the trainsty release, in `teneerai.web`)

```bash
trainsty_step() { # start|end label — a no-op outside `trainsty wrap`
  [ -n "${TRAINSTY_PID:-}" ] || return 0
  command -v trainsty >/dev/null 2>&1 || return 0
  trainsty step "$1" "$2"
}
```

- `dispatch_serial`: `trainsty_step start "${JOB_LABEL[$i]}"` before
  `"${JOB_FUNC[$i]}"`, and `trainsty_step end` after it, keeping `$?` for
  `record_result`.
- `dispatch_parallel`: the same, inside the `( … ) &` subshell.
- No `|| true` is needed, because the command always exits 0. Its stderr line
  stays visible on purpose.
- On an older Daemon, `/step` is a 404, which surfaces as one stderr line per
  Step: noisy but harmless. Upgrading the Daemon silences it.
- `teneerai.web-dbchecks` needs the same change, or a rebase onto it.
- Edit the script only when no run of it is active: bash reads a script as it
  executes.
