---
okf_version: "0.1"
type: playbook
title: "Recovering a Stuck Lock"
description: "Diagnosis order for a Lock that will not release: confirm the daemon, read the status, check whether the holder still exists, then the three real causes — a non-leader PID, a buffered SSE stream, and a daemon restarted across a running job. Ends with the restart of last resort and what it costs."
tags: [playbook, operations, debugging, lock, deadlock]
timestamp: "2026-09-13"
---

# Recovering a Stuck Lock

> A Lock that will not release is the one failure that makes trainsty worse than
> not having it: every suite on the machine waits. **The design has five release
> paths precisely so this should not happen** (`CLAUDE.md` → The Release Paths),
> so a real occurrence is evidence of a defect and ends with an entry in
> [`../ERRORS.md`](../ERRORS.md).
>
> **The Daemon holds no logs of the tests** (ADR-003) and writes nothing to disk
> (DDR-001). Every diagnosis below therefore reads the *machine*, not a log file —
> and `architecture/adr.md` → OD-2 is the open decision that would change that.

## Work in this order

Each step rules something out. Skipping to the restart at the end destroys the
evidence that would have prevented the next occurrence.

### 1. Is the Daemon even running?

```sh
trainsty status
lsof -nP -iTCP:45678
```

- **Nothing on 45678** — there is no Lock to be stuck. A Runner reporting a wait
  is a Runner that cannot reach the Daemon; re-read `wrapper-integration.md` step 5.
- **Something on 45678 that is not trainsty** — the port is taken by another
  process. `trainsty start` refused to bind and said so (ADR-011).

### 2. What does the Daemon think is happening?

```sh
curl -s http://localhost:45678/status | python3 -m json.tool
```

Read two things: the Job's `pid`, and how long ago it was granted. A Job granted
an hour ago is the case in hand; one granted 30 seconds ago is a suite running.

### 3. Does the holder still exist?

```sh
ps -o pid,pgid,etime,command -p <job pid>
```

| Result | Meaning |
| ------ | ------- |
| The process exists, `PID == PGID`, and it is a test run | **Not stuck.** A long suite. Stop it from the Dashboard if it needs to die. |
| The process exists and `PID != PGID` | **The Runner is not a group leader** — see cause A. |
| **No such process** | The Daemon is holding a Lock for a dead Job. Both release paths have failed — see cause B. |

## The three causes, in the order they actually occur

### A. The Runner registered a non-leader PID

The Daemon refuses these (`conventions/api.md`), so this appears when the refusal
is missing or was bypassed. The Stop then signals the wrong group — nothing, or
the developer's shell.

**Confirm:** `PID != PGID` in step 3.
**Fix:** the Runner, not the Daemon. `wrapper-integration.md` step 1.
**Then:** it is an `ERRORS.md` entry against the Daemon too — a refused
Registration should have made this unreachable.

### B. The Registration dropped and nothing noticed

The stream is the primary liveness signal and the probe is the backstop
(ADR-008), so a Lock held for a dead PID means **both** failed. Two known shapes:

- **A buffered or proxied stream.** The Daemon still holds an open connection it
  believes is a live Waiter. Check: `lsof -nP -iTCP:45678 | wc -l` — more
  connections than Waiters plus browser tabs is the signal.
- **The probe is not running, or not releasing.** A probe that outlived its Job,
  or one treating `EPERM` as *alive* for a PID it cannot signal.

**Confirm:** step 3 says no such process, and `/status` still names it.
**Fix:** a defect in `process/` or the probe's lifecycle. Reproduce it with the
mandatory test in `conventions/testing.md` before changing anything.

### C. The Daemon was restarted across a running Job

`trainsty stop` releases the Lock **without killing the Job** (ADR-009), and
nothing is persisted (DDR-001), so the new Daemon knows nothing about a suite that
is still running. The symptom is the opposite of a stuck lock — **two suites
running at once** — and it is worth knowing here because it is what a developer
causes while trying to fix one.

**Confirm:** two test processes with different PGIDs, one of them not in
`/status`.
**Fix:** nothing to fix — it is documented behaviour. Wait for the untracked suite
to finish, or kill its group by hand:
`kill -TERM -<its pgid>`.

## The restart of last resort

Only after steps 1–3, and only with the evidence recorded:

```sh
trainsty stop        # warns and confirms if it thinks a Job is active
trainsty start
```

**What it costs:** any suite actually running keeps running, unsupervised, and is
now invisible to the scheduler (cause C above). So **check for live test processes
first** — `pgrep -f 'playwright|cypress|chromedriver'` — and decide whether to let
them finish before restarting.

**What it does not cost:** nothing is lost from disk, because nothing is on disk.
Every Waiter's Runner will see its stream close and can re-register.

## Afterwards

A genuine stuck Lock is a defect in the release paths, so:

1. Append an entry to [`../ERRORS.md`](../ERRORS.md) in `RULES.md` §8.1's shape.
   **The `Prevention:` line is the point** — which of the five paths failed, and
   which test would have caught it.
2. If no test covers it, that test is the fix. `conventions/testing.md` → *Every
   release path has a test*.
