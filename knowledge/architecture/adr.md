---
okf_version: "0.1"
type: decision-record
title: "Architecture Decision Records"
description: "Log of architectural decisions for trainsty, newest first. Records the stack — Go, one static binary, stdlib only, the dashboard embedded with go:embed — and the scheduling architecture: the daemon is a traffic light the Runner obeys (003), termination targets the process group (002), the queue wait is Server-Sent Events (004), the dashboard polls and nothing is pushed to it (005), the lock is FIFO with exactly one holder (006), a reconnect keeps its queue position (007), the open registration is the primary liveness signal and the PID probe only its backstop (008), a shutdown releases the job without killing it (009), and the command is named trainsty rather than the note's e2e-scheduler (010). Also carries the decisions still open, including who may call /stop."
tags: [architecture, adr, decisions, go, scheduler]
timestamp: "2026-09-13"
---

# Architecture Decision Records

> Append new ADRs at the top, newest first. `RULES.md` §8.3 requires an ADR when
> adding an architectural component, changing an existing pattern, or
> introducing a significant dependency.
>
> **Every record below is derived from
> [`../product/local-ci-scheduler-requirements.md`](../product/local-ci-scheduler-requirements.md),
> which is a mirror of the Obsidian vault.** Where an ADR goes beyond the note,
> it says so and says why — the note specifies the *what*, and several of these
> are the *how* it leaves open. Where an ADR contradicts the note, the vault
> should move so the two agree; ADR-010 is the worked case.

## Template

```markdown
## ADR-00N — <Decision, stated as the outcome>

- **Status:** proposed | accepted | superseded by ADR-00M
- **Date:** YYYY-MM-DD
- **Context:** the forces at play. What made this a decision rather than a default.
- **Decision:** what we do. Present tense, unambiguous.
- **Consequences:** what this makes easy, what it makes hard, what it forecloses.
- **Alternatives considered:** each with the reason it lost.
```

Keep each ADR to one screen. A decision that needs more is a design document;
link to it from here.

## Open decisions

Recorded here so they are not made silently in a pull request.

| # | Decision | Blocks |
| - | -------- | ------ |
| OD-3 | **Whether `repo` means anything to the Daemon.** Today it is a display label for the Dashboard. If it ever gates anything — per-repository queues, a concurrency above 1 — that is a MAJOR constitution amendment, not a feature | Nothing yet. Recorded so the field is not quietly promoted |
| OD-4 | **How the binary is distributed.** `go install`, a Homebrew tap, or a released archive. The constitution fixes *one static binary*; it does not fix how it arrives | The install instructions in `README.md`, which today say *build from source* |

**Closed by the requirements note itself**, and recorded below rather than
treated as defaults: the language and distribution shape (ADR-001), the platform
(ADR-002), the execution model (ADR-003), the wait protocol (ADR-004), the
Dashboard's update mechanism (ADR-005), the queue discipline (ADR-006), and the
port (ADR-011).

**Closed 2026-09-13 by `specs/001-serialize-e2e-runs/spec.md`**, whose clarification
round forced all three:

- **`OD-1` — who may terminate a Job.** The floor is sufficient; no shared secret.
  It is a security decision, so the record is
  [`../security/sdr.md`](../security/sdr.md) → **SDR-001**, not an ADR — which is what
  [`../document-routing.md`](../document-routing.md) said would happen when this
  settled.
- **`OD-2` — where the Daemon logs.** A per-user file in the operating system's log
  location, append-only and never read back:
  [`../data/ddr.md`](../data/ddr.md) → **DDR-002**, now accepted.
- **The Runner ships**, as `trainsty wrap` — **ADR-012** below. This was not an `OD-`
  row at all, because nothing here had recorded that it was a question.

## Decisions

## ADR-012 — `trainsty wrap` ships the Runner, and it is a client rather than the Daemon

- **Status:** accepted
- **Date:** 2026-09-13
- **Context:** ADR-003 makes the Daemon a traffic light and leaves execution to the
  Runner — a script in each repository. That left every repository to re-implement the
  same four steps, one of which is the product's worst hazard: a Runner that registers
  a PID it does not lead makes a Stop either useless or lethal to the developer's
  shell (ADR-002). `playbooks/wrapper-integration.md` existed to warn about it, and a
  warning is a weaker control than a design in which the mistake cannot be made.
