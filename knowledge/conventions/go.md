---
okf_version: "0.1"
type: convention
title: "Go Conventions"
description: "Project-specific Go conventions for trainsty: package layout, error wrapping, the syscall policy, concurrency discipline around the one mutex, naming, and the gofmt-versus-house-style resolution. The authoritative rules live in CLAUDE.md → Go Rules."
tags: [conventions, go, concurrency, syscall]
timestamp: "2026-09-13"
---

# Go Conventions

> The authoritative Go rules live in `CLAUDE.md` → **Go Rules**. This file holds
> the project-specific conventions that do not fit there — add new ones here
> rather than duplicating `CLAUDE.md`.

## Source of truth

- `CLAUDE.md` → **Go Rules**: no bare `panic` on a reachable path, no ignored
  errors, wrap with `%w`, no third-party dependency without a justification.
- `CLAUDE.md` → **Repository Architecture**: `cmd/` is thin, `scheduler/` is
  pure, `process/` owns every syscall, `httpapi/` owns the wire format.
- `.specify/memory/constitution.md` → Principle I (stdlib first) and Principle V
  (one state, one mutex).

## Indentation, and the one place the global style loses

The global instruction set asks for four-space indentation in Go. **`gofmt`
indents with tabs and is not configurable**, and reformatting against it would
mean either abandoning `gofmt` — which every Go tool and every contributor
assumes — or fighting it on every save.

**`gofmt` wins, and nothing else about the house style does.** A tab renders at
whatever width the reader has configured, so the *intent* behind the rule is
satisfied by a four-wide tab setting, which is an editor preference rather than a
property of the file. Line length (≤100 for code, ≤72 for comments), descriptive
names and LF endings all still apply, and `gofmt` has no opinion on any of them.

## Package layout

```text
trainsty/
├── main.go              # flag parsing and subcommand dispatch only
├── scheduler/           # the Lock and the Queue. No net/http, no syscall
├── process/             # every syscall: group liveness, group termination
├── httpapi/             # handlers, the JSON wire types, the SSE writer
└── dashboard/           # the embedded page — go:embed lives here
```

Rules that follow from the layout:

- **`scheduler/` imports neither `net/http` nor `syscall`.** If it needs to, the
  abstraction is in the wrong place. This is what makes the queue logic testable
  with `go test` and no server and no child processes — and the queue logic is
  the part that must not be wrong.
- **`process/` owns every syscall, and exposes intent, not mechanism.**
  `process.GroupAlive(pid) (bool, error)` and `process.KillGroup(pid) error`,
  never a `syscall.Kill` call site anywhere else. The `-pid` negation lives
  inside it, in one place, with the comment explaining it — see the warning in
  `CLAUDE.md` → Process Rules.
- **`httpapi/` owns the wire format and nothing else.** It decodes, validates,
  delegates to `scheduler/`, and encodes. A scheduling decision written inline in
  a handler is a scheduling decision that cannot be tested without a server.
- **`main.go` is dispatch.** A subcommand that grows logic grows a file in its
  own package, not a longer `main`.

## Naming

The cardinal rule (`CLAUDE.md` → Naming): a name states intent in the domain's
words, never the mechanism.

| Kind | Shape | Example |
| ---- | ----- | ------- |
| Function | `VerbObject` | `GrantLock`, `ReleaseJob`, `KillGroup` |
| File | `object_role.go` | `liveness_probe.go`, `sse_writer.go` |
| Type | `ObjectRole` | `Scheduler`, `Waiter`, `LivenessProbe` |
| Error value | `ErrObjectCondition` | `ErrNotGroupLeader`, `ErrNoActiveJob` |
| Test func | `TestBehaviourCondition` | `TestReleasesLockWhenRegistrationDrops` |

- **The domain says `Job`, `Waiter`, `Lock`, `Queue`, `Release`.** Never `task`,
  `client`, `mutex`, `slot`, `finish` —
  [`../domains/ubiquitous-language.md`](../domains/ubiquitous-language.md) carries
  the ban lists.
