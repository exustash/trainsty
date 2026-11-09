# Lock Safety & Concurrency Checklist: Runner-reported Steps

**Purpose**: Test whether the requirements that keep a Step on the right Job, and keep Steps out of scheduling, are complete, unambiguous and consistent enough to implement. Decision hierarchy #1 is the Lock's correctness; a Step attached to the wrong Job, or an announcement that delays a Grant, is a failure of it.
**Created**: 2026-11-07
**Feature**: [spec.md](../spec.md) · [data-model.md](../data-model.md) · [contracts/http-api.md](../contracts/http-api.md) · [tasks.md](../tasks.md)

**Depth**: standard, a PR-review gate before `/speckit-implement`. **Audience**: the reviewer.
**Scope**: complements [api.md](api.md), whose CHK015–CHK019 already cover re-registration, post-Shutdown announcements, PID reuse, the hand-over and per-path traceability. Nothing here repeats them.

## Requirement Completeness

- [x] CHK001 - Do the requirements state that handling an announcement must not delay a Grant or a Release, for example by bounding how long it may hold the one mutex? [Gap, Non-Functional, Spec §FR-052]
- [ ] CHK002 - Is it required that `/status` reports the Job and its Steps from one consistent instant, so a report can never show the Steps of one Job beside the identity of another? [Gap, Spec §FR-045]
- [x] CHK003 - Is the behaviour defined when two parallel stages announce the **same** name: one line for both, and ending either removes it while the other still runs? [Edge Case, Gap, Spec §FR-044]
- [x] CHK004 - Does the spec say what happens to an inherited `TRAINSTY_PID` when an inner `wrap` runs unserialized? tasks.md T018 removes it, but FR-048 and contracts/cli.md say only when the variable is set, not when it is cleared. [Conflict, Spec §FR-048, Tasks T018]

## Requirement Clarity

- [ ] CHK005 - Is "a Lock hand-over" (FR-041) defined as a span, from the previous Job's Release to the next Grant, so "exactly one Job or none" has a definite boundary? [Ambiguity, Spec §FR-041]
- [ ] CHK006 - Is a restarted Step's position specified in the spec? data-model.md moves it to the end; US2 scenario 2 says only that "its time restarts", which leaves "oldest first" unclear for that line. [Clarity, Spec §US2 #2, data-model.md → Restart]
- [ ] CHK007 - Is it clear when "a re-registration by the Job's own pid" can occur, given that a dropped Registration releases the Lock (release path 2)? If it needs the first stream still open, is it specified which stream's later drop ends the Job, and with it the Steps? [Ambiguity, Spec §FR-044, ADR-007]
- [ ] CHK008 - Is the window between the scheduler's Grant and the Runner receiving the Grant event addressed? A hand-written Runner could announce in it; is acceptance then intended? [Ambiguity, Spec §Edge Cases → Steps announced while waiting]

## Requirement Consistency

- [ ] CHK009 - Do data-model.md's four invariants match FR-041, FR-044 and FR-052 one for one, with no invariant that exists only in the data model? [Consistency, data-model.md → Invariants]
- [x] CHK010 - Is the time for a Step to disappear after a Liveness Probe release (up to one probe interval plus one poll, about 7 s, ADR-005) reconciled with SC-011's 3-second bound, or is SC-011 explicitly limited to announced starts and ends? [Consistency, Spec §SC-011, §US1 #4]
- [ ] CHK011 - Is the Stop path's order (signal the group, then Release) reconciled with announcements the dying group sends in between? The pid still matches, so they are accepted; is that stated as intended? [Consistency, Spec §FR-044, Spec §Edge Cases]
- [ ] CHK012 - Do the release rules ("a second Release for the same Job is a no-op, never a Release of its successor") explicitly extend to Steps, so a stale Release can never clear the successor's Steps? [Consistency, Gap, Spec §FR-044]

## Scenario Coverage

- [ ] CHK013 - Are announcements arriving **during** a Shutdown, after the Lock is dropped but before the Daemon exits, covered by the no-op rule, not only those reaching a later Daemon? [Coverage, Spec §FR-042, ADR-009]
- [ ] CHK014 - Is an announcement racing a Liveness Probe release covered by the same one-Job-or-none rule as a normal hand-over? [Coverage, Spec §FR-041]
- [ ] CHK015 - Is a concurrent burst of new names at the cap specified, for example 16 running and two new starts racing for no slot, so the outcome (both refused) is deterministic? [Edge Case, Spec §FR-044]
- [ ] CHK016 - Is the behaviour defined if the system clock steps backwards while a Step runs, so its elapsed time never goes negative? [Edge Case, Gap, Spec §FR-045]

## Acceptance Criteria Quality

- [x] CHK017 - Is FR-052 ("Steps never affect a Grant, a Release or the Queue") backed by a success criterion or acceptance scenario that can fail? Today no SC names it. [Measurability, Gap, Spec §FR-052]
- [ ] CHK018 - Is SC-014's hand-over case stated so a test can check it deterministically, for example as a property under the race detector rather than one lucky interleaving? [Measurability, Spec §SC-014]
- [x] CHK019 - Does an acceptance criterion cover CHK001's non-delay requirement, if it is added, with a stated bound? [Measurability, Gap]

## Dependencies & Assumptions

- [ ] CHK020 - Does the spec state that Steps live under the constitution's one state and one mutex (Principle V), rather than leaving it to plan.md and research.md R4? [Assumption, Spec §Key Entities]
- [ ] CHK021 - Is the PID-reuse limitation's window stated with its trigger and cost for Steps specifically: a wrong label, never a wrong Grant? Is it consistent with how ADR-007 and ADR-008 phrase the same risk? [Assumption, Spec §Assumptions]

## Notes

- Check items off as they are resolved in spec.md, or record why an item is intentionally left open.
- **CHK004 is a concrete conflict** between tasks.md and the contract: resolve it in the spec and contract before T018, or drop the behaviour from T018.
- **CHK007 may be a 001 question** surfacing here: if the answer changes release path 2, it belongs in 001's record, not in 002.
