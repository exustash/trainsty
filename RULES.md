# Rules

## Response Style

Keep responses concise; break large outputs into steps and avoid single responses
exceeding output token limits.

## Verification

Always verify against the real artifact and the real machine state, never the
source tree:

- **Build state** — `go build` succeeding says nothing about behaviour. A daemon
  that builds, binds and never flushes an SSE event passes every compile-time check.
- **Daemon state** — read it off the running process: `curl -s
  http://localhost:45678/status`, `lsof -nP -iTCP:45678`. Not off a document, and
  not off the last thing you were told.
- **Process state** — `ps -o pid,pgid,command`. A PID that exists is not a PID that
  leads its group, and only the second one is safe to signal.
- **Whether something exists at all** — this repository is mostly documentation.
  **Check the tree before asserting that any code is wired**; a document describing
  a package is not a package.

## Environment

This shell is zsh — avoid bash-only constructs (`BASH_REMATCH`), watch for aliases,
and confirm the working directory before running commands.

`cd` may be aliased to a ranked-jump tool, so `cd <name>` can land elsewhere — use
absolute paths when you need deterministic navigation.

**Unix-only steps.** Process-group termination, `setsid` and PID probing behave
differently or not at all outside Linux and macOS, and `setsid` is not in macOS's
base system. There is no Windows path (ADR-002).

**The machine is a shared resource, and this project is about that.** Port 45678 is
global; a real Daemon and the E2E suite cannot both have it. Check before you bind:
`trainsty status`, or `lsof -nP -iTCP:45678`.

---

## Autonomous Coding Agent Operating Rules

**Stack:** Go, standard library only · Unix (Linux, macOS) · one static binary ·
port 45678, hardcoded · no dependencies, no configuration, no persistence.
**Goal:** Autonomous coding agents produce production-grade work with maximum rigor
and minimal supervision.

### 1. General Execution Model

#### 1.1 Autonomous Loop

Identify the next task, plan it, explain and validate the plan, then implement,
review, test, validate, and document — and repeat. Surface progress at each
iteration boundary.

#### 1.2 Parallelization via Sub-Agents

Spin off sub-agents whenever task boundaries are disjoint. Each must report its diff
summary, test results, and documentation updates before the parent merges.

**`scheduler/`, `process/` and `httpapi/` are not disjoint in the way they look.**
The dependency direction means a change to the Lock's semantics changes what every
handler may assume. Split by package only when the contract between them is already
written down.

#### 1.3 Validation Gate for Irreversible Actions

Do **not** execute irreversible operations without explicit human approval. On this
project the irreversible things are mostly other people's processes:

- **Signalling any process this session did not start.** `kill`, `pkill`, `killall`
  — including "just the test browsers". A wrong pattern reaches the developer's
  editor, their database, or their shell.
- **`trainsty stop` while a Job is active.** It releases the Lock and leaves a
  suite running and invisible to the scheduler (ADR-009). State which repo is
  running before doing it.
- **Anything that writes outside the repository** — a log path, a state file, a
  launch agent. DDR-001 and DDR-002 make each one a decision, not an implementation
  detail.
- **Publishing the binary**, or any distribution step (OD-4 is open).
- **Deletion of presumed dead code files.**
- **`git push --force`, branch deletion, history rewriting**, once there is a
  remote.

Before requesting approval, summarize concisely: **what** will be executed, **why**,
**how much it can break** (whose processes, whose work), and the **rollback path** —
and if there is none, say so explicitly. Proceed only after explicit confirmation.

#### 1.4 Convention Compliance

Adhere to all rules in this repository — `.specify/memory/constitution.md`,
`CLAUDE.md`, `CONTRIBUTING.md`, this file, and `knowledge/`.

**The constitution is the one that can refuse a requirement.** Five of its
principles forbid things that look like ordinary good ideas: a dependency, a config
flag, a state file, a concurrency setting, a daemon that owns execution. Read it
before proposing any of them.

#### 1.5 Clean Code & Screaming Architecture

- **Clean Code:** meaningful names, small functions, single responsibility, minimal
  arguments, no dead code, no commented-out code, and **comments that do not
  outweigh the logic**.
- **Screaming Architecture:** package names communicate domain intent, not
  mechanism — `scheduler`, not `statemanager`; `process`, not `syscallutil`.

---

### 2. Testing

**Every behaviour is validated at the lowest layer that can see it**, and the layer
boundaries are in `knowledge/conventions/testing.md`. Each unit covers the
**nominal case**, the **error cases** — every step that can fail and what the caller
is told — and the **edge cases**: the boundary, the empty input, and where the code
is a control, the state it exists to refuse.

**`go test -race ./...` is the gate, not `go test`.** The product is one shared
structure read by concurrent handlers; a data race is a wrong Grant, which means two
suites running at once — the failure the tool exists to prevent.

