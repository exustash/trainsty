# Plan: Runner-reported Steps on the Dashboard

**Status:** deferred, 2026-11-04. Not started. Pick up when `teneerai.web` is idle
(see *Before starting*).

## Goal

The Dashboard's Job row shows **the Steps running now** (name and elapsed time for
each), like a CI provider's step list but with **no history**: a Step disappears when
it ends. Today the row shows only repo, pid and elapsed time:

```text
Holding the lock
Repository             Process    Running now                  Elapsed
teneerai.web-dbchecks  pid 96032  Unit tests (coverage) 4m12s  6m03s   Stop
                                  Scanners: osv          12s
```

## Why the Runner must report Steps

The Daemon never sees test output (ADR-003), and `wrap` hands the suite the
terminal directly (FR-037). Parsing output for step markers would need a pipe, which
turns off colour and TTY-aware reporters. So the Runner **announces** each Step's
start and end; nothing is inferred.

## Findings that shape the design

- **`teneerai.web`** wraps its whole CI: `trainsty wrap -- scripts/ci-local.sh`.
  Jobs run **in parallel by default** (`dispatch_parallel`; `--sequential` turns it
  off), so several Steps can be running at once. The CI job also runs its own
  sequence inside it: lint, typecheck, test:coverage, coverage ratchet, build.
- **`teneerai.desk`** wraps only `npm run test:e2e`, so its build and lint run
  outside the Lock. Steps inside its Lock would come from WebdriverIO hooks. Out of
  scope here.
- **Naming clash:** `ci-local.sh` calls its units "jobs". In trainsty's vocabulary
  they are **Steps**, and the **Job** is the whole locked run. The glossary entry
  must say so.

## Decisions (defaults, confirm when picking up)

1. Show **every running Step**, not one "current step", because jobs run in
   parallel.
2. `teneerai.web` reports at the **job level**. Marking the CI job's internal
   sequence is optional and comes after.
3. trainsty ships first (tagged release); the `teneerai.web` script change follows
   as a separate commit in that repository.

## Design

| Area | Change |
| --- | --- |
| `scheduler/` | `Job` keeps its running Steps: name → start time, **capped at 16** (no unbounded buffer). New methods `StartStep(pid, name)` and `EndStep(pid, name)` act only when `pid == job.PID`, so a late call from a finished run cannot label its successor. Release drops the Job and its Steps with it. Starting a name already running restarts its timer. |
| `httpapi/` | `POST /step?pid=N&name=…&state=start\|end` behind `guardMutating`. `pid` **required**, with no "whatever holds the Lock" fallback. `name` validated like `repo` (≤128 chars, no control characters). Stale or unknown pid, or ending a Step that is not running: `200 {"recorded":false}`. Bad input: `400` with a stable code (`invalid_step_name`, `invalid_step_state`, `too_many_steps`). |
| `/status` | `job.steps: [{name, startedAt, elapsedSeconds}]`, oldest first, never `null`. Additive, so existing clients keep working. |
| `runner/wrap.go` | After the Grant, run the suite with `TRAINSTY_PID=<wrap's pid>` added to its environment. Do **not** set it on the unserialized paths (no daemon, refused, lost stream), which makes reporting a no-op there. |
| CLI | New `trainsty step start\|end <name>`: reads `TRAINSTY_PID` and POSTs with a 1 s timeout. Always exits `0` (clarified 2026-11-05). Without `TRAINSTY_PID` it is silent. When not recorded (no daemon, refused) it prints one line on stderr. |
| `dashboard/index.html` | A "Running now" cell on the Job row: one line per Step with its own ticking elapsed time. `textContent` only. |
| `trainsty status` | Optional: print the running Steps. |

Mutex discipline: Step updates are a map write under the one mutex; no channel send,
syscall or flush while holding it.

## Tasks (trainsty)

Tests come before each fix or feature and are watched failing first (`RULES.md` §3.1).

1. **Glossary first:** add **Step** to `knowledge/domains/ubiquitous-language.md`.
   Banned names: stage, phase, task, and job (the ci-local word).
