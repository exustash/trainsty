---
okf_version: "0.1"
type: reference
title: "Requirement Register"
description: "Every FR, TS and Q identifier this repository cites, with its one-line statement and the files that restate it. Derived from the requirements-note mirror — the vault wins on every disagreement. The last column is what makes a sweep mechanical rather than a semantic grep."
tags: [product, requirements, register, drift, mirror]
timestamp: "2026-09-13"
---

# Requirement Register

> **Derived, not authoritative.** Every row here compresses a line of
> [local-ci-scheduler-requirements.md](local-ci-scheduler-requirements.md), which
> is itself a **byte-identical mirror** of
> `~/Obsidian/Notes/Inbox/local-ci scheduler.md`. **The vault wins over the
> mirror, and the mirror wins over this register.** If a row and the note differ,
> the row is the defect.

## What this file is for

The repository restates requirements in its own words — in `CLAUDE.md`, the
glossary, the ADRs and DDRs, the constitution. That is deliberate: a rule an
engineer meets in context is worth more than a citation they have to resolve. It
also means **one requirement can live in six files**.

**The last column is the whole point.** It names the files that restate each
identifier, so a sweep is a list to work through rather than a judgement call
about where to look. Two habits follow:

- **A note change starts here.** Re-copy the mirror, read the row, then re-read
  every file the row names. Finishing the row is finishing the sweep.
- **A repository change that restates an identifier adds itself to the row**, in
  the same commit. `RULES.md` §8.6.

**The identifiers are this register's own.** The note does not number its
requirements, so `FR-`/`TS-`/`Q-` are assigned here, in the note's own order, and
**they are stable**: renumbering breaks every citation in the repository.

## Re-copying the mirror

```sh
cp "$HOME/Obsidian/Notes/Inbox/local-ci scheduler.md" \
   knowledge/product/local-ci-scheduler-requirements.md
```

Verified 2026-09-13 as byte-identical, SHA-256
`dfb62ba5…86f578dd`. **Nothing checks this automatically** — there is no sync
script and no gate, so an edit in either place drifts silently until someone
re-runs `cmp`. Re-verify before citing a row you have not read today:

```sh
cmp "$HOME/Obsidian/Notes/Inbox/local-ci scheduler.md" \
    knowledge/product/local-ci-scheduler-requirements.md && echo identical
```

## Functional requirements — note §2

| # | Statement | Restated in |
| - | --------- | ----------- |
| FR-1 | The scheduler is a globally installed background CLI; developers initialise and manage the daemon explicitly | `README.md`, `CLAUDE.md` → What this repository is |
| FR-2 | Strict FIFO with exactly one Lock holder; every other request waits in order | `constitution.md` → Principle V, `architecture/adr.md` → ADR-006, `domains/ubiquitous-language.md` → Lock, Queue |
| FR-3 | A dashboard at `http://localhost:45678` shows the active Job and the pending Queue, polling every 2 seconds | `architecture/adr.md` → ADR-005, `conventions/dashboard.md`, `conventions/api.md` → `/status` |
| FR-4 | A Stop button aborts the active Job: frees the Lock, terminates the process group, advances the Queue | `architecture/adr.md` → ADR-002, `conventions/dashboard.md` → Stop, `conventions/api.md` → `/stop`, `domains/ubiquitous-language.md` → Stop |
| FR-5 | Dropped connections and closed terminals release the Lock automatically, with no manual intervention | `CLAUDE.md` → The Release Paths, `architecture/adr.md` → ADR-008, `conventions/testing.md` → Every release path has a test |

## Technical specifications — note §3

