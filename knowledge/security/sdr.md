---
okf_version: "0.1"
type: decision-record
title: "Security Decision Records (SDR)"
description: "Log of security decisions for trainsty. SDR-001 settles who may terminate a job: the loopback bind, POST-only, JSON content type and Origin check are sufficient, no shared secret is required, and the residual — another process running as the same developer — is accepted because it could already end the suite directly."
tags: [security, sdr, decisions, csrf, loopback]
timestamp: "2026-09-13"
---

# Security Decision Records (SDR)

> This file exists because `OD-1` was settled.
> [`../document-routing.md`](../document-routing.md) called for it in advance: a
> security decision filed as an ADR would be buried in a list of protocol choices.
>
> The standing security rules that apply to every change are in `CLAUDE.md` →
> **Security**. This file records the decisions behind them.

## Template

```markdown
## SDR-00N — <Decision, stated as the outcome>

- **Status:** proposed | accepted | superseded by SDR-00M
- **Date:** YYYY-MM-DD
- **Context:** what is exposed, to whom, and what an attacker would gain.
- **Decision:** what we do.
- **Residual risk:** what remains reachable, and why that is acceptable.
- **Consequences:** what this makes easy, what it makes hard.
- **Alternatives considered:** each with the reason it lost.
```

## What makes this product's posture unusual

**An unauthenticated local endpoint terminates process groups.** That is the whole
surface, and it is small — but the action it exposes is destructive and irreversible
for the developer's current run.

Two properties shape every decision here:

- **Loopback is not a boundary.** It keeps other machines out. It keeps no local
  process out, and a web page the developer has open can issue requests to it.
- **The attacker with the most access is already inside.** Any process running as the
  developer can end their suite with a signal, without involving trainsty at all.
  A control that stops such a process from using trainsty's endpoint has not stopped
  it from doing the thing.

## Decisions

## SDR-001 — The termination floor is sufficient; no shared secret

- **Status:** accepted
- **Date:** 2026-09-13
- **Context:** `POST /stop` terminates the active Job's whole process group and
  `POST /shutdown` exits the Daemon. Neither is authenticated. Two routes could reach
  them without the developer intending it: **a page in the developer's browser**, and
  **another process on the machine**. Closing `OD-1` meant deciding whether a shared
  secret was needed on top of the transport-level refusals.
- **Decision:** The floor ships and **nothing more**:
  - **Bind `127.0.0.1`**, never `0.0.0.0`. Other machines cannot reach it at all.
  - **`POST` only**; `GET` on a mutating route is `405`. This is what makes an
    `<img src>` or a link inert.
  - **Require `Content-Type: application/json`.** A cross-origin HTML form can only
    send three content types, none of them this one, so a form-based request cannot be
    formed. A cross-origin `fetch` setting it triggers a preflight, which is refused.
  - **Refuse a foreign `Origin`** where one is present.
  - **Take no target parameter** on `/stop` — it acts on whatever the Job is, so a
    stale page cannot name a suite that started after it was rendered.
- **Residual risk:** **another process running as the same developer on the same
  machine can terminate the Job.** Accepted, on the reasoning at the head of this
  file: such a process can already signal the suite directly, so a token would guard
  a door with no wall beside it. **This is a judgement about blast radius, not a
  claim that the endpoint is authenticated** — the worst outcome is one local test
  suite ending early, on a machine the actor already controls.
- **Consequences:**
  - **The dashboard stays a static embedded page.** A token would have to be injected
    into it at serve time, making the page generated rather than embedded and giving
    the Daemon one more piece of state to hold.
  - **The four refusals are a security control, not configuration.** Weakening any of
    them — binding a wider address, accepting `GET`, dropping the content-type
    requirement, adding a `pid` parameter to `/stop` — is a change to this record.
  - **The reasoning is load-bearing and conditional.** It holds because the endpoint
    does nothing an equally-privileged local process could not already do. **If
    `/stop` ever gains an ability beyond that** — writing files, reaching the network,
    acting on something outside the current Job — the residual is no longer bounded
    and this record must be revisited before the ability ships.
- **Alternatives considered:**
  - **A token generated at start and embedded in the served page** — rejected for now
    on the cost above, and it remains the obvious upgrade if the conditional in the
    last consequence is ever breached.
  - **Unix domain socket instead of a TCP port** — genuinely stronger for the
    machine-local case, and rejected because the dashboard is the product's
    observability surface and a browser cannot open a Unix socket. Worth revisiting if
    the dashboard ever stops being a browser page.
  - **Requiring confirmation in the UI as the control** — rejected as a category
    error: a confirmation dialog guards against the developer's own mis-click, not
    against a request that never rendered a dialog.
