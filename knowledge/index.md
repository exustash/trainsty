---
okf_version: "0.1"
type: index
title: "Knowledge Base Index"
description: "Names every document in the trainsty knowledge base and what it is authoritative for, states the reading order for a new agent, and marks which files are auto-loaded versus loaded on demand."
tags: [knowledge, architecture, conventions, data, domains, product, playbooks]
timestamp: "2026-09-13"
---

## Knowledge Base Index

> **Overview:** Central hub for project context — architecture, conventions,
> state, vocabulary, requirements and operational playbooks.
> **Primary audience:** AI coding agents (Claude Code, etc.) & the engineering
> team.
> **Maintenance:** Update this index whenever files are added, moved, or renamed.
> **Layout:** Knowledge is organized into folders by intent. Use progressive
> disclosure — load only what the current task needs.

### Reading order for a new agent

`.specify/memory/constitution.md` (what cannot be traded away) → `CLAUDE.md`
(conventions) → `RULES.md` (how work is executed and verified) → this index → the
conventions for the layer you are touching. Everything below that is loaded on
demand.

**The constitution comes first on this project**, which is not the usual order.
Four of its five principles forbid something that looks like an ordinary good
idea — a dependency, a daemon that owns execution, a portability abstraction, a
state file, a concurrency above one — and the Additional Constraints forbid a
config flag. An agent that meets any of them only in review has already written
the wrong thing.

Every document carries OKF frontmatter — `okf_version`, `type`, `title`,
`description`, `tags`, `timestamp`. The `description` is what an agent reads to
decide whether to open the file, so write it as a claim about the content, not a
label.

### Layout

```text
knowledge/
├── index.md             # this file
├── document-routing.md  # which folder owns a new document
├── ERRORS.md            # error log & pattern prevention — empty, by honesty
├── architecture/        # system shape and the decisions behind it
├── conventions/         # how we write code, per layer
├── data/                # what state exists, and what is deliberately not written
├── domains/             # the vocabulary
├── playbooks/           # step-by-step procedures
└── product/             # the mirrored requirements note + the register
```

**Before filing a document**, read [Document Routing](document-routing.md) — which
folder owns it, which of the two decision records a decision belongs in, and the
one `product/` file that is a mirror rather than a source. Not auto-loaded, by the
same test that keeps this index out: you know when you are filing something.

**Imported into every session** (`CLAUDE.md` → Always loaded): the glossary and all
four conventions files. **Not this file** — nobody violates a table of contents by
not having read it. Everything else is loaded on demand.

**`product/` is the one that must never be imported.** The requirements mirror is
the source document, and it is there to be *cited into* so `FR-2` or `TS-6`
resolves for anyone rather than only for the vault holder.

---

### Conventions

The stable half of the knowledge base. All four are auto-loaded.

- [Go Conventions](conventions/go.md) — package layout and the dependency
  direction (`scheduler/` imports neither `net/http` nor `syscall`), error wrapping
  and the `syscall.Errno` matching rule, the concurrency discipline around the one
  mutex, naming, doc comments. **Read the indentation section before formatting
  anything**: `gofmt` wins over the global four-space rule, and that is the only
  place the house style loses.
- [API Conventions](conventions/api.md) — the five-endpoint contract as a
  compatibility boundary, because **the clients are Runner scripts in repositories
  this project has never seen**. Carries the two mistakes that pass a short test
  and fail a real wait: a non-zero `WriteTimeout`, and a missing `Flush()`.
- [Testing Conventions](conventions/testing.md) — what each layer may assume, why
  `-race` is a gate rather than an option, the table of **five release paths that
  each need their own test**, and the process-test rules for real children.
- [Dashboard Conventions](conventions/dashboard.md) — one embedded file, no build
  step, no framework, no state of its own; the 7-second worst-case lag it must show
  honestly rather than paper over; and `textContent`, never `innerHTML`, for the one
  untrusted value it renders.

### Domains

