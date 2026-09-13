# Phase 0 Research: Serialize Local E2E Runs Behind a Single Lock

**Feature**: [spec.md](spec.md) · **Plan**: [plan.md](plan.md) · **Date**: 2026-09-13

Four unknowns blocked the Technical Context. All four are resolved. **Two of the
findings correct guidance already written into the knowledge base** — those are called
out as *Corrects:* and carry a follow-up.

---

## R1 — Holding an SSE stream open for 30+ minutes without disabling timeouts globally

**Decision**: In the `/register` handler, call
`http.NewResponseController(w)` once, then `SetWriteDeadline(time.Time{})` to remove
the write deadline **for that request only**, and `Flush()` after writing the Grant
event. Keep `Server.ReadHeaderTimeout` set. **Do not zero `Server.WriteTimeout`.**

```go
rc := http.NewResponseController(w)
if err := rc.SetWriteDeadline(time.Time{}); err != nil {   // zero == no deadline
    // Not supported by this ResponseWriter — refuse rather than hang.
}
// …later, on Grant:
fmt.Fprintf(w, "event: grant\ndata: %s\n\n", payload)
if err := rc.Flush(); err != nil { /* client gone */ }
```

**Rationale**: `ResponseController` exists precisely for this (Go 1.20+). Its
`SetWriteDeadline` documentation states *"A zero value means no deadline"*, and it
dispatches to the underlying writer by type assertion, unwrapping middleware. The
alternative — `Server.WriteTimeout = 0` — is **server-wide**, so protecting a
20-minute wait would mean every other route also loses its write timeout. Per-request
is strictly better and costs one line.

**Alternatives considered**:

- **`Server.WriteTimeout = 0`** — works, and throws away a useful protection on four
  other routes to fix one. This is what the knowledge base currently prescribes.
- **`http.Flusher` alone** — still required *conceptually*, but `ResponseController`
  supersedes the interface assertion and reports errors (`Flush() error`), where
  `Flusher.Flush()` cannot tell you the client vanished.
- **Hijacking the connection** — full control, and hands us responsibility for the
  HTTP framing we would then have to write by hand.
- **A periodic comment heartbeat (`: ping`)** — not needed for the deadline (the
  deadline is gone), and explicitly **not** a liveness mechanism: ADR-008 gives that
  job to the stream itself plus the PID probe. Skip it.

> **Corrects `knowledge/conventions/api.md`**, which says *"`http.Server.WriteTimeout`
> and `IdleTimeout` must be zero for `/register`"* and names a missing `Flush()` as the
> other trap. The traps are real; the remedy is now per-request. **Follow-up: update
> that convention doc when this lands.**

---

## R2 — `Ctrl+C` and a suite that lives in its own process group

**Decision**: `trainsty wrap` starts the suite with
`SysProcAttr{Setpgid: true}` — making the **child** its own group leader — registers
the **child's** PID, and **forwards `SIGINT`/`SIGTERM` to the child's group**:

```go
cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}   // Pgid==0 ⇒ child leads
// after Start(), child.Pid is a group leader by construction
sigs := make(chan os.Signal, 1)
signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
go func() {
    for s := range sigs {
        syscall.Kill(-cmd.Process.Pid, s.(syscall.Signal))   // the GROUP
    }
}()
```

**Rationale, and this is the finding that matters**: the terminal delivers `Ctrl+C`
as `SIGINT` to the **foreground process group only**. FR-035 requires the suite to be
in a group trainsty can terminate as a unit, which necessarily makes it *not* the
foreground group — so **`Ctrl+C` would reach `wrap` and never reach the suite.** The
suite would keep running while the developer's interrupt appeared to do nothing. So
forwarding is not a nicety; it is what makes US1 §5 and §15 true.

Registering the **child's** PID rather than re-execing `wrap` itself under `setsid`
is the simpler half: `Setpgid` guarantees the registered PID leads its group, so
FR-010's refusal is unreachable by construction, and `wrap` stays an ordinary
foreground process the developer can interrupt.

**Alternatives considered**:

- **`exec setsid "$0"` on `wrap` itself** (what the shell playbook does) — detaches
  `wrap` from the terminal too, which loses `Ctrl+C` *and* complicates output
  inheritance. Worse in every respect now that we control the code.
- **`SysProcAttr{Foreground: true, Ctty: fd}`** — places the child's group in the
  terminal foreground, so `Ctrl+C` reaches the suite directly with no forwarding. It
  needs a real TTY fd and fails when output is piped or run under CI, so it would
  require both paths anyway. **Revisit only if signal forwarding proves lossy**; the
  forwarding path works in both cases with one implementation.
- **Not putting the suite in its own group** — forfeits FR-028, which is the whole
  point of the product being Unix-only.

**Consequence to accept and state**: a suite in a background process group that
**reads from the terminal** gets `SIGTTIN` and stops. Interactive/watch-mode suites
are therefore out of scope for `wrap`, which is correct — a queued batch run is not
an interactive session. Worth one line in `trainsty help`.

