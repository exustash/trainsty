---
okf_version: "0.1"
type: reference
title: "State — What Exists and Who May Touch It"
description: "The complete state of a running trainsty daemon: one Lock holder, one ordered Queue keyed by PID, and the timers around them — all in memory behind one mutex. Names every field, what reads it, and the two invariants a reviewer should check any change against."
tags: [data, state, concurrency, invariants]
timestamp: "2026-09-13"
---

# State — What Exists and Who May Touch It

> There is no schema, because nothing is written down —
> [`ddr.md`](ddr.md) → DDR-001. This file is the equivalent: the shape of the
> state a running Daemon holds, and the rules for touching it.
>
> Vocabulary is binding: **Lock**, **Queue**, **Job**, **Waiter**,
> **Registration**, **Process Group** — see
> [`../domains/ubiquitous-language.md`](../domains/ubiquitous-language.md).

## The whole of it

```text
Scheduler                      # one instance, one mutex, the only state there is
├── mu          sync.Mutex     # guards every field below, without exception
├── job         *Job           # nil when the Lock is free
└── queue       []*Waiter      # FIFO, keyed by pid — never two entries per pid
```

| Field | Holds | Read by | Written by |
| ----- | ----- | ------- | ---------- |
| `job.pid` | The Process Group leader's PID, as registered | `/status`, `/stop`, the Liveness Probe | Grant only |
| `job.repo` | A display label, nothing more (`architecture/adr.md` → OD-3) | `/status` | Grant only |
| `job.grantedAt` | When the Grant happened — the Dashboard's elapsed time | `/status` | Grant only |
| `job.release` | The one-shot that ends this Job, whichever path calls it | all five release paths | Grant only |
| `waiter.pid` | **The Queue's identity** (ADR-007) | `/status`, `/register` | `/register` |
| `waiter.repo` | Display label | `/status` | `/register` |
| `waiter.queuedAt` | Arrival time — the FIFO order, and the wait the Dashboard shows | `/status` | `/register` |
| `waiter.grant` | The channel the Grant is delivered on, closing the wait | `/register`'s handler | Grant only |

Everything else a handler needs — the `*http.Request` context, the probe's
ticker — belongs to that handler, not here. **A field that does not need to
outlive one request does not go in `Scheduler`.**

## Two invariants

Both are cheap to state, and a change that breaks either is a collision or a
deadlock — the two failures the product exists to prevent.

1. **At most one Job, and it is not in the Queue.** A Grant removes the Waiter
   from the Queue and installs it as the Job in **one critical section**. Two
   sections means a window where a second Grant can see a free Lock.
2. **One entry per PID, across the Job and the Queue together.** A re-
   registration for a PID that is already queued resumes its position; for the
   PID that is already the Job it is answered with an immediate Grant (ADR-007).
   Neither creates a second entry.

## Rules for touching it

- **One mutex, held for the shortest possible span, and never across a send.**
  Delivering a Grant while holding the mutex deadlocks the moment the receiving
  handler wants it back. Compute the decision under the lock, release it, then
  send.
- **`/status` serialises a snapshot taken under the lock**, never the live
  structs. A handler that marshals while another goroutine mutates is a data
  race that `go test -race` will find and a reviewer will not.
- **Release is idempotent.** Five paths can call it for one Job and at least two
  routinely race — a `/release` from the Runner's `trap` and the Registration
  dropping as its terminal closes. A second call for the same Job is a no-op, and
  **must never Release its successor**: compare identity, not just "is there a
  Job".
- **Never signal a process while holding the mutex.** `syscall.Kill` on a large
  process group is not instant, and `/status` must stay answerable while a Stop
  is in flight — the Dashboard is how the developer watches it happen.
- **Timers are owned by the Job, not by the Scheduler.** The Liveness Probe
  starts on Grant and stops on Release; a probe outliving its Job is how a
  healthy successor gets released out from under itself.

## What a reviewer checks

- Does every new field belong to the Job's lifetime, or is it request-scoped?
- Is every read and write inside the critical section, including the ones in
  `/status`?
- Does the change add a sixth way to Release? If so, is it idempotent, and does
  it appear in `CLAUDE.md` → The Release Paths?
- Does anything block — a channel send, a `Kill`, a `Flush` — while the mutex is
  held?
