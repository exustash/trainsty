---
okf_version: "0.1"
type: playbook
title: "Running trainsty with Podman"
description: "How to use trainsty with Podman: building the image, running the scheduler in a Podman Pod with shared PID and network namespaces, and wrapping containerized suites on macOS and Linux."
tags: [playbook, podman, containers, pod, linux, macos]
timestamp: "2026-09-19"
---

# Running trainsty with Podman

> The **Daemon** is a local process-group scheduler that verifies process group leaders,
> probes liveness via `syscall.Kill(pid, 0)`, and terminates orphaned process trees with
> `syscall.Kill(-pid, SIGKILL)` ([ADR-002](../architecture/adr.md), [ADR-008](../architecture/adr.md)).
>
> In container environments, these checks require **shared PID and network namespaces**.
> Podman provides a first-class solution through **Pods** (`podman pod`), which share
> kernel namespaces across containers natively.

---

## 1. Building the Container Image

`Containerfile` builds a static binary with no external dependencies ([Principle I](../../.specify/memory/constitution.md)):

```bash
podman build -t trainsty -f Containerfile .
```

The container uses `trainsty serve` as its entrypoint, which runs the daemon in the
foreground so PID 1 remains active inside the container.

---

## 2. Pattern 1: Native Podman Pod (All-in-Podman)

Use this pattern when both the scheduler daemon and the test suite runners run inside
Podman containers (e.g. CI environments or container-only workflows).

### Why the Pod pattern works

A Podman Pod shares both networking and the PID namespace across all containers in the pod:

1. **Network sharing (`net`):** The daemon binds to `127.0.0.1:45678`. Other containers in
   the pod reach it directly over loopback (`localhost:45678`).
2. **PID sharing (`pid`):** Containers in the pod see the same PID tree. The daemon's
   `process.Alive(pid)` check, liveness probe, and whole-group `KillGroup` (`syscall.Kill(-pid, SIGKILL)`)
   target real processes inside the pod without namespace isolation errors.
3. **Host port mapping:** Publishing `-p 127.0.0.1:45678:45678` on the pod exposes the web dashboard
   to your host browser at `http://localhost:45678`.

### Step 1: Create the Pod

```bash
podman pod create \
  --name trainsty-pod \
  --share ipc,net,uts,pid \
  -p 127.0.0.1:45678:45678
```

### Step 2: Start the Daemon in the Pod

```bash
podman run -d \
  --name trainsty-daemon \
  --pod trainsty-pod \
  --restart unless-stopped \
  localhost/trainsty:latest
```

Confirm the daemon is listening:

```bash
podman exec -it trainsty-daemon trainsty status
# Or from host:
curl -s http://localhost:45678/status
```

### Step 3: Run Test Suites inside the Pod

Launch your test runner container attached to `trainsty-pod`:

```bash
podman run --rm -it \
  --pod trainsty-pod \
  -v "$PWD:/workspace:z" \
  -w /workspace \
  your-test-image \
  trainsty wrap -- npm run test:e2e
```

A second suite running concurrently in another container attached to `trainsty-pod` will
queue behind the first and wait for the lock.

---

## 3. Accessing via `http://trainsty.local/` (Port 80)

To replace `http://localhost:45678/` with a clean `http://trainsty.local/` URL on standard port 80:

### 1. Map `trainsty.local` in `/etc/hosts`
```bash
echo "127.0.0.1 trainsty.local" | sudo tee -a /etc/hosts
```

### 2. Run the Pod with Port 80
Because trainsty binds to `127.0.0.1:45678` internally and checks allowed origins, run the Pod with Caddy as a lightweight sidecar to bridge external port 80 to internal port 45678:

Create `Caddyfile`:
```caddyfile
:80 {
    reverse_proxy 127.0.0.1:45678 {
        header_up Host localhost:45678
        header_up Origin "http://localhost:45678"
    }
}
```

Start the Pod:
```bash
# 1. Create the pod publishing port 80
podman pod create --name trainsty-pod -p 127.0.0.1:80:80 --share ipc,net,uts,pid

# 2. Start the trainsty scheduler
podman run -d --name trainsty-daemon --pod trainsty-pod localhost/trainsty:latest

# 3. Start Caddy sidecar on port 80
podman run -d --name trainsty-proxy --pod trainsty-pod \
  -v "$PWD/Caddyfile:/etc/caddy/Caddyfile:ro,Z" \
  docker.io/library/caddy:alpine
```

Now open `http://trainsty.local/` in your browser. The dashboard updates in real time and the **Stop** button terminates jobs without origin errors.

---

## 4. Pattern 2: macOS Workstation Hybrid (Recommended for Local Dev)

On macOS, Podman containers run inside an Apple Hypervisor Linux virtual machine
(`podman-machine-default`).

Because macOS Darwin PIDs and Linux VM PIDs live in different operating system kernels,
a daemon inside the Podman VM **cannot** see or signal processes running directly on
the macOS host.

If you are running your tests from your native macOS terminal, run the **trainsty daemon on
macOS** and containerize the test suite with Podman:

```text
Mac Terminal (macOS Host)
  │
  ├── trainsty start                         # native daemon on macOS
  │
  └── trainsty wrap -- podman run --rm ...   # containerized test suite
```

### Procedure

1. **Start trainsty natively on macOS:**
   ```bash
   trainsty start
   ```

2. **Wrap your Podman test run:**
   ```bash
   trainsty wrap -- podman run --rm -v "$PWD:/app:z" -w /app my-e2e-image npm run test:e2e
   ```

3. **Why this is safe:**
   * `trainsty wrap` is a macOS process group leader.
   * `podman run` is a child in that group.
   * If the job is canceled via `Ctrl+C` or the dashboard's "Stop" button, trainsty
     signals the process group. Podman's `conmon` monitor catches the signal, stops the
     container inside the VM, and cleans up resources without leaks.

---

## 5. Pattern 3: Linux Host Native Integration

On Linux hosts, you can run the trainsty daemon container with host namespaces:

```bash
podman run -d \
  --name trainsty \
  --net=host \
  --pid=host \
  --restart unless-stopped \
  localhost/trainsty:latest
```

* `--net=host` binds to host loopback `127.0.0.1:45678`.
* `--pid=host` allows the daemon to probe and signal runner processes on the host.

---

## 6. Teardown and Cleanup

To stop and remove the Podman Pod and daemon:

```bash
podman pod stop trainsty-pod
podman pod rm -f trainsty-pod
```