2. `scheduler/`: `StartStep` / `EndStep` and the Snapshot field. Table-driven tests:
   - records a Step for the Job's pid;
   - ignores a Waiter's pid, an unknown pid, and a stale pid after the Lock passes
     to a successor;
   - `EndStep` removes; ending an unknown name is a no-op;
   - the cap refuses the 17th Step;
   - Release clears the Steps;
   - elapsed time comes from the injected clock.
   Keep `scheduler/` coverage ≥80%.
3. `httpapi/`: the `/step` handler. Tests:
   - `GET` → 405; missing JSON content type → refused; foreign `Origin` → 403;
   - missing pid, bad name and bad state → 400 with their codes;
   - a no-op returns 200;
   - `/status` shows `steps` as `[]` and populated.
4. `runner/`: export `TRAINSTY_PID` only when granted. Test: the child sees it when
   granted, and does not see it with no daemon reachable.
5. CLI `step` subcommand in `main.go` dispatch (logic in `daemonctl/` or `runner/`)
   and a `help` line. Tests: no-op without `TRAINSTY_PID`; posts with the right pid;
   exit code with no daemon.
6. Dashboard cell and ticking per-step elapsed time. Extend `dashboard/embed_test.go`
   if it asserts on markup.
7. Acceptance (`--everything`): `wrap` a script that starts and ends two Steps;
   assert `/status` shows them while running and drops them at the end.

## Docs in the same change

- `.specify/memory/constitution.md`: **MINOR amendment 1.0.0 → 1.1.0** (new
  endpoint), version line and Sync Impact Report.
- `knowledge/conventions/api.md`: endpoint table and a `/step` section.
- `CLAUDE.md`: the CLI table goes from seven to eight subcommands.
- `knowledge/data/state.md` and a new **DDR**: the Daemon now holds Step labels, in
  memory only, and still writes nothing.
- `knowledge/architecture/adr.md`: **ADR-014**, the Runner reports Steps. Rejected
  alternatives: parsing output (breaks the TTY, ADR-003); pushing to the Dashboard
  (ADR-005); a step file the Daemon reads (DDR-001/002); a Step history (more state,
  not asked for).
- `knowledge/security/sdr.md`: a note under SDR-001 that `/step` only stores a
  label. The worst a same-user process can do is mislabel, so the no-token
  reasoning holds.
- `knowledge/product/decisions.md`: register row. The requirements note does not
  mention Steps; **add the requirement to the Obsidian vault first**, then re-copy
  the mirror.
- `README.md`: a short "Reporting Steps" section with the `curl` form for
  hand-written Runners.

## Client change (`teneerai.web/scripts/ci-local.sh`, after the trainsty release)

```bash
trainsty_step() { # start|end label — a no-op outside `trainsty wrap`
  [ -n "${TRAINSTY_PID:-}" ] || return 0
  trainsty step "$1" "$2" 2>/dev/null || true   # a progress label never fails CI
}
```

- `dispatch_serial`: `trainsty_step start "${JOB_LABEL[$i]}"` before
  `"${JOB_FUNC[$i]}"` and `trainsty_step end …` after, keeping `$?` for
  `record_result`.
- `dispatch_parallel`: the same, inside the `( … ) &` subshell.
- Optional: mark the CI job's internal sequence (`info "npm run typecheck" && …`)
  with a helper that starts and ends around each command.
- Compatibility: on an older Daemon, `/step` is a 404 that the helper ignores. The
  `teneerai.web-dbchecks` clone needs the same change, or a rebase onto it.

Hand-written Runner equivalent:

```bash
curl -X POST -H 'Content-Type: application/json' \
  "http://127.0.0.1:45678/step?pid=$$&name=Build&state=start"
```

## Before starting

- [ ] `trainsty status` shows no `teneerai.web*` Job running.
- [ ] Decisions 1–3 confirmed.
- [ ] Requirement added to the vault note.
- [ ] Gates green: `scripts/ci-local.sh --everything`. The acceptance suite binds
      port 45678, so it needs the real Daemon stopped (`trainsty stop`) with no
      Job active.
- [ ] Afterwards: cut a release (an RULES.md §1.3 approval step), then change the
      client scripts.
