# trainsty

A **local E2E test scheduler** built by Cilantro Lab: a single-binary Unix daemon
that serializes end-to-end test runs across multiple repository clones on one
developer machine.

Four clones of a repository on one laptop cannot run two E2E suites at once. They
contend for fixed ports, local database locks, and enough memory that two headless
browser fleets thrash the machine. The failures look like flaky tests. trainsty is
a semaphore of depth one that queues them instead — **without taking your terminal
away.**

| Layer | Stack |
| ----- | ----- |
| Daemon | Go, standard library only — `net/http`, `syscall`, `encoding/json`, `embed` |
| Dashboard | One embedded HTML file. No framework, no build step, no CDN |
| Platform | Linux and macOS. Unix only — process-group termination is the product |

Every choice above is recorded in
[`knowledge/architecture/adr.md`](knowledge/architecture/adr.md).

> **Nothing is built yet.** This repository currently holds a constitution and a
> documentation set, and no Go code. The design is settled to the point where the
> first handler can be written; what it does *not* contain is an implementation.
> See [Status](#status).
>
> **Check the tree before believing a document.** A described package is not a built
> one.

---

## Table of Contents

- [How it works](#how-it-works)
- [Status](#status)
- [Prerequisites](#prerequisites)
- [Installation](#installation)
- [Usage](#usage)
- [Integrating a repository](#integrating-a-repository)
- [Development](#development)
- [Project Structure](#project-structure)
- [Contributing](#contributing)
- [Documentation](#documentation)

---

## How it works

The daemon is a **traffic light**. It never runs your tests.

```text
Runner (your local CI script)          Daemon (trainsty)
  │                                      │
  ├─ GET /register?pid&repo ────────────►│  queued, FIFO
  │     …blocked on an SSE stream…       │
  │◄──── event: grant ───────────────────┤  the Lock is yours
  ├─ run the suite in YOUR terminal      │
  ├─ POST /release  (from a trap) ──────►│  next in line is granted
```

Your suite's `stdout` and `stderr` never leave your terminal — colour, TTY-aware
reporters and `Ctrl+C` all keep working, because the daemon holds the lock and
nothing else ([ADR-003](knowledge/architecture/adr.md)).

**The lock is released five ways**, and four of them are you not behaving: a normal
`/release`, the stream dropping when your terminal closes, the daemon noticing your
process died, the Stop button, and a daemon shutdown. A lock that outlives its
holder is the one failure that would make this tool worse than nothing, so all five
are specified and each one is tested — `CLAUDE.md` → The Release Paths.

---

## Status

| Area | State |
| ---- | ----- |
| Constitution | **Written and binding** — `.specify/memory/constitution.md`, v1.0.0. Five principles, and four of them forbid something that looks like an ordinary good idea |
| Engineering conventions | **Written and binding** — `CLAUDE.md`, `RULES.md`, `CONTRIBUTING.md`, `knowledge/conventions/` |
| Architecture | **Decided.** Eleven ADRs, including the three that go beyond the requirements note: the liveness signal (008), queue identity (007), and shutdown semantics (009) |
| Vocabulary | **Fixed.** Sixteen terms. Splits the note's single word *orphan* into an Orphaned Lock and an Orphaned Process |
| Requirements | Mirrored from the Obsidian vault, verified byte-identical 2026-09-13. Two known disagreements await a **vault** edit, not a repository one |
| Go code | **None.** No `go.mod`, no packages, no tests. `knowledge/architecture/structure.md` has the tree the first commit lands in |
| Git | Pushed to `git@github.com:exustash/trainsty.git` (**public**). `main` is protected: force-push and deletion refused for everyone, `enforce_admins` on |
| CI | No workflow, so nothing on the remote checks `main`. **`scripts/ci-local.sh` is the gate**, with a `.githooks/pre-push` hook — wire it with `git config core.hooksPath .githooks` (`RULES.md` §7.3) |
| Security posture | **Decided** — `knowledge/security/sdr.md` → SDR-001: loopback, POST-only, JSON content type, `Origin` check, and no shared secret. The residual is bounded by a stated condition, not by hope |
| Distribution | **Undecided** — `OD-4`. Build from source meanwhile |

---

## Prerequisites

| Dependency | Version | Notes |
| ---------- | ------- | ----- |
| Go | 1.16+ | `embed` is the oldest feature required. Verified against go1.27.1 darwin/arm64; no other build tooling is used |
| Linux or macOS | — | Unix only. Process-group termination is the product, not a detail ([ADR-002](knowledge/architecture/adr.md)) |
| `setsid` | — | Needed by the **Runner**, not by trainsty. Present on Linux; on macOS it comes from Homebrew's `util-linux` |

There is **nothing else to install**: no runtime, no package manager, no
dependencies. `go.mod` is expected to require nothing, which is why the binary
installs anywhere ([ADR-001](knowledge/architecture/adr.md)).

---

## Installation

There is nothing to install yet. Once there is:

```bash
go build -o trainsty .
```

How the binary is distributed — `go install`, a Homebrew tap, or a released archive
— is **not decided** (`OD-4`).

---

## Usage

```bash
trainsty wrap -- npm run test:e2e    # run a suite under the lock — the whole integration
trainsty start     # spawn the daemon detached; binds 127.0.0.1:45678
trainsty status    # the active job and the queue length
trainsty ui        # open the dashboard in your browser
trainsty stop      # shut the daemon down
trainsty help      # the commands, and what trainsty is for
```

The dashboard is at **<http://localhost:45678>** — it shows the active job, the
queue in order, and a Stop button for the job that is running.

Three things worth knowing before you use it:

- **`trainsty stop` while a suite is running releases the lock but does not kill the
  suite** ([ADR-009](knowledge/architecture/adr.md)). The tests keep going, and the
  next daemon knows nothing about them. The command warns and asks first.
- **The port is 45678, hardcoded, with no fallback.** A failed bind means either a
  daemon is already running or something else holds the port — the error says which
  to check, and the remedies differ ([ADR-011](knowledge/architecture/adr.md)).
- **The dashboard can be ~7 seconds behind** for a job that died without closing its
  connection. That is why it shows a ticking elapsed time rather than a *running*
  badge — a counter tells you what you are looking at
  ([ADR-005](knowledge/architecture/adr.md)).

---

## Integrating a repository

One line. Wrap whatever command runs your suite:

```diff
- npm run test:e2e
+ trainsty wrap -- npm run test:e2e
```

That is the whole integration. `wrap` is the **Runner**: it puts itself in its own
process group, takes the lock, waits, runs your command with its output untouched,
releases on every exit path — including `Ctrl+C` — and exits with your command's own
status, so a failing suite still fails.

> ### Why this is a command and not a snippet you copy
>
> trainsty terminates a process **group**, with `kill(-pid)`. Get the identified
> process wrong and a Stop either leaves the headless browsers behind — the failure
> this tool exists to prevent — or **signals your own foreground process group and
> kills your shell.**
>
> A hand-written Runner has to get that right. `wrap` gets it right once, for
> everybody ([ADR-012](knowledge/architecture/adr.md)). The daemon also **refuses** a
> registration whose process does not lead its group, as a backstop.

Two behaviours worth knowing before you rely on it:

- **No daemon running? The suite runs anyway**, and `wrap` says once that the run was
  not scheduled. A missing scheduler costs you a convenience, not your work.
- **Output is untouched.** No capture, no prefixing, no reformatting — colour and
  TTY-aware reporters keep working, because the daemon never sees a test
  ([ADR-003](knowledge/architecture/adr.md)).

**Writing your own Runner instead** is still supported — the API is a compatibility
surface. The procedure, including the `setsid` requirement and a six-step
verification, is
[`knowledge/playbooks/wrapper-integration.md`](knowledge/playbooks/wrapper-integration.md).

---

## Development

```bash
go build ./...
```

### Gates

```bash
scripts/ci-local.sh --list        # what would run, and which jobs block
scripts/ci-local.sh               # auto: classify the diff, run what matches
scripts/ci-local.sh --everything  # everything, including the acceptance suite
git config core.hooksPath .githooks   # wire the push hook, once
```

**`scripts/ci-local.sh` is the merge gate** — there is no CI workflow and no
required check on `main`, so this script is the gate rather than a mirror of one
([`knowledge/playbooks/local-ci.md`](knowledge/playbooks/local-ci.md)). It also
enforces two properties a hand-run command set cannot: **`go.mod` declares no
dependencies**, and **`os/exec` is imported only by `runner/` and `daemonctl/`**.

The four commands underneath (`RULES.md` §3.3):

```bash
gofmt -l .              # must print nothing
go vet ./...
go build ./...
go test -race ./...
```

**`-race` is not an optional extra.** The entire product is one shared structure
read by concurrent handlers, so a data race is a wrong grant — two suites running at
once, which is the failure the tool exists to prevent. A build verified without
`-race` has not been verified.

**`gofmt` wins over the project's four-space indentation rule.** It is the one place
the house style loses, and
[`knowledge/conventions/go.md`](knowledge/conventions/go.md) carries the reasoning.

### Running the tests on a machine already running trainsty

The end-to-end layer binds the real port 45678, so it cannot run beside a live
daemon — it skips with a message rather than failing as though the code were broken.

```bash
trainsty status                 # or:
lsof -nP -iTCP:45678
```

This is the product's own contention problem applied to itself.

### Two ways to ship a broken daemon that passes every short test

Both concern `/register`, and both are in
[`knowledge/conventions/api.md`](knowledge/conventions/api.md):

- **A non-zero `http.Server.WriteTimeout`** severs the wait the endpoint exists to
  hold open — at the timeout. A queue of 90 seconds passes; 20 minutes does not.
- **A missing `Flush()`** leaves the grant in a buffer. The runner waits forever
  while everything looks healthy.

---

## Project Structure

The annotated tree — what exists today, what the first commit of code adds, and the
four things absent on purpose — is in
[`knowledge/architecture/structure.md`](knowledge/architecture/structure.md). The
shape of it:

```text
main.go         # flag parsing + subcommand dispatch ONLY
scheduler/      # the Lock and the Queue. Imports neither net/http nor syscall
process/        # every syscall: group liveness, group termination
httpapi/        # handlers, the JSON wire types, the SSE writer
dashboard/      # the embedded page — go:embed
```

**The dividing line:** if getting it wrong would grant the lock twice or release it
once too often, it belongs in `scheduler/` as a pure function — not inline in a
handler where it needs a server to test.

---

## Contributing

See [`CONTRIBUTING.md`](CONTRIBUTING.md) for branch naming, Conventional Commit
scopes, the PR checklist, and the six categories of change that need explicit
sign-off before merge.

---

## Documentation

| Document | Contents |
| -------- | -------- |
| [`.specify/memory/constitution.md`](.specify/memory/constitution.md) | The five principles that cannot be traded away. **Read first** — four of them forbid something that looks like an ordinary good idea |
| [`CLAUDE.md`](CLAUDE.md) | Engineering conventions, architectural constraints, AI behaviour. Auto-loaded every session |
| [`RULES.md`](RULES.md) | Operating rules: execution, testing, review, error handling, documentation, version control |
| [`CONTRIBUTING.md`](CONTRIBUTING.md) | Branches, commits, pull requests, review |
| [`knowledge/index.md`](knowledge/index.md) | The knowledge base — architecture, conventions, state, vocabulary, playbooks. Start here |

---

© Cilantro Lab