- **Decision:** The product ships `trainsty wrap -- <command>`. It puts itself in a
  process group it leads, registers, waits for the Grant, runs the command with its
  streams untouched, releases on every exit path, and exits with the command's status.
  A repository's integration becomes one line.
- **Consequences:**
  - **The group-leader hazard is removed by construction.** `/register`'s refusal of a
    non-leader PID (ADR-002) becomes a backstop for hand-written Runners rather than
    the thing standing between a developer and a killed shell.
  - **This does not violate the constitution's Principle II**, and the reason must be
    written down because the code will look like it does: **`trainsty wrap` is a
    client**, a separate process the developer starts, which happens to be compiled
    into the same binary. The **Daemon** still spawns nothing, supervises nothing, and
    reads no output. Principle II constrains the Daemon, not the binary.
  - **The binary now contains process-spawning code**, in the `wrap` path only. A
    reviewer who finds `exec.Command` outside it should treat it as a defect and this
    record as the reason.
  - **A hand-written Runner stays supported**, because the API is a compatibility
    surface and existing scripts cannot be upgraded by anyone here.
  - `wrap` must **pass the exit status through**. A wrapper that swallows a failing
    suite's status turns a red suite green, which is worse than no scheduler at all.
  - **When the Daemon is unreachable, `wrap` runs the suite anyway** and says so once.
    A tool that blocks work when its convenience is unavailable gets removed from the
    script.
- **Alternatives considered:**
  - **Documentation only**, the pre-2026-09-13 position — rejected: it leaves the
    hazard live in every repository and asks each one to get `setsid` and a `trap`
    right.
  - **A committed example script** — rejected as the worst of both: copies drift, and
    each copy still has to be correct about process groups.
  - **`wrap` as a separate binary** — rejected: two artifacts to install where the
    constitution promises one, for a client that shares the Daemon's constants.

## ADR-011 — The port is 45678, hardcoded, and the bind is the single-instance lock

- **Status:** accepted
- **Date:** 2026-09-13
- **Context:** The note fixes port 45678 to stay clear of application
  development ports. Two questions it does not answer: what happens when the
  port is taken, and how a second `trainsty start` is prevented.
- **Decision:** The port is a constant with **no fallback and no flag**. A failed
  bind exits non-zero naming the port and the likely cause. **The successful
  bind is what makes an instance unique** — there is no PID file and no lock
  file.
- **Consequences:**
  - A second `trainsty start` fails fast and harmlessly, which is the correct
    outcome and needs no code of its own.
  - **No stale-lock-file failure mode exists**, which is the failure mode a PID
    file would add — and it is precisely the one this product exists to prevent
    in other people's tooling.
  - A fallback port would be worse than the error: the Runner's URL is a
    constant too, so a Daemon on another port is a Daemon nothing can reach
    while looking healthy.
  - The error message must distinguish *already running* from *something else
    holds 45678*, because the remedies differ. `lsof -nP -iTCP:45678` is the
    line to print.
- **Alternatives considered:** a `--port` flag — rejected, it makes the Runner's
  constant a lie and there is nothing to configure between two developers'
  machines. A PID file — rejected, see above.

## ADR-010 — The command is `trainsty`

- **Status:** accepted
- **Date:** 2026-09-13
- **Context:** The note's CLI section names the binary `e2e-scheduler`
  throughout. The project is named **trainsty**, which was settled after the
  note was written.
- **Decision:** The binary, the module path and every subcommand example are
  `trainsty`. `e2e-scheduler` appears nowhere in the code.
- **Consequences:**
  - **The note is now wrong in five places** — every bullet of its §5. This is a
    vault edit, not a repository edit
    ([`../product/local-ci-scheduler-requirements.md`](../product/local-ci-scheduler-requirements.md)
    is a mirror), and until it happens the mirror and the code disagree by
    design, with this record as the reason.
  - `trainsty` is not a word anyone will guess from the product's behaviour, so
    `trainsty help` and the README carry the one-line description that
    `e2e-scheduler` gave away for free.
- **Alternatives considered:** keeping `e2e-scheduler` as the command with
  trainsty as the project name — rejected: two names for one thing is what
  `domains/ubiquitous-language.md` exists to prevent, and the note's name is
  also a description of a category rather than a name.