#### 2.1 Unit Tests

- `scheduler/` is pure and carries the coverage floor: **≥80% statements** on lock,
  queue and release logic (constitution → Development Workflow & Quality Gates).
- Tests live in `_test.go` beside the code. Table-driven where cases differ only in
  data. Arrange–Act–Assert, visually separated.
- Named for the behaviour and the condition:
  `TestReleasesLockWhenRegistrationDrops`, never `TestRelease2`.
- **No test framework.** `testing`, `httptest`, `errors.Is` and `t.Cleanup` cover
  every layer; a matcher library would be the first entry in `go.mod`.

**A fix for a review finding carries the same obligation:** find the covering test
or write one, and **see it fail before it passes**. Review is not a regression
detector.

#### 2.2 Process and End-to-End Tests

`process/` cannot be faked — the thing under test is the syscall.

- **Start real children the test owns**, with `SysProcAttr{Setpgid: true}`, and for
  group tests a child that starts children of its own.
- **Always reap**, via `t.Cleanup` with a group kill. A leaked group leaves several
  processes for the session.
- **Never probe or signal a PID the test did not create.** A hardcoded PID in a
  test is a signal aimed at whatever the machine happens to be running.
- **Do not sleep a magic interval.** Poll with a deadline.
- **The E2E layer binds the real port 45678**, so it cannot run beside a real
  Daemon. It **skips with a message naming `trainsty status`** rather than failing as
  though the code were broken.

#### 2.3 The mandatory release test

The constitution requires a test that **kills a Lock holder without letting it call
`/release`** and asserts the next Waiter is promoted. It is not one test among
several: it is the product's one catastrophic failure, asserted.

All five release paths need their own test — `CLAUDE.md` → The Release Paths, and
the table in `knowledge/conventions/testing.md`. **Path 5 asserts that the Job is
still alive** after a shutdown; ADR-009 is a behaviour, not an implementation
detail.

---

### 3. Code Quality & Review

#### 3.1 Mandatory Code Review

Every change is reviewed before it merges. Resolve every issue **the work
introduced**; a finding pre-existing on the mainline is flagged, not fixed
(`CLAUDE.md` → Modification Policy forbids unrelated repairs).

- **Green first.** The gates in §3.3 pass before a review is requested. A review
  spent on red code is spent twice.
- **Zero critical and zero major** introduced by the work, before merge.
- **Writing the fix:** one commit per finding, **test committed before the fix**,
  watched failing against the unfixed code. Repair narrowly — nothing beyond what the
  finding named, no new product code, and no new explanation: answer with code, a
  test, or a deletion.
- **Closing several findings at once:** don't. Six findings are six commits; in one
  commit you cannot see how much each one can break.
- **A fix that would itself be a §1.3 action stops for approval first.**

#### 3.2 What a reviewer looks for here

Generic review misses this codebase's failure modes. The five to check every time:

1. **Is the mutex held across anything that blocks?** A channel send, a
   `syscall.Kill`, a `Flush`. Each is a deadlock.
2. **Does this add a release path?** If so: is it idempotent, does it compare Job
   identity rather than "is there a Job", and is it in `CLAUDE.md` → The Release
   Paths?
3. **Does anything signal a PID that was not validated as a group leader?**
4. **Is an errno being treated as a boolean?** `ESRCH` releases; `EPERM` is a bug
   that must be loud.
5. **Does the API surface change?** Then it is a constitution amendment and a
   register update, in the same commit — and Runner scripts in other repositories are
   clients nobody here can upgrade.

#### 3.3 Format, Vet & Test

**`scripts/ci-local.sh` is the gate.** It runs these and more, and it is what the
pre-push hook invokes:

```bash
scripts/ci-local.sh --quick     # what the hook runs
scripts/ci-local.sh --everything   # everything, incl. the acceptance suite
```

The four commands underneath, runnable by hand:

```bash
gofmt -l .              # prints nothing
go vet ./...
go build ./...
go test -race ./...
```

`gofmt -l .` printing a filename is a failure, not a suggestion. **`gofmt` wins
over the global four-space style** — `knowledge/conventions/go.md` carries the
reasoning and the scope of that exception.

Three things the script adds that a hand-run set cannot:

- **`go.mod` declares no dependencies** (Principle I) and **`os/exec` is imported
  only by `runner/` and `daemonctl/`** (Principle II, ADR-012). Both are
  constitutional properties, and both are now mechanical rather than remembered.
- **`scheduler/` coverage ≥80%**, the constitution's floor.
- **A job that could not run is reported as such, not as a pass.** Default runs
  exit 0 on an unrun blocking job so the hook stays usable; set
  `CI_LOCAL_STRICT_UNRUN=1` when green must mean everything ran.

