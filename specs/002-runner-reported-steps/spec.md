# Feature Specification: Runner-reported Steps on the Dashboard

**Feature Branch**: `002-runner-reported-steps` *(spec directory; no git branch was created — no `before_specify` hook is registered)*

**Created**: 2026-11-05

**Status**: Draft — deferred until `teneerai.web` is idle

**Input**: User description: "specs/002-runner-reported-steps/plan.md" — the
Dashboard should show which step of the running Job is executing and for how long,
like a CI provider's step list, but only the steps running now rather than a
history.

> **Binding context, already decided:** `.specify/memory/constitution.md`,
> `knowledge/architecture/adr.md` (ADR-003: the Daemon never sees test output;
> ADR-005: the Dashboard polls; ADR-012: `wrap` is the shipped Runner),
> `knowledge/security/sdr.md` → SDR-001, and the glossary. The design sketch this
> spec was written from is [`plan.md`](plan.md) in this directory.
>
> **Numbering continues from feature 001** (FR-040 onward, SC-011 onward), because
> the code cites requirement numbers bare (`FR-037`), and a second FR-001 would
> make every citation ambiguous.

## Clarifications

### Session 2026-11-05

- Q: Several Steps can run at once; what does the Job's row show? → A: Every
  running Step, one line each with its own elapsed time, oldest first, capped at
  16 (User Story 2, FR-044–FR-046, SC-012).
- Q: What exit status does the Step command return when the announcement is not
  recorded? → A: Always 0, with one line on stderr when it was not recorded
  (FR-049).
- Q: Does the Daemon log Step announcements? → A: Only refused ones, with the
  reason. Accepted and stale announcements are not logged (FR-051).
- Q: The requirements review (`checklists/api.md`, 27 items) found two conflicts
  and several gaps. → A: Resolved in this spec: names are limited in bytes; the
  1-second bound is per announcement; restarts are allowed at the cap; Steps
  survive a re-registration; `TRAINSTY_PID` is contract; FR-052 was added.

### Session 2026-11-08

- Q: Does an unserialized `wrap` clear a `TRAINSTY_PID` it inherited? → A: No.
  `wrap` only sets it on its own Grant and never removes one (FR-048).
- Q: Does SC-011's 3-second bound cover a Step removed by a Release? → A: No. It
  covers announced starts and ends; a Step removed by a Release disappears with
  its Job, inheriting the Job row's lag (up to ~7 s, ADR-005).
- Q: Which success criterion checks FR-052? → A: SC-013, extended: Grants and
  Releases happen in the same order and at the same moments with and without
  announcements.
- Q: Two parallel stages announce the same name; what is shown? → A: One line;
  the first end removes it. Distinct stages need distinct names (Edge Cases).

## User Scenarios & Testing *(mandatory)*

Actors: the **Developer**, watching the Dashboard; and the **Runner**, the local CI
script in a repository. The Runner is the only source of Step information:
trainsty learns what a Step is only because the Runner says so.

### User Story 1 - See which Step the Job is on, and for how long (Priority: P1)

A Developer starts a long local CI run in one clone and opens the Dashboard from
another. Today the row says only that `teneerai.web-dbchecks` has held the Lock for
6 minutes. Is it nearly done, or stuck in the first stage? With this story, the
Job's row also names the Step running now, such as *Unit tests (with coverage)*,
with that Step's own elapsed time. When the Step ends and the next begins, the row
shows the next one. There is no list of finished Steps.

**Why this priority**: This is the whole request. A Developer deciding whether to
wait, go for coffee, or press Stop needs to know where the run is, not only how
long it has taken.

**Independent Test**: Run, under the shipped Runner, a script that announces three
Steps in sequence, each lasting a few seconds. Watch the Dashboard: each Step
appears while it runs, with a ticking time, and disappears when it ends. After the
run, the Job row is gone as before.

**Acceptance Scenarios**:

1. **Given** a Job whose Runner has announced the start of a Step, **When** the
   Dashboard next refreshes, **Then** the Job's row shows that Step's name and its
   elapsed time since it started.
