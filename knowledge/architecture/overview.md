---
okf_version: "0.1"
type: architecture-note
title: "Architecture — Overview"
description: "How trainsty is shaped: one daemon holding one lock, eight packages with a strict dependency direction, and the three boundaries that matter — the API the Runner speaks, the syscall seam, and the mutex. Includes the full lifecycle of one job and an index of the decisions already recorded."
tags: [architecture, overview, go, scheduler, lifecycle]
timestamp: "2026-09-14"
---

# Architecture — Overview

> **The module boundaries below are built** — `v1.0.0`, eight packages — and
> `CLAUDE.md` → **Repository Architecture** is the authoritative statement of them.
> This file is the *why*, and the sequence diagram a listing cannot show;
> [structure.md](structure.md) is the file-by-file map.

## The system in one paragraph

A developer's local CI script — the **Runner** — asks the **Daemon** for the
**Lock** and blocks. The Daemon grants it to one Runner at a time, in arrival
order. The Runner then runs its own tests in its own terminal, and releases the
Lock when it is done — or the Daemon releases it for them when they die, close the
terminal, or are stopped from the **Dashboard**. That is the whole product. The
Daemon never sees a test.

## The dependency direction

```text
main.go  ──►  httpapi/  ──►  scheduler/        (pure: Lock + Queue)
                  │              ▲
                  └──►  process/ ┘             (syscall: liveness, group kill)
                  └──►  dashboard/             (go:embed)
```

**The arrows never reverse.** `scheduler/` imports neither `net/http` nor
`syscall`, which is what lets the part that must not be wrong be tested with no
server and no child processes. `conventions/go.md` → *Package layout* carries the
rules; this is why they exist.

## The three boundaries

| Boundary | What is on the other side | Enforced by |
| -------- | ------------------------- | ----------- |
| **The API** | Runner scripts in repositories this project has never seen, unversioned and unupgradable | Validation on every parameter; a fixed five-endpoint surface the constitution makes a MAJOR amendment to change. [`../conventions/api.md`](../conventions/api.md) |
| **The syscall seam** | The operating system, and process groups trainsty did not create | Everything in `process/`, exposing intent (`KillGroup`) rather than mechanism. A non-leader PID is refused at the boundary — ADR-002 |
| **The mutex** | Every concurrent handler | One mutex, one critical section per decision, never held across a send or a signal. [`../data/state.md`](../data/state.md) |

**Only the first is a security boundary**, and it is a weak one: loopback keeps
other machines out and keeps no local process out. The API can kill process groups,
which is why `CLAUDE.md` → Security states a floor that ships regardless of how
`OD-1` is settled.

## The life of one Job

```text
Runner                          Daemon                        Dashboard
  │                               │                               │
  ├─ setsid, become leader        │                               │
  ├─ GET /register?pid&repo ─────►│ validate: pid == getpgid(pid)  │
  │                               ├─ queued (FIFO, keyed by pid)   │
  │   …blocked on the stream…     │                               ├─ GET /status (2s)
  │                               ├─ head of queue, Lock free      │
  │◄──── event: grant ────────────┤ ─ becomes the Job              │
  │                               ├─ Liveness Probe starts (3–5s)  │
  ├─ run the suite (own tty) ─────┤                               ├─ shows Job + elapsed
  │                               │                               │
  │   …one of five endings…       │                               │
  ├─ POST /release (trap) ───────►│ ─ Release, idempotent          │
  │   or terminal closes ────────►│ ─ ctx.Done() → Release         │
  │   or SIGKILL ─────────────────┤ ─ probe ESRCH → Release        │
  │   or  ◄─ SIGKILL to -pgid ────┤ ◄──── POST /stop ──────────────┤
  │   or                          │ ◄──── POST /shutdown (CLI): Release, do NOT kill
  │                               ├─ Grant to the next Waiter      │
```

**The five endings are the design.** Four of them are the Runner failing to behave,
and all four must work — `CLAUDE.md` → The Release Paths, and
`conventions/testing.md` requires a test for each.

## What is deliberately absent

Naming these keeps them from arriving as accidents:

- **No persistence.** No database, no state file, no PID file. `data/ddr.md` →
  DDR-001, and the port bind is the single-instance mechanism (ADR-011).
- **No configuration.** No config file, no `--port`, no concurrency setting. The
  only inputs are two query parameters.
- **No dependencies.** `go.mod` is expected to require nothing.
- **No logs of the tests.** The Daemon never sees test output (ADR-003), so a
  diagnosis reads the machine rather than a log — `playbooks/stuck-lock-recovery.md`.
- **No push to the Dashboard.** It polls (ADR-005).
- **No Windows support.** Not deferred — out of scope, because the product *is*
  process-group semantics (ADR-002).

**One thing that stopped being absent.** The Runner used to be entirely outside the
product; since **ADR-012** it ships as `trainsty wrap`. That is the single place in
the binary that spawns a process, and it is a **client** — the Daemon still spawns
nothing. A reviewer who finds `exec.Command` outside the `wrap` path should treat it
as a defect.

## Already decided

| Decision | Where it is recorded |
| -------- | -------------------- |
| Go, one static binary, stdlib only | [adr.md](adr.md) → ADR-001 |
| Unix only; terminate the process group | [adr.md](adr.md) → ADR-002 |
| The Daemon is a traffic light; the Runner executes | [adr.md](adr.md) → ADR-003 |
| SSE for the queue wait | [adr.md](adr.md) → ADR-004 |
| The Dashboard polls; nothing is pushed | [adr.md](adr.md) → ADR-005 |
| One Lock holder, strict FIFO | [adr.md](adr.md) → ADR-006 |
| A reconnect keeps its queue position | [adr.md](adr.md) → ADR-007 |
| The Registration is the primary liveness signal | [adr.md](adr.md) → ADR-008 |
| Shutdown releases the Job without killing it | [adr.md](adr.md) → ADR-009 |
| The command is `trainsty` | [adr.md](adr.md) → ADR-010 |
| Port 45678, no fallback; the bind is the instance lock | [adr.md](adr.md) → ADR-011 |
| Nothing is persisted, except an append-only log never read back | [../data/ddr.md](../data/ddr.md) → DDR-001, DDR-002 |
| The Runner ships, as `trainsty wrap` | [adr.md](adr.md) → ADR-012 |
| No secret gates termination; the floor suffices | [../security/sdr.md](../security/sdr.md) → SDR-001 |
| Module boundaries | `CLAUDE.md` → Repository Architecture |

## Open decisions

Each needs a record before the corresponding code is written. **One is still open**,
in [adr.md](adr.md) → *Open decisions*:

- **OD-3 — whether `repo` ever means anything.** Recorded so it cannot become a
  back door to a concurrency above 1.

**OD-4 closed on 2026-09-14** — ADR-013: tagged release archives for four Unix
targets, with `go install` alongside. Principle I is what ranked them: it forbids
installation *requiring* a toolchain or a package manager, so the archive is the
channel and the others may only accompany it.

**OD-1 and OD-2 closed on 2026-09-13** — SDR-001 and DDR-002 respectively, both forced
by `specs/001-serialize-e2e-runs/spec.md`'s clarification round.
