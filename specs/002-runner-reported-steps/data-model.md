# Data Model: Runner-reported Steps

**Feature**: [spec.md](spec.md) · **Date**: 2026-11-05

All in memory, in `scheduler/`, behind the one mutex. Nothing is persisted
(DDR-001).

## Step (new)

| Field | Type | Rule |
| --- | --- | --- |
| `Name` | string | 1–128 bytes, no U+0000–U+001F or U+007F, compared exactly (no trimming). Unique among the Job's running Steps |
| `StartedAt` | time | Taken from the Scheduler's injected clock |

- **Owner:** exactly one Job. A Job has 0–16 running Steps.
- **Lifecycle:** started → running → ended, or gone with its Job. An ended Step is
  removed; there is no history.
- **Restart:** starting a running name removes it and appends it with a fresh
  `StartedAt`. It is allowed at the cap, because the count does not change.
- **Re-registration** by the Job's own pid (ADR-007) leaves the Job, and so its
  Steps, untouched.

## Job (changed)

Gains `steps []Step`, unexported, oldest first. **Mutable, so it is read and
written only under `s.mu`**, unlike `PID`, `Repo` and `GrantedAt`, which stay
immutable after Grant and may be read through `Current()`.

## Scheduler operations (new)

| Operation | Effect | Result |
| --- | --- | --- |
| `StartStep(pid, name)` | If `pid == job.PID`: restart a running name (allowed at the cap), or append a new one (refused at 16) | `recorded`, or `ErrTooManySteps` |
| `EndStep(pid, name)` | If `pid == job.PID` and the name is running: remove | `recorded` (false when nothing changed) |

`ErrTooManySteps` is a sentinel, matched with `errors.Is`. Name validation happens
in `httpapi/` before either call, as `repo` validation does.

## Snapshot (changed)

```text
JobView
  pid, repo, grantedAt, elapsedSeconds   (unchanged)
  steps: [StepView]                      (new, never null, oldest first)

StepView
  name            string
  startedAt       RFC 3339 time
  elapsedSeconds  int, computed under the mutex from the same "now" as the Job's
```

## Invariants

1. A Step belongs to the Job whose pid announced it, and **never** to any other Job.
2. No Job has more than 16 running Steps.
3. When a Job ends, by any of the five release paths, its Steps end with it. No
   release path changes.
4. Steps never affect a Grant, a Release or the Queue.
