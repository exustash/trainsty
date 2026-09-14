# Contributing

Thank you for contributing.

The objective of this document is to keep the codebase:

- Maintainable
- Predictable
- Easy to review
- Easy to evolve

For coding conventions and AI-specific instructions, refer to `CLAUDE.md`. For the
operating rules that govern how work is executed and verified, refer to `RULES.md`.
For what cannot be traded away at all, refer to `.specify/memory/constitution.md`.

> **`main` is protected against destruction, and against nothing else.**
> Force-push and deletion are refused for everyone including the owner; there is no
> required review and **no CI**, so nothing verifies a commit before it lands.
> Everything below is the workflow, and the gates are voluntary until a hook
> enforces them (`RULES.md` §7.3).
>
> One consequence for this document's whole PR section: **a direct push to `main`
> is allowed.** The pull-request workflow below is the convention, not a rule the
> remote enforces.

---

## Development Principles

- Prefer small incremental changes.
- One logical change per pull request.
- Avoid unrelated refactoring.
- Reuse existing patterns.
- Optimize for the next engineer.
- **Prefer deleting to adding.** The whole product is a semaphore of depth one;
  anything that reads as clever is probably a second source of truth.

---

## Branch Naming

```text
feat/<description>
fix/<description>
refactor/<description>
chore/<description>
docs/<description>
```

Examples:

```text
feat/sse-register-endpoint
fix/release-on-stream-drop
refactor/extract-liveness-probe
docs/correct-runner-playbook
```

Keep branch names short and descriptive.

---

## Commit Messages