**`--quick` is a weaker gate, not only a faster one** — it omits the acceptance
suite, the only layer that drives the built binary. `knowledge/playbooks/local-ci.md`
has the detail, including why the acceptance suite can never be auto-selected.

#### 3.4 Strictness

- No `//nolint`-style suppression without a one-line reason on the same line.
- No ignored errors (`CLAUDE.md` → Go Rules).
- No new exported identifier without a doc comment.

---

### 4. Dependencies & Configuration

#### 4.1 Dependency Additions

Before adding a dependency:

1. Confirm the standard library cannot do it. It has so far, every time.
2. Confirm the thing it replaces is more than a few lines.
3. Document the rationale in the plan's Complexity Tracking table **before** use,
   per the constitution.

`go.mod` requiring nothing is a property worth defending: it is why the binary
installs anywhere. **A test-only dependency is still a dependency.**

#### 4.2 Configuration

**There is none, and that is a decision.** No config file, no `--port`, no
concurrency setting. The port is a constant (ADR-011); concurrency above one is a
MAJOR constitution amendment. The only runtime inputs are `pid` and `repo`.

---

### 5. Error Handling

#### 5.1 Responses

A handler returns a status code and, on failure, `{"error":"<stable_snake_case>"}`.
The code is the machine-readable contract the Dashboard and the Runner branch on;
additions to the set are reviewed as a contract change.

**Never leak an internal error string, a filesystem path, or an errno text** to a
client. Log the detail; return the code.

#### 5.2 Logging