2. **Given** a Step on display, **When** the Runner announces that it ended,
   **Then** it no longer appears after the next refresh.
3. **Given** a Job whose Runner announces no Steps, **When** the Dashboard shows
   it, **Then** the row looks as it does today, with no empty or placeholder Step
   text.
4. **Given** a Job with a Step on display, **When** the Lock is released by any of
   the five release paths, **Then** the Step disappears with the Job.

---

### User Story 2 - See every Step running at once (Priority: P2)

The `teneerai.web` CI script runs its stages in parallel by default: linting, unit
tests and security scanners at the same time. A single "current step" would show
whichever stage announced itself last, which is wrong. With this story, the Job's
row shows **each** running Step on its own line, each with its own elapsed time,
oldest first.

**Why this priority**: Without it, Story 1 misleads on the Developer's main
repository. It is separate because a strictly sequential Runner gets full value
from Story 1 alone.

**Independent Test**: Run a script that starts two Steps at once, ends the first,
then ends the second. The Dashboard shows two lines, then one, then none.

**Acceptance Scenarios**:

1. **Given** two Steps running at once, **When** the Dashboard refreshes, **Then**
   both appear, oldest first, each with its own elapsed time.
2. **Given** a Step already running, **When** the Runner announces a Step with the
   same name, **Then** one line remains and its time restarts.

---

### User Story 3 - Report Steps from any Runner, at no risk to the run (Priority: P3)

Most repositories use the shipped `trainsty wrap`. Some have hand-written Runners,
and every repository's scripts will at some point run on a machine with no
scheduler, or with an older one. With this story, a script announces a Step with
one command, which does nothing harmful in any of those situations. A hand-written
Runner can announce Steps with an ordinary HTTP request.

**Why this priority**: Step reporting is a convenience. If it can fail a CI run,
developers will remove it, and the API's clients are scripts nobody here can
upgrade. This story is what makes Stories 1 and 2 safe to adopt.

**Independent Test**: Run the same Step-announcing script (a) outside any Runner,
(b) under the Runner with no scheduler running, and (c) under the Runner against a
scheduler that predates this feature. Each run's result and exit status are the
same as without the announcements.

**Acceptance Scenarios**:

1. **Given** a script not running under the Runner, **When** it announces a Step,
   **Then** nothing is reported and the announcement succeeds silently.
2. **Given** a script under the Runner with no scheduler reachable, **When** it
   announces a Step, **Then** the script's own result is unaffected.
3. **Given** a hand-written Runner that holds the Lock, **When** it announces a
   Step with its own process identifier, **Then** the Step appears on the
   Dashboard.

---

### Edge Cases

- **A late announcement from a finished run.** A Step announced by a Runner that no
  longer holds the Lock (finished, stopped or still waiting in the Queue) is
  ignored and never appears on the successor's row.
- **A Step that never ends.** The Runner crashes or forgets to announce the end.
  The Step stays on display until the Job ends, then disappears with it. Its
  ticking time is what makes this visible.
- **An end announced for a Step that is not running.** Ignored, and not an error.
- **Too many Steps at once.** Beyond the cap (16 running at once), starting a
  **new** name is refused. Steps already on display are kept. Restarting a name
  that is already running is allowed at the cap, because it adds nothing.
- **A hostile or broken Step name.** Over-long names, or names containing control
  characters or newlines, are refused. Markup in a name is displayed as literal
  text and never interpreted.
- **Names that differ only in whitespace.** Names are compared byte for byte, with
  no trimming, so `Lint` and `Lint ` are two different Steps.
- **Two parallel stages with the same name.** The name is the Step's identity, so
  they share one line, and the first end removes it while the other may still
  run. Starts and ends are not counted. A Runner that wants both shown gives them
  distinct names.
- **The same Runner re-registers** (ADR-007: a re-registration by the Job's own pid
  is granted at once). The Job is unchanged, so its running Steps are kept.
