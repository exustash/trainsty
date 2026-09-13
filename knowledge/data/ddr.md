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
> them, which is why the file is short.
>
> **Two records, and they look contradictory until you read the test.** DDR-001
> persists nothing; DDR-002 appends to a log file. Both hold because the property
> that matters is not *does trainsty write* but **does trainsty read anything back**.
> It does not, and FR-013b makes that a requirement.

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

**None.** DDR-002 closed 2026-09-13, and it was the only one.

## Decisions

## DDR-002 — The Daemon appends to a per-user log file it never reads

- **Status:** accepted
- **Date:** 2026-09-13
- **Context:** `trainsty start` detaches from the terminal, so anything the
  Daemon writes to stderr is discarded unless it is redirected. The Daemon's
  interesting events are exactly the ones nobody is watching: a Release with no
  matching `/release`, a probe returning `EPERM`, a refused Registration. Closed by
  `specs/001-serialize-e2e-runs/spec.md` → FR-013a and FR-013b, which needed an answer
  for `trainsty start`.
- **Decision:** The Daemon **appends** to a per-user file in the operating system's
  designated log location — `~/Library/Logs/trainsty.log` on Darwin,
  `${XDG_STATE_HOME:-~/.local/state}/trainsty/trainsty.log` on Linux — and
  **`trainsty start` prints that path.**
- **At rest:** plaintext, in the user's own directory, mode `0600`. It holds grants,
  releases, refusals and their causes; it holds nothing about what any suite was
  testing (`CLAUDE.md` → Logging).
- **Lifetime:** indefinite, and **unbounded — see the consequence below.** Removed by
  the user deleting it; nothing in trainsty removes it.
- **Consequences:**
  - **This does not violate DDR-001, and the reason is the whole test:** the file is
    **append-only and is never read**. FR-013b makes that a requirement rather than a
    habit. **Anything trainsty reads at startup is a stale lock waiting to happen;
    anything it only appends to is not.**
  - **A crash now leaves evidence**, which is what
    `playbooks/stuck-lock-recovery.md` previously had to work without.
  - **It costs the project's first platform branch.** Accepted: the alternative was a
    dotfile in `$HOME` that neither platform's conventions would put there.
  - **Nothing rotates it.** One line per lock event on a developer machine makes this a
    slow problem rather than no problem, and the honest position is that **rotation is
    unsolved, not handled** — revisit when a real file gets large rather than building
    a rotator for a file nobody has yet.
  - **A failure to open or write the log must not stop the Daemon.** Losing the record
    is worse than nothing; refusing to schedule because a log file is unwritable is
    worse than losing the record.

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
