# API Contract & Lock Safety Checklist: Runner-reported Steps

**Purpose**: Test whether the requirements for `/step`, `trainsty step` and `TRAINSTY_PID` are complete, unambiguous and consistent enough to implement. These are clients nobody here can upgrade, and a Step must never attach to the wrong Job.
**Created**: 2026-11-05
**Feature**: [spec.md](../spec.md) · [contracts/http-api.md](../contracts/http-api.md) · [contracts/cli.md](../contracts/cli.md)

**Depth**: review before `/speckit-tasks`. **Audience**: the author.

## Requirement Completeness

- [x] CHK001 - Is `TRAINSTY_PID` named in the spec as part of the compatibility surface, given that client scripts will depend on its exact name and meaning? [Gap, Spec §FR-048]
- [x] CHK002 - Do the requirements state that Steps never affect a Grant, a Release or the Queue? Today that invariant appears only in data-model.md. [Traceability, Gap]
- [x] CHK003 - Are the stable error codes for refusals (`invalid_step_state`, `invalid_step_name`, `too_many_steps`) named in, or explicitly delegated from, the spec rather than only the contract? [Traceability, Spec §FR-043]
- [x] CHK004 - Does the spec state that the `/status` change is additive, with no existing field renamed, retyped or removed? [Completeness, Spec §FR-045]
- [x] CHK005 - Is the behaviour defined for a malformed `TRAINSTY_PID` (non-numeric, zero, negative) in the Step command? [Gap, Contract cli.md]

## Requirement Clarity

- [x] CHK006 - Is the 128 limit for a Step name expressed in one unit? FR-043 says "characters", while data-model.md and the contract say "bytes". [Conflict, Spec §FR-043]
- [x] CHK007 - Is "control characters" defined precisely, for example ASCII C0 and DEL versus every Unicode control category, and is it the same rule as `repo`'s? [Ambiguity, Spec §FR-043]
- [x] CHK008 - Is it specified whether Step names are compared exactly or after trimming whitespace, so that `end "Lint "` does or does not end `start "Lint"`? [Ambiguity, Spec §US2 #2]
- [x] CHK009 - Is "the identifier it carried" in FR-051 defined for a refused announcement whose pid is itself invalid (logged raw, quoted or omitted)? [Clarity, Spec §FR-051]
- [x] CHK010 - Is the wording of the Step command's stderr line declared to be stable or explicitly unstable, so scripts know whether they may match it? [Ambiguity, Contract cli.md]

## Requirement Consistency

- [x] CHK011 - Are FR-049's "give up within 1 second" and SC-015's "under 1 second in the worst case" consistent, given that SC-015 counts per run while FR-049 is per announcement (N announcements against a hanging scheduler could add N seconds)? [Conflict, Spec §FR-049, §SC-015]
- [x] CHK012 - Is the always-exit-0 rule (FR-049) reconciled in writing with `CLAUDE.md`'s "exit codes are the contract", so the exception does not read as a defect? [Consistency, Spec §FR-049]
- [x] CHK013 - Is the pid-matching rule (FR-041: exact match, no fallback) stated consistently with `/release`'s, which allows a pid-less "whatever holds the Lock" form, with the difference justified? [Consistency, Spec §FR-041]
- [x] CHK014 - Is the Step term in the spec consistent with the glossary, which does not yet define it, including the "job" naming clash with `ci-local.sh`? [Consistency, Spec §Key Entities]

## Lock-Safety Scenario Coverage

- [x] CHK015 - Is it specified whether a Job's Steps survive a re-registration by the same pid (ADR-007: immediate re-Grant), or are reset? [Gap, Spec §FR-044]
- [x] CHK016 - Are announcements from a suite that keeps running after a Shutdown (ADR-009), arriving at a new Daemon with no Job, covered by the no-op rule? [Coverage, Spec §FR-042]
- [x] CHK017 - Is the PID-reuse case (the previous Job's pid reassigned to the next Runner) recorded as an accepted limitation in the spec, not only in research.md R3? [Assumption, Gap]
- [x] CHK018 - Do the requirements cover an announcement that arrives while the Lock is changing hands, so that it attaches to exactly one Job or none? [Coverage, Spec §FR-041]
- [x] CHK019 - Is US1 scenario 4 ("released by any of the five release paths") traceable to each path, including Stop and the Liveness Probe, rather than to Release in general? [Coverage, Spec §US1 #4]

## Edge Case Coverage

- [x] CHK020 - Is it defined whether restarting a Step that is already running counts against the 16-Step cap when 16 are running? [Edge Case, Spec §FR-044]
- [x] CHK021 - Is the outcome defined for an `end` with the right pid but a name that was never started, as distinct from a stale pid (both "no-op", but are both `recorded:false`)? [Clarity, Spec §FR-042]
- [x] CHK022 - Is the order of precedence defined when a request has several invalid parameters, so the returned code is deterministic? [Gap, Contract http-api.md]

## Acceptance Criteria Quality

- [x] CHK023 - Is SC-011's "within one refresh (about 2 seconds)" stated as a bound a test can assert, such as one poll interval plus a stated margin? [Measurability, Spec §SC-011]
- [x] CHK024 - Is SC-015's "imperceptible normally" quantified? [Measurability, Spec §SC-015]
- [x] CHK025 - Can SC-014 ("never shown on another run's row") be checked against named scenarios, such as stale pid, Waiter pid and post-Shutdown? [Measurability, Spec §SC-014]

## Dependencies & Assumptions

- [x] CHK026 - Is the compatibility matrix complete for old and new Daemons and old and new scripts, including the "one stderr line per Step against an older Daemon" cost? [Coverage, Spec §US3]
- [x] CHK027 - Is the vault-first dependency (the requirement added to the requirements note before the register rows) stated as a precondition rather than only an assumption? [Dependency, Spec §Assumptions]

## Notes

- Check items off as they are resolved in spec.md, or record why an item is intentionally left open.
- CHK006 and CHK011 are concrete conflicts between documents, not only gaps. Fix them before `/speckit-tasks`.
