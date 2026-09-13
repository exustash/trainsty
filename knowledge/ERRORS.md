---
okf_version: "0.1"
type: error-log
title: "Error Log & Pattern Prevention Guide"
description: "A living log of errors and bugs after they are resolved, in RULES.md §8.1's four-part shape. Empty today because no code exists yet. The value of an entry is its Prevention line and the test that line names."
tags: [knowledge, logs, errors, patterns]
timestamp: "2026-09-13"
---

# Error Log & Pattern Prevention Guide

> `RULES.md` §8.1 requires an entry after resolving **any** error or bug. The
> value of this file is the `Prevention:` line: an entry that only records what
> broke is a changelog, not a guard.

## The shape

```markdown
## [date] — [short title]

- **Symptom:** what was observed.
- **Root cause:** why it happened.
- **Fix:** what was changed.
- **Prevention:** how to avoid recurrence — name the test, not the intention.
```

Maximum three lines per section. Keep it scannable.

## What belongs here, and what does not

| Thing | Where it goes |
| ----- | ------------- |
| A bug that has been **resolved** | Here, in the shape above |
| A bug that is **known and unfixed** | A task line in `specs/**/tasks.md`. It cannot supply `Fix:` or `Prevention:`, so it cannot be an entry here |
| A failure mode the design **anticipates** but has never seen | Not here. It belongs to whichever record decided the mitigation — `architecture/adr.md`, or `playbooks/stuck-lock-recovery.md` for a diagnosis procedure |
| A decision made while fixing something | The decision record, with this entry citing it |

**The third row is the one that gets violated on a project with no code.** It is
tempting to seed this file with the ways trainsty is expected to break — a
buffered SSE stream, a non-leader PID, a probe misreading `EPERM`. Those are
predictions, and a log of predictions is unfalsifiable. They are already recorded
where they were decided: `architecture/adr.md` → ADR-002, ADR-004 and ADR-008, and
[`playbooks/stuck-lock-recovery.md`](playbooks/stuck-lock-recovery.md) for the
diagnosis order.

## Entries

**None yet.** No Go code has been written, so nothing has broken.

The first entry will almost certainly concern the Lock failing to release, because
that is the product's one catastrophic failure and it has five paths that can each
fail independently. When it happens:
[`playbooks/stuck-lock-recovery.md`](playbooks/stuck-lock-recovery.md) →
*Afterwards* is the procedure, and **the `Prevention:` line must name which of the
five release paths failed and which test would have caught it** — if no test
covers it, that test is the fix.