- **A suite still running after a Shutdown** (ADR-009) announces Steps to a later
  Daemon. That Daemon has no Job for its pid, so every announcement is a no-op.
- **An announcement during a Lock hand-over** attaches to the Job that holds the
  Lock at that instant, or to none. It never attaches to both, and never to the
  wrong one.
- **A cross-site page tries to announce a Step.** Refused, the same way every other
  state-changing request is refused (SDR-001's floor).
- **Steps announced while waiting.** A Runner still in the Queue holds no Job, so
  its announcements are ignored. Steps describe a held Lock only.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-040**: The Runner MUST be able to announce the start and the end of a named
  Step of the Job it is running.
- **FR-041**: An announcement MUST identify the Runner by the same process
  identifier it registered with. One whose identifier is not the current Job's
  MUST be ignored and MUST NOT affect any Job. The match and the change MUST happen
  as one step with respect to a Lock hand-over, so an announcement reaches exactly
  one Job or none. There is no "whichever Job holds the Lock" form: `/release` has
  one only for hand-written Runners written before identity checks existed, and an
  endpoint that is new has no such clients to honour.
- **FR-042**: An announcement that changes nothing MUST be answered as a success,
  not an error, so a Runner's normal path prints no failure. That covers a stale
  identifier, no Job, and ending a name that is not running. All of these get the
  same "not recorded" answer.
- **FR-043**: Step names MUST be validated before they are kept: non-empty, at most
  128 **bytes**, and free of control characters under the same rule as `repo`
  (U+0000–U+001F and U+007F). Names are kept and compared exactly, with no
  trimming. An invalid request is refused with a stable, machine-readable reason.
  When several parameters are invalid, the reported reason follows a fixed order:
  identifier, then state, then name. The codes are listed in
  [contracts/http-api.md](contracts/http-api.md); adding one is a contract change.
- **FR-044**: The scheduler MUST keep only the Steps running now, with each one's
  name and start time, at most 16 per Job. A new name beyond 16 is refused;
  restarting a running name is not. It keeps no history of ended Steps. A Job's
  Steps survive a re-registration by its own pid, and end when the Job ends by any
  of the five release paths: the Runner's release, a dropped Registration, the
  Liveness Probe, a Stop, or a Shutdown.
- **FR-045**: The status report MUST include the Job's running Steps, oldest
  first, each with its name, start time and elapsed seconds. It MUST report an
  empty list (never an absent one) when there are none. The change is additive:
  no existing field is renamed, retyped or removed.
- **FR-046**: The Dashboard MUST show each running Step on the Job's row with its
  own advancing elapsed time, and MUST show nothing extra when there are none.
- **FR-047**: Step announcements MUST get the same protection as every other
  state-changing request: one sent by a web page in the Developer's browser is
  refused (SDR-001's floor).
- **FR-048**: The shipped Runner MUST make its registered process identifier
  available to the command it runs, as the environment variable `TRAINSTY_PID`, and
  only once the Lock is granted, so a script can announce Steps with no
  configuration. It MUST NOT remove a `TRAINSTY_PID` it inherited: an inner run
  that is not serialized leaves the outer Job's value, so its Steps label the Job
  actually running them. The variable's name and meaning are part of the compatibility
  surface: changing either is a breaking change.
- **FR-049**: The product MUST provide a command a script can call to announce a
  Step. It MUST always exit with status 0, so a script that aborts on the first
  failure is never stopped by it. It MUST do nothing, silently, when not running
  under a granted Runner. When the scheduler does not answer within 1 second, or
  refuses the announcement, or `TRAINSTY_PID` is set but is not a positive integer,
  it MUST print one line on stderr saying the Step was not recorded. That line is
  for people; its wording is not a contract. This is a deliberate exception to the
  CLI's exit-code contract: a progress label is not a result a script branches on.
  `CLAUDE.md`'s CLI rules MUST record the exception.
- **FR-050**: The scheduler MUST NOT read, capture or parse the Runner's output to
  learn about Steps. Steps come only from announcements (ADR-003).
- **FR-051**: The Daemon MUST log each **refused** announcement (invalid
  identifier, invalid state, invalid name, Step cap reached) with the reason, the
  identifier and the name, **as received**. Both are untrusted, so both are logged
  quoted and truncated to 128 bytes, and can never break a log line. Accepted and
  stale announcements MUST NOT be logged.
- **FR-052**: Steps MUST NOT affect a Grant, a Release or the Queue. They are a
  label on the Job and nothing else.

### Key Entities

- **Step**: one named stage of a Job, as announced by its Runner. It has a name and
  a start time. Belongs to exactly one Job, and a Job has 0–16 running Steps. Its
  lifecycle is announced → running → ended (or gone with its Job). It is not a
  Job: in a repository's CI script these are often called "jobs", and in trainsty
  they are Steps.
- **Job** (existing): gains its set of running Steps. Nothing else changes, and
  Steps never influence how the Job is granted or released (FR-052).

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-011**: While a Runner reports Steps, a Developer can tell from the Dashboard
  alone which stage the Job is in. The displayed Step matches the running stage
  within 3 seconds of each **announced** start and end: one 2-second poll plus 1
  second of margin. A Step removed by a Release leaves with its Job, as fast as
  the Job's row does (up to about 7 seconds after a Liveness Probe release,
  ADR-005).
- **SC-012**: On a run whose stages run in parallel, every stage running at that
  moment is on display. None is hidden by another.
- **SC-013**: Adding Step announcements to a repository's CI script changes no
  run's result. The exit status is identical with and without them, including with
  no scheduler, an older scheduler, or outside a Runner. Scheduling is unchanged
  too (FR-052): the same Runners queued the same way are granted in the same order,
  and released at the same moments, with and without announcements.
- **SC-014**: No Step announced by one run is ever shown on another run's row.
  That holds in each named case: a stale identifier after Release, a Waiter's
  identifier, a suite announcing to a Daemon started after a Shutdown, and an
  announcement during a Lock hand-over.
- **SC-015**: **Each** announcement adds at most 1 second when no scheduler
  answers, and at most 100 milliseconds against a responsive local one. A run's
  added time is therefore bounded by its number of announcements. An absent
  scheduler normally refuses the connection at once rather than hanging.

## Assumptions

- **Granularity is the Runner's choice.** `teneerai.web` will announce one Step per
  CI stage first. Finer Steps inside its main CI stage are optional and come later.
- **Changing the repositories' CI scripts is out of scope** for this feature. It
  follows as separate work in each repository, after a trainsty release that
  includes this feature.
- **Steps inside `teneerai.desk`'s end-to-end suite are out of scope.** That
  repository locks only the suite, and reporting its test files would need hooks
  in its test runner.
- **`trainsty status` showing Steps is out of scope.** It can be added later
  without changing this spec's requirements.
- **The Dashboard's lag is unchanged.** It polls every 2 seconds (ADR-005), and a
  Step's display inherits that lag.
- **Precondition for implementation: the vault note gains the requirement first.**
  The requirements note does not mention Steps today. It must be updated and the
  mirror re-copied before the register rows for FR-040–FR-052 are written.
- **Accepted limitation: PID reuse.** If the previous Job's leader dies and the
  operating system gives its pid to the next Runner, the previous run's leftover
  processes could label the successor's row. This takes pid reuse inside one Lock
  hand-over, and the cost is a wrong label, never a wrong Grant (research.md R3).
- **Step joins the glossary before any code** uses it, with the ci-local "job"
  clash recorded.
- **Compatibility across versions:**

  | Daemon \ script | Without announcements | With announcements |
  | --- | --- | --- |
  | Older (no Steps) | unchanged | each announcement prints one "not recorded" line; results unchanged |
  | This feature | unchanged; no Steps shown | Steps shown |
- **This is a MINOR constitution amendment.** It adds one endpoint, changes none,
  and the "no token" reasoning in SDR-001 still holds: a label is the only thing
  the new request can change.
