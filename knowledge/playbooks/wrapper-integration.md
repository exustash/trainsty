---
okf_version: "0.1"
type: playbook
title: "Integrating a Repository's Runner"
description: "One line for the supported path — trainsty wrap -- <suite> — and the full hand-written procedure as the fallback, including the setsid requirement that prevents killing your own shell and the six-step verification. Read §1 before writing a Runner by hand."
tags: [playbook, runner, integration, shell, setsid, wrap]
timestamp: "2026-09-13"
---

# Integrating a Repository's Runner

> The **Runner** is whatever holds the Lock while a suite runs. Since **ADR-012** the
> product ships one, so integration is one line and this playbook is mostly the
> fallback.
>
> Vocabulary:
> [`../domains/ubiquitous-language.md`](../domains/ubiquitous-language.md). The
> endpoint contract, for a hand-written Runner:
> [`../conventions/api.md`](../conventions/api.md).

## The supported path

Wrap whatever command runs the suite:

```diff
- npm run test:e2e
+ trainsty wrap -- npm run test:e2e
```

Done. `trainsty wrap` leads its own process group, registers, waits for the Grant,
runs the command with its streams untouched, releases on **every** exit path, and
exits with the command's own status.

Three behaviours to know rather than discover:

| Situation | What happens |
| --------- | ------------ |
| No Daemon running | **The suite runs anyway**, and `wrap` says once that it was not scheduled. A missing scheduler costs a convenience, not the work |
| The suite fails | `wrap` exits with the suite's status. A red suite stays red |
| `Ctrl+C` | The Lock is released and nothing the suite started survives, with no cleanup written by anyone |

**Verify once per repository**, with the suite running: open the Dashboard, press
Stop, then check for survivors —
`pgrep -f 'chrome|chromium|firefox|playwright' | head`. Anything left is a defect in
`wrap`, not in the integration.

---

## Writing a Runner by hand

**Only when `wrap` genuinely cannot be used** — an existing script that must stay in
charge of its own process handling, or a language runtime that already owns the
process group. The API is a compatibility surface, so a hand-written Runner remains
supported ([`../conventions/api.md`](../conventions/api.md)).

### 1. The warning that comes before the steps

**`trainsty` terminates a process *group*, using `kill(-pid)`.** If the PID you
register is not a group leader, one of two things happens:

- the Stop signals nothing useful and the headless browsers survive — the failure
  this tool exists to prevent; or
- **the signal reaches your own foreground process group and kills your shell**,
  along with everything else in it.

The Daemon refuses a non-leader PID, which turns the second outcome into a `400` —
but the refusal is the backstop, not the design. **`wrap` exists so this section does
not have to be read.**

### 2. Make the Runner a process group leader

```sh
#!/usr/bin/env bash
set -euo pipefail

# Become a process group leader so trainsty can kill this run and every child
# it starts — headless browsers included. See ADR-002.
if [ "$$" != "$(ps -o pgid= -p $$ | tr -d ' ')" ]; then
    exec setsid "$0" "$@"
fi
```

On macOS `setsid` is not in the base system; `util-linux` provides it on Linux and
Homebrew's `util-linux` provides it on Darwin. Where it is unavailable, a
`Setpgid`-style equivalent is required — **do not skip the check and register
anyway.**

**Verify before going further.** With the script running:

```sh
ps -o pid,pgid,command -p <the script's pid>
```

`PID` and `PGID` must be equal. If they are not, nothing below is safe.

### 3. Register, and wait for the Grant

```sh
REPO="$(basename "$(git rev-parse --show-toplevel)")"

# Blocks until the Grant. -N disables curl's buffering so the event arrives when
# it is sent rather than when the stream closes.
curl -sN "http://localhost:45678/register?pid=$$&repo=$REPO" \
    | grep -q '^event: grant'
```

- **`-N` matters.** Without it curl buffers and the wait appears to hang past the
  Grant.
- **No `--max-time`.** A timeout here defeats the entire reason the wait is SSE
  (ADR-004). Never add one.

### 4. Release on every exit path

```sh
release() {
    curl -s -X POST -H 'Content-Type: application/json' \
        http://localhost:45678/release >/dev/null || true
}
trap release EXIT INT TERM
```

- **`trap ... EXIT` covers the ordinary path, the failure path, and `Ctrl+C`.** `INT`
  and `TERM` are listed too because a bare `EXIT` trap is not run for every signal in
  every shell.
- **`|| true` is deliberate.** A Release that cannot reach the Daemon must not turn a
  passing suite into a failing one, and the Lock is freed anyway when the Registration
  drops (ADR-008). This is the one place a swallowed error is correct, and it is
  correct because a second mechanism covers it.
- **`/release` is idempotent and a no-op is a `200`.**

### 5. Run the suite between them

```sh
npm run test:e2e
```

Nothing else. **The suite's `stdout` and `stderr` stay exactly where they were** — a
Runner that pipes, captures or reformats output has thrown away the reason to use
trainsty rather than a CI server.

### 6. Decide what happens when the Daemon is not running

`wrap` runs the suite anyway and says so once. A hand-written Runner must pick
**explicitly**:

| Choice | When it is right |
| ------ | ---------------- |
| **Run anyway, with a warning** | The default, and what `wrap` does. trainsty is a convenience; a missing daemon should not block work |
| **Fail with instructions** | A repository whose suite reliably collides. Print `trainsty start` |
| **Start the daemon** | Tempting and wrong as a default — a script that starts background daemons surprises people, and two Runners racing to start one is a new failure mode |

Whichever is chosen, **say it in the output.** A silently unscheduled run is
indistinguishable from a scheduled one until two of them collide.

### 7. Verify, in this order

Do not trust any step until the one before it is proven.

1. **Group leadership** — `ps -o pid,pgid` as in §2. Equal, or stop.
2. **The Grant arrives** — with no other Job, the script proceeds immediately.
3. **The wait works** — start a second Runner in another terminal. It blocks;
   `trainsty status` shows it queued behind the first.
4. **`Ctrl+C` releases** — interrupt the first Runner. The second is granted **within
   a second or two**, not after the probe interval. If it takes five seconds, the
   Registration is not being dropped and something is buffering the stream.
5. **Stop kills the children** — open the Dashboard, Stop the Job, then check:
   `pgrep -f 'chrome|chromium|firefox|playwright' | head`. Anything left is a
   process-group defect, not a slow shutdown.

**Step 5 is the one people skip**, and it is the one that catches a non-leader PID
that the Daemon happened to accept.