## ADR-009 — A shutdown releases the Job; it does not kill it

- **Status:** accepted
- **Date:** 2026-09-13
- **Context:** `POST /shutdown` must "drop any active locks and cleanly
  terminate". Dropping a Lock while tests run is exactly the collision the
  product prevents — but so is killing a developer's suite because someone typed
  `trainsty stop`.
- **Decision:** Shutdown **closes every Registration and exits**. It does not
  signal the Job's Process Group. The running tests continue, unsupervised.
- **Consequences:**
  - **A restart across a running Job is unprotected**, and the new Daemon cannot
    learn about it: nothing is persisted (DDR-001) and the Registration is gone.
    `trainsty stop` while a Job runs is therefore a real hazard, and both the
    CLI and `playbooks/stuck-lock-recovery.md` must say so before the developer
    does it.
  - **`trainsty stop` warns and requires confirmation when a Job is active**,
    naming the repo — the one interactive prompt in the CLI.
  - Stop (the Dashboard control) and Shutdown stay different verbs with
    different blast radii, which `domains/ubiquitous-language.md` fixes.
- **Alternatives considered:** stopping the Job first — rejected: it destroys
  work the developer did not ask to lose, and the daemon is a traffic light
  (ADR-003), not an owner of the run. Refusing to shut down while a Job runs —
  rejected: it makes the daemon unkillable by its own CLI exactly when something
  has gone wrong.

## ADR-008 — The open Registration is the primary liveness signal; the PID probe is its backstop

- **Status:** accepted
- **Date:** 2026-09-13
- **Context:** The note specifies a 3–5 second `kill -0` loop, and separately
  requires that a closed terminal releases the Lock. Treating the PID probe as
  the primary signal has two defects: `kill(pid, 0)` succeeds for a **reused**
  PID, and it returns `EPERM` — not `ESRCH` — for a live process owned by
  another user, so "the error was not `ESRCH`" is not "my Job is alive".
- **Decision:** A Job is alive while its **Registration stream is open**;
  `Request.Context().Done()` is the signal, and it fires on a closed terminal
  within milliseconds. The **Liveness Probe runs anyway**, every 3–5 seconds, as
  a backstop for the case the stream cannot see: a `SIGKILL`ed Runner whose
  socket the kernel has not yet torn down, and a wedged connection.
- **Consequences:**
  - Release is fast in the common case and bounded in every other.
  - **`ESRCH` releases; any other probe error is logged and does not.** An
    `EPERM` is a bug in registration — the PID is not the developer's process —
    and must be loud rather than silently freeing someone else's Lock.
  - **PID reuse cannot be ruled out by the probe alone.** The stream makes it
    almost unreachable: a reused PID matters only in the window between a
    `SIGKILL` and the socket teardown. The residual risk is accepted and named
    here rather than mitigated with a start-time comparison, which is
    `/proc`-shaped and not portable to Darwin.
  - Two signals means two release paths for one cause, so **Release is
    idempotent by construction** (see `CLAUDE.md` → The Release Paths).
- **Alternatives considered:** a client heartbeat — rejected: it puts a timer in
  the Runner, which is a shell script, and a missed heartbeat under load would
  release a healthy Job. The probe alone, as the note describes — rejected on
  the `EPERM`/reuse reasoning above, which the note does not address.

## ADR-007 — A reconnecting Registration keeps its place in the Queue

- **Status:** accepted
- **Date:** 2026-09-13
- **Context:** A Registration is an SSE stream over a 20-minute wait. If a drop
  and reconnect appends to the tail, a developer on a flaky connection — or one
  whose laptop slept — is starved, and FIFO stops being true of anything the
  developer can observe.
- **Decision:** The Queue is keyed by the Registration's **`pid`**. A
  re-registration for a `pid` already queued **resumes that position** rather
  than adding a second entry. A `pid` that is already the Job is answered with
  an immediate Grant.
- **Consequences:**
  - The Queue is a set keyed by PID, not a list of independent entries, and
    `/status` must never show one `pid` twice.
  - **A PID is the identity**, so ADR-008's residual reuse risk extends here:
    a reused PID inherits a queue position. Same window, same accepted risk.
  - An immediate Grant for the current Job makes the Runner's retry loop safe to
    write naively, which matters because it is written in shell.