- [Ubiquitous Language](domains/ubiquitous-language.md) — **auto-loaded.** Sixteen
  terms with lifecycles and ban lists. Three things it settles that the requirements
  note leaves loose: it splits the note's single word *orphan* into an **Orphaned
  Lock** and an **Orphaned Process** (one symptom, two mechanisms), it names the
  repository's CI script the **Runner** rather than the note's "wrapper script", and
  it fixes **Stop** and **Shutdown** as different verbs with different blast radii.

### Architecture

- [Overview](architecture/overview.md) — the dependency direction, the three
  boundaries, and the **life of one Job** as a sequence including all five of its
  endings. Also *What is deliberately absent*, which is the list that keeps a
  config file or a state file from arriving as an accident.
- [Architecture Decision Records](architecture/adr.md) — eleven records, newest
  first. ADR-001 through ADR-006 and ADR-011 are the requirements note's own
  decisions, recorded rather than assumed. **The three that go beyond it are worth
  reading before writing any handler**: ADR-008 demotes the note's `kill -0` loop to
  a backstop and makes the open Registration the primary liveness signal, because
  `kill(pid, 0)` returns `EPERM` for another user's live process and succeeds for a
  reused PID; ADR-007 keys the Queue by PID so a reconnect is not starved; ADR-009
  makes a shutdown release the Job **without killing it**, and that asymmetry is a
  behaviour to assert, not an implementation detail. Carries four open decisions —
  **OD-1, who may call `/stop`, is the one with a security consequence.**
- [Repository Structure](architecture/structure.md) — what exists today (no Go
  code, an empty remote) and the tree the first commit lands in. Names the
  four things absent on purpose, including `internal/` and a `Makefile`.

### Data

- [State](data/state.md) — the complete state of a running Daemon, field by field,
  with **two invariants** and the rules for touching them. The one to internalise:
  never hold the mutex across a channel send, a `Kill`, or a `Flush`.
- [Data Decision Records](data/ddr.md) — DDR-001: nothing is persisted, and why
  persisting the Queue would be *actively harmful* rather than merely unnecessary.
  DDR-002 holds the log destination open, and states the test that separates a
  permissible file from a forbidden one — **anything trainsty reads at startup is a
  stale lock waiting to happen; anything it only appends to is not.**

### Playbooks

Procedures, not rules — deliberately not auto-loaded.

- [Integrating a Repository's Runner](playbooks/wrapper-integration.md) — the
  procedure for wiring a repository's local CI script. **§0 is the loudest warning
  in the knowledge base**: register a PID that is not a process group leader and a
  Stop can signal the developer's own shell. Ends with a six-step verification in
  dependency order, of which step 5 — *did the browsers actually die* — is the one
  people skip and the one that catches a bad PID.
- [Recovering a Stuck Lock](playbooks/stuck-lock-recovery.md) — diagnosis order for
  the product's one catastrophic failure, then its three real causes. Includes the
  restart of last resort and **what it costs**, which is a suite that keeps running
  while invisible to the scheduler.

### Product

- [Requirements (mirror)](product/local-ci-scheduler-requirements.md) —
  **read-only.** A byte-identical copy of the Obsidian note that specifies the
  product. Verified identical on 2026-09-13; nothing checks it automatically.
- [Requirement Register](product/decisions.md) — every `FR` / `TS` / `Q`
  identifier with its one-line statement and **the files that restate it**. The last
  column is what makes a sweep a list rather than a judgement call. Ends with the
  two places the repository and the note disagree — the command name and the
  Runner's name — **both of which are vault edits, not repository edits.**

---

### What is not here, and why

| Missing | Reason |
| ------- | ------ |
| `security/` | Real surface, no folder yet. The floor is in `CLAUDE.md` → Security and the open question is `architecture/adr.md` → OD-1; **settling OD-1 is what creates `security/sdr.md`** — [document-routing.md](document-routing.md) says so rather than leaving it to be noticed |
| `design/` | The Dashboard is one embedded page, specified in `conventions/dashboard.md`. A handoff folder for two lists and a button would be ceremony |
| `audits/`, `plans/` | Both are records of current work. There is no code to audit and no sequence to plan beyond `specs/`, which the Spec Kit owns |
| `data/schema.md` | Nothing is persisted. [data/state.md](data/state.md) is the equivalent, and DDR-001 is why |
