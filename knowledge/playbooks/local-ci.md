---
okf_version: "0.1"
type: playbook
title: "Local CI"
description: "How the merge gate works on this workstation: scripts/ci-local.sh's jobs, what --quick deliberately omits, the two gates and which one is strict about jobs that could not run, and the four bugs the gate found in itself on its first run. Read before relying on a green push."
tags: [ci, tooling, local, gates, shellcheck]
timestamp: "2026-09-14"
---

# Local CI

> **There is no GitHub Actions workflow and no required check on `main`**
> (`RULES.md` §7.3). `scripts/ci-local.sh` **is** the gate, not a mirror of one.
> When a workflow appears, keep the job list in lockstep and say so in both places.

## 1. The two gates

| Gate | When | Runs | Strict about unrun jobs | Enforced by |
| ---- | ---- | ---- | ----------------------- | ----------- |
| `.githooks/pre-push` | every push | `ci-local.sh --quick` | **No** | git, bypassable with `--no-verify` |
| By hand, before you rely on green | before a merge, a release, or trusting the suite | `ci-local.sh --everything` | **Yes, if you set `CI_LOCAL_STRICT_UNRUN=1`** | **nobody — see §4** |

Wire the hook once:

```sh
git config core.hooksPath .githooks
```

## 2. The jobs

**Blocking** — a non-zero exit fails the run:

| Job | Checks | Why it exists |
| --- | ------ | ------------- |
| `gofmt` | `gofmt -l .` prints nothing | Formatting is mechanical; a filename in the output is a failure, not a suggestion |
| `go vet` | `go vet ./...` | |
| `go build` | `go build ./...` | |
| `go test -race` | the whole suite **with the race detector** | **The gate.** A race here is a wrong Grant — two suites at once, the exact failure the product prevents |
| `coverage floor` | `scheduler/` ≥ 80% statements | The constitution's floor, on the package it names |
| `zero dependencies` | `go.mod` declares nothing | Principle I, made mechanical. The cheapest gate here, guarding the property that lets the binary install anywhere |
| `os/exec boundary` | `os/exec` imported only by `runner/` and `daemonctl/` | Principle II. Both allowed sites are client-side; an import elsewhere means the **Daemon** spawns something (ADR-012) |
| `gitleaks` | secret scan, whole tree | Always runs — a secret can land in any file |
| `shell lint` | every `scripts/*.sh` and `.githooks/*` | |
| `acceptance suite` | the built binary, serially | **Explicit-only** — see §3 |

**Report-only** — findings are printed, the run still exits 0:

| Job | Checks | Why report-only |
| --- | ------ | --------------- |
| `govulncheck` | **stdlib** vulnerabilities | Valuable *because* there are no dependencies: with an empty `go.mod`, the only vulnerable code that can reach the binary is the standard library. A stdlib advisory is a reason to know, not to block a commit |
| `vocabulary` | Go identifiers against the glossary's ban lists | A banned word can legitimately appear in a comment explaining why it is banned |
| `docs-lint` | markdownlint on changed docs | Opt-in with `--lint-docs` |

## 3. `--quick` is a weaker gate, not just a faster one

The push hook runs `--quick`, which **omits the acceptance suite** — the only layer
that exercises the built binary, signal forwarding, and the Lock surviving a killed
holder. **A green push is not a green `--everything`.**

**The acceptance suite is never auto-selected**, and this is the one piece of the
arrangement worth understanding rather than memorising: it **binds port 45678**, the
same machine-wide port a real daemon holds. So:

- it is explicit-only (`--e2e` or `--everything`);
- when the port is busy it reports itself **unrun** rather than failing, and names
  `trainsty stop`.

**This is trainsty's own contention problem applied to trainsty's own test suite.**
The product exists because concurrent local suites collide over machine-wide
resources, and its acceptance suite is one of those suites. Worth noticing rather
than working around — and the eventual joke is that the right fix is
`trainsty wrap -- go test ...`, which cannot be used to test trainsty itself until
trainsty works.

## 4. What nothing enforces

- **`main` has no protection against content.** Force-push and deletion are refused
  for everyone, but there is no CI, no required check, and a direct push to `main` is
  allowed (`RULES.md` §7.3). **The hook is the only thing making the gates real**, and
  `git push --no-verify` bypasses it — the honest name for that is *skipping the gate*,
  not *the gate passed*.
- **A job that could not run is not a job that passed.** The summary says so
  explicitly. By default an unrun **blocking** job still exits 0, so the push hook
  stays usable on a machine with no podman — and, right now, on a repository with no Go
  code. Set `CI_LOCAL_STRICT_UNRUN=1` when green must mean *everything ran*:

  ```sh
  CI_LOCAL_STRICT_UNRUN=1 scripts/ci-local.sh --everything
  ```

## 5. Useful invocations

```sh
scripts/ci-local.sh                  # auto: classify the diff, run what matches
scripts/ci-local.sh --list           # print the plan and exit
scripts/ci-local.sh --quick          # what the push hook runs
scripts/ci-local.sh --everything     # everything, including the acceptance suite
scripts/ci-local.sh --test           # just go test -race
scripts/ci-local.sh --deps           # just the zero-dependency check
scripts/ci-local.sh --sequential     # one job at a time, to debug
scripts/ci-local.sh --help           # the header block
```

Auto mode triggers: a `.go` change pulls in the whole Go set plus the boundary and
vocabulary checks; **a `go.mod` change pulls in the zero-dependency check and
`govulncheck`** — that is the trigger that matters most, because adding a dependency
is exactly the change that must not pass unnoticed; a shell change pulls in the shell
lint; `gitleaks` always runs.

## 6. Prerequisites, and what degrades

| Tool | Needed for | Absent → |
| ---- | ---------- | -------- |
| Go | every Go job | those jobs report **unrun** |
| podman or docker | `gitleaks`, `shell lint` | those jobs report **unrun** (`RUNTIME=` overrides detection) |
| `govulncheck` | stdlib vulnerabilities | **unrun** — `go install golang.org/x/vuln/cmd/govulncheck@latest` |
| `markdownlint` | `--lint-docs` | **unrun** |

**Nothing here fails because a tool is missing.** A gate that refuses to run is worse
than a gate that says what it could not check — which is why the summary distinguishes
*passed*, *failed* and *did NOT run*, and why only a strict run turns the third into
the second.

The two scanner images are **digest-pinned**, reused verbatim from the sibling
repositories where they are already verified. Both pins are multi-arch **index**
digests: a single-architecture pin cannot exec on the other host type, and that fails
a blocking gate for a reason unrelated to what it was checking.

## 7. The gate found four bugs in itself on its first run

Recorded because it is the argument for running a gate rather than reading it, and
because three of the four are the same mistake:

1. **`# shellcheck, e2e) fail the run on error.`** — a comment line **beginning** with
   the tool's name is parsed as a directive (SC1073). The sibling repo's script carries
   a warning about exactly this trap; the warning was copied and then walked into.
2. **The comment explaining the SC2329 disable** began with the same word. Same bug,
   twelve lines from the note warning about it.
3. **`.githooks/*` did not expand** while the directory was empty, so the linter was
   handed a literal path and failed on a file that does not exist — which reads as a
   lint failure rather than an empty directory. Fixed by collecting the file list
   explicitly.
4. **A confounded verification.** The report-only semantics were first "tested" while a
   blocking job was still red, so the exit code proved nothing. The finding is about
   the test, not the script: **an exit code only tells you what you think it does when
   everything else is green.**

**Rule that follows:** no comment line in `scripts/ci-local.sh` may begin with the
linter's name. The header says so, and the gate enforces it.
