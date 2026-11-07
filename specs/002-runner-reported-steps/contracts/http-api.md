# Contract: HTTP API changes for Steps

**Feature**: [../spec.md](../spec.md) · **Date**: 2026-11-05

These are changes to
[`001`'s contract](../../001-serialize-e2e-runs/contracts/http-api.md); everything
not named here is unchanged. Adding `/step` is a **MINOR** constitution amendment
(1.0.0 → 1.1.0). The `/status` change is additive.

## `POST /step` — announce a Step (new)

```http
POST /step?pid=12345&name=Unit%20tests&state=start HTTP/1.1
Content-Type: application/json
```

| Parameter | Required | Validation |
| --- | --- | --- |
| `pid` | yes | positive integer. Compared with the Job's pid; **not** probed |
| `state` | yes | `start` or `end` |
| `name` | yes | 1–128 bytes, no U+0000–U+001F or U+007F (the `repo` rule). Compared exactly, never trimmed |

The guard is the same as `/release`, `/stop` and `/shutdown`.

| Case | Status | Body |
| --- | --- | --- |
| Recorded (started, restarted or ended) | `200` | `{"recorded":true}` |
| No-op: no Job, pid not the Job's, ending a name that is not running | `200` | `{"recorded":false}` |
| `GET` or other method | `405` | `{"error":"method_not_allowed"}` |
| Missing JSON content type | `415` | `{"error":"unsupported_media_type"}` |
| Foreign `Origin` | `403` | `{"error":"forbidden_origin"}` |
| Bad or missing `pid` | `400` | `{"error":"invalid_pid"}` |
| Bad or missing `state` | `400` | `{"error":"invalid_step_state"}` (new code) |
| Bad or missing `name` | `400` | `{"error":"invalid_step_name"}` (new code) |
| 16 Steps running and `name` is a new one | `400` | `{"error":"too_many_steps"}` (new code) |

**Precedence:** the guard (method, content type, Origin), then `pid`, then
`state`, then `name`, then the cap. The first failure is the code returned.

Refusals are logged with the code and with `pid` and `name` **as received**, each
`%q`-quoted and truncated to 128 bytes. `200` responses are not logged.

## `GET /status` — the Job gains `steps` (changed)

```json
{
  "job": {
    "pid": 96032,
    "repo": "teneerai.web-dbchecks",
    "grantedAt": "2026-11-05T09:12:00Z",
    "elapsedSeconds": 363,
    "steps": [
      { "name": "Unit tests (with coverage)", "startedAt": "2026-11-05T09:14:00Z", "elapsedSeconds": 252 },
      { "name": "Scanners: osv", "startedAt": "2026-11-05T09:17:51Z", "elapsedSeconds": 12 }
    ]
  },
  "queue": []
}
```

- `steps` is present whenever `job` is, `[]` when none are running, and never
  `null`. It is oldest first.
- `job` stays `null` with no Job. Waiters carry no Steps.
- Older clients ignore the new key. `trainsty status` and the Dashboard are the
  in-repository readers.
