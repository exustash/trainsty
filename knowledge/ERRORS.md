---
okf_version: "0.1"
type: error-log
title: "Error Log & Pattern Prevention Guide"
description: "A living log of errors and bugs after they are resolved, in RULES.md §8.1's four-part shape. Carries the six defects found while implementing feature 001 — three in the product, three in the tooling and tests that were supposed to catch it."
tags: [knowledge, logs, errors, patterns]
timestamp: "2026-09-14"
---

# Error Log & Pattern Prevention Guide

> `RULES.md` §8.1 requires an entry after resolving **any** error or bug. The value
> of this file is the `Prevention:` line: an entry that only records what broke is a
> changelog, not a guard.

## The shape

```markdown
## [date] — [short title]

- **Symptom:** what was observed.
- **Root cause:** why it happened.
- **Fix:** what was changed.
- **Prevention:** how to avoid recurrence — name the test, not the intention.
```

Maximum three lines per section. Keep it scannable.

## What belongs here, and what does not

| Thing | Where it goes |
| ----- | ------------- |
| A bug that has been **resolved** | Here, in the shape above |
| A bug that is **known and unfixed** | A task line in `specs/**/tasks.md` |
| A failure mode the design **anticipates** but has never seen | Not here — the record that decided the mitigation |
| A decision made while fixing something | The decision record, with the entry citing it |

## Entries

## 2026-09-14 — a wrapped suite ran before it held the lock

- **Symptom:** `wrap` as designed would start the suite, then queue for the Lock —
  so two suites could run concurrently, which is the one thing the product exists to
  prevent.
- **Root cause:** `research.md` → R2 specified registering the **suite's** PID.
  Registration validates group leadership, so the PID must exist first — meaning the
  suite must be started before the Grant. The ordering is circular.
- **Fix:** `wrap` makes **itself** a group leader, registers its own PID, waits, then
  starts the suite as a child in its own group. `kill(-wrapPID)` still reaches the
  whole tree.
- **Prevention:** `TestAcceptanceTwoRunsQueueInOrder` asserts the second suite
  produces **no output** while the first holds the Lock. A design that runs first and
  queues second fails it immediately.

## 2026-09-14 — Ctrl+C left the suite running and the lock held

- **Symptom:** interrupting a wrapped run freed nothing. `TestAcceptanceInterrupt…`
  timed out with the suite alive and the next waiter never promoted.
- **Root cause:** the signal was forwarded to the direct child only. A POSIX shell
  waiting on a foreground child **does not run its trap until that child exits**, so
  `sh -c '…; sleep 300'` swallowed it entirely. Confirmed with a standalone shell
  experiment before changing any code.
- **Fix:** forward to the whole process group, once. `wrap` survives its own signal
  because `signal.Notify` has already disabled the default action, and a re-entry
  guard drops the copy it sent itself.
- **Prevention:** the acceptance test asserts on the **suite's own recorded PID**, not
  the registered one, so "wrap exited" can never be mistaken for "the suite died".
  `knowledge/conventions/go.md` → *Signals and process groups* carries the rule.

## 2026-09-14 — a registration for another user's process could hold the lock for ever

- **Symptom:** `/register?pid=1` was accepted. The Lock could then only be freed by
  the stream dropping — the probe never would, because `EPERM` correctly means alive.
- **Root cause:** validation checked existence and group leadership, but not
  **signalability**. `getpgid(2)` needs no permission, so another user's process
  passes the leader check. The contract had always said the pid must be signalable;
  the check was simply missing.
- **Fix:** `process.Alive` is called during validation, and any error refuses the
  registration.
- **Prevention:** `TestRegisterRefusesAProcessOwnedByAnotherUser`, which skips only
  when running as root — where the case is genuinely unreachable.

## 2026-09-14 — the local CI gate reported a pass for an empty run

- **Symptom:** after putting the acceptance suite behind a build tag, the gate's
  acceptance job printed `✓ acceptance suite` while executing **no tests**.
- **Root cause:** the tag was added to the files and not to the job's command, so
  `-run TestAcceptance` matched nothing — and `go test` exits 0 for no tests. The tag
  itself was needed because the suite otherwise ran **twice concurrently**, once via
  `go test ./...`, and the two fought over port 45678.
- **Fix:** the job passes `-tags e2e`, and **counts the cases first**, returning
  "did not run" rather than a pass when the count is zero.
- **Prevention:** the count guard is the test. Verified by removing the tag from the
  `-list` call and confirming the job reports *refusing to report a pass for an empty
  run*.

## 2026-09-14 — the acceptance harness raced on its own output buffer

- **Symptom:** `-race` reported a data race inside the acceptance tests, between
  `os/exec`'s output copier and the test's polling goroutine.
- **Root cause:** a plain `strings.Builder` used as `cmd.Stdout` and read
  concurrently by `waitFor`.
- **Fix:** a mutex-protected `syncBuffer`.
- **Prevention:** `-race` is already a blocking gate, and this is the entry that
  records why it is one for **test code** too — the race was in the harness, not the
  product.

## 2026-09-14 — "waiting for the lock" was never printed

- **Symptom:** two contending suites queued correctly, and the promised
  `trainsty: waiting for the lock…` line never appeared. Found by running two suites
  by hand, not by a test.
- **Root cause:** the announcement was checked inside the SSE read loop. The daemon
  sends nothing until the Grant, so `ReadString` blocks for the whole wait and the
  check is only reached once the Grant has arrived — exactly when the message is
  useless.
- **Fix:** a `time.AfterFunc` timer, stopped on the Grant.
- **Prevention:** no automated test covers terminal progress output, and that gap is
  stated rather than papered over: this one was found by using the tool, which is the
  argument for `quickstart.md` being walked by hand and not only executed.
