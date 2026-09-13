# Feature Specification: Serialize Local E2E Runs Behind a Single Lock

**Feature Branch**: `001-serialize-e2e-runs` *(spec directory; no git branch was created — no `before_specify` hook is registered)*

**Created**: 2026-09-13

**Status**: Draft

**Input**: User description: "Serialize local end-to-end test runs on one developer machine behind a single exclusive lock held by a background daemon, so that multiple repository clones cannot run E2E suites concurrently. Three prioritized, independently shippable user stories: (P1) the lock itself; (P2) a read-only web dashboard; (P3) manual job termination."

> **This feature is the whole product.** Nothing exists yet, so the three stories
> below are the first three increments of trainsty rather than additions to it.
>
> **Binding context that is already decided and must not be re-litigated here:**
> `.specify/memory/constitution.md` (five principles),
> `knowledge/architecture/adr.md` (eleven ADRs),
> `knowledge/conventions/api.md`, `knowledge/conventions/dashboard.md`,
> `knowledge/domains/ubiquitous-language.md` (binding vocabulary), and
> `knowledge/product/local-ci-scheduler-requirements.md` (the read-only
> requirements mirror). Where a requirement below restates one of the mirror's,
> the row in `knowledge/product/decisions.md` names this file.

## User Scenarios & Testing *(mandatory)*

Actors: the **Developer**, who runs suites and watches the Dashboard; and the
**Runner**, the local CI script in each repository that acts on the Developer's
behalf. The Runner is not part of this feature — it lives in the repositories being
tested — but it is the only client of the Lock.

### User Story 1 - Queue my suite behind whoever is already running (Priority: P1)

A Developer has four clones of a repository on one laptop. They start an E2E suite
in one clone and, a minute later, another in a second clone. Today the second run
fails with port conflicts and a thrashed machine. With this story, the second run
**waits** — quietly, in its own terminal — until the first finishes, then starts
automatically. When either Developer interrupts a run with `Ctrl+C`, or closes the
terminal, or the process is killed outright, the next run in line starts without
anyone intervening.

**Why this priority**: This is the entire value of the product. It is also the only
story that can ship alone and be useful: with no dashboard and no stop button, a
Developer who wires their Runner stops getting collisions. Stories 2 and 3 make the
Lock observable and interruptible; this one makes it exist.

**Independent Test**: Wrap each clone's suite in the provided command — one line per
repository — then start two of them seconds apart. The first runs; the second blocks
with no output of its own and then runs to completion after the first exits. Repeat
while ending the first run in each of three ways — clean exit, `Ctrl+C`, and killed
outright — and confirm the second always starts. Delivers collision-free local E2E
with nothing else built.

**Acceptance Scenarios**:

1. **Given** no suite is running, **When** a Runner requests the Lock, **Then** it is
   granted immediately and the suite's own output appears in its own terminal,
   unchanged and uncaptured.
2. **Given** one suite holds the Lock, **When** a second Runner requests it, **Then**
   the second blocks without failing, without timing out, and without polling output.
3. **Given** two suites are waiting, **When** the Lock is released, **Then** the one
   that asked **first** is granted it.
4. **Given** a suite holds the Lock, **When** its Runner exits normally, **Then** the
   Lock is released and the next waiter starts within two seconds.
5. **Given** a suite holds the Lock, **When** the Developer presses `Ctrl+C` or closes
   the terminal, **Then** the Lock is released and the next waiter starts within two
   seconds, with no manual intervention.
6. **Given** a suite holds the Lock, **When** its Runner is killed outright with no
   chance to say so, **Then** the Lock is released within five seconds and the next
   waiter starts.
7. **Given** a suite has been waiting for thirty minutes, **When** the Lock finally
   frees, **Then** it is granted and the wait is not reported as an error, a timeout,
   or a dropped connection.
