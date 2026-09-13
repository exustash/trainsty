# trainsty — Engineering Conventions

## Always loaded

Auto-loaded every session. **One bare `@path` per line — never in backticks and
never brace-expanded.** Import parsing skips code spans, so a backticked path is a
mention, not an import, and it fails silently: nothing errors, the file simply is
not there. Brace expansion is not supported either; each path is literal.

@RULES.md
@.specify/memory/constitution.md
@knowledge/domains/ubiquitous-language.md
@knowledge/conventions/go.md
@knowledge/conventions/api.md
@knowledge/conventions/testing.md
@knowledge/conventions/dashboard.md

**What is resident, and why.** Import what you can violate *without knowing*;
reference what you look up once you already have the question. Rules, the
constitution and the glossary qualify — you would add a `--port` flag without
knowing the constitution forbids it, and write "task" without knowing `Job` is the
canonical word.

**An index fails that test, which is why `knowledge/index.md` is not imported.**
Nobody violates a table of contents by not having read it; you consult it once you
already know you need something from `knowledge/`. It is the first row below.

**Read these before the work they govern.**

| Before you… | Read |
| --- | --- |
| look anything up under `knowledge/` | `knowledge/index.md` (the map) |
| file a new document under `knowledge/` | `knowledge/document-routing.md` |
| write any handler | `knowledge/architecture/adr.md` → ADR-007, ADR-008, ADR-009 |
| touch anything that signals a process | `knowledge/playbooks/wrapper-integration.md` §0 |
| change what state the daemon holds | `knowledge/data/state.md` |
| write anything outside memory | `knowledge/data/ddr.md` → DDR-001 |
| wire a repository's local CI script | `knowledge/playbooks/wrapper-integration.md` |
| diagnose a Lock that will not release | `knowledge/playbooks/stuck-lock-recovery.md` |
| resolve an `FR` / `TS` / `Q` citation | `knowledge/product/decisions.md` |

*Not imported:* `knowledge/playbooks/` are long procedures you follow
deliberately. **`knowledge/product/` must never be imported** — the requirements
mirror is a source document that exists to be *cited into*, so `FR-2` or `TS-6`
resolves for anyone rather than only the vault holder.

---

## What this repository is

**trainsty is a local E2E test scheduler**: a single-binary Unix daemon that
serializes end-to-end test runs across multiple repository clones on one developer
machine.

The problem it solves: a developer with four clones of a repository on one laptop
cannot run two E2E suites at once. They contend for fixed ports, local database
locks, and enough memory that two headless browser fleets thrash the machine. The
failures look like flaky tests.

**The solution is a semaphore of depth one, and nothing more.** A developer's local
CI script — the **Runner** — asks the **Daemon** for the **Lock** and blocks. The
Daemon grants it to one Runner at a time, in arrival order. The Runner then runs its
own tests, in its own terminal, and releases the Lock when done.

Three consequences shape nearly every rule below:

- **The Daemon never sees a test.** It grants and revokes; the Runner executes
  (ADR-003). Preserving the developer's terminal — native `stdout`/`stderr`, colour,
  a TTY-aware reporter, `Ctrl+C` — *is* the feature.
- **The one catastrophic failure is a Lock that will not release**, because then
  every suite on the machine waits. There are **five release paths** and all five
  must work; four of them are the Runner failing rather than behaving.
- **It kills process groups on request.** `syscall.Kill(-pid, SIGKILL)` is what
  reclaims orphaned headless browsers, and it is also what can kill a developer's
  shell if the registered PID is not a group leader.

> **Nothing is built yet.** There is no Go code — only this documentation set and
> a constitution, on a fresh `main` with an empty remote. Every module boundary
> below is binding as design; nothing below describes code you can read.
> `knowledge/architecture/structure.md` says what exists and what is planned —
> **check the repository, not a document**, and never assert that something is
> wired because a document describes it.

