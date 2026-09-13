---
okf_version: "0.1"
type: convention
title: "Dashboard Conventions"
description: "Rules for the embedded web dashboard: one file, no build step, no framework, no state of its own, and the known 7-second worst-case lag it must show honestly. Also the escaping rule for the one untrusted value it renders."
tags: [conventions, dashboard, html, javascript, embed]
timestamp: "2026-09-13"
---

# Dashboard Conventions

> The Dashboard is the developer's only view of a Lock they cannot otherwise see.
> Its job is to be **trusted**, which means never showing a state the Daemon did
> not just report.

## What it is

One HTML file with inline CSS and inline JavaScript, served from `dashboard/` via
`go:embed`, at `http://localhost:45678`.

- **No build step, no bundler, no npm.** The constitution's Principle I makes the
  binary self-contained, and a build step would put an asset pipeline in a Go
  project to render two lists.
- **No framework.** The page renders a Job and an ordered Queue and has one
  button. `document.createElement` and `textContent` cover it.
- **No dependency from a CDN.** The Daemon must work with no network at all —
  developers run local CI on planes — and a page that silently loses its layout
  offline is worse than a plain one.

## It holds no state

The constitution's Principle V makes the page a thin poller: **every displayed
value derives from the latest `/status` response.**

- `fetch('/status')` every 2000 ms, render, repeat.
- **No optimistic update.** Pressing Stop does not grey the row out or show
  "stopping…" as though it were state — it posts, then lets the next poll say what
  happened. An optimistic UI here invents the one thing a developer came to the
  page to check.
- **No client-side cache, no `localStorage`.** There is nothing worth remembering
  between loads; the Queue is not the browser's to know.
- A failed poll shows **"cannot reach the daemon"** and keeps trying. It does not
  clear the last known state without saying it is stale, and it does not render an
  empty Queue — *no daemon* and *no jobs* are different facts and look different.

## The lag is real and is shown

The page can be up to **one poll interval plus one probe interval** behind — about
**7 seconds** — for a Job that died without closing its Registration
(`architecture/adr.md` → ADR-005).

So the page shows **elapsed time since the Grant**, computed from
`grantedAt`, rather than a bare "running". A developer watching a counter tick
knows what they are looking at; a static badge that says *running* for seven
seconds after the process died is the page lying.

**Do not fix this with a push channel.** ADR-005 decided the mechanism and names
the lag as the accepted cost.

## The one injection vector

`repo` is developer-supplied, arrives as a query parameter, and is rendered on
this page. **Use `textContent`, never `innerHTML`**, for it and for every value
from `/status`. A template literal assembling markup from a response is rejected
in review.

That is not a theoretical concern here: the value comes from a shell script's
variable expansion in someone else's repository, and the page it lands on can kill
process groups.

## Stop

- **One button, on the Job's row only.** A Waiter has nothing to stop — it is
  waiting.
- **It confirms**, naming the repo and stating that the suite is killed
  immediately. It is the destructive control on the surface, and the developer may
  well be looking at someone else's run.
- It posts to `/stop` with no body and no `pid` — see
  [`api.md`](api.md) → *Validation every endpoint owes* for why naming the target
  would be worse.
- **`Content-Type: application/json`** on the request, which is also part of the
  CSRF floor in `CLAUDE.md` → Security.

## Accessibility and the shape of the page

Not negotiable, and cheap at this size:

- The Job and the Queue are a **table or a list with headers**, not nested divs.
  A queue is tabular data.
- The Stop button is a `<button>` and reachable by keyboard.
- The poll updates announce nothing, but the **stale/unreachable state is text**,
  not only a colour.
- It is legible at a glance on a second monitor: the Job, the repo, the elapsed
  time, and how many are waiting.
