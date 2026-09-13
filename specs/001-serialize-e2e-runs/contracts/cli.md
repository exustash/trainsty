# Contract: The Command-Line Interface

**Feature**: [../spec.md](../spec.md) · **Date**: 2026-09-13

Six public subcommands, one hidden one. Exit codes are part of the contract because
scripts branch on them.

## Exit codes, across every subcommand

| Code | Meaning |
| ---- | ------- |
| `0` | Success |
| `1` | The command failed for its own reason |
| `3` | **The Daemon is unreachable.** Distinct so a script can tell *no scheduler* from *the scheduler said no* (FR-018) |
| — | `wrap` is the exception: it exits with **the suite's** status |

## `trainsty wrap -- <command> [args...]`

The Runner (ADR-012). The whole of a repository's integration.

```console
$ trainsty wrap -- npm run test:e2e
trainsty: waiting for the lock (2 ahead)…
trainsty: granted after 4m12s

> e2e tests
… the suite's own output, untouched …

$ echo $?
0
```

**Behaviour, in order:**

1. Resolve the repo label: `basename` of the git toplevel, else of `$PWD` (FR-039).
2. Start `<command>` with `SysProcAttr{Setpgid: true}` so **the child leads its own
   group** (R2, FR-035).
3. `GET /register` with the **child's** PID, and wait.
4. Forward `SIGINT`/`SIGTERM` to `-childPid` — **without this, `Ctrl+C` never reaches
   the suite** (R2).
5. `Wait()`, then `POST /release`, then exit with the child's status (FR-037).

**Non-negotiable properties:**

| Property | Requirement |
| -------- | ----------- |
| Output | `Stdout`/`Stderr` inherited directly. **No pipe, no capture, no prefix** — a pipe alone would make the suite think it is not a terminal and disable colour |
| Exit status | The suite's own. Swallowing a failure turns a red suite green (FR-037) |
| No Daemon | **Run anyway**, print `trainsty: no scheduler reachable — this run is NOT serialized` once, and exit with the suite's status (FR-038) |
| Release | On every path: success, failure, signal (FR-036) |
| Its own status messages | To **stderr**, prefixed `trainsty:`, so they never pollute a suite's parsed stdout |

**Progress output is only printed when the run actually waits.** A granted-immediately
run prints nothing before the suite — wrapping a fast suite must not add noise.

> **Interactive suites are out of scope.** A suite in a background process group that
> reads the terminal receives `SIGTTIN` and stops (R2). Watch mode is not a queued
> batch run; `help` says so in one line.

## `trainsty start`

Spawns the Daemon detached (R3) and returns the terminal.

```console
$ trainsty start
trainsty: listening on http://localhost:45678
trainsty: logging to /Users/you/Library/Logs/trainsty.log
```

- Re-executes itself as the hidden `serve` subcommand with `Setsid: true`.
- **Verifies the bind before exiting `0`** — polls `/status` for up to ~1 s. Without
  this, `start` reports success for a daemon that died on a taken port and ADR-011's
  error lands in a log nobody is watching.
- **Prints the log path** (FR-013a), so nobody discovers it during an incident.

| Failure | Output | Code |
| ------- | ------ | ---- |
| Already running | `trainsty: already running (pid 900)` + `trainsty status` hint | `1` |
| Port taken by something else | `trainsty: port 45678 is in use by another program` + `lsof -nP -iTCP:45678` | `1` |

Distinguishing those two matters because the remedies differ (ADR-011).

## `trainsty stop`

`POST /shutdown`. **The one interactive prompt in the product.**

```console
$ trainsty stop
trainsty: capture.web is running (6m12s elapsed).
          Shutting down frees the lock but does NOT stop that suite —
          it keeps running, and the next scheduler will not know about it.
Shut down anyway? [y/N]
```

- **Prompts only when a Job is active**, naming the repo (FR-014, ADR-009).
- `--force` skips the prompt, for scripts.
- Not a TTY and no `--force`: **refuse** rather than assume yes.
- No Job: shuts down silently, exit `0`.

## `trainsty status`

```console
$ trainsty status
running: capture.web (pid 12345) for 6m12s
waiting: 2
  1. capture.desk (pid 12408) for 5m21s
  2. postman (pid 12511) for 1m02s
```

Idle prints `idle: nothing holds the lock`. Unreachable prints
`trainsty: no scheduler reachable on port 45678` and exits **3**.

## `trainsty ui`

Opens `http://localhost:45678` — `open` on darwin, `xdg-open` on linux. If the opener
fails, **print the URL** and exit `0`: the developer can click it, and a failed browser
launch is not a failed command.

## `trainsty help`

Lists the six public subcommands and **one line saying what trainsty is for** — the
name gives nothing away, where the requirements note's `e2e-scheduler` did (ADR-010).
Mentions the interactive-suite limitation. `serve` is **not** listed.

## `trainsty serve` — hidden

Runs the Daemon in the foreground. An implementation detail of `start` (R3), not a
command to reach for. Excluded from `help`; documented in its own `--help`.