8. **Given** a Runner's connection drops and it asks again, **When** it re-requests
   the Lock, **Then** it keeps its original place in line rather than going to the back.
9. **Given** a Runner asks for the Lock identifying a process that is not the leader
   of its own process group, **Then** the request is **refused with a message naming
   the problem**, because granting it would make Story 3's termination unsafe.
10. **Given** the scheduler is not running, **When** a Runner asks for the Lock, **Then** it
    receives a distinguishable failure — *cannot reach the scheduler*, not *the
    scheduler says no* — so the Runner can choose its own fallback.
11. **Given** the Developer runs the status command, **Then** it prints what holds
    the Lock and how many are waiting, and exits non-zero only when the scheduler is not
    reachable.
12. **Given** a suite is running, **When** the Developer shuts the scheduler down,
    **Then** they are **warned and asked to confirm**, told which repository is
    affected, and — on confirmation — the suite keeps running while the Lock is
    dropped.
13. **Given** a Developer wraps their existing suite command, **When** they run it,
    **Then** the suite's output is byte-for-byte what it was unwrapped, and the
    command exits with the suite's own exit status — a failing suite still fails, and
    a passing one still passes.
14. **Given** the scheduler is not running, **When** a Developer runs their wrapped
    suite, **Then** the suite runs anyway and they are told **once** that it was not
    scheduled.
15. **Given** a wrapped suite is interrupted with `Ctrl+C`, **When** the Developer
    checks, **Then** the Lock is free and nothing the suite started survives — without
    the Developer having written any cleanup themselves.

---

### User Story 2 - See what is holding the lock (Priority: P2)

A Developer's suite has been waiting for six minutes. They want to know whether
something is genuinely running or whether the scheduler is stuck, and they want to
know it at a glance from a second monitor rather than by reading a terminal.

**Why this priority**: The Lock is invisible by nature, and an invisible lock is one
Developers stop trusting the first time a wait feels too long. This story is
read-only and cannot break Story 1, which is why it comes second rather than being
bundled into it.

**Independent Test**: With one suite running and two waiting, open the dashboard. It
shows the running repository with a ticking elapsed time and the two waiters in the
order they will be granted. Stop the scheduler; the page says it cannot reach the
scheduler rather than showing an empty queue. Delivers observability with no new
control surface.

**Acceptance Scenarios**:

1. **Given** a suite holds the Lock, **When** the Developer opens the dashboard,
   **Then** it shows that repository and an **elapsed time that advances**, not a
   static status word.
2. **Given** three Runners are waiting, **Then** the dashboard lists them **in the
   order they will be granted**, and never shows the same Runner twice.
3. **Given** nothing holds the Lock and nothing waits, **Then** the dashboard says so
   explicitly.
4. **Given** the scheduler is not running or stops while the page is open, **Then** the
   dashboard says **it cannot reach the scheduler** — visibly distinct from "no jobs"
   — and keeps trying without the Developer reloading.
5. **Given** a repository label contains characters that could be interpreted as
   markup, **When** the dashboard renders it, **Then** it is displayed as text and
   changes nothing about the page.
6. **Given** the page has been open for an hour, **Then** it is still current and has
   not accumulated state that disagrees with the scheduler.

---

### User Story 3 - Kill a run that must not finish (Priority: P3)

A Developer sees a suite that has been running for forty minutes and is plainly
hung. They need it gone — along with the headless browsers it started, which
between them hold the ports and the memory the next run needs — and they need the
queue to move on.

**Why this priority**: It is the only destructive action in the product, and it is
worth building after the Lock's semantics are proven rather than alongside them. It
is also the story that most needs the other two: without Story 1 there is nothing
to stop, and without Story 2 there is nowhere to press the button.

**Independent Test**: Start a suite that spawns several child processes, confirm
they exist, press Stop, then confirm **no** child survived and the next waiter was
granted. Delivers recovery from a hung run without restarting the scheduler.

**Acceptance Scenarios**:

1. **Given** a suite holds the Lock and has started several child processes, **When**
   the Developer presses Stop and confirms, **Then** the suite **and every one of its
   children** are terminated.
2. **Given** the suite has been terminated, **Then** the Lock is released and the next
   waiter is granted it.
3. **Given** the Developer presses Stop, **Then** they are asked to confirm first,
   and the confirmation names the repository and states that the suite dies
   immediately.
4. **Given** nothing holds the Lock, **When** Stop is triggered anyway — a stale page,
   a double click — **Then** nothing is terminated and the Developer is not shown a
   failure.
5. **Given** the suite finished on its own between the page's last refresh and the
   click, **Then** the Developer's click does not terminate whatever holds the Lock
   **now**.
6. **Given** a suite has been stopped, **When** the Developer checks for surviving
   test processes, **Then** there are none.

### Edge Cases

- **A Runner identifies a process it does not lead.** Refused (US1 §9). The reason
  this is a hard refusal rather than a warning: terminating a group the Runner does
  not lead can reach the Developer's own shell.
- **Two Runners ask at the same instant.** One is granted, one waits. Never both.
- **A Runner asks twice without releasing.** The second request does not create a
  second place in line, and does not release the first.
- **A Runner releases twice**, or releases after the Lock has already been freed for
  it. Accepted silently — never a failure, and never a release of whoever holds the
  Lock now.
- **The scheduler is already running** when the Developer starts it again. Refused
  with a message that distinguishes *already running* from *something else is using
  that address*.
- **The Developer shuts the scheduler down mid-suite.** The suite survives and
  becomes invisible to scheduling; the Developer is warned first (US1 §12).
- **The scheduler dies mid-suite.** Whatever was running keeps running; the next
  request is granted immediately. Visible, not silent.
- **A process identifier is reused** by the operating system after a Runner is killed.
  Bounded by the primary liveness signal — recorded in `ADR-008` with its residual
  risk accepted.
- **A wait longer than any reasonable network timeout** — thirty minutes or more
  (US1 §7).

## Requirements *(mandatory)*

> **Two numbering namespaces, deliberately.** `FR-0NN` below are **this feature's**
> requirements. `FR-1`–`FR-7` and `TS-1`–`TS-6` in
> `knowledge/product/decisions.md` are the **requirements note's** register. Always
> cite the latter with its file. They are not the same identifiers.

### Functional Requirements

**The lock (US1)**

- **FR-001**: The system MUST grant exclusive permission to run a suite to **at most
  one** requester at a time.
- **FR-002**: The system MUST grant permission in the order requests arrived.
- **FR-003**: A waiting requester MUST block until granted, for an unbounded
  duration, without failing and without being required to ask repeatedly.
- **FR-004**: The system MUST NOT run, supervise, capture, buffer or reformat the
  suite or its output. The requester keeps its own terminal.
- **FR-005**: The system MUST release permission when the holder says it is finished.
- **FR-006**: The system MUST release permission when the holder's connection ends —
  including when the Developer interrupts it or closes the terminal — within two
  seconds, with no Developer action.
- **FR-007**: The system MUST release permission when the holder's process no longer
  exists, within five seconds, even if its connection has not ended.
- **FR-008**: Releasing MUST be safe to attempt more than once, and MUST never revoke
  permission from a **later** holder.
- **FR-009**: A failure while cleaning up after a holder MUST NOT prevent the release.
- **FR-010**: The system MUST refuse a request that identifies a process which does
  not lead its own process group, and MUST say why.
- **FR-011**: A requester that reconnects MUST keep its position in the queue.
- **FR-012**: The system MUST validate every request before it enters the queue, so a
  malformed request can never become a holder.
- **FR-013**: Developers MUST be able to start the scheduler so that it keeps running
  after the starting terminal is closed, and MUST be told where its output goes.