**Requirement precedence, a strict order:**
`knowledge/product/local-ci-scheduler-requirements.md` (what the product is) →
`knowledge/product/decisions.md` (the register that resolves a citation). **Both
lose to the Obsidian vault** — the requirements file is a byte-identical mirror. An
edit made in this repository is a defect, not an update: change the vault, then
re-copy.

**And `.specify/memory/constitution.md` outranks all three.** It is the one
document that can forbid something the note asked for, and ADR-010 is the worked
case of a repository record superseding the note — with the obligation that the
vault should still move so the two agree.

---

## Definition of Done

A change is not done when it compiles, and not done when it merges.

- **Behaviour that changed has a test that changed with it — in the same commit.**
  New logic, modified logic and bug fixes all count. "Small", "low-risk" and "the
  file has no tests yet" are not exemptions.
- **Gates pass**: `gofmt -l .` prints nothing, `go vet ./...`, `go build ./...`,
  and **`go test -race ./...`**. The race detector is not optional here — the whole
  product is one shared structure read by concurrent handlers.
- **A new or changed release path is tested by killing the holder**, not by calling
  the happy path. `knowledge/conventions/testing.md` → *Every release path has a
  test*.
- **Docs move with the code.** A term change updates
  `knowledge/domains/ubiquitous-language.md` first, then the code.
- **A change that restates an `FR` / `TS` / `Q` updates that identifier's row in
  `knowledge/product/decisions.md`** — in the same commit, adding the file it just
  wrote the restatement into. The register's file column is the only thing that
  makes a sweep mechanical.
- **A change to the API surface is a constitution amendment** — adding an endpoint
  is MINOR, changing or removing one is MAJOR — carried in the same commit.
- **A bug fix appends to `knowledge/ERRORS.md`** in `RULES.md` §8.1's shape, and the
  `Prevention:` line names a test rather than an intention.

---

## Decision Hierarchy

1. Correctness of the Lock — never two holders, never a holder that cannot release
2. Not destroying the developer's work
3. Security
4. Simplicity
5. Maintainability
6. Readability
7. Performance
8. Developer convenience

**The first two are above security deliberately, and they rarely conflict.** A
scheduler that grants the Lock twice has failed at the only thing it does, and a
Stop that kills the wrong process group destroys work no test can recover.

---

## Modification Policy

Before writing code: understand the existing implementation, search for similar
code, and reuse the patterns already there.

- **Stay in scope.** Never refactor unrelated code; never solve a problem that was
  not requested. Separate cleanup work from feature work.
- **Prefer extending existing abstractions** over adding one. Create a new
  abstraction only when duplication actually emerges, never in anticipation.
- **When the existing pattern is defective, do not reuse it.** Defective means it
  swallows an error, skips validation, holds the mutex across a blocking call, or
  cannot be tested — not that you dislike its shape. Write the new code correctly,
  keep it contained, and flag the old; do not repair it in the same commit.
- **The API is a compatibility surface.** Runner scripts live in repositories this
  project has never seen, are not versioned with it, and cannot be upgraded by
  anyone here. Treat a breaking change to `/register`, `/release` or their
  parameters the way you would a published API.
- **Write idiomatic Go.** No `Result`-shaped error unions, no builder hierarchies,
  no interface with one implementation.

---

## Architecture Stability

Architectural consistency matters more than introducing a new pattern. Do not
introduce, unless explicitly requested:

- **A dependency.** The standard library is the whole toolbox (ADR-001), and
  `go.mod` is expected to require nothing.
- **A second mutex, or any lock-free scheme.** One mutex guards all state
  (constitution → Principle V).
- **Persistence of any kind** — a state file, a PID file, a database. DDR-001, and
  the port bind is already the single-instance mechanism (ADR-011).
- **Configuration.** No config file, no `--port`, no concurrency setting. A
  concurrency above one is a MAJOR constitution amendment, not a flag.
- **A router, a web framework, or an SSE library.** Five routes is
  `http.ServeMux`; SSE is three headers, a `fmt.Fprintf` and a `Flush`.