- **Alternatives considered:** a server-issued token the Runner must carry —
  rejected: it makes the Runner stateful across processes, and the PID is already the
  thing the Daemon must know. **This no longer has a free ride available**: SDR-001
  settled that no token is introduced for termination either, so a token here would be
  the only one in the product rather than a reuse of an existing one.

## ADR-006 — Exactly one Lock holder, strict FIFO

- **Status:** accepted
- **Date:** 2026-09-13
- **Context:** The note requires a "strict 1-on-1 queue" to guarantee collision
  prevention. E2E suites contend for fixed ports, database locks, and enough
  memory that two headless browser fleets thrash a laptop.
- **Decision:** Depth one, forever, and order of arrival. **No fairness policy,
  no priorities, no per-repository lanes.**
- **Consequences:**
  - The whole state is one holder and one ordered set, which is what makes
    `/status` honest and the Dashboard trivial.
  - A concurrency above 1 is a **MAJOR constitution amendment**, not a flag —
    the constitution's Principle V says so, and OD-3 keeps `repo` from becoming
    the back door to it.
  - A long suite blocks short ones with no way to jump. Accepted: the
    alternative is a scheduler with a policy, and the policy is what nobody can
    agree on.
- **Alternatives considered:** a configurable depth — rejected, it reintroduces
  exactly the collisions the tool exists to prevent, and the failure is silent
  and machine-specific. Priority by repository — rejected as unrequested
  policy.

## ADR-005 — The Dashboard polls; nothing is pushed to it

- **Status:** accepted
- **Date:** 2026-09-13
- **Context:** The Daemon already speaks SSE for `/register`, so pushing state
  to the Dashboard over the same mechanism is nearly free.
- **Decision:** The Dashboard fetches `GET /status` every 2000 ms. The SSE
  mechanism is reserved for `/register` and carries exactly one kind of message:
  a Grant.
- **Consequences:**
  - **The Dashboard can be a single static page with no reconnect logic**, which
    is what keeps it inside `go:embed` and inside one binary (ADR-001).
  - **The UI lags reality by up to one poll plus one probe interval** — about 7
    seconds worst case for a Job that died without closing its stream. That is a
    known ceiling, stated in `conventions/dashboard.md`, not a defect to fix
    with a push channel.
  - `/register` stays the only long-lived connection, so the Daemon's connection
    count is the Waiter count plus open browser tabs, and nothing else.
- **Alternatives considered:** SSE or a WebSocket to the Dashboard — rejected as
  unrequested complexity for a page a developer glances at; it also makes the
  page stateful and therefore a second source of truth, which the constitution's
  Principle V forbids.

## ADR-004 — The queue wait is Server-Sent Events

- **Status:** accepted
- **Date:** 2026-09-13
- **Context:** A Waiter can wait 20+ minutes. A plain HTTP request held open
  crosses default client and proxy timeouts, and polling from a shell script
  costs a process per tick and loses FIFO precision.
- **Decision:** `GET /register` responds `text/event-stream` and holds the
  connection open until the Grant. The stream is also the liveness signal
  (ADR-008).
- **Consequences:**
  - **`http.Server.WriteTimeout` and `IdleTimeout` must be zero** for this
    route, or the Daemon itself severs the wait it exists to hold open. This is
    the single most likely way to ship a broken Daemon that passes every test
    shorter than the timeout — `conventions/api.md` carries it.
  - Each event must be followed by an explicit `http.Flusher.Flush()`; without
    it the Grant sits in a buffer and the Runner waits forever with everything
    apparently healthy.
  - The Runner needs only `curl -N`, so it stays a shell script with no
    dependency on a trainsty client library.
  - A comment heartbeat (`: keep-alive`) is optional on loopback and is **not**
    a liveness mechanism — ADR-008 owns that.
- **Alternatives considered:** long polling — rejected on timeouts, which is the
  note's own reasoning. A WebSocket — rejected: bidirectional framing for a
  one-way, one-message channel, and `curl` cannot speak it.

## ADR-003 — The Daemon is a traffic light; the Runner owns execution

