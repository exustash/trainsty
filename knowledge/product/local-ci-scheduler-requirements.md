# Local E2E Scheduler
## Functional & Technical Requirements Specification

### 1. Overview
**The Issue:** 
Developers frequently run multiple clones of repositories on a single local machine. When launching local CI processes—notably end-to-end (E2E) tests—this creates process collisions, massive resource consumption, and execution failures because the E2E applications locally compete for the same resources (e.g., ports, local database locks, headless browser memory).

**The Solution:**
A lightweight local daemon (scheduler) that prevents resource collisions by strictly queuing test executions. It acts as a semaphore—preserving developer terminal workflows while enforcing exclusive, one-at-a-time access to local machine resources.

---

### 2. Functional Requirements

*   **Global CLI Management:** The scheduler operates as a globally installed background CLI tool. Developers explicitly initialize and manage the daemon using terminal commands.
*   **Sequential FIFO Queueing:** To guarantee absolute collision prevention, only one execution lock is granted at a time (strict 1-on-1 queue). All other execution requests are held in an ordered queue.
*   **Real-Time Monitoring:** A lightweight web dashboard accessible at `http://localhost:45678` allows developers to visually monitor the active job and the pending queue order. The dashboard UI fetches updates by polling the server every 2 seconds.
*   **Manual Job Termination:** The web dashboard features a "Stop" button that allows developers to forcefully abort the actively running job. This instantly frees the lock, terminates the running process group, and advances the queue.
*   **Automated Orphan Cleanup:** The system must automatically detect dropped connections or forcefully closed developer terminals (e.g., `Ctrl+C`), automatically releasing the lock without manual intervention to prevent indefinite deadlocks.

---

### 3. Technical Specifications

*   **Backend Tech Stack:** **Go (Golang)**. Distributed as a single, static compiled binary for zero-dependency local installation.
*   **OS Support:** Exclusively targets Unix-based systems (**Linux and macOS**). This enables the native use of process group termination (e.g., `syscall.Kill(-pid, syscall.SIGKILL)`) to completely clean up orphaned headless browsers and child processes if a test suite is aborted.
*   **Daemon Footprint:** Binds to a dedicated, hardcoded port (**45678**) to prevent conflicts with standard frontend/backend application development ports.
*   **Execution Architecture (Traffic Light Model):** The daemon operates strictly as a traffic light. The local CI wrapper script retains control of the actual E2E test execution, keeping all native `stdout`/`stderr` terminal logging intact. The daemon only handles the lock and queue state.
*   **Queue Wait Protocol:** The wait mechanism uses **Server-Sent Events (SSE)** to maintain a persistent connection between the client script and the daemon. This completely circumvents standard HTTP timeouts during potentially long queue waits (e.g., 20+ minutes).
*   **Process Tracking:** The daemon validates the health of the active job via OS-level PID tracking. It initiates a 3-5 second polling loop, running the equivalent of `kill -0 <pid>` to ping the operating system. An `ESRCH` exception triggers an immediate lock release.

---

### 4. API Endpoints

The API is served locally on `http://localhost:45678`.

| Endpoint | Method | Protocol | Purpose |
| :--- | :--- | :--- | :--- |
| `/register` | `GET` | SSE | Connects the local CI script, holding the stream open until the queue lock is granted. Expects `pid` and `repo` query parameters. |
| `/release` | `POST` | HTTP | Called via `try/finally` or `trap` in the CI wrapper script to signal test completion and advance the queue. |
| `/status` | `GET` | HTTP | Polled by the UI every 2000ms to fetch active job details and the pending queue array. |
| `/stop` | `POST` | HTTP | Triggered by the UI "Stop" button. Executes `syscall.Kill(-pid)` to forcefully terminate the target process group. |
| `/shutdown` | `POST` | HTTP | Triggered by the CLI `stop` command to cleanly exit the background daemon. |

---

### 5. CLI Interface

The Go binary provides the following core subcommands for the developer workflow:

*   `e2e-scheduler start` - Spawns the daemon in the background (detached), binds to port 45678, and returns control to the terminal.
*   `e2e-scheduler stop` - Sends a `POST /shutdown` request to the daemon to drop any active locks and cleanly terminate the background process.
*   `e2e-scheduler status` - Pings the local API and prints the currently active job and the length of the queue directly in the terminal output.
*   `e2e-scheduler ui` - Triggers the OS default web browser to open the dashboard (`http://localhost:45678`).
*   `e2e-scheduler help` - Displays the list of available commands and usage instructions.
