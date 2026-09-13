---
okf_version: "0.1"
type: decision-record
title: "Data Decision Records (DDR)"
description: "Log of data-layer decisions for trainsty. DDR-001 records that nothing is persisted and why a stale on-disk lock would be worse than losing the queue; DDR-002 holds the log destination open as the one thing trainsty might write to disk. The open question is whether a log file is worth breaking DDR-001's property for."
tags: [data, ddr, decisions, state]
timestamp: "2026-09-13"
---

# Data Decision Records (DDR)

> `RULES.md` §8.2 requires a DDR entry after any change to what trainsty writes
> or reads outside its own memory — **including the decisions where writing
> nothing was chosen**, with the reasoning. On this project that is nearly all of
> them, which is why the file is short and the one open row matters.

## Template

```markdown
## DDR-00N — <Decision, stated as the outcome>

- **Status:** proposed | accepted | superseded by DDR-00M
- **Date:** YYYY-MM-DD
- **Context:** what data, what volume, what lifetime, what reads it.
- **Decision:** what we store, where, in what shape.
- **At rest:** nothing | a file, and what protects it.
- **Lifetime:** how long it lives, and what removes it.
- **Consequences:** what this makes easy, what it makes hard.
```

## Why a data decision here is unusual

Most projects ask *where does this live*. On trainsty the question is almost
always **whether anything may live outside memory at all**, because the product's
whole value is that a Lock cannot outlive the thing holding it. A file that
survives a crash is a stale lock, and a stale lock is the deadlock this tool
exists to prevent in other people's tooling.

## Open decisions

| # | Decision | Blocks |
| - | -------- | ------ |
| — | **Whether the Daemon writes a log file, and where** — DDR-002. It is the only candidate for on-disk state, and the case for it is real: a detached daemon's stderr goes nowhere, so today a crash leaves no evidence at all | `trainsty start`'s detach, and every diagnosis in `playbooks/stuck-lock-recovery.md`. Tracked as `architecture/adr.md` → OD-2 |

## Decisions

## DDR-002 — The log destination is undecided, and stderr-to-nowhere is the current behaviour

- **Status:** proposed
- **Date:** 2026-09-13
- **Context:** `trainsty start` detaches from the terminal, so anything the
  Daemon writes to stderr is discarded unless it is redirected. The Daemon's
  interesting events are exactly the ones nobody is watching: a Release with no
  matching `/release`, a probe returning `EPERM`, a refused Registration.
- **Decision:** **Not made.** Until it is, `trainsty start` must say where output
  goes — including "nowhere" — rather than leaving a developer to discover it
  during an incident.
- **At rest:** nothing today.
- **Lifetime:** n/a.
- **Consequences:**
  - **A crash currently leaves no evidence**, which makes the first real
    stuck-lock report much harder than it needs to be.
  - A log file does **not** violate DDR-001: it is a record of what happened,
    never a source the Daemon reads back. That distinction is the whole test —
    **anything trainsty reads at startup is a stale lock waiting to happen;
    anything it only ever appends to is not.**
  - The candidates are `~/Library/Logs/trainsty.log` on Darwin and
    `$XDG_STATE_HOME/trainsty/trainsty.log` on Linux, which is two paths and a
    platform branch in a project that has otherwise avoided both.

## DDR-001 — Nothing is persisted; the Lock and the Queue live in memory only

- **Status:** accepted
- **Date:** 2026-09-13
- **Context:** The state is one Lock holder and one ordered set of Waiters, each
  of them a live process on this machine. The largest it gets is a handful of
  entries. The question is not size — it is what a restart should mean.
- **Decision:** All state is **in-memory, guarded by one mutex**, and dies with
  the Daemon. There is **no database, no state file, and no PID file** — the port
  bind is the single-instance mechanism (ADR-011).
- **At rest:** nothing.
- **Lifetime:** the Daemon's process lifetime.
- **Consequences:**
  - **A restart means "nobody holds the Lock"**, which is the only answer that
    cannot be wrong for long: every Registration is gone, so every Waiter has to
    come back and say so.
  - **Persisting the Queue would be actively harmful.** A restored holder is a
    PID that may no longer exist, may have been reused, and has no Registration
    to drop — the exact orphaned lock the Liveness Probe was added to clear, made
    durable. A stale lock file inherited from a previous boot is unrecoverable
    without a manual command, and needing that command is the product failing.
  - **The cost is real and accepted:** `trainsty stop` while a Job runs loses
    track of it (ADR-009), and a Daemon crash mid-suite leaves the suite running
    with nobody scheduling around it. Both are visible — the next Registration is
    granted immediately — rather than silent.
  - There is no migration concern, no schema, and no `data/schema.md`. What state
    exists and who may touch it is [`state.md`](state.md).