- **FR-013a**: The scheduler MUST append its records to a per-user file in the
  location the operating system designates for logs, and MUST report that path when
  started.
- **FR-013b**: The scheduler MUST NOT read that file, or any other file, when it
  starts. Nothing it writes may influence a later run — a record it read back could
  describe a holder that no longer exists.
- **FR-014**: Developers MUST be able to stop the scheduler; when a suite is running,
  stopping MUST warn, name the repository, require confirmation, and leave the suite
  running.
- **FR-015**: Developers MUST be able to ask what holds permission and how many are
  waiting, from the terminal.
- **FR-016**: Developers MUST be able to open the dashboard with one command.
- **FR-017**: Developers MUST be able to list the available commands and read one
  line saying what the product is for.
- **FR-018**: A requester MUST be able to distinguish *the scheduler is unreachable*
  from *the scheduler refused* so it can choose its own fallback.
- **FR-019**: Starting a second scheduler MUST fail immediately with a message that
  distinguishes *already running* from *that address is in use by something else*.
- **FR-020**: The system MUST record every grant, release and refusal, with the cause,
  and MUST NOT record the contents of anything being tested.

**Observability (US2)**

- **FR-021**: Developers MUST be able to see, in a browser, what holds permission and
  the full queue in grant order.
- **FR-022**: The dashboard MUST show **how long** the current holder has held
  permission, advancing as it runs, rather than a static status.
- **FR-023**: The dashboard MUST distinguish *nothing is running* from *the scheduler
  cannot be reached*, and MUST recover from the latter without a reload.
- **FR-024**: The dashboard MUST derive everything it shows from the scheduler's
  latest answer, holding no state of its own and showing no action as done before the
  scheduler reports it.
- **FR-025**: The dashboard MUST render every requester-supplied value as text, with
  no possibility of it altering the page.
- **FR-026**: The dashboard MUST never show the same requester twice.

**Termination (US3)**

- **FR-027**: Developers MUST be able to terminate the current holder from the
  dashboard.
- **FR-028**: Terminating MUST end the holder **and every process it started**, so
  that nothing it spawned survives to hold ports or memory.
- **FR-029**: Terminating MUST release permission and grant it to the next waiter.
- **FR-030**: Terminating MUST require confirmation, naming the repository and stating
  that the suite ends immediately.
- **FR-031**: Terminating when nothing holds permission MUST do nothing and MUST NOT
  be presented as a failure.
- **FR-032**: A termination request MUST NOT be able to name its target, so that a
  stale view cannot end a suite that started after it was rendered.
- **FR-033**: The system MUST refuse termination and shutdown requests that did not
  originate from the dashboard on this machine. Specifically it MUST be unreachable
  from other machines, MUST refuse requests shaped as ordinary page navigations, MUST
  require a request shape a cross-origin page cannot produce, and MUST refuse a
  request declaring a foreign origin.
- **FR-033a**: No shared secret is required. Together the refusals in FR-033 close
  the browser route completely; what remains reachable is another process running as
  the same Developer on the same machine, **which could already end their suite
  directly**. This residual is accepted deliberately rather than overlooked, and
  reversing it is a security decision with a record, not a configuration change.

**Scope**

**Integration (US1) — the Runner ships**

- **FR-034**: The product MUST provide a command that takes a Developer's suite as
  its argument and performs the whole protocol around it: acquire, run, release. A
  repository's integration MUST be one line, requiring no knowledge of the
  scheduler's interface.
- **FR-035**: That command MUST place the suite in a process group it leads, so that
  the suite and everything it starts can be terminated as one unit (FR-028) — and so
  that FR-010's refusal becomes **unreachable in normal use** rather than a trap a
  Developer must avoid by hand.
- **FR-036**: That command MUST release the Lock on **every** way its suite can end —
  success, failure, interruption, or termination — and MUST NOT report a release
  failure as a suite failure.