- **A frontend framework or build step.** The Dashboard is one embedded file.
- **A push channel to the Dashboard.** It polls (ADR-005), and the lag is a stated
  cost rather than a defect.
- **A `syscall` abstraction layer or build tags for an unsupported platform.**
  Unix only (ADR-002).

**Stack:** Go (stdlib only — `net/http`, `syscall`, `encoding/json`, `embed`,
`testing`) · Unix: Linux and macOS · port 45678, hardcoded · one static binary.
Tooling: `go build`, `gofmt`, `go vet`, `go test -race`. No third-party
dependencies, no build system, no bundler.

---

## Repository Architecture

The dependency direction is strict and never reverses:

```text
main.go  ──►  httpapi/  ──►  scheduler/        (pure: Lock + Queue)
                  │              ▲
                  └──►  process/ ┘             (syscall: liveness, group kill)
                  └──►  dashboard/             (go:embed)
```

- `main.go` — flag parsing and subcommand dispatch. **Composition only**; no
  logic. A subcommand that grows logic grows a package.
- `scheduler/` — the Lock and the Queue, in plain Go. **Imports neither `net/http`
  nor `syscall`.** This is where the logic that must not be wrong goes to be
  testable with no server and no child processes.
- `process/` — every syscall. `GroupAlive`, `KillGroup`, and the `-pid` negation,
  in one place. Exposes intent, never mechanism.
- `httpapi/` — handlers, the JSON wire types, the SSE writer. Decodes, validates,
  delegates, encodes.
- `dashboard/` — the embedded page. `go:embed` is what lets one binary serve a UI.

**The dividing line.** If getting it wrong would grant the Lock twice or release
it once too often, it belongs in `scheduler/` as a pure function — not inline in a
handler where it needs a server to test.

---

## Dependency Policy

Before adding a dependency: can the standard library solve it? Has it been tried?
Is the thing it replaces more than a few lines? The answer has been *yes, the
stdlib* for every need so far, and `go.mod` requiring nothing is a property worth
defending: it is why the binary installs anywhere with no toolchain and no lockfile
drift.

A dependency needs a justification in the plan's Complexity Tracking table
**before** use (constitution → Principle I), and a test-only dependency is still a
dependency.

---

## Go Rules

Write Go that `go vet` and `gofmt` accept without argument.

Avoid:

- **Ignored errors.** `_ = doThing()` is rejected in review. The one exception is a
  deferred `Close` on a read-only handle, and even then prefer logging it.
- **`panic` on any reachable path.** Acceptable in `main` where failure means the
  process cannot start, and in tests.
- **Holding the mutex across a channel send, a `syscall.Kill`, or a `Flush`.**
  Decide under the lock, release, then act. This is the deadlock this codebase is
  shaped to avoid.
- **A goroutine with no owner and no stop condition.** On a daemon, a leak is
  permanent.
- **String-comparing an errno.** `errors.Is(err, syscall.ESRCH)`, never
  `strings.Contains(err.Error(), ...)`.
- **`interface{}` / `any` in the wire types.** The JSON surface is a contract;
  give it structs.
- **An interface with one implementation.**

Prefer: wrapped errors with `%w` and a verb phrase naming what failed; sentinel
errors compared with `errors.Is` for anything a caller branches on; small packages
with a documented public surface; `select` on both the work and
`ctx.Done()`; table-driven tests.

See `knowledge/conventions/go.md`.

---

## Process Rules

**`syscall.Kill(-pid, sig)` signals a process group. Get the PID wrong and you
signal the developer's own shell.**

- **A registered `pid` must be a process group leader**, and `/register` refuses
  one that is not: `getpgid(pid) == pid`. The Daemon cannot otherwise tell, and the
  consequence of accepting a non-leader is either surviving headless browsers or a
  killed terminal session.
- **Every syscall lives in `process/`**, behind a name that states intent. There is
  exactly one `syscall.Kill` call site and exactly one place the `-pid` negation is
  written, with the comment explaining it.