| # | Statement | Restated in |
| - | --------- | ----------- |
| TS-1 | Go, distributed as a single static compiled binary with zero dependencies | `constitution.md` → Principle I, `architecture/adr.md` → ADR-001, `conventions/go.md` |
| TS-2 | Unix only (Linux and macOS), so that `syscall.Kill(-pid, SIGKILL)` can clean up orphaned browsers and children | `constitution.md` → Principle IV, `architecture/adr.md` → ADR-002, `conventions/go.md` → Package layout, `playbooks/wrapper-integration.md` §0 |
| TS-3 | Binds a dedicated, hardcoded port 45678 to stay clear of application development ports | `constitution.md` → Additional Constraints, `architecture/adr.md` → ADR-011, `conventions/api.md` |
| TS-4 | Traffic-light model: the Runner retains control of execution, keeping native `stdout`/`stderr` intact; the daemon handles only lock and queue state | `constitution.md` → Principle II, `architecture/adr.md` → ADR-003, `domains/ubiquitous-language.md` → Runner |
| TS-5 | The queue wait uses SSE to hold a persistent connection, circumventing HTTP timeouts on 20+ minute waits | `constitution.md` → Additional Constraints, `architecture/adr.md` → ADR-004, `conventions/api.md` → `/register` |
| TS-6 | PID tracking: a 3–5 second loop equivalent to `kill -0`; an `ESRCH` triggers immediate release | `architecture/adr.md` → ADR-008 *(which demotes this to a backstop — see below)*, `conventions/api.md`, `conventions/testing.md` |

> **TS-6 is the one place the repository deliberately goes beyond the note.**
> ADR-008 keeps the probe at the specified interval but makes the **open
> Registration** the primary liveness signal, because `kill(pid, 0)` returns
> `EPERM` for a live process owned by another user and succeeds for a reused PID —
> neither of which the note addresses. The note is not contradicted; it is
> completed. **No vault edit is required for this row.**

## API surface — note §4

| # | Statement | Restated in |
| - | --------- | ----------- |
| FR-6 | Five endpoints: `GET /register` (SSE, `pid` + `repo`), `POST /release`, `GET /status`, `POST /stop`, `POST /shutdown` | `constitution.md` → Additional Constraints, `conventions/api.md` → The contract |

## CLI — note §5

| # | Statement | Restated in |
| - | --------- | ----------- |
| FR-7 | Five subcommands: `start` (detached), `stop` (`POST /shutdown`), `status`, `ui` (opens the browser), `help`. **The repository ships six** — see *Where the repository and the note disagree* | `README.md`, `CLAUDE.md` → The CLI, `architecture/adr.md` → ADR-012 |

## Open questions

Where the note is silent and a decision is needed. Each is tracked as an `OD-` row
in [`../architecture/adr.md`](../architecture/adr.md) → *Open decisions*; this
table exists so the note's silences are visible from the product side too.

| # | Question | Tracked as |
| - | -------- | ---------- |
| Q-3 | Does `repo` mean anything to the daemon beyond a label? | OD-3 |
| Q-4 | How is the binary distributed — `go install`, a tap, or a release archive? The note says "globally installed" without saying how | **Closed 2026-09-14** — [ADR-013](../architecture/adr.md): release archives, `go install` alongside, no tap |

**Closed 2026-09-13** by `specs/001-serialize-e2e-runs/spec.md`'s clarification round:

| # | Question | Answer |
| - | -------- | ------ |
| Q-1 | Who may call `/stop` and `/shutdown`? | **The floor is sufficient; no shared secret** — `security/sdr.md` → SDR-001. Spec FR-033, FR-033a |
| Q-2 | Where does the detached daemon's output go? | **A per-user log file in the OS log location, appended and never read back** — `data/ddr.md` → DDR-002. Spec FR-013, FR-013a, FR-013b |

## Where the repository and the note disagree

One row, and it is a naming change rather than a requirement change.

| What | Note says | Repository says | Resolution |
| ---- | --------- | --------------- | ---------- |
| The command name | `e2e-scheduler`, in all five §5 bullets | `trainsty` | **The vault moves.** `architecture/adr.md` → ADR-010: the project was named after the note was written. Until the vault is edited, the mirror and the code disagree by design, with ADR-010 as the reason. |
| The Runner's name | "local CI wrapper script" | **Runner** | **The vault moves.** `domains/ubiquitous-language.md` → *Words this project does not use*. A glossary term cannot be a four-word phrase that also names a shell idiom. |
| The number of subcommands | Five: `start`, `stop`, `status`, `ui`, `help` (FR-7) | **Six** — `wrap` is added | **The vault moves.** `architecture/adr.md` → ADR-012 ships the Runner as a subcommand, which the note did not anticipate because it assumed each repository would write its own. FR-7's statement above is left as the note's, not corrected in place. |

**Both are vault edits, not repository edits.** Editing the mirror to agree would
break the byte-identical property that makes a re-copy safe.
