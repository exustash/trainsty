# Phase 1 Data Model: Serialize Local E2E Runs Behind a Single Lock

**Feature**: [spec.md](spec.md) · **Plan**: [plan.md](plan.md) · **Date**: 2026-09-13

There is no database and no schema — DDR-001. This document is the equivalent: the
entities, the one structure that holds them, the invariants, and the state machine.

Vocabulary is binding:
[`../../knowledge/domains/ubiquitous-language.md`](../../knowledge/domains/ubiquitous-language.md).
The standing rules for touching this state are
[`../../knowledge/data/state.md`](../../knowledge/data/state.md).

## Entities

### Scheduler

The single owner of all mutable state. One per process.

| Field | Type | Meaning |
| ----- | ---- | ------- |
| `mu` | `sync.Mutex` | Guards `job` and `queue`. **The only mutex in the product** (Principle V) |
| `job` | `*Job` | The Lock holder. `nil` ⇔ the Lock is free |
| `queue` | `[]*Waiter` | Waiters in arrival order. Index 0 is granted next |
| `log` | `*log.Logger` | Write-only sink (DDR-002). Not state — never read back |

### Job

| Field | Type | Validation | Meaning |
| ----- | ---- | ---------- | ------- |
| `PID` | `int` | > 0; exists; `Getpgid(PID) == PID` | The Process Group leader. **The unit of termination** |
| `Repo` | `string` | 1–128 bytes; no control characters or newlines | Display label only (OD-3) |
| `GrantedAt` | `time.Time` | set at Grant | Drives the Dashboard's elapsed time (FR-022) |
| `releaseOnce` | `sync.Once` | — | Makes Release idempotent (FR-008) |
| `stopProbe` | `func()` | — | Cancels this Job's Liveness Probe. **Owned by the Job, not the Scheduler** |

### Waiter

| Field | Type | Validation | Meaning |
| ----- | ---- | ---------- | ------- |
| `PID` | `int` | as `Job.PID` | **The queue's identity** (ADR-007) |
| `Repo` | `string` | as `Job.Repo` | Display label |
| `QueuedAt` | `time.Time` | set on Register | FIFO order, and the wait the Dashboard shows |
| `grant` | `chan struct{}` | buffered, cap 1 | Closed/signalled on Grant. **Cap 1 so a Grant never blocks** |

> **`grant` is capacity 1 deliberately.** A Grant is computed under the mutex and
> delivered after releasing it; a buffered channel means even a Waiter whose handler
> has not yet reached its `select` cannot make the granting path block.

### Registration

Not a stored entity — it **is** the live `/register` request. Its identity is the
Waiter's `PID`; its lifetime is the request's; and `Request.Context().Done()` firing
is the primary liveness signal (ADR-008). Nothing about it outlives the handler.

## The state machine

A PID is in exactly one of three states. There is no other.

```text
            Register (valid, lock free)
   ABSENT ──────────────────────────────────► HOLDING ──────────┐
      │                                        ▲   │            │
      │ Register (valid, lock held)            │   │ Release ×5 │
      └──────────────► WAITING ────────────────┘   └───────────►│
                          │      Grant                          │
                          │                                     ▼
                          └────────────────────────────────► ABSENT
                             stream drops while waiting
```

| From | Event | To | Notes |
| ---- | ----- | -- | ----- |
| ABSENT | `/register`, Lock free | HOLDING | Immediate Grant |
| ABSENT | `/register`, Lock held | WAITING | Appended to the tail |
| WAITING | Lock released, at head | HOLDING | Removed from queue and installed as Job **in one critical section** |
| WAITING | its stream drops | ABSENT | Removed from queue; **no Release** — it held nothing |
| WAITING | `/register` again, same PID | WAITING | **Position preserved** (ADR-007). Not a second entry |
| HOLDING | `/register` again, same PID | HOLDING | Immediate Grant. Idempotent |
| HOLDING | any of the five release paths | ABSENT | Then Grant to the new head |

**The five transitions out of HOLDING** — `CLAUDE.md` → The Release Paths. All five
converge on one `release(job *Job)` function; `releaseOnce` makes the convergence safe.

## Invariants

Both are cheap to state and a change that breaks either is a collision or a deadlock.

1. **`job != nil` ⇒ `job.PID` appears nowhere in `queue`.** Grant removes from the
   queue and installs as Job **inside one `mu.Lock()`**. Two critical sections leave a
   window where a second Grant sees a free Lock.
2. **Every PID appears at most once across `job` and `queue` combined.** Enforced on
   the `/register` path, which checks the Job first and then the queue before
   appending.

A third, about the release path rather than the shape:

3. **A Release names its Job.** `release(job)` compares `s.job == job` by pointer
   before clearing. Checking only *is there a Job* lets a late Release from a finished
   holder revoke its **successor's** Lock — plausible, wrong, and invisible until two
   suites collide.

## Concurrency rules

From [`../../knowledge/data/state.md`](../../knowledge/data/state.md), restated where
this design makes them concrete:

- **Never hold `mu` across a channel send, a `syscall.Kill`, or a `Flush`.** Compute
  the decision under the lock, release it, then act. The Grant path is the one that
  matters: it must collect the Waiter to notify under the lock and signal it after.
- **`/status` marshals a snapshot taken under the lock**, never the live structs. A
  handler marshalling while another goroutine mutates is a race `-race` finds and a
  reviewer does not.
- **A Job's Liveness Probe is owned by the Job** and stopped by its Release. A probe
  outliving its Job releases a healthy successor.
- **`syscall.Kill` runs outside the lock**, so `/status` stays answerable while a Stop
  is in flight — the Dashboard is how a developer watches it happen.

## Validation, in one place

Applied by `/register` **before** anything enters the queue (FR-012):

| Rule | Failure |
| ---- | ------- |
| `pid` parses as a positive integer | `400 invalid_pid` |
| the process exists and is signalable | `400 no_such_process` |
| `Getpgid(pid) == pid` | `400 not_group_leader` |
| `repo` is non-empty, ≤128 bytes, no control characters | `400 invalid_repo` |

`Getpgid` returning `EPERM` is **not** a validation failure — it means the PID belongs
to another user, which is a `400` *and* a loud log line (R4). No other endpoint takes
parameters, which is itself a design decision: `/stop` acting on "whatever the Job is"
is what stops a stale Dashboard tab killing the wrong suite (FR-032).