- **Killing a bare PID and leaving descendants is a defect**, not a partial
  success. Reclaiming orphaned browsers is the reason this project is Unix-only.
- **`ESRCH` means dead. Nothing else does.** `kill(pid, 0)` returns `EPERM` for a
  live process owned by another user — so "the error was not `ESRCH`" is not "my Job
  is alive", and an `EPERM` is a bug in registration that must be loud rather than
  silently freeing someone else's Lock (ADR-008).
- **Never signal a PID this process did not learn from a validated Registration.**
  A hardcoded or inferred PID is a signal aimed at whatever the machine happens to
  be running — and that includes in tests, which must only ever kill children they
  started.

---

## The Release Paths

There are **five**, they share one implementation and not one of them shares a
trigger, and the product's one catastrophic failure is any of them not working.

| # | Trigger | Mechanism |
| - | ------- | --------- |
| 1 | The Runner finishes | `POST /release` from its `trap` |
| 2 | The terminal closes, or `Ctrl+C` | The Registration's stream drops — `Request.Context().Done()` |
| 3 | The Runner is `SIGKILL`ed | The Liveness Probe reads `ESRCH`, within 3–5 seconds |
| 4 | A developer presses Stop | `POST /stop` — signal the group, then release |
| 5 | `trainsty stop` | `POST /shutdown` — release **without** killing the Job (ADR-009) |

Four rules over all five:

- **Release is idempotent.** Paths 1 and 2 routinely race — a `trap` firing as the
  terminal closes — so a second Release for the same Job is a no-op.
- **It must never release the *successor*.** Compare identity, not "is there a
  Job". This is the plausible implementation that is wrong in a way nobody notices
  until two suites collide.
- **Errors in a release path are logged and still release.** A release skipped
  because a cleanup step failed is exactly the indefinite deadlock the product
  exists to prevent.
- **Each has its own test**, and path 3 — killing the holder without letting it
  call `/release` — is required by the constitution.

**Adding a sixth means updating this table, the constitution's Principle III, and
`knowledge/conventions/testing.md` in the same commit.**

---

## The CLI

Five subcommands, and the only interactive prompt in the product:

| Command | Does |
| ------- | ---- |
| `trainsty start` | Spawns the Daemon detached, binds 45678, returns the terminal |
| `trainsty stop` | `POST /shutdown`. **Warns and confirms when a Job is active**, naming the repo — the suite keeps running and becomes invisible to the scheduler (ADR-009) |
| `trainsty status` | Prints the active Job and the queue length |
| `trainsty ui` | Opens `http://localhost:45678` in the default browser |
| `trainsty help` | The commands, and one line saying what trainsty is for |

- **Exit codes are the contract**, because a Runner branches on them: `0` success,
  non-zero for "no daemon reachable". A script must be able to tell *no daemon* from
  *daemon says no*.
- **`start` says where output goes**, including "nowhere" while DDR-002 is open. A
  developer should not discover that during an incident.
- **A failed bind names the port and the likely cause**, and prints
  `lsof -nP -iTCP:45678` — *already running* and *something else holds it* have
  different remedies (ADR-011).
- **`help` carries the one-line description.** `trainsty` is not a word anyone
  guesses the behaviour from, which the note's `e2e-scheduler` gave away for free.

---

## API Rules

The API is a **compatibility boundary**: its clients are shell scripts in
repositories this project has never seen and cannot upgrade. Treat it as published.

- **Validate every parameter, including values a Runner "cannot" produce.** `pid`
  must exist, be signalable and lead its group; `repo` is a bounded label and
  nothing more.
- **Validate before queueing, never at Grant time.** A malformed Registration in
  the Queue becomes a Grant to nobody, and a Lock held by an entry that can never
  release.
- **A no-op is a success.** `/release` with no Job, `/stop` with no Job, a second
  `/release` — all `200`. A `trap` that prints an error on the normal path is a
  `trap` developers delete.
