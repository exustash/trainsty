# Quickstart: Validating Serialized E2E Runs

**Feature**: [spec.md](spec.md) · **Plan**: [plan.md](plan.md) · **Date**: 2026-09-13

How to prove the feature works, end to end, against the built binary. Each scenario
names the requirements it validates so a failure points at a line in the spec rather
than a feeling.

Contracts: [contracts/cli.md](contracts/cli.md),
[contracts/http-api.md](contracts/http-api.md). State and invariants:
[data-model.md](data-model.md).

## Prerequisites

```bash
go version                       # 1.20 or newer (plan.md → Technical Context)
lsof -nP -iTCP:45678             # MUST be empty — see below
uname -s                         # Darwin or Linux only
```

> **Port 45678 must be free before you start, and this is the product's own problem
> applied to itself.** trainsty exists because concurrent local suites collide over
> machine-wide resources; its own validation needs the same machine-wide port. If a
> real Daemon is running, stop it (`trainsty stop`) — the automated suite skips rather
> than failing, but these manual scenarios cannot.

## Build and the gates

```bash
gofmt -l .                       # must print nothing
go vet ./...
go build -o trainsty .
go test -race ./...              # -race is a gate, not an option
```

`go test` without `-race` proves almost nothing here: the whole product is one shared
structure read by concurrent handlers, and a race is a *wrong Grant* — two suites
running at once, the failure the tool exists to prevent.

```bash
go test -race -cover ./scheduler/    # must report ≥ 80% (constitution)
```

## Scenario 1 — Two runs queue, in order

**Validates**: FR-001, FR-002, FR-003, FR-005 · US1 §1–4 · SC-001

```bash
./trainsty start                             # prints the address and the log path

# terminal A
./trainsty wrap -- sh -c 'echo A start; sleep 20; echo A done'
# terminal B, a second later
./trainsty wrap -- sh -c 'echo B start; sleep 5;  echo B done'
```

**Expect**: A prints `A start` immediately. **B prints nothing of its own** but does
report waiting. `A done` appears, then `B start`. `./trainsty status` during the
overlap shows A running and B waiting.

**Fails if**: both start at once (FR-001), or B is granted before A finishes
(FR-005), or B produces suite output while waiting (FR-003).

## Scenario 2 — `Ctrl+C` frees the lock, and reaches the suite

**Validates**: FR-006, FR-036 · US1 §5, §15 · SC-002 · **research R2**

With Scenario 1's two runs overlapping, press `Ctrl+C` in terminal A.

**Expect**: A's suite **stops** — not just `wrap` — and B is granted **within two
seconds**, not after the 3–5 s probe interval.

**Fails if**: the suite keeps running after `Ctrl+C` (signal forwarding missing — the
whole of R2), or B waits ~5 s (the Registration is not being dropped, or something is
buffering the stream), or B never starts (FR-006).

## Scenario 3 — The holder is killed outright *(the mandatory case)*

**Validates**: FR-007, FR-008 · US1 §6 · SC-003 · **the constitution's required test**

```bash
./trainsty wrap -- sleep 300 &        # terminal A
./trainsty wrap -- sh -c 'echo B ran' # terminal B — waits
./trainsty status                     # note A's pid
kill -9 <A's pid>                     # NO chance to release
```

**Expect**: `B ran` within **five seconds**.

**Fails if**: B never starts — the Lock is held by a dead Job, which is the product's
one catastrophic failure. Diagnose with
[`../../knowledge/playbooks/stuck-lock-recovery.md`](../../knowledge/playbooks/stuck-lock-recovery.md),
now starting from the log.

> This is the scenario the constitution makes mandatory. Everything else can work and
> the product is still unusable if this one does not.

## Scenario 4 — A long wait is not severed

**Validates**: FR-003 · US1 §7 · SC-004 · **research R1**

```bash
./trainsty wrap -- sleep 1900 &            # holds for ~32 minutes
./trainsty wrap -- sh -c 'echo finally'    # waits the whole time
```

**Expect**: `finally` prints after ~32 minutes. **Fails if** the waiter errors at
~30 s or ~30 minutes — a write deadline is still in force, which means the
per-request `SetWriteDeadline(zero)` is missing or `Server.WriteTimeout` is being
relied on.

