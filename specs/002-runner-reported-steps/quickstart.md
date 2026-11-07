# Quickstart: validate Runner-reported Steps

**Feature**: [spec.md](spec.md) · **Date**: 2026-11-05

## Prerequisites

- A build of this branch: `go build -o ./trainsty .`
- **No real Daemon on 45678.** Check `trainsty status`. If a Job is active, wait
  for it to finish: `trainsty stop` would leave that suite running and unscheduled
  (ADR-009).
- Start the build: `./trainsty start`, and open `./trainsty ui`.

## A stand-in suite

Save as `$TMPDIR/steps.sh` and `chmod +x` it:

```sh
#!/bin/sh
./trainsty step start "Lint";       sleep 4; ./trainsty step end "Lint"
./trainsty step start "Unit tests"; ./trainsty step start "Scanners"
sleep 6; ./trainsty step end "Scanners"
sleep 4; ./trainsty step end "Unit tests"
```

## Scenarios

| # | Run | Expect | Proves |
| --- | --- | --- | --- |
| 1 | `./trainsty wrap -- "$TMPDIR/steps.sh"` | Dashboard: *Lint* ticking, then *Unit tests* and *Scanners* together (oldest first), then only *Unit tests*, then nothing. Exit `0` | US1, US2, FR-044–046 |
| 2 | `curl -s localhost:45678/status` during scenario 1 | `job.steps` as in [contracts/http-api.md](contracts/http-api.md); `[]` between Steps | FR-045 |
| 3 | `"$TMPDIR/steps.sh"` without `wrap` | No output from `step`, exit `0` | US3 #1 |
| 4 | `./trainsty stop`, then scenario 1 again | One `not recorded (no scheduler reachable)` line per announcement; the script's exit status is unchanged | US3 #2, SC-013 |
| 5 | A second terminal: `TRAINSTY_PID=1 ./trainsty step start X` during scenario 1 | Nothing new on the Dashboard | FR-041, SC-014 |
| 6 | Press **Stop** mid-run | Row and Steps disappear together | US1 #4 |
| 7 | `curl -X POST -H 'Content-Type: application/json' "localhost:45678/step?pid=1&name=%0A&state=start"` | `400 {"error":"invalid_step_name"}`, one log line with the name quoted | FR-043, FR-051 |

## Gates

`scripts/ci-local.sh --everything`. The acceptance suite needs port 45678 free:
stop the build from the steps above first.