- **Mutating endpoints are `POST` and refuse `GET` with `405`.** Not tidiness: a
  `GET /stop` can be triggered by an `<img src>` on any page the developer has open,
  and this daemon kills process groups.
- **Never return an internal error string, a path, or an errno text.** A stable
  `snake_case` code the Dashboard and the Runner can branch on; the detail goes to
  the log.
- **`WriteTimeout` and `IdleTimeout` must be zero for `/register`, and every event
  is followed by `Flush()`.** These are the two ways to ship a Daemon that passes
  every test shorter than the timeout and fails every real 20-minute wait.

See `knowledge/conventions/api.md`.

---

## Dashboard Rules

The Dashboard is the developer's only view of a Lock they cannot otherwise see. Its
job is to be **trusted**.

- **It holds no state.** Every displayed value derives from the latest `/status`.
  No optimistic update, no cache, no `localStorage` — the Queue is not the
  browser's to know.
- **`textContent`, never `innerHTML`.** `repo` is developer-supplied, arrives as a
  query parameter, and lands on a page that can kill process groups.
- **Show elapsed time, not a badge.** The page can be ~7 seconds behind reality
  (ADR-005); a counter ticking tells the developer what they are looking at, where a
  static *running* is the page lying.
- **Distinguish "no daemon" from "no jobs".** Different facts, different displays.
- **Stop confirms**, naming the repo, and posts with no `pid` — a stale tab must not
  be able to name a target that has since been replaced.

See `knowledge/conventions/dashboard.md`.

---

## Error Handling

Do not silently ignore failures. Prefer explicit logging, wrapped errors,
actionable messages.

```go
// Bad
_ = releaseJob(job)

// Good
if err := releaseJob(job); err != nil {
    log.Printf("release job for pid %d: %v", job.PID, err)
    // …and release anyway: a skipped release is a deadlock.
}
```

That comment is the project's whole error-handling philosophy in one line. **A
failure in a cleanup path must not prevent the cleanup.**

---

## Logging

Log: every Grant and Release with its cause, a refused Registration and why, a
probe error that is not `ESRCH`, a Stop and what it signalled, and the bind.

**Never log:** the contents of anything the Runner is testing, environment
variables, or absolute paths inside the user's home directory. This daemon never
sees test output (ADR-003) and must not start by accident — no proxying, no
capturing, and nothing that reads the Runner's streams.

`repo` is a developer-supplied string reaching a log line: bound its length and do
not let it carry a newline into the log.

**While DDR-002 is open, a detached Daemon's log goes nowhere**, which means a crash
leaves no evidence. Say so in `trainsty start` rather than letting it be discovered
during an incident.

---

## Security

The API can terminate process groups, and it is unauthenticated.

**The floor, which ships regardless of how OD-1 is settled:**

- **Bind `127.0.0.1`, never `0.0.0.0`.** The second exposes a remote process-kill
  endpoint to the network.
- **Mutating endpoints are `POST` only** — a `GET` is reachable from an `<img>`
  tag.
- **Require `Content-Type: application/json`** on mutating endpoints. A
  non-simple content type cannot be produced by a cross-origin HTML form, which is
  the cheapest CSRF defence that exists.
- **Check `Origin` where present** and refuse anything but the Daemon's own.
- **Escape `repo` where it is rendered.**

**Loopback is not a security boundary.** It keeps other machines out; it keeps no
local process out, and any web page the developer has open can reach it. **Whether a
token is also required is `knowledge/architecture/adr.md` → OD-1, and settling it
needs a security decision record** — `knowledge/document-routing.md` says where.

Never commit secrets. There are none to commit today, and that is a property worth
keeping.

---

## Testing Philosophy

**The target is a reviewer who cannot find a real bug.** Not one who finds none
because nobody looked. Review runs once, reads a diff, and cannot execute the code
against the case that breaks it. Tests run on every push, for ever.

- **No logic merges untested.** Not scoped down by file size, perceived risk, or
  deadline pressure. If existing code in the same file is untested, that is a gap to
  flag, not a precedent to extend.
