---
okf_version: "0.1"
type: convention
title: "API Conventions"
description: "The five-endpoint local API as a compatibility and trust boundary: the exact contract of /register, /release, /status, /stop and /shutdown, the SSE mechanics that a default http.Server breaks, the timeouts that must be zero, and the validation every endpoint owes. Read before changing any handler."
tags: [conventions, api, http, sse, contract]
timestamp: "2026-09-13"
---

# API Conventions

> The API is the contract between the Daemon and every Runner script in every
> repository on the machine. **Those scripts are not in this repository and are
> not versioned with it** — which makes this surface closer to a published API
> than to an internal interface, even though it never leaves loopback.
>
> The constitution fixes the surface: adding an endpoint is a MINOR amendment,
> changing or removing one is MAJOR.

## The contract

Served on `127.0.0.1:45678` — **loopback, never `0.0.0.0`**
(`CLAUDE.md` → Security).

| Endpoint | Method | Protocol | Purpose |
| --- | --- | --- | --- |
| `/register` | `GET` | SSE | Holds the stream open until the Grant. Requires `pid` and `repo` query parameters. |
| `/release` | `POST` | JSON | Called from the Runner's `trap`. Signals completion and advances the Queue. |
| `/status` | `GET` | JSON | Polled by the Dashboard every 2000 ms. The Job and the ordered Queue. |
| `/stop` | `POST` | JSON | The Dashboard's Stop control. Terminates the Job's Process Group. |
| `/shutdown` | `POST` | JSON | `trainsty stop`. Releases the Lock and exits — **it does not kill the Job** (ADR-009). |

`/` serves the Dashboard. Every other path is a 404 with no body.

## `/register` — the one that is easy to break silently

```text
GET /register?pid=<group leader pid>&repo=<label>
Accept: text/event-stream
```

Response, immediately:

```text
HTTP/1.1 200 OK
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive
```

Then nothing until the Grant, which is one event and the last one:

```text
event: grant
data: {"pid":12345,"grantedAt":"2026-09-13T10:04:11Z"}
```

Four ways to get this wrong, all of which pass a short test:

- **`http.Server.WriteTimeout` and `IdleTimeout` must be zero.** A non-zero
  `WriteTimeout` severs the wait the endpoint exists to hold open — and it does it
  at the timeout, so a suite that queues for 90 seconds passes and one that queues
  for 20 minutes does not. This is the single most likely way to ship a broken
  Daemon (ADR-004). `ReadHeaderTimeout` **should** be set; it bounds the headers,
  not the body.
- **`Flush()` after the event, or it never arrives.** Go buffers the response;
  without an explicit `http.Flusher.Flush()` the Grant sits in the buffer and the
  Runner waits forever while every metric looks healthy.
- **Refuse a `pid` that is not a group leader.** `400` with
  `{"error":"not_group_leader"}`. The Daemon cannot otherwise know, and the
  consequence of accepting one is signalling the developer's own shell — ADR-002.
- **Validate before queueing, not at Grant time.** A malformed Registration that
  reaches the Queue is a Grant to nobody, and the Lock is then held by an entry
  that can never Release.

**A re-registration for a queued `pid` resumes its position**; for the `pid` that
is already the Job it is answered with an immediate Grant (ADR-007).

## Validation every endpoint owes

`pid` and `repo` are **untrusted input** — the constitution says so, and on this
API the reason is concrete rather than theoretical.

| Field | Rule |
| ----- | ---- |
| `pid` | Required. A positive integer within the platform's PID range. Must exist, be signalable, and satisfy `getpgid(pid) == pid`. |
| `repo` | Required, but a **label only** (`architecture/adr.md` → OD-3). Bounded length, and **escaped where the Dashboard renders it** — it reaches an HTML page, so it is the one injection vector this API has. |

`/stop` and `/shutdown` take no parameters: `/stop` acts on whatever the Job is,
which means a stale Dashboard tab cannot stop a job that has since been replaced
by naming it. **That is deliberate** — the alternative is a `pid` parameter that
turns a stale click into a kill of the wrong suite.

## Responses

- **JSON for everything but `/register`.** One shape, always: a `200` with the
  payload, or a non-2xx with `{"error":"<stable_snake_case_code>"}`.
- **The code is the contract; the message is for humans.** The Dashboard and the
  Runner branch on the code, so the set is fixed and additions are reviewed as a
  contract change.
- **Never return an internal error string, a path, or an errno text.** Log the
  detail, return the code.
- **A no-op is a success.** `/release` with no Job, `/stop` with no Job, a
  `/release` for a Job that already ended — all `200`. The Runner's `trap` fires
  on every exit path including the ones where the Lock is already free, and a
  `trap` that prints an error on the normal path is a `trap` developers delete.

## Method discipline

Every mutating endpoint is `POST` and **refuses `GET`** with `405`. This is not
REST tidiness: a `GET /stop` can be triggered by an `<img src>` on any page the
developer has open, and the Daemon kills process groups. `CLAUDE.md` → Security
carries the rest of that floor — the `Origin` check and the non-simple content type —
and [`../security/sdr.md`](../security/sdr.md) → **SDR-001** is the decision that the
floor is *sufficient*: no token, with the residual risk stated and the condition under
which it must be revisited.

## Changing the surface

1. The constitution's Additional Constraints name the five endpoints. Adding one
   is a MINOR amendment to `.specify/memory/constitution.md`, in the same change.
2. Update this file, and the table in
   [`../product/local-ci-scheduler-requirements.md`](../product/local-ci-scheduler-requirements.md)'s
   register row — that mirror is read-only, so the register in
   [`../product/decisions.md`](../product/decisions.md) is what moves.
3. **Every Runner script on every machine is a client you cannot see.** A change
   that is not backward compatible needs a note in `README.md`, because there is
   no other way to reach the people it breaks.
