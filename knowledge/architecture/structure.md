---
okf_version: "0.1"
type: architecture-note
title: "Repository Structure"
description: "The repository as it actually stands: the eight Go packages file by file, the documentation set beside them, the names that are not free to change, and which file is a read-only mirror."
tags: [architecture, structure, layout]
timestamp: "2026-09-14"
---

# Repository Structure

`CLAUDE.md` → **Repository Architecture** states the responsibilities;
[`../conventions/go.md`](../conventions/go.md) states the rules that follow from
them. What is added here is the part a listing does not show: what exists, what
does not, and which file must not be edited in place.

## What exists today

**All of it.** The Go tree below is built and released as `v1.0.0` (ADR-013), and
`main` is pushed. Beside the code sit the documentation set, the constitution, and a
vendored Spec Kit:

```text
trainsty/
├── CLAUDE.md                  # conventions, auto-loaded every session
├── RULES.md                   # operating rules
├── LICENSE                    # MIT — required before binaries could ship (ADR-013)
├── .gitignore                 # Go output; .claude/skills/ IS tracked
├── CONTRIBUTING.md            # workflow
├── README.md
├── knowledge/                 # see knowledge/index.md
├── specs/                     # Spec Kit features. 001-serialize-e2e-runs is the
│                              #   first, covering all three priorities
├── .claude/
│   └── skills/speckit-*/      # the Spec Kit slash commands
└── .specify/
    ├── memory/constitution.md # the project constitution, v1.0.0
    ├── templates/             # spec / plan / tasks / checklist
    └── scripts/bash/          # feature + plan + task scaffolding
```

Two things to know about that tree:

- **`.specify/` is vendored and executable.** Its scripts are run by the
  `speckit-*` skills. Reformatting them is not a neutral act.
- **`knowledge/product/local-ci-scheduler-requirements.md` is a byte-identical
  mirror** of a file in an Obsidian vault. An edit here is a defect, not an
  update — [`../product/decisions.md`](../product/decisions.md) → *Re-copying the
  mirror* has the command and the verification.

## The Go tree

```text
trainsty/
├── go.mod                      # requires NOTHING — ADR-001, enforced by ci-local.sh
├── main.go                     # subcommand dispatch only; the table is help's source
├── doc.go                      # what trainsty is, for `go doc`
├── version.go                  # `trainsty version` — read from the build, not -ldflags
├── scheduler/                  # the Lock and the Queue. No net/http, no syscall
│   ├── scheduler.go            #   Register, Grant, Release, Withdraw, Snapshot
│   └── scheduler_test.go       #   99% statements; the constitution's floor is 80%
├── process/                    # the ONLY syscall site
│   ├── group.go                #   Alive (ESRCH vs EPERM), IsGroupLeader
│   ├── kill.go                 #   KillGroup — the one real signal, refuses pid <= 1
│   └── *_test.go               #   real children, always reaped
├── httpapi/                    # the five endpoints
│   ├── server.go               #   mux, loopback bind, timeouts, bind classification
│   ├── guard.go                #   the SDR-001 floor; the only error-body writer
│   ├── register.go             #   SSE: per-request deadline, two flushes
│   ├── status.go               #   snapshot under the mutex, marshal after
│   ├── control.go              #   /release /stop /shutdown
│   ├── probe.go                #   the liveness backstop, owned by the Job
│   ├── log.go                  #   append-only, never read back (DDR-002)
│   └── *_test.go
├── runner/                     # `trainsty wrap` — client-side (ADR-012)
│   ├── wrap.go                 #   becomes a group leader, registers ITSELF
│   └── wrap_test.go
├── daemonctl/                  # start / serve / stop / status / ui — client-side
│   ├── start.go                #   re-exec with Setsid, then VERIFY the bind
│   ├── stop.go                 #   the confirmation prompt (ADR-009)
│   ├── status.go               #   exit 3 when unreachable
│   └── ui.go
├── logpath/                    # the project's only platform branch
├── dashboard/                  # go:embed — one file, no build step
│   ├── embed.go
│   ├── index.html
│   └── embed_test.go           #   greps the page for .innerHTML and remote URLs
├── e2e_test.go                 # ACCEPTANCE — build tag `e2e`, see below
├── scripts/ci-local.sh         # the merge gate
├── scripts/release.sh          # builds the four release archives + SHA256SUMS
└── .githooks/pre-push          # runs ci-local.sh --quick
```

**`e2e_test.go` carries `//go:build e2e`, and the tag is load-bearing.** Without it
the acceptance suite matches `go test ./...` and runs concurrently with the
dedicated acceptance job under `scripts/ci-local.sh` — two runs fighting over port
45678. Run it with `go test -tags e2e -race -p 1 -run TestAcceptance .`, and note
that the gate refuses to report a pass when it finds no cases, because the first
version of that fix turned the failure into a false green.

## Names that are not free to change

| Name | Why it is fixed |
| ---- | --------------- |
| Port `45678` | The Runner's URL is a constant in scripts this repository cannot see. ADR-011 |
| `/register`, `/release`, `/status`, `/stop`, `/shutdown` | The same reason, plus the constitution: changing one is a MAJOR amendment |
| The `pid` and `repo` query parameter names | Part of the same contract |
| `trainsty` | The command name. ADR-010 |

## What is deliberately absent from the tree

- **No `internal/`.** Eight packages in one small binary; the extra path segment
  buys nothing and `go.mod` already makes the module private in practice.
- **No `cmd/trainsty/`.** One binary, so `main.go` at the root is the shorter
  truth. Add the directory when there is a second binary, not before.
- **No `config/`, no `migrations/`, no `assets/`.** There is no configuration
  (ADR-011), nothing persisted (DDR-001), and the one asset is embedded.
- **No `Makefile`.** `go build`, `go test -race ./...`, `gofmt -l .`. A wrapper
  over three commands is a fourth thing to keep correct.
- **No `.github/`.** `main` is pushed and `v1.0.0` is released, so this is now a
  deliberate gap rather than an empty one: `scripts/ci-local.sh` is the gate and
  nothing on the remote checks what lands (`RULES.md` §7.3). A workflow would run
  the same jobs; until one exists, `.githooks/pre-push` is the only enforcement.