- **`go test -race ./...` is a gate.** A suite that passes without `-race` proves
  almost nothing about a daemon whose entire state is shared.
- **Every release path has its own test**, and the mandatory one kills the holder
  without letting it call `/release`. That test is the whole product working when
  everything else has gone wrong.
- **Drive the rejection, not only the acceptance.** A `pid` validator with no test
  for what it refuses could return `nil` unconditionally and stay green.
- **`EPERM` is asserted separately from `ESRCH`.** *Not dead* and *not mine to
  probe* are different answers and only one releases a Lock. A test asserting "the
  probe errored, so we released" enshrines the bug ADR-008 exists to prevent.
- **Process tests use real children the test started, and reap them.** The thing
  under test is the syscall, so it cannot be faked — but a test that leaks a group
  leaves several processes behind for the session.
- **Do not assert on timing.** A killed group is gone *soon*; poll with a deadline
  rather than sleeping a magic interval.
- **The E2E layer binds the real port**, so it cannot run beside a real Daemon. It
  must **skip with a message naming `trainsty status`** rather than failing as though
  the code were broken — the product's own contention problem, applied to itself.

See `knowledge/conventions/testing.md`.

---

## Ubiquitous Language

The canonical terms in `knowledge/domains/ubiquitous-language.md` are the required
vocabulary for design, code and conversation — a first-class convention, not
documentation.

- **Name code after the model.** Types, functions, fields, JSON keys and log lines
  use the domain term — never a synonym, abbreviation, or technical stand-in.
- **One concept, one word.** No "task" or "run" for a Job, no "client" for a
  Runner, no "mutex" or "slot" for the Lock.
- **Two pairs the glossary keeps apart, because the code will not survive
  conflating them**: **Stop** (kill the Job) versus **Shutdown** (exit the Daemon),
  and **Orphaned Lock** (held by a dead Job) versus **Orphaned Process** (a child
  that outlived its Runner). One symptom, two mechanisms.
- **`pgid` is an implementation word** and stays inside `process/`.
- **Language, model and code move together.** Update the glossary first, then
  carry it into the code in the same change.

---

## Naming

**The main rule:** a name must communicate intent within its domain — what the
thing is *for*, not the mechanism it uses or the caller that invokes it. The
Ubiquitous Language fixes the *word*; this fixes the *form*.

| Thing | Shape | Example |
| --- | --- | --- |
| Function | `VerbObject` | `GrantLock`, `ReleaseJob`, `KillGroup` |
| File | `object_role.go` | `liveness_probe.go`, `sse_writer.go` |
| Type | `ObjectRole` | `Scheduler`, `Waiter`, `LivenessProbe` |
| Error value | `ErrObjectCondition` | `ErrNotGroupLeader`, `ErrNoActiveJob` |
| Test function | `TestBehaviourCondition` | `TestReleasesLockWhenRegistrationDrops` |
| JSON key | `lowerCamelCase` | `grantedAt`, `queuedAt` |

Prefer descriptive names and existing project vocabulary. Avoid abbreviations,
generic names, single-letter variables — a one-letter receiver on `*Scheduler` is
fine, anything else is not.

Worked examples in `knowledge/conventions/go.md`.

---

## Technical Debt

When shortcuts are necessary: document them, explain why, create follow-up work.
Never leave unexplained hacks.

A deliberate simplification with a known ceiling — a global lock, an O(n) scan, a
naive heuristic — carries a comment naming the ceiling and the upgrade path, not an
apology.

**When a pattern recurs, promote it.** Update *this file* if it changes how an
agent should behave, `CONTRIBUTING.md` if it changes team workflow, and
`knowledge/` if it changes design or operational knowledge.

---

## Final Rule

When uncertain: choose the solution an experienced engineer joining the project six
months from now would immediately understand and feel comfortable maintaining. On
this project that is usually the smaller one — the whole product is a semaphore of
depth one, and anything that reads as clever is probably a second source of truth.