All caught errors are logged with the operation name, the identifiers needed to
correlate them (the Job's `pid`, the `repo` label), and the error. Log before
handling.

**A failure in a cleanup path must not prevent the cleanup.** Log it and release
anyway — a skipped release is the deadlock this product exists to prevent.

Re-read `CLAUDE.md` → Logging before adding a field, and note that **while DDR-002
is open a detached Daemon's log goes nowhere.**

---

### 6. Documentation

#### 6.1 Error Log

After resolving any error or bug, append to `knowledge/ERRORS.md`:

```markdown
## [date] — [short title]

- **Symptom:** what was observed.
- **Root cause:** why it happened.
- **Fix:** what was changed.
- **Prevention:** how to avoid recurrence.
```

Maximum three lines per section. **The `Prevention:` line names a test**, not an
intention. An unfixed finding cannot supply `Fix:` or `Prevention:`, so it is a task
line, not an entry.

#### 6.2 State Documentation

After any change to what the Daemon holds, or to anything it writes outside memory:
update `knowledge/data/state.md` and add a `knowledge/data/ddr.md` entry — **including
when the decision was to write nothing**, with the reasoning.

#### 6.3 Architecture Decision Records

When adding a component, changing a pattern, or introducing a dependency: create or
update an ADR in `knowledge/architecture/adr.md`, newest first, in the existing
format. **A decision about the API surface is also a constitution amendment.**

#### 6.4 Post-Task Documentation Sync

After completing a task, update all relevant documentation. Do this as the **final
step** before shipping.

#### 6.5 Conciseness

- Prefer bullet points and tables over prose paragraphs.
- Target one screen of content per section.
- Link to existing documentation rather than duplicating it.
- Keep the agent context window uncluttered.

#### 6.6 Every Restatement Moves With the Original

A rule stated in more than one place has **one copy that is right and the rest
drifting.** Carry every restatement in the same commit.

The registers on this project, and what each one catches:

| Register | Catches |
| -------- | ------- |
| `knowledge/product/decisions.md` last column | Which files restate an `FR`/`TS`/`Q`. **A `grep` for the identifier does not find them** — a restatement usually does not cite the number |
| `knowledge/architecture/adr.md` → *Open decisions* | An `OD-` that has quietly been decided in code |
| `knowledge/index.md` | A document added, renamed or reclassified |
| `CLAUDE.md` → The Release Paths | A sixth release path |
| `.specify/memory/constitution.md` | An endpoint added or changed |

- **Assert it or do not write it.** No sweep reaches a sentence that was never
  true. In particular: **do not write that a package, test or gate exists** — this
  repository is documentation, and a claim about wiring must be checked against the
  tree.
- **Code first, prose after.** Claim only what is in the code, as briefly as it can
  be said. What a future reader must find goes in a document; why *this* edit
  happened goes in the commit body.
- **The requirements mirror is never edited.** A disagreement between it and the
  repository is resolved in the vault, and recorded in
  `knowledge/product/decisions.md` → *Where the repository and the note disagree*.

---

### 7. Version Control

#### 7.1 Commit Hygiene

- **Atomic commits:** one logical change per commit.
- **Format:** Conventional Commits — see `CONTRIBUTING.md`.
- **Body:** what changed and why, wrapped at 72 columns.
- **Read `git status` before `git add -A`**, or stage explicit paths, which is
  better. Agents and build steps write scratch into the repository while they run.

#### 7.2 Branch Naming

`type/short-description`: `feat/sse-register-endpoint`,
`fix/release-on-stream-drop`, `refactor/extract-liveness-probe`.

#### 7.3 The remote, and what `main` refuses

Initialised 2026-09-13 on `main`, with `origin` at
`git@github.com:exustash/trainsty.git`. **The repository is public.**

`main` is protected, and the protection is **destructive-only**:

| Setting | State |
| ------- | ----- |
| Force-push | **Refused, for everyone** |
| Branch deletion | **Refused, for everyone** |
| `enforce_admins` | **On** — the owner is not exempt |
| Required status checks | None. **There is no CI to require** |
| Required pull request | None. A direct push to `main` is allowed |

**So `main` cannot be destroyed, and nothing checks what lands on it.** That is
the deliberate shape while there is no CI: the destructive protections cost
nothing, and a required review with no suite behind it gates nothing except a
second look.

Two consequences:

- **The §3.3 gates are voluntary.** Nothing refuses a commit that fails
  `gofmt -l .` or `go test -race`. Wiring a `pre-commit` hook is what makes them
  real, and it is worth doing **before** there is Go code — a race gate that
  arrives after the concurrent handlers do finds bugs instead of preventing them.
- **`enforce_admins` is on, which is stricter than the sibling repositories.**
  There is no admin bypass, deliberately: without it, protection on a
  single-maintainer repository is a setting rather than a constraint. The cost is
  that a genuinely wrong commit on `main` is fixed by a **new commit**, never by
  rewriting history.

**When CI exists**, add the check to this protection rather than replacing it, and
record whether `strict` (up-to-date-before-merge) is worth a rebase per merge.

---

### Quick Reference

| Rule | Trigger | Action |
| --- | --- | --- |
| 1.3 | Signalling a process this session did not start | Pause, name whose process it is, request approval |
| 1.3 | `trainsty stop` while a Job is active | Say which repo is running and that it becomes invisible to the scheduler (ADR-009) |
| 1.4 | About to add a dependency, a flag, a state file, or a concurrency setting | Read the constitution first — it forbids each of these, three in a principle and the flag in Additional Constraints |
| 2 | Any logic change | `go test -race ./...`, not `go test` |
| 2.1 | Lock, queue or release logic changed | ≥80% statement coverage, table-driven, behaviour-named |
| 2.2 | Touching `process/` | Real children the test started, `t.Cleanup` group kill, no hardcoded PID, no magic sleep |
| 2.3 | A release path added or changed | Its own test, killing the holder rather than calling the happy path. Five paths, five tests |
| 3.1 | Before any review | Green first — §3.3 in full |
| 3.1 | Writing the fix for a finding | One commit per finding, test committed first and watched failing, repair narrowly |
| 3.2 | Reviewing anything here | The five checks: mutex across a block, a new release path, an unvalidated PID, an errno as a boolean, an API change |
| 3.3 | Before any commit or push | **`scripts/ci-local.sh`** — `--quick` is what the hook runs, `--everything` is what a merge needs. It adds the two constitutional checks a hand-run set cannot: no dependencies in `go.mod`, `os/exec` only in `runner/` and `daemonctl/` |
| 4.1 | A dependency looks necessary | Justify in Complexity Tracking **before** use; a test-only one counts |
| 5.2 | An error in a cleanup path | Log it and **release anyway** |
| 6.1 | After fixing a bug | Append to `knowledge/ERRORS.md`; `Prevention:` names a test |
| 6.2 | The Daemon's state changed, or it now writes something | Update `data/state.md` + a DDR entry — including a decision to write nothing |
| 6.3 | Component, pattern or dependency change | ADR in `knowledge/architecture/adr.md` |
| 6.6 | An endpoint added or changed | Constitution amendment (MINOR / MAJOR) + `conventions/api.md` + the register row, same commit |
| 6.6 | A rule, value or name stated twice | Carry every restatement in the same commit. The register's file column finds what `grep` cannot |
| 6.6 | Writing that a package, test or gate exists | **Check the tree.** This repository is documentation; a described thing is not a built thing |
| 6.6 | The requirements note and the repository disagree | The vault moves. Record it in `product/decisions.md`, never by editing the mirror |
| 7.3 | Reaching for `git push --force` or a history rewrite on `main` | **It is refused, for everyone.** Fix a wrong commit with a new commit |
| 7.3 | Relying on the remote to catch a failing gate | **It will not.** No CI, no required check, and a direct push to `main` is allowed. **`.githooks/pre-push` is the only enforcement** — wire it with `git config core.hooksPath .githooks`, and `--no-verify` skips it |