This repository follows the [Conventional Commits](https://www.conventionalcommits.org)
specification.

```text
<type>(<scope>): <description>
```

### Types

| Type | Purpose |
| ---- | ------- |
| `feat` | New functionality |
| `fix` | Bug fix |
| `refactor` | Internal code improvement |
| `perf` | Performance improvement |
| `test` | Add or update tests |
| `docs` | Documentation |
| `build` | Build configuration |
| `ci` | CI workflows |
| `chore` | Maintenance |
| `style` | Formatting only |

### Scopes

Scopes name the part of the system that changed, in domain terms:

| Scope | Covers |
| ----- | ------ |
| `scheduler` | The Lock and the Queue — Grant, Release, FIFO order |
| `process` | Every syscall: group liveness, group termination |
| `api` | The five HTTP endpoints and the JSON wire types |
| `sse` | The `/register` stream specifically, where it differs from the rest |
| `dashboard` | The embedded page |
| `cli` | Subcommands, flags, exit codes |
| `docs` | `knowledge/`, and the root documents |

Examples:

```text
feat(scheduler): grant the lock to the head of the queue
fix(sse): flush the grant event so the runner stops waiting
fix(process): refuse a pid that does not lead its group
test(scheduler): release the lock when the registration drops
docs(adr): record why the probe is a backstop, not the signal
```

Rules:

- Use lowercase.
- Use the imperative mood.
- Keep the first line under 72 characters.
- Wrap the body at 72 columns.
- Do not use generic messages like `update`, `changes`, `fix stuff`, `wip`.

---

## Pull Requests

### Keep PRs Small

Prefer one feature, one bug fix, or one refactoring. Avoid mixing feature work,
dependency changes, formatting changes, and large cleanups.

### Before Opening a PR

```bash
scripts/ci-local.sh --everything
```

That runs the gate in full. The Go half by hand, if you want it piecemeal:

```bash
gofmt -l .              # must print nothing
go vet ./...
go build ./...
go test -race ./...
```

…all pass, and:

- Behaviour that changed has a test that changed with it.
- **A changed release path has a test that kills the holder**, not one that calls
  the happy path (`RULES.md` §2.3).
- Documentation is updated if needed.
- An ADR is added if an architectural decision was made.
- **If the API surface changed**, the constitution is amended in the same PR —
  adding an endpoint is MINOR, changing or removing one is MAJOR.

### `-race` is not optional

`go test` and `go test -race` are different gates on this codebase. The entire
product is one shared structure read by concurrent handlers, so a data race is a
wrong Grant — two suites running at once, which is the failure the tool exists to
prevent. **A PR verified without `-race` has not been verified.**

### Running the suite on a machine that is already running trainsty

The acceptance layer binds the real port 45678, so it cannot run beside a live
Daemon — which is why `scripts/ci-local.sh` **never auto-selects it** and reports it
unrun rather than failing when the port is busy. Check first:

```bash
trainsty status
lsof -nP -iTCP:45678
```

**This is the product's own problem applied to itself**, which is worth appreciating
rather than working around. `knowledge/playbooks/local-ci.md` §3 has the reasoning.

### Wire the push hook once

```bash
git config core.hooksPath .githooks
```

`main` has no CI and no required check, so **the hook is the only thing that makes
the gates real**. `git push --no-verify` bypasses it; the honest name for that is
*skipping the gate*, not *the gate passed*.

---

## Code Review

Review the code, not the engineer. Look for correctness, simplicity, readability,
maintainability, security.

Prefer asking:

> Can this be simplified?

instead of:

> This is wrong.

Every PR is reviewed before merge — `RULES.md` §3.1 for the mechanics. **`RULES.md`
§3.2 is the five checks specific to this codebase**, and generic review misses all
five:

1. Is the mutex held across a channel send, a `Kill`, or a `Flush`?
2. Does this add a release path — and is it idempotent, and does it compare Job
   identity?
3. Does anything signal a PID that was not validated as a group leader?
4. Is an errno being treated as a boolean? `ESRCH` releases; `EPERM` is a bug.
5. Does the API surface change?

---

## Changes That Need Extra Care

These require explicit sign-off before merge (`RULES.md` §1.3), and the PR
description must state how much it can break.

| Change | Why it needs sign-off |
| ------ | --------------------- |
| Anything that signals a process | A wrong PID or a bare PID reaches the developer's shell, or leaves the browsers this tool exists to reclaim |
| A new or altered release path | The one catastrophic failure is a Lock that does not release; there are five paths and they share an implementation |
| The API surface | Runner scripts live in repositories nobody here can see or upgrade. Treat it as a published API |
| Anything written outside memory | DDR-001 says nothing is persisted, and a file trainsty *reads back* is a stale lock waiting to happen |
| A dependency | `go.mod` requiring nothing is why the binary installs anywhere |
| Concurrency above one Lock holder | A MAJOR constitution amendment, not a flag — it reintroduces exactly the collisions the tool prevents |

---

## Changing the Requirements

The requirements live in an Obsidian vault.
`knowledge/product/local-ci-scheduler-requirements.md` is a **byte-identical
mirror**, and editing it is a defect rather than an update.

To change a requirement: **edit the vault**, re-copy the mirror, then work through
the affected rows in `knowledge/product/decisions.md` — the last column names every
file that restates the identifier, which is what makes the sweep a list rather than
a guess.

Two disagreements are already recorded and are **awaiting a vault edit, not a
repository one**: the command is `trainsty` rather than the note's `e2e-scheduler`
(ADR-010), and the repository's CI script is the **Runner** rather than the note's
"wrapper script".

---

## Documentation

Documentation is part of the product.

Update documentation when behaviour changes, the API surface changes, new patterns
emerge, a decision evolves, or a procedure changes.

Recurring knowledge is promoted into `knowledge/` —
`knowledge/document-routing.md` says which folder owns it. After fixing a bug,
append an entry to `knowledge/ERRORS.md`, and **make the `Prevention:` line name a
test**.

**Do not write that something exists without checking the tree.** This repository is
currently documentation and a constitution; a described package is not a built one
(`RULES.md` §6.6).

---

## Continuous Improvement

Leave the codebase in a better state than you found it.

Small improvements made consistently are preferred over large disruptive rewrites.
