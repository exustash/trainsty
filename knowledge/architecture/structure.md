---
okf_version: "0.1"
type: architecture-note
title: "Repository Structure"
description: "The repository as it actually stands — documentation and a constitution, with no Go code yet — and the intended tree the first commit of code lands in. Says which parts exist today, which are planned, and which file is a read-only mirror."
tags: [architecture, structure, layout]
timestamp: "2026-09-13"
---

# Repository Structure

`CLAUDE.md` → **Repository Architecture** states the responsibilities;
[`../conventions/go.md`](../conventions/go.md) states the rules that follow from
them. What is added here is the part a listing does not show: what exists, what
does not, and which file must not be edited in place.

## What exists today

**No Go code.** The project is documentation, a constitution, and a vendored Spec
Kit, on a fresh `main` whose remote is empty.

```text
trainsty/
├── CLAUDE.md                  # conventions, auto-loaded every session
├── RULES.md                   # operating rules
├── .gitignore                 # Go output; .claude/skills/ IS tracked
├── CONTRIBUTING.md            # workflow
├── README.md
├── knowledge/                 # see knowledge/index.md
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

## The tree the first code lands in

Planned, not present. It follows the dependency direction in
[overview.md](overview.md).

```text
trainsty/
├── go.mod                     # expected to require nothing — ADR-001
├── main.go                    # flag parsing + subcommand dispatch ONLY
├── scheduler/
│   ├── scheduler.go           # the Lock, the Queue, Grant, Release
│   └── scheduler_test.go      #   pure: no server, no children, -race
├── process/
│   ├── group.go               # GroupAlive, KillGroup — the only syscall site
│   └── group_test.go          #   real short-lived children, always reaped
├── httpapi/
│   ├── api.go                 # ServeMux, the five routes, the JSON wire types
│   ├── register.go            # the SSE handler — zero WriteTimeout, Flush
│   └── *_test.go              #   httptest, including the Grant event
├── dashboard/
│   ├── embed.go               # go:embed — what makes one binary serve a UI
│   └── index.html             #   inline CSS + JS, no build step
└── knowledge/, .specify/, …
```

## Names that are not free to change

| Name | Why it is fixed |
| ---- | --------------- |
| Port `45678` | The Runner's URL is a constant in scripts this repository cannot see. ADR-011 |
| `/register`, `/release`, `/status`, `/stop`, `/shutdown` | The same reason, plus the constitution: changing one is a MAJOR amendment |
| The `pid` and `repo` query parameter names | Part of the same contract |
| `trainsty` | The command name. ADR-010 |

## What is deliberately absent from the tree

- **No `internal/`.** Four packages in one small binary; the extra path segment
  buys nothing and `go.mod` already makes the module private in practice.
- **No `cmd/trainsty/`.** One binary, so `main.go` at the root is the shorter
  truth. Add the directory when there is a second binary, not before.
- **No `config/`, no `migrations/`, no `assets/`.** There is no configuration
  (ADR-011), nothing persisted (DDR-001), and the one asset is embedded.
- **No `Makefile`.** `go build`, `go test -race ./...`, `gofmt -l .`. A wrapper
  over three commands is a fourth thing to keep correct.
- **No `.github/`.** The remote exists but nothing is pushed, so there is nothing
  for a workflow to run. When there is, the gate is the four commands in
  `RULES.md` §3.3, which `CONTRIBUTING.md` already names.