**Shorten it for iteration** by temporarily lowering nothing — set `WriteTimeout` to
`5s` in a local build and confirm a 30-second wait still succeeds. A wait that
survives a deliberately short server-wide timeout proves the per-request removal
works, in seconds rather than half an hour.

## Scenario 5 — A non-leader PID is refused

**Validates**: FR-010, FR-035 · US1 §9

```bash
# $$ in a subshell is not a group leader
curl -sN "http://localhost:45678/register?pid=$$&repo=probe" -w '\n%{http_code}\n'
```

**Expect**: `400` with `{"error":"not_group_leader"}`. **Fails if** it is accepted —
a later Stop would then signal the wrong process group, which is how a developer's
shell gets killed.

Confirm `wrap` cannot reach this state: `ps -o pid,pgid` on a wrapped suite's process
must show equal values.

## Scenario 6 — The dashboard reflects reality

**Validates**: FR-021–FR-026 · US2 §1–6 · SC-006

```bash
./trainsty ui
```

With one running and two waiting, **expect**: the running repo with an **advancing**
elapsed time, and both waiters in grant order. Then:

- `./trainsty stop` → the page says **it cannot reach the scheduler**, and keeps
  retrying without a reload. It must **not** render an empty queue (FR-023).
- Restart, and wrap with an awkward label:
  `cd /tmp && mkdir -p '<b>x' && cd '<b>x' && trainsty wrap -- sleep 30`. The label
  renders as **text** (FR-025).

**Fails if**: a static *running* badge (FR-022), a duplicated PID (FR-026), or the
label altering the page (FR-025).

## Scenario 7 — Stop kills the whole tree

**Validates**: FR-027–FR-032 · US3 §1–6 · SC-005

```bash
./trainsty wrap -- sh -c 'sleep 300 & sleep 300 & sleep 300 & wait'
pgrep -f 'sleep 300' | wc -l      # expect 3
```

Press **Stop** in the dashboard and confirm.

```bash
pgrep -f 'sleep 300' | wc -l      # expect 0
```

**Expect**: confirmation names the repo; all three children gone; the next waiter
granted. **Fails if any child survives** — the signal went to a PID rather than a
group (FR-028), which is the defect that leaves headless browsers holding ports.

Then press Stop with nothing running: **nothing happens and no error is shown**
(FR-031).

## Scenario 8 — Shutdown does not kill the suite

**Validates**: FR-014 · US1 §12 · ADR-009

```bash
./trainsty wrap -- sleep 120 &
./trainsty stop                   # expect a prompt naming the repo
# answer y
pgrep -f 'sleep 120'              # expect it to STILL be there
```

**Expect**: the prompt appears and states the consequence; after confirming, the suite
**survives**. **Fails if** the suite dies — that is Stop's behaviour, not Shutdown's,
and conflating them destroys work the developer did not ask to lose.

## Scenario 9 — No daemon is not a blocker

**Validates**: FR-018, FR-038 · contracts/cli.md

```bash
./trainsty stop || true
./trainsty wrap -- sh -c 'echo ran unscheduled'; echo "exit=$?"
./trainsty status; echo "exit=$?"
```

**Expect**: the suite **runs**, with one `trainsty:` notice that it was not
serialized, exiting `0`. `status` prints the unreachable message and exits **3** —
distinct from `1`, so a script can tell *no scheduler* from *scheduler said no*.

## Scenario 10 — The log exists and is never read

**Validates**: FR-013, FR-013a, FR-013b · DDR-002

```bash
tail -20 ~/Library/Logs/trainsty.log        # or the XDG path on Linux
ls -l ~/Library/Logs/trainsty.log           # expect mode 0600
```

**Expect**: grants, releases and refusals with causes; **nothing about what any suite
was testing**. Then the load-bearing half — with the Daemon stopped, append a
fabricated line to the log, start it, and confirm `./trainsty status` reports **idle**:

**the Daemon must not have read it.** A Daemon that reads its own log is a Daemon that
can inherit a stale lock, which is what DDR-001 exists to prevent.

## Coverage map

| Story | Scenarios |
| ----- | --------- |
| **US1 (P1)** — the lock | 1, 2, 3, 4, 5, 8, 9, 10 |
| **US2 (P2)** — the dashboard | 6 |
| **US3 (P3)** — termination | 7 |

**Ship-alone check**: scenarios 1–5 and 8–10 pass with no dashboard and no Stop built,
which is what makes US1 an independently shippable MVP.
