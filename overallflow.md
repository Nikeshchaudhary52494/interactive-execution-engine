# 🌊 Overall System Architecture & Data Flow

This document explains the internal workings of the **Interactive Execution Engine**. It is designed to help developers understand how online coding sandboxes (like LeetCode, Replit, or coding interview tools) actually work under the hood.

---

## 🗺️ High-Level Concept

At its core, this system acts as a **bridge** between a web browser and a secure, isolated Linux environment.

1.  **The Client** sends code and listens for results.
2.  **The Server** acts as a manager, orchestrating resources.
3.  **The Worker (Docker)** creates a disposable "computer" for just that one piece of code.

---

## 🔄 The Lifecycle of a Code Snippet

### Step 1: Session Creation (The Handshake)
**Protocol:** HTTP `POST`

1.  The user types code in the browser and clicks "Run".
2.  The browser sends a `POST /session` request with the code and language (e.g., Python).
3.  **The Engine**:
    *   Creates a `Pending` session object in memory.
    *   Generates a unique `SessionID`.
    *   Stores the code in the session struct.
    *   **Crucially**, it *does not* start the Docker container yet. It waits for the client to connect via WebSocket. This prevents "headless" containers running with no one watching.
4.  The server returns the `SessionID` to the browser.

### Step 2: Connection & Activation
**Protocol:** WebSocket

1.  The browser opens a WebSocket connection to `ws://localhost:8080/ws/session/{id}`.
2.  **The API Layer**:
    *   Validates the ID.
    *   Upgrades the connection to WebSocket.
    *   Starts a "Pump" loop to read input and write output.
3.  **The Engine**:
    *   Detects the connection.
    *   Triggers the **Executor** to start the Docker container.
    *   Moves the session state from `WAITING` to `RUNNING`.

### Step 3: The Sandbox (Docker Execution)
**Mechanism:** Docker API (via Unix Socket)

1.  **The Executor**:
    *   Creates a temporary directory on the host (or inside the shared volume if running in Docker).
    *   Writes the user's code to a file (e.g., `main.py`).
    *   Calls the Docker API to create a container with strict limits (CPU, Memory, Network disabled).
    *   **Mounts** the code directory into the container at `/workspace`.
2.  **Container Startup**:
    *   The container starts and immediately executes the run command (e.g., `python -u main.py`).
    *   The `-u` flag in Python is vital: it forces **unbuffered** output, ensuring real-time streaming.

---

## 📡 Data Flow: How "Real-Time" Works

This is the most critical part of the system. How does a `print()` statement in a Linux container appear instantly on your web page?

### 1. The Output Path (Container → Browser)

```
[Container Process] (python main.py)
       │
       │ Writes to Stdout/Stderr (File Descriptor 1 & 2)
       ▼
[Docker Daemon]
       │
       │ Docker API Stream (Hijacked Connection)
       ▼
[Executor (Go Routine)]
       │
       │ stdcopy.StdCopy() separates stdout/stderr frames
       ▼
[Session Buffer]
       │
       │ Thread-safe Write() to strings.Builder
       │ Triggers "Activity" timestamp update
       ▼
[WebSocket Handler (Go Routine)]
       │
       │ Ticker Loop (every 40ms) checks buffer for new data
       ▼
[WebSocket Connection]
       │
       │ JSON Message: { "type": "stdout", "data": "Hello" }
       ▼
[Browser JavaScript]
       │
       ▼
[DOM / Xterm.js] (User sees text)
```

**Why a Ticker?**
We use a 40ms ticker loop to "poll" the buffer and send diffs. This is often more robust than triggering a write for every single byte, which can overwhelm the WebSocket during massive output bursts (like an infinite print loop).

### 2. The Input Path (Browser → Container)

```
[User Keyboard]
       │
       ▼
[Browser JavaScript]
       │
       │ JSON Message: { "type": "input", "data": "Nikesh\n" }
       ▼
[WebSocket Handler]
       │
       ▼
[Session.WriteInput()]
       │
       │ Resets Idle Timer (Activity Detected)
       ▼
[Docker Attach Stream]
       │
       │ Writes to Container's Stdin (File Descriptor 0)
       ▼
[Container Process] (input() function reads data)
```

---

## ⚙️ Key Technical Mechanisms

### Docker-in-Docker (Sibling Containers)
When you run this project via `docker-compose`, the main application runs inside a container. To spawn *new* containers (for the user code), we don't put Docker *inside* Docker.

Instead, we mount the host's Docker socket:
`-v /var/run/docker.sock:/var/run/docker.sock`

This allows our container to talk to the **Host's Docker Daemon**. When we say "Create Container", the Host creates it side-by-side with our server container (hence "siblings"), not inside it.

### File Sharing (Volumes)
Because the containers are siblings, they don't share a filesystem.
*   **Locally:** We use `bind mounts` (Host Path -> Container Path).
*   **In Docker:** We use a **Named Volume**. Both the Engine container and the Worker container mount the same named volume. The Engine writes code to it, and the Worker runs it.

### Concurrency Limiter (Semaphore)
We use a buffered channel as a semaphore:
`sem := make(chan struct{}, 10)`

*   Before starting a container: `sem <- struct{}{}` (Acquire slot)
*   After finishing: `<-sem` (Release slot)

If 10 users are running code, the 11th will wait until a slot opens. This prevents the server from crashing due to resource exhaustion.

### Graceful Shutdown
When you stop the server (SIGINT):
1.  The HTTP server stops accepting *new* requests.
2.  The Engine calls `Shutdown()`.
3.  It waits for the `WaitGroup` counter to reach zero.
4.  Existing sessions continue until they finish executing.
5.  Only then does the program exit.

---

## 🧠 Summary

This architecture decouples the **Control Plane** (Go Server) from the **Data Plane** (Docker Containers).
*   **Go** handles concurrency, WebSockets, and logic.
*   **Docker** handles isolation, security, and the runtime environment.
*   **WebSockets** provide the real-time, interactive glue.