- **Status:** accepted
- **Date:** 2026-09-13
- **Context:** A scheduler could run the suite itself and stream the output
  back. That is a local CI server, and it takes the developer's terminal with
  it: native `stdout`/`stderr`, colour, a TTY-aware test reporter, and
  `Ctrl+C`.
- **Decision:** The Daemon grants and revokes. **The Runner executes.** The
  Daemon never spawns, supervises, proxies, captures or reformats the suite or
  its output. The single exception is termination: a Stop signals a Process
  Group the Daemon never started.
- **Consequences:**
  - Preserving the terminal workflow is the feature, and this is the decision
    that preserves it.
  - **The Daemon holds no logs**, so a diagnosis is always *which Job holds the
    Lock*, never *what did the tests print* — see
    `playbooks/stuck-lock-recovery.md`.
  - Signalling a group it did not create is why the Runner's process-group
    discipline is the Runner's responsibility and must be verified there
    (ADR-002).
  - A change that moves execution into the Daemon is a **MAJOR** constitution
    amendment, because it is a different product.
- **Alternatives considered:** running the suite in the Daemon and streaming
  output — rejected above. A PTY proxy to preserve colour — rejected: it is most
  of a terminal multiplexer, to end up where doing nothing already is.

## ADR-002 — Unix only, and termination targets the Process Group

- **Status:** accepted
- **Date:** 2026-09-13
- **Context:** An aborted E2E suite leaves headless browsers holding hundreds of
  megabytes and the ports the next run needs. Killing the Runner's PID leaves
  every one of them behind.
- **Decision:** Linux and macOS only. A Stop calls
  `syscall.Kill(-pid, syscall.SIGKILL)` — the **negative PID**, which signals the
  whole group. No Windows support, no `syscall` abstraction layer, no build tags
  for a platform that is not shipped.
- **Consequences:**
  - **A Registration's `pid` must be a process group leader**, and nothing in
    the protocol can verify that: the Daemon sees a number. A Runner that
    registers a non-leader PID either fails to stop or — the dangerous case —
    **signals the developer's own foreground process group and kills their
    shell.** The Runner must therefore call `setsid` or an equivalent, this is
    the loudest warning in
    [`../playbooks/wrapper-integration.md`](../playbooks/wrapper-integration.md),
    and it is why `/register` validates `pid == getpgid(pid)` and refuses
    otherwise.
  - Killing a bare PID and leaving descendants is a **defect**, not a partial
    success.
  - Windows support would need a job-object design and a different Stop
    contract. It is out of scope, not deferred.
- **Alternatives considered:** `SIGTERM` then `SIGKILL` — worth revisiting so a
  suite can clean up its own artifacts, and not chosen now because the note
  specifies `SIGKILL` and a grace period is a timer with a policy. A
  cross-platform process library — rejected: it would forfeit exactly the
  group-kill semantics that make the product work.

## ADR-001 — Go, one static binary, standard library only

- **Status:** accepted
- **Date:** 2026-09-13
- **Context:** The tool has to be installed on every developer machine that runs
  local E2E tests, and it exists to *remove* local setup friction. A runtime
  prerequisite or a dependency tree would reintroduce it.
- **Decision:** Go, compiled to a single static binary with no runtime
  dependencies. The **standard library is the whole toolbox**: `net/http` for
  the API and the Dashboard, `syscall` for process control, `encoding/json` for
  the wire format, and **`embed` for the Dashboard's assets**, which is what lets
  a web UI ship inside one file.
- **Consequences:**
  - `go build` produces the deliverable; there is no bundler, no `npm install`,
    and no asset directory to install alongside the binary.
  - A third-party dependency needs a justification in the plan's Complexity
    Tracking table before use (the constitution's Principle I). `go.mod` is
    expected to list none.
  - **No web framework and no SSE library.** SSE is three headers, a
    `fmt.Fprintf` and a `Flush`; a router for five routes is
    `http.ServeMux`.
  - The Dashboard's JavaScript is hand-written and small enough to read, because
    there is no build step to hide it behind.
- **Alternatives considered:** Rust — equally capable, and Go's `net/http` plus
  `embed` is a shorter path to this specific shape. Node or Python — rejected:
  both are the runtime prerequisite this decision exists to avoid.
