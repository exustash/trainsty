# Feature Specification: `wrap` Starts the Daemon When None Is Reachable

**Feature Branch**: `docs/wrap-starts-daemon-spec`

**Created**: 2026-11-07

**Status**: Draft — implementation deferred

**Input**: User description: "trainsty wrap starts the Daemon when none is
reachable, instead of running the suite unserialized. Concurrent starts are
arbitrated by the port bind; if the Daemon cannot be started, fall back to
today's run-anyway behaviour. No API change, no flag."

**Decision record**: `knowledge/architecture/adr.md` → **ADR-014** (proposed).
**Amends**: `specs/001-serialize-e2e-runs/spec.md` → **FR-038**.

> Requirements continue the repository's sequence: 001 ends at FR-039 and
> SC-010; 002 (Runner-reported Steps, on its own branch) holds FR-040–FR-051
> and SC-010–SC-014. This feature starts at FR-052 and SC-015.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A suite is serialized even when nobody started the Daemon (Priority: P1)

A Developer opens a laptop, never runs `trainsty start`, and launches a local
CI script whose suite is wrapped with `trainsty wrap`. Today that run is
unscheduled and collides with the next clone's suite. With this feature the
first wrapped run brings the Daemon up, takes the Lock, and every later run
queues behind it as usual.

**Why this priority**: it is the whole feature. A scheduler that is only
present when someone remembered to start it fails exactly when it is needed.

**Independent Test**: with no Daemon running, run one wrapped suite; assert the
Daemon is now reachable, the run held the Lock, and the suite's output and
exit status are unchanged.

**Acceptance Scenarios**:

1. **Given** no Daemon is running, **When** a Developer runs
   `trainsty wrap -- <suite>`, **Then** a Daemon is started, the run registers
   with it and is granted the Lock, and `wrap` says once that it started the
   Daemon and where the Daemon logs.
2. **Given** the run above has finished, **When** the Developer runs
   `trainsty status`, **Then** the Daemon is still running with no Job — it
   outlives the `wrap` that started it.
3. **Given** a Daemon is already running, **When** a wrapped suite runs,
   **Then** behaviour is exactly as today: no start attempt and no extra
   output.

---

### User Story 2 - Two suites launched at once still run one at a time (Priority: P1)

A Developer starts the local CI in two clones within the same second, with no
Daemon running. Both `wrap`s find no Daemon and both try to start one.

**Why this priority**: the race is the obvious objection to a client that
starts a daemon (`knowledge/playbooks/wrapper-integration.md` §6). If it ends
in two Daemons, or in one run left unserialized, the feature is worse than
today's warning.

**Independent Test**: with no Daemon running, launch N wrapped suites
concurrently; assert exactly one Daemon exists afterwards and the suites never
overlapped.

**Acceptance Scenarios**:

1. **Given** no Daemon is running, **When** several wrapped suites start
   concurrently, **Then** exactly one Daemon ends up running, every run
   registers with it, and the runs execute one at a time.
2. **Given** a `wrap` lost the race, **When** its own start attempt fails
   because another Daemon now holds the port, **Then** it registers with that
   Daemon and does **not** report the run as unserialized.

---

### User Story 3 - A Daemon that cannot start never blocks the suite (Priority: P2)

The port is held by an unrelated program, or the start fails for any other
reason. The Developer's suite must still run.

**Why this priority**: FR-038's promise — a missing scheduler costs a
convenience, not the work — survives this feature unchanged. Only the trigger
for the fallback narrows.

**Independent Test**: occupy the port with a non-trainsty listener, run a
wrapped suite; assert the suite ran, its exit status passed through, and `wrap`
said once why the run is not serialized.

**Acceptance Scenarios**:

1. **Given** another program holds the port, **When** a wrapped suite runs,
   **Then** the suite runs unserialized, and `wrap` says once that it is not
   serialized and that the port is held by another program, naming the
   command that shows the holder.
2. **Given** the Daemon cannot be started for any other reason, **When** a
   wrapped suite runs, **Then** the suite runs unserialized and `wrap` says
   once why, with a pointer to the Daemon's log where one exists.
3. **Given** either fallback, **When** the suite exits with status S, **Then**
   `wrap` exits with S.

---

### Edge Cases

- **The Developer just ran `trainsty stop`.** The next wrapped suite starts a
  new Daemon. Stopping the Daemon therefore lasts until the next wrapped run;
  `trainsty stop`'s output must say so, so the Developer is not surprised.
