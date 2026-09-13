# Contract: The Local HTTP API

**Feature**: [../spec.md](../spec.md) · **Date**: 2026-09-13

Served on **`127.0.0.1:45678`** only. This is a **compatibility surface**: its clients
include hand-written Runner scripts in repositories this project cannot see and cannot
upgrade. Standing conventions:
[`../../../knowledge/conventions/api.md`](../../../knowledge/conventions/api.md).

Adding an endpoint is a MINOR constitution amendment; changing or removing one is
MAJOR.

## Server configuration

| Setting | Value | Why |
| ------- | ----- | --- |
| Address | `127.0.0.1:45678` | Loopback only (SDR-001); hardcoded, no fallback (ADR-011) |
| `ReadHeaderTimeout` | `5s` | Bounds the headers; does not bound a wait |
| `WriteTimeout` | **left set** (`30s`) | Per-request removal on `/register` only — R1 |
| `IdleTimeout` | `120s` | Harmless: `/register` is an active write, not idle |
| Single instance | the successful bind | No PID file, no lock file (ADR-011) |

## `GET /register` — acquire the Lock (SSE)

```http
GET /register?pid=12345&repo=capture.web HTTP/1.1
Accept: text/event-stream
```

| Parameter | Required | Validation |
| --------- | -------- | ---------- |
| `pid` | yes | positive integer; process exists; `Getpgid(pid) == pid` |
| `repo` | yes | 1–128 bytes; no control characters |

**Response, immediately on acceptance:**

```http
HTTP/1.1 200 OK
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive
```

Then **nothing at all** until the Grant, which is one event and the last one:

```text
event: grant
data: {"pid":12345,"grantedAt":"2026-09-13T10:04:11Z"}

```

**Implementation requirements** (R1, and each one is a way to ship this broken):

- `http.NewResponseController(w).SetWriteDeadline(time.Time{})` **before** the wait.
  Not `Server.WriteTimeout = 0`.
- `rc.Flush()` after the headers **and** after the event. Without the second, the Grant
  sits in a buffer and the Runner waits for ever while everything looks healthy.
- `select` on **both** `waiter.grant` and `r.Context().Done()`. The context firing is
  the primary Release signal (ADR-008), not an edge case.
- A re-registration for a queued PID **resumes its position**; for the holding PID it
  is answered with an immediate Grant (ADR-007).

**Refusals** (all before anything enters the queue):

| Status | Body | Cause |
| ------ | ---- | ----- |
| `400` | `{"error":"invalid_pid"}` | Missing, unparseable, or ≤ 0 |
| `400` | `{"error":"no_such_process"}` | `ESRCH` |
| `400` | `{"error":"not_group_leader"}` | `Getpgid(pid) != pid`. **The refusal that prevents a killed shell** |
| `400` | `{"error":"invalid_repo"}` | Missing, too long, or control characters |
| `405` | `{"error":"method_not_allowed"}` | Anything but `GET` |

## `POST /release` — hand the Lock back

```http
POST /release HTTP/1.1
Content-Type: application/json
```

No parameters. Releases the Lock **if the caller holds it**, then Grants to the head
of the queue.

| Status | Body | Cause |
| ------ | ---- | ----- |
| `200` | `{"released":true}` | The Lock was held and is now free |
| `200` | `{"released":false}` | **Nothing held it.** A no-op is a success — a Runner's `trap` fires on paths where the Lock is already free, and a `trap` that prints errors on the normal path gets deleted |
| `405` | `{"error":"method_not_allowed"}` | Not `POST` |
| `415` | `{"error":"unsupported_media_type"}` | Content type is not `application/json` (SDR-001) |

## `GET /status` — the whole state

Polled by the Dashboard every 2000 ms (ADR-005). A snapshot taken under the mutex,
marshalled after releasing it.

```json
{
  "job": {
    "pid": 12345,
    "repo": "capture.web",
    "grantedAt": "2026-09-13T10:04:11Z",
    "elapsedSeconds": 372
  },
  "queue": [
    { "pid": 12408, "repo": "capture.desk", "queuedAt": "2026-09-13T10:05:02Z", "waitingSeconds": 321 }
  ]
}
```

- **`job` is `null`** when the Lock is free. `queue` is `[]`, never `null`, when empty —
  a client must not have to distinguish the two.
- `elapsedSeconds` and `waitingSeconds` are **computed server-side**, so the page needs
  no clock of its own and cannot disagree about "now".
- The same PID **never** appears twice, in `job` or across `queue` (invariant 2).
- Ordered: `queue[0]` is granted next.

## `POST /stop` — terminate the Job

```http
POST /stop HTTP/1.1
Content-Type: application/json
Origin: http://localhost:45678
```

**Takes no parameters, deliberately** (FR-032): it acts on whatever the Job *is*, so a
stale Dashboard tab cannot name a suite that started after it rendered.

Signals the Job's whole Process Group — `syscall.Kill(-pid, SIGKILL)` — then Releases
and Grants to the next Waiter. **The kill happens outside the mutex.**

| Status | Body | Cause |
| ------ | ---- | ----- |
| `200` | `{"stopped":true,"pid":12345}` | Terminated and released |
| `200` | `{"stopped":false}` | **No Job.** Not a failure (FR-031) |
| `403` | `{"error":"forbidden_origin"}` | `Origin` present and not the Daemon's own |
| `405` / `415` | as above | |

## `POST /shutdown` — exit the Daemon

Releases the Lock, closes every Registration, and exits. **It does not terminate the
Job** (ADR-009) — the suite keeps running, unsupervised.

| Status | Body | Cause |
| ------ | ---- | ----- |
| `200` | `{"shuttingDown":true,"jobWasActive":true}` | `jobWasActive` is what lets `trainsty stop` warn *before* confirming |
| `403` / `405` / `415` | as above | |

## `GET /` — the Dashboard

The embedded page (`go:embed`). Any other path is `404` with an empty body.

## Rules that apply to every endpoint

- **A stable `snake_case` error code is the contract**; the message is for humans.
  Adding a code is a contract change.
- **Never leak** an internal error string, a filesystem path, or an errno text. Log the
  detail; return the code.
- **Mutating endpoints are `POST`, refuse `GET` with `405`, and require
  `Content-Type: application/json`.** Not tidiness — a `GET /stop` is reachable from an
  `<img src>` on any page the developer has open, and the JSON content type is what a
  cross-origin form cannot produce (SDR-001).
- **Every no-op is a `200`.** `/release` and `/stop` with no Job are ordinary.