- **`pgid` is an implementation word** and stays inside `process/`. Everywhere
  else the field is `pid`, because that is what the Registration carries and what
  the developer sees.
- **Avoid the Go habit of one-letter receivers where the type matters.** `s
  *Scheduler` is fine; a one-letter name for anything else is not, per the global
  style.

## Errors

- **Wrap with `%w` and a verb phrase naming what failed**:
  `fmt.Errorf("probe process group %d: %w", pid, err)`. Never `fmt.Errorf("error:
  %w", err)`.
- **Sentinel errors for anything a caller branches on**, compared with
  `errors.Is`. The two that matter are a refused Registration
  (`ErrNotGroupLeader`) and a Stop with nothing to stop (`ErrNoActiveJob`).
- **`syscall.Errno` is matched, never string-compared.** `errors.Is(err,
  syscall.ESRCH)` is a dead Job; `errors.Is(err, syscall.EPERM)` is a bug in
  registration and must be loud. Any other errno is logged and changes nothing —
  `architecture/adr.md` → ADR-008.
- **A handler never returns an internal error text to the client.** The response
  is a status code and a short JSON `error` string from a fixed set; the detail is
  logged. Nothing on this API is worth leaking a path over, and a fixed set is
  what the Dashboard can branch on.
- **`_ = someCall()` is rejected in review**, with one exception: a deferred
  `Close` on a read-only handle, and even then prefer logging it.

## Concurrency

The constitution allows exactly one mutex, so the discipline is about where it is
held rather than how many there are. [`../data/state.md`](../data/state.md) →
*Rules for touching it* is the binding list. The Go-specific parts:

- **`go test -race` is a gate, not an option.** The whole product is one shared
  structure read by concurrent handlers; a race here is a wrong Grant.
- **Never hold the mutex across a channel send, a `syscall.Kill`, or a
  `Flush`.** Decide under the lock, release, then act.
- **Every goroutine has an owner and a stop condition.** The Liveness Probe is
  owned by the Job and stops on Release; a `/register` handler's goroutine ends
  with the request. A `go func()` with neither is a leak, and on a daemon a leak
  is permanent.
- **`Request.Context()` is the disconnect signal** — `<-ctx.Done()` in the
  `/register` handler is how a closed terminal releases the Lock. Do not reach for
  the deprecated `CloseNotifier`.
- **`select` on both the Grant and `ctx.Done()`**, always. A Waiter that goes away
  while its Grant is in flight is the ordinary case, not an edge case.

## Signals and process groups

Three rules, each learned from a defect the acceptance suite caught:

- **A child in its own process group does not receive the terminal's `SIGINT`.** The
  terminal signals the *foreground* group only. A parent that moves a child out of
  that group — which `wrap` must, so the suite can be terminated as a unit — is
  responsible for forwarding, or `Ctrl+C` silently does nothing.
- **Forward to the GROUP, not to the direct child.** A POSIX shell waiting on a
  foreground child does not run its trap until that child exits, so
  `sh -c '…; sleep 300'` swallows a signal sent only to the shell. Signalling the
  group reaches every descendant, which is what a terminal does. The signalling
  process survives its own signal because `signal.Notify` has already disabled the
  default action — guard against re-entry so the forwarded copy is not forwarded
  again.
- **A suite in a background process group that reads the terminal gets `SIGTTIN`
  and stops.** Interactive and watch-mode suites are therefore out of scope for a
  queued batch run, and `trainsty help` says so. That is a property of job control,
  not a limitation to engineer around.

## Doc comments

Every exported identifier carries a doc comment, starting with its own name,
saying what it is **for** — and for anything touching processes, what it assumes.
A new file gets a package comment; an existing one does not acquire the debt.

```go
// KillGroup terminates the process group led by pid, including every child the
// Runner started — headless browsers among them. pid MUST be a group leader:
// signalling a non-leader can reach the caller's own foreground group. See
// ADR-002.
func KillGroup(pid int) error
```

That comment is the shape: what it does, what the caller must guarantee, and the
record that explains why.
