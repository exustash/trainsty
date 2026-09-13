---
okf_version: "0.1"
type: playbook
title: "Integrating a Repository's Runner"
description: "Step-by-step procedure for wiring a repository's local CI script to trainsty: the setsid requirement that prevents killing your own shell, the trap that releases on every exit path, the curl invocation that holds the SSE wait open, and how to verify each part before trusting it. Read before adding trainsty to any repository."
tags: [playbook, runner, integration, shell, setsid]
timestamp: "2026-09-13"
---

# Integrating a Repository's Runner

> The **Runner** is the local CI script in the repository being tested. It is not
> part of trainsty (`architecture/adr.md` → ADR-003: the Daemon is a traffic
> light). This playbook is the procedure for making one obey the Lock.
>
> Vocabulary:
> [`../domains/ubiquitous-language.md`](../domains/ubiquitous-language.md). The
> endpoint contract: [`../conventions/api.md`](../conventions/api.md).

## 0. The warning that comes before the steps

**`trainsty` terminates a process *group*, using `kill(-pid)`.** If the PID you
register is not a group leader, one of two things happens:

- the Stop signals nothing useful and the headless browsers survive — the failure
  this tool exists to prevent; or
- **the signal reaches your own foreground process group and kills your shell**,
  along with everything else in it.

So **step 1 is not optional and is not a detail.** The Daemon refuses a
non-leader PID (`conventions/api.md`), which turns the second outcome into a
`400` — but the refusal is the backstop, not the design.

## 1. Make the Runner a process group leader

Re-exec the script under `setsid` if it is not already leading a group:

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

**Verify it before going further.** With the script running:

```sh
ps -o pid,pgid,command -p <the script's pid>
```

`PID` and `PGID` must be equal. If they are not, nothing below is safe.

## 2. Register, and wait for the Grant

```sh
REPO="$(basename "$(git rev-parse --show-toplevel)")"

# Blocks until the Grant. -N disables curl's buffering so the event arrives when
# it is sent rather than when the stream closes.
curl -sN "http://localhost:45678/register?pid=$$&repo=$REPO" \
    | grep -q '^event: grant'
```

- **`-N` matters.** Without it curl buffers and the wait appears to hang past the
  Grant.
- **`-s` and no `--max-time`.** A timeout here defeats the entire reason the wait
  is SSE (`architecture/adr.md` → ADR-004). Never add one.
- A non-zero exit means the Daemon refused or is not running — see step 5.

## 3. Release on every exit path

```sh
release() {
    curl -s -X POST -H 'Content-Type: application/json' \
        http://localhost:45678/release >/dev/null || true
}
trap release EXIT INT TERM
```

- **`trap ... EXIT` covers the ordinary path, the failure path, and `Ctrl+C`.**
  `INT` and `TERM` are listed as well because a bare `EXIT` trap is not run for
  every signal in every shell.
- **`|| true` is deliberate.** A Release that cannot reach the Daemon must not
  turn a passing suite into a failing one, and the Lock is freed anyway when the
  Registration drops (ADR-008). This is the one place a swallowed error is correct,
  and it is correct because a second mechanism covers it.
- **`/release` is idempotent and a no-op is a `200`** — the trap firing after the
  Lock has already been freed is the normal case, not an error.

## 4. Run the suite between them

```sh
npm run test:e2e        # or whatever the repository's suite is
```

Nothing else. **The suite's `stdout` and `stderr` stay exactly where they were** —
that is the whole point of the traffic-light model, and a Runner that pipes,
captures or reformats output has thrown away the reason to use trainsty rather
than a CI server.

## 5. Decide what happens when the Daemon is not running

Three defensible answers, and the Runner must pick one **explicitly**:

| Choice | When it is right |
| ------ | ---------------- |
| **Run anyway, with a warning** | A repository where a solo run is usually safe. The default we recommend: trainsty is a convenience, and a missing daemon should not block work. |
| **Fail with instructions** | A repository whose suite reliably collides. Print `trainsty start`. |
| **Start the daemon** | Tempting and wrong as a default — a script that starts background daemons surprises people, and two Runners racing to start one is a new failure mode. |

Whichever is chosen, **say it in the output.** A silently unscheduled run is
indistinguishable from a scheduled one until two of them collide.

## 6. Verify the integration, in this order

Do not trust any step until the one before it is proven.

1. **Group leadership** — `ps -o pid,pgid` as in step 1. Equal, or stop.
2. **The Grant arrives** — with no other Job, the script proceeds immediately.
3. **The wait works** — start a second Runner in another terminal. It blocks;
   `trainsty status` shows it queued behind the first.
4. **`Ctrl+C` releases** — interrupt the first Runner. The second is granted
   **within a second or two**, not after the probe interval. If it takes five
   seconds, the Registration is not being dropped and something is buffering the
   stream.
5. **Stop kills the children** — open the Dashboard, Stop the Job, then check for
   surviving browsers:
   `pgrep -f 'chrome|chromium|firefox|playwright' | head`. Anything left is a
   process-group defect, not a slow shutdown.

**Step 5 is the one people skip**, and it is the one that catches a non-leader PID
that the Daemon happened to accept.
