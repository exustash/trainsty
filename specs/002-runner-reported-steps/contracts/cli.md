# Contract: CLI changes for Steps

**Feature**: [../spec.md](../spec.md) · **Date**: 2026-11-05

These are changes to
[`001`'s CLI contract](../../001-serialize-e2e-runs/contracts/cli.md). The CLI
grows from seven to eight subcommands.

## `trainsty step start|end <name>` (new)

```console
$ trainsty step start "Unit tests (with coverage)"
$ trainsty step end "Unit tests (with coverage)"
```

Reads `TRAINSTY_PID` and sends `POST /step` with a 1 s timeout (`stepTimeout`).

| Situation | Output | Exit |
| --- | --- | --- |
| `TRAINSTY_PID` unset or empty | nothing | `0` |
| `TRAINSTY_PID` set but not a positive integer | `trainsty: step "<name>" not recorded (invalid TRAINSTY_PID)` | `0` |
| Recorded, or a no-op (`recorded:false`) | nothing | `0` |
| No scheduler answers within 1 s | `trainsty: step "<name>" not recorded (no scheduler reachable)` | `0` |
| Refused (`400`, or `404` from an older Daemon) | `trainsty: step "<name>" not recorded (<code>)` | `0` |
| Missing or extra arguments, or a state that is not `start`/`end` | `trainsty: usage: trainsty step start\|end <name>` | `0` |

The stderr wording is for people and is **not** a contract; scripts must not match
it.

**Always `0`, deliberately** (spec clarification Q2, FR-049). This is the CLI's
one exception to "exit codes are the contract": a progress label is not a result a
script branches on, and a `set -e` script must survive it.

`help` line: `step   announce a stage of the running suite to the dashboard`.

## `trainsty wrap` (changed)

Once the Lock is **granted**, the suite's environment gains
`TRAINSTY_PID=<wrap's own pid>`, the pid it registered with. It is not set when the
run is unserialized (no daemon, refused, lost stream), and `wrap` never removes one
it inherited: a nested, unserialized run keeps the outer Job's value. Nothing else about `wrap`
changes: streams, signals and exit status are as in 001.