> **New — nothing in the knowledge base had noticed this.** It belongs in
> `knowledge/conventions/go.md` under the process rules. **Follow-up when this lands.**

---

## R3 — Detaching the daemon, with no `fork()` in Go

**Decision**: `trainsty start` **re-executes its own binary** with a hidden
`serve` subcommand and `SysProcAttr{Setsid: true}`, redirects the child's three
standard descriptors (stdin to `/dev/null`, stdout and stderr to the DDR-002 log
file), does **not** wait, prints the bound address and the log path, and exits 0.

```go
cmd := exec.Command(self, "serve")                       // self from os.Executable()
cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}     // new session, no TTY
cmd.Stdin, cmd.Stdout, cmd.Stderr = devNull, logFile, logFile
err := cmd.Start()                                        // Start, never Wait
```

**Rationale**: Go's runtime is multi-threaded, so the classic double-`fork()` daemon
idiom is unavailable — only `fork`+`exec` is safe, which is exactly what `os/exec`
does. `Setsid: true` (available on both darwin and linux per the `syscall` docs) makes
the child a session leader with no controlling terminal, so it survives the terminal
closing. Re-exec keeps one binary and one code path.

**The bind must be verified before the parent exits**, or `trainsty start` reports
success for a daemon that died on a taken port. **The parent polls `/status` briefly
(up to ~1 s) and reports the child's failure if it never answers** — without this,
ADR-011's carefully-worded bind error is written to a log nobody is watching.

**Alternatives considered**:

- **A `serve` flag the user is told to run under `nohup`/`&`** — pushes the job to the
  developer and to five different shells' quirks.
- **A launchd/systemd unit** — a second artifact to install, contradicting the
  one-binary promise, and it makes `start`/`stop` someone else's commands.
- **`daemon(3)` via cgo** — cgo for two syscalls, and it breaks static linking.
- **A third-party daemonize library** — a dependency for ~15 lines (Principle I).

**`serve` is hidden from `help`**: it is an implementation detail of `start`, not a
sixth command a developer should reach for. It stays undocumented in `cli.md`'s
public table and documented in its own `--help`.

---

## R4 — Telling *dead* from *not mine to signal*, portably

**Decision**: `process.GroupAlive(pid)` calls `syscall.Kill(pid, 0)` and classifies:

| Result | Meaning | Action |
| ------ | ------- | ------ |
| `nil` | Process exists and is signalable | Alive |
| `syscall.ESRCH` | **No such process** | **Dead → Release** |
| `syscall.EPERM` | Exists, owned by someone else | **Alive, and a bug** — log loudly, do not release |
| anything else | Unexpected | Log, treat as alive |

`process.IsGroupLeader(pid)` uses `syscall.Getpgid(pid)` and compares to `pid`,
mapping `ESRCH` to "does not exist".

**Rationale**: this is ADR-008's reasoning made concrete. Treating any error as
"dead" would release a healthy Job the moment the probe hit an `EPERM`; treating any
non-`nil` as "alive" would deadlock on a dead one. `EPERM` in this product means the
registered PID is **not the developer's process**, which is a registration defect
worth shouting about — never a reason to silently free someone's Lock.

**Alternatives considered**:

- **Comparing process start time** to defeat PID reuse — the portable way to read it
  differs on Darwin (`sysctl`, `KERN_PROC`) and Linux (`/proc/<pid>/stat`), which
  would add cgo or a `/proc` dependency. **Rejected**, with the residual reuse risk
  accepted in ADR-008 and bounded by the stream being the primary signal.
- **`os.FindProcess` + `Signal(syscall.Signal(0))`** — on Unix `FindProcess` never
  fails, so this is the same syscall with an extra allocation and a less direct error.
- **`wait4`/`waitpid`** — only works for the caller's own children. The Daemon never
  spawns the Job (Principle II), so it can never be its parent. **This is the reason
  a liveness probe is needed at all** rather than simply awaiting the child.

---

## Resolved unknowns

| Unknown | Resolution |
| ------- | ---------- |
| Go version floor | **1.20**, set by `http.NewResponseController` (R1) |
| Long-lived SSE without global timeout loss | Per-request `SetWriteDeadline(zero)` + `Flush()` (R1) |
| `Ctrl+C` with the suite in its own group | Forward `SIGINT`/`SIGTERM` to `-childPid` (R2) |
| Detaching without `fork()` | Re-exec self with `Setsid`, verify the bind before exiting (R3) |
| `ESRCH` vs `EPERM` | Classified explicitly; only `ESRCH` releases (R4) |

## Follow-ups this research creates

Both are corrections to documents that already exist, and `RULES.md` §6.6 requires
them to move with the code that proves them — **not now, with the implementation**:

1. `knowledge/conventions/api.md` → replace the global `WriteTimeout` advice with
   per-request `ResponseController` (R1).
2. `knowledge/conventions/go.md` → add the signal-forwarding rule and the `SIGTTIN`
   consequence to the process rules (R2).