- **FR-037**: That command MUST pass the suite's output through untouched and MUST
  exit with the suite's own exit status, so that wrapping a suite changes neither
  what a Developer sees nor what their tooling concludes.
- **FR-038**: When the scheduler is unreachable, that command MUST run the suite
  anyway and MUST say, once, that the run was not scheduled. A missing scheduler is a
  lost convenience, not a reason to block work — and a silently unscheduled run is
  indistinguishable from a scheduled one until two collide.
- **FR-039**: That command MUST identify the repository it was invoked from, for the
  Developer reading the dashboard, without that identity affecting scheduling.

### Key Entities

Defined normatively in `knowledge/domains/ubiquitous-language.md`; summarised here
for readers of this spec only.

- **Lock**: the single exclusive right to run a suite on this machine. Held by at
  most one Job.
- **Job**: the holder of the Lock, and the suite it is running. At most one exists.
- **Waiter**: a requester in the queue that does not hold the Lock.
- **Queue**: the waiters, in arrival order, identified one-per-process.
- **Registration**: one requester's open request for the Lock, held for the whole
  wait and doubling as the signal that the requester is still alive.
- **Process Group**: the Job and every process it started, as one terminable unit.
- **Runner**: whatever requests the Lock, waits, runs the suite and releases. **This
  feature ships one** (FR-034–FR-039), so in normal use the Runner is the wrapping
  command rather than a script each repository writes. A hand-written Runner remains
  possible, and FR-010 exists for it.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Across 20 consecutive runs started from two or more clones with
  overlapping timing, **zero** pairs of suites execute concurrently.
- **SC-002**: A Developer who interrupts or closes a run frees the machine for the
  next one within **2 seconds**, with **zero** manual recovery steps.
- **SC-003**: A run killed outright frees the machine within **5 seconds**.
- **SC-004**: A suite that waits **30 minutes or more** still starts and completes,
  with no wait-related failure.
- **SC-005**: After terminating a run, **zero** processes it started remain.
- **SC-006**: A Developer can determine what is running and how long it has been
  running within **5 seconds** of looking, without using a terminal.
- **SC-007**: **Zero** occurrences, across a month of daily use, of a machine left
  blocked by a run that had already finished.
- **SC-008**: A Developer integrating a repository for the first time gets a
  correctly queued run by changing **one line** of their local CI script, and needs
  no knowledge of the scheduler's interface to do it.
- **SC-010**: **Zero** integrations can reach the state where terminating a run
  signals the wrong process group, when the provided wrapping command is used.
- **SC-009**: The scheduler's own footprint is negligible while idle — indistinguishable
  from an idle background process in ordinary system monitoring.

## Assumptions

- **One machine, one Developer, cooperating clones.** The scheduler arbitrates
  between processes started by the same person; it is not a multi-user or
  multi-machine scheduler and defends against mistakes, not adversaries.
- **A suite that holds the Lock is trusted to release it**, and every non-cooperative
  ending is covered by FR-006 and FR-007 rather than by policing the suite.
- **Repositories are willing to change one line of their local CI script.** Nothing
  works without a Runner, and FR-034 makes the product supply it rather than asking
  each repository to write one.
- **Depth one is correct**, permanently. Any concurrency above one is a constitution
  amendment, not a setting (`knowledge/architecture/adr.md` → ADR-006).
- **The repository label is descriptive only** and gates nothing
  (`knowledge/architecture/adr.md` → OD-3).
- **Nothing is remembered across restarts**, by decision (`knowledge/data/ddr.md` →
  DDR-001). A restart means the Lock is free, and a suite that was running becomes
  invisible to scheduling.
- **Unix only** (Linux and macOS), because terminating a whole process group is the
  mechanism FR-028 depends on (`knowledge/architecture/adr.md` → ADR-002).
- **Stories ship in priority order.** US2 and US3 each assume US1 exists; US3
  assumes US2 provides the surface its control lives on.
