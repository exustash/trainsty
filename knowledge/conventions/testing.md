---
okf_version: "0.1"
type: convention
title: "Testing Conventions"
description: "Where each kind of test lives and what it may assume: pure scheduler unit tests, process tests against real short-lived children, httptest-driven API tests including the SSE wait, and the end-to-end script that drives the built binary. Names the five release paths every one of which needs a test, and the abrupt-kill case the constitution makes mandatory."
tags: [conventions, testing, go, race, e2e]
timestamp: "2026-09-13"
---

# Testing Conventions

> The authoritative rules live in `CLAUDE.md` → **Testing Philosophy** and
> `RULES.md` §2. The constitution's Development Workflow & Quality Gates fixes the
> coverage floor and the one mandatory case.

## Source of truth

- `.specify/memory/constitution.md`: lock, queue and release logic hold **≥80%
  statement coverage**; OS-level process control is exercised against **real
  short-lived child processes, not mocks**; the suite **must** include a case that
  kills a Lock holder without letting it call `/release`.
- `CLAUDE.md` → Testing Philosophy: no logic merges untested; behaviour over
  implementation.

## The layers, stated as boundaries

| Layer | Runs | Asserts | May assume |
| ----- | ---- | ------- | ---------- |
| `scheduler/` unit | `go test ./scheduler` | Grant order, FIFO, re-registration, idempotent Release, the two invariants | **Nothing.** No server, no child process, no clock beyond an injected one |
| `process/` integration | `go test ./process` | Group liveness, group termination, leader refusal, `ESRCH` vs `EPERM` | A real forked child it started and will reap. A tempdir. Never the developer's own processes |
| `httpapi/` integration | `go test ./httpapi` | Status codes, the JSON contract, validation refusals, the SSE handshake and the Grant event | `httptest.Server`. No real Runner |
| End to end | `go test -tags e2e` or the script | The built binary: two Runners contending, `Ctrl+C` release, Stop killing a browser-shaped child tree | A Unix machine, port 45678 free, and the binary built |

Each row is more expensive and worse at localising a failure. **Push every
assertion as far up as it goes**: "the second Runner waits" is a `scheduler/` unit
test; "a killed terminal frees the Lock" needs the stream, so it is `httpapi/` at
the highest.

## The race detector is a gate

**`go test -race ./...` is required, not optional.** The entire product is one
shared structure read by concurrent handlers, so a data race here is a wrong
Grant — two suites running at once, which is the failure the tool exists to
prevent. A test suite that passes without `-race` proves less than nothing about
this codebase.

## Every release path has a test

There are five, `CLAUDE.md` → The Release Paths names them, and **each one needs
its own test** — they share an implementation and not one of them shares a trigger:

| Path | Test shape |
| ---- | ---------- |
| `POST /release` | Ordinary: register, grant, release, assert the next Waiter is granted |
| Registration dropped | Cancel the request context mid-wait; assert Release |
| Liveness Probe `ESRCH` | Start a real child, register its PID, kill it **without** closing the stream, assert Release within the probe interval |
| `POST /stop` | Start a real child with children of its own; assert the whole group is gone **and** the Lock freed |
| `POST /shutdown` | Assert the Lock is dropped and the Job's process is **still alive** — ADR-009 is a behaviour, so it is asserted, not assumed |

**The mandatory case is the third one.** The constitution requires a test that
kills a Lock holder without letting it call `/release` and asserts the next waiter
is promoted. It is the whole product working when everything else has gone wrong.

## Failure cases, not only success

For each unit, drive the rejection, the empty input, the boundary, and — where the
code is a control — the state it exists to refuse:

- **The negative branch of every guard.** A `pid` validator with no test for what
  it rejects could return `nil` unconditionally and stay green.
- **`EPERM` distinct from `ESRCH`.** *Not dead* and *not mine to probe* are
  different answers and only one of them releases a Lock. A test asserting "the
  probe returned an error, so we released" would enshrine the bug ADR-008 exists
  to prevent.
- **A Release for a Job that already ended.** Assert it does **not** release the
  successor. This is the one where a plausible implementation is wrong in a way no
  developer would notice until two suites collided.
- **An empty Queue everywhere.** `/stop`, `/release` and `/status` with no Job are
  all ordinary, and all `200`.

## Process tests: real children, and clean up after them

`process/` cannot be tested with a fake — the thing under test *is* the syscall.

- **Start a real child** you control: `exec.Command("sleep", "30")` with
  `SysProcAttr{Setpgid: true}`, and for group tests a child that starts children
  of its own (a `sh -c 'sleep 30 & sleep 30 & wait'` stands in for a browser
  fleet).
- **Always reap.** A test that leaks a `sleep` leaves it for the session; a test
  that leaks the *group* leaves several. `t.Cleanup` with a group kill, every time.
- **Observe a death by WAITING, never by probing the pid.** A killed child of the test
  process stays a **zombie** until it is reaped, and a zombie still answers
  `kill(pid, 0)` successfully — so a probe loop reports *survived* for a process that
  is already dead. Either `Wait()` on it (a goroutine closing a channel, if the test
  needs a deadline) or reap it before asserting.

  **This is the rule with the highest cost-to-obviousness ratio in the repository: it
  has broken three separate tests here** — `process/kill_test.go`,
  `httpapi/stop_test.go` and `runner/wrap_test.go` — each time looking exactly like a
  product bug. If a process-termination assertion fails and the code looks right, check
  this first.
- **Never probe or signal a PID the test did not create.** A hardcoded PID in a
  test is a signal aimed at whatever the machine happens to be running.
- **Do not assert on timing.** A killed group is gone "soon"; poll with a deadline
  rather than sleeping a magic interval.

## End to end, and the resource nobody else can share

The E2E layer drives the **built binary** and therefore binds the real port 45678.

- **It cannot run while a real Daemon is running on the machine**, and it must say
  so rather than failing as though the code were broken. Check the port first and
  skip with a message naming `trainsty status`.
- **This is the product's own dogfooding problem**: trainsty exists because
  concurrent local suites collide over machine-wide resources, and its own suite is
  one of them. State it in the skip message.
- Cover: two Runners contending in order; a Runner killed with `SIGKILL`; a Stop
  against a child tree; and `trainsty status` agreeing with `/status`.

## Names and shape

- `TestReleasesLockWhenRegistrationDrops`, not `TestRelease2`. The behaviour and
  the condition, per `go.md` → Naming.
- **Table-driven where cases differ only in data**, per the constitution.
- Arrange–Act–Assert with the three parts visually separated.
- **No test framework.** `testing`, `httptest`, `errors.Is`, and `t.Cleanup` cover
  every layer above; a matcher library would be the first dependency in `go.mod`
  and would buy nothing.