- **A Daemon is shutting down while `wrap` checks.** `wrap` may see it as
  reachable, then lose the Registration. It must end in one of the two defined
  outcomes — registered with a live Daemon, or the stated fallback — never a
  hang.
- **The Daemon starts but is slow to answer.** The start attempt is bounded;
  past the bound, `wrap` takes the fallback. A slow start must not add an
  unbounded delay to the suite.
- **The Runner is in a container that cannot reach the host's Daemon**
  (`knowledge/playbooks/podman.md`). `wrap` will start a Daemon inside the
  container, which serializes nothing outside it. The start message names the
  Daemon it started, so this is visible rather than silent; the playbook must
  say so.
- **trainsty's own test suites.** A test that runs `wrap` with no Daemon would
  now start a real Daemon on the machine's port and leave it running. Tests of
  the fallback must be able to exercise it without starting one, and any test
  that does start one must stop it.
- **A hand-written Runner.** Unaffected: this changes `wrap` only. A repository
  that does not want a Daemon started for it writes its own Runner.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-052**: When no Daemon is reachable, `trainsty wrap` MUST start one —
  detached, outliving `wrap`, and logging where `trainsty start` would log —
  and then register with it as usual.
- **FR-053**: When `wrap` starts a Daemon, it MUST say so once, naming where the
  Daemon logs. When a Daemon was already running, `wrap` MUST add no output.
- **FR-054**: Concurrent `wrap`s that each find no Daemon MUST end with exactly
  one Daemon running and every one of them registered with it. Losing the race
  to start the Daemon MUST NOT be reported as a failure or as an unserialized
  run.
- **FR-055**: The start attempt MUST be bounded in time. If no Daemon is
  reachable once the bound passes, `wrap` MUST fall back to FR-038's behaviour.
- **FR-056**: If the Daemon cannot be started, `wrap` MUST run the suite anyway
  and MUST say, once, that the run is not serialized **and why** —
  distinguishing a port held by another program (naming the command that shows
  the holder) from any other failure (pointing at the log where one exists).
- **FR-057**: `wrap` MUST pass the suite's output through untouched and exit
  with the suite's own status on every path this feature adds, as FR-037
  requires today.
- **FR-058**: This feature MUST NOT change the Daemon, the HTTP API, or the
  CLI surface: no new endpoint, parameter, flag, subcommand, or configuration.
- **FR-059**: `trainsty stop` MUST tell the Developer that the next wrapped run
  will start the Daemon again.

**FR-038 is amended, not removed**: "when the scheduler is unreachable" now
means "when it is unreachable **and cannot be started**". Its run-anyway and
say-once obligations stand.

### Key Entities

- **Daemon** — unchanged. Now also started on demand by a Runner, after which
  it is an ordinary Daemon: stoppable with `trainsty stop`, visible to
  `trainsty status`.
- **Runner** (`trainsty wrap`) — gains one responsibility: ensure a Daemon
  exists before registering. It stays a client (ADR-012).

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-015**: With no Daemon running, 100% of wrapped runs are serialized
  unless the port is held by another program or the start fails.
- **SC-016**: With no Daemon running, 10 wrapped suites launched within one
  second never overlap, and exactly one Daemon is running afterwards, in 20 of
  20 repetitions.
- **SC-017**: When the Daemon cannot be started, the suite begins within 3
  seconds of `wrap` being invoked.
- **SC-018**: When a Daemon is already running, a wrapped run's behaviour and
  output are identical to today's.
- **SC-019**: A Developer who never runs `trainsty start` sees no "NOT
  serialized" message in ordinary use.

## Assumptions

- `trainsty start`'s existing detached start — its log location (DDR-002) and
  its check that the Daemon really came up — is the mechanism `wrap` reuses.
  No second way of starting the Daemon is introduced.
- The port bind remains the only single-instance mechanism (ADR-011). No PID
  file, lock file or other persistence is added (DDR-001).
- Spawning the Daemon from `wrap` is consistent with Principle II: `wrap` is a
  client (ADR-012), and process-spawning code stays confined to `runner/` and
  `daemonctl/`, as the local CI gate already enforces.
- There is no opt-out flag. Configuration is forbidden by the constitution, and
  a hand-written Runner remains the opt-out.
- On acceptance, the documents that restate today's behaviour move with the
  code: `CLAUDE.md` (The CLI), `knowledge/playbooks/wrapper-integration.md`
  §6, `README.md`, `knowledge/playbooks/podman.md`, and ADR-012's consequence
  about the unreachable Daemon.
- Out of scope: starting the Daemon at login (launchd / systemd), and any change
  to what a hand-written Runner does.
