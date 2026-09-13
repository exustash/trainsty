---
okf_version: "0.1"
type: domain
title: "Ubiquitous Language"
description: "Canonical vocabulary for trainsty — sixteen terms with lifecycles, plus the synonyms now banned in code and conversation. Separates the two things the requirements note calls an orphan, and replaces its wrapper script with Runner."
tags: [domains, ubiquitous-language, glossary]
timestamp: "2026-09-13"
---

# Ubiquitous Language

> The binding rules for using this glossary live in `CLAUDE.md` → **Ubiquitous
> Language**. This file is the term list itself.

## How to add a term

A term change is a change to this file **first**, then to the code, in the same
commit. Never coin a term ad hoc in code.

| Field | Content |
| ----- | ------- |
| Term | The canonical word. One concept, one word. |
| Definition | What it is, in the developer's words — not the implementation's. |
| Lifecycle | The states it moves through, if any. |
| Relationships | What it belongs to, contains, or references, with cardinality. |
| Not to be called | Words banned **as a name for this concept** — in code, in the dashboard, and in conversation. **Not banned as English**: a definition may use one to explain the concept, and another term's row may use it for something else. |

## Terms

### The lock domain `[core]`

| Term | Definition | Lifecycle | Relationships | Not to be called |
| ---- | ---------- | --------- | ------------- | ---------------- |
| Lock | The single exclusive right to run end-to-end tests on this machine. There is exactly one, and it is held by at most one Job. | free ⇄ held | Held by zero or one Job. Never two — see `architecture/adr.md` → ADR-006. | mutex, semaphore *(it is one of depth 1, and the word invites a depth above 1)*, token, ticket, slot, turn |
| Daemon | The long-lived trainsty process that owns the Lock and the Queue and nothing else. | started → serving → shut down | Exactly one per machine, identified by holding port 45678. Owns one Lock, one Queue. | server, service, agent, broker, coordinator |
| Queue | The ordered list of Waiters, strictly first-in-first-out. | empty ⇄ occupied | Zero or more Waiters. Ordered by the time their Registration arrived, **not** re-ordered by a reconnect (ADR-007). | backlog, pool, pending list, buffer |
| Registration | One Runner's request for the Lock, held open as a Server-Sent Events stream for as long as it waits. | opened → queued → granted, or dropped | Exactly one per Waiter and per Job. Carries a `pid` and a `repo`. **The open stream is the primary liveness signal** — ADR-008. | connection, request, session, subscription, handshake |
| Waiter | A Runner whose Registration is in the Queue and which does not hold the Lock. | queued → granted *(becoming the Job)*, or dropped | Zero or more per Queue. Becomes the Job on Grant. | pending job, queued job, client, blocked run |
| Job | The Waiter that holds the Lock and is running tests. **At most one exists.** | granted → running → released | Exactly zero or one per Daemon. Has one Process Group and one Registration. | task, run, execution, build, active test, current |
| Grant | The moment the Lock passes to the head of the Queue, making that Waiter the Job. The Daemon's only outbound message on a Registration. | — | One per Job. | acquire, unlock, green light, go, dequeue |
| Release | The Lock returning to nobody, and the next Grant that follows it. **Five causes, and every one of them must work** — see `CLAUDE.md` → The Release Paths. | — | One per Job, exactly once. Idempotent: a second Release for the same Job is a no-op, never a Release of its successor. | unlock, finish, complete, done, close |
| Runner | The local CI script that owns test execution: it registers, waits for the Grant, runs the suite in the developer's terminal, and releases in a `trap`. **Not part of trainsty** — it lives in each repository being tested. | — | One per Registration. Leads one Process Group. | wrapper, wrapper script *(the requirements note's word — see below)*, client, harness, agent, hook |
| Process Group | The operating-system grouping holding the Runner and every process it started, including headless browsers. Identified by its leader's PID, which is what a Registration's `pid` must be. | — | Exactly one per Job. **The unit of termination** — a Stop signals the group, never the bare PID (ADR-002). | process tree, children, pgid *(the implementation word — it stays inside the process package)* |

### Operator actions `[cli, dashboard]`

| Term | Definition | Lifecycle | Relationships | Not to be called |
| ---- | ---------- | --------- | ------------- | ---------------- |
| Stop | A developer forcibly ending the Job from the Dashboard: signal its Process Group, then Release. **Not a Release** — it is one of the five causes of one. | — | Targets exactly one Job. A Stop with no Job is a no-op, never an error the UI shows as a failure. | kill, abort, cancel, terminate *(the mechanism, not the action)* |
| Shutdown | The Daemon exiting on the `trainsty stop` command, dropping the Lock and closing every Registration first. | — | Ends one Daemon. Releases at most one Job — **it does not Stop it**: the tests keep running, unsupervised, and the next Daemon knows nothing about them. That asymmetry is deliberate and is ADR-009. | quit, exit, halt, kill the daemon |
| Dashboard | The web page at `http://localhost:45678` that shows the Job and the Queue, and carries the Stop control. It polls `/status`; nothing is pushed to it (ADR-005). | — | One per Daemon. Reads state; the only state it may change is by Stop. | UI, admin, panel, monitor, console |
| Liveness Probe | The Daemon's periodic check that the Job's Process Group leader still exists. The **backstop** to a dropped Registration, not the primary signal. | — | One per Job, every 3–5 seconds. `ESRCH` triggers a Release. | health check, heartbeat *(the client sends nothing — that is the point)*, ping, watchdog |

### The two orphans

The requirements note calls both of these "orphans", and they need different
words because they need different mechanisms.

| Term | Definition | Relationships | Not to be called |
| ---- | ---------- | ------------- | ---------------- |
| Orphaned Lock | A Lock still held by a Job whose Runner is gone — the `Ctrl+C` case. **This is what the note's *Automated Orphan Cleanup* means.** | Cleared by a dropped Registration or the Liveness Probe. | stale lock, deadlock *(a deadlock is the consequence, not the state)*, zombie lock |
| Orphaned Process | A process that outlives the Runner that started it — typically a headless browser. **Killing the bare PID is what creates these.** | Prevented by signalling the Process Group (ADR-002). | zombie *(that is a specific, different POSIX state — a reaped-pending child)*, leaked process, stray |

> **A `Ctrl+C` produces the first and, if the Stop path is wrong, the second.**
> They are one symptom with two causes, which is why the glossary splits them
> before any code exists to confuse them.

## Words this project does not use

- **"CI"** on its own, for what trainsty does. It schedules; it does not build,
  test, or report. The Runner is the CI. `local-ci` survives only as the
  requirements note's filename.
- **"wrapper" / "wrapper script"** for the Runner. The requirements note uses it
  throughout, and the note is the source document, so the obligation runs the
  other way: **the vault should move to `Runner`** so the two agree. Until it
  does, cite the note's wording and use `Runner` in code. Same shape as the
  binary name below.
- **"e2e-scheduler"** as the command name. The note's CLI section names it
  `e2e-scheduler`; the project is **trainsty**, so the command is `trainsty`
  (`architecture/adr.md` → ADR-010). The note predates the name.
- **"server"** for the Daemon, even though it serves HTTP. The Daemon is the
  thing that holds a Lock; HTTP is how it is reached.
