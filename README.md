# Aycorn

A personal, self-hosted task manager that blends Jira-style project tracking with Notion-style flexibility. Runs entirely on your machine — no account, no cloud, no subscription.

---

## Task AI

Open a task and choose **Ask AI** to ask a question, plan work, implement a change, or review the linked repository. Manual AI runs are explicit: changing an owner or stage does not start one. Conductor can automatically manage tasks you delegate to it. The same panel shows progress, Stop, Markdown answers, and preserved file changes. Use **AI** in the sidebar to choose each built-in agent’s OpenAI model and read its fixed instructions and skills. Conductor is the default project orchestrator; configure its workflow stages in Project Settings → Conductor. See [built-in agents](Documentation/built-in-agents.md).

AI requires a current Codex CLI, access to the configured OpenAI model, Node.js for task-description conversion, and the `aycorn-mcp` companion executable. `make dev`, `make build`, and `make install` build the companion. For a release download, also download the matching `aycorn-mcp-<platform>` asset, rename it to `aycorn-mcp` (`aycorn-mcp.exe` on Windows), and place it beside the Aycorn executable. On macOS/Linux, make it executable; on macOS, remove its quarantine flag as with the main binary. Sign in with `codex login` on the server; authentication remains managed by Codex. The startup check reports if the installed CLI is missing required noninteractive features.

Ask and Plan work with just the task. To include code, link a Git repository in project settings. Implement edits an isolated worktree and can run project commands and tests in the Codex workspace sandbox. Command network access is disabled; unavailable dependencies are reported as blockers. Results preserve the actual branch, workspace, and patch for review. Agent branches appear in a **Code branches** section on the task page and drawer. Choose **Review & merge**, select a local destination branch, and inspect the changed files and diff. **Commit & merge** includes uncommitted agent edits; **Merge branch** merges existing commits. Both actions require confirmation. Workspace cleanup remains manual. Existing persona-stage bindings are inactive, and prior AI history is retained.

Branch status comes directly from Git, so merges performed in a terminal also appear on the task. Aycorn checks for active runs, uncommitted destination changes, and stale previews. It tests merges in a temporary workspace; conflicts preserve the agent branch and leave the destination unchanged. Resolve conflicts in the terminal and review again. Commits use your configured Git identity. Branches stay local and are kept after merging.

See [Conductor mode](Documentation/conductor-mode.md) for workflow and agent setup.

## Branch previews with Kubernetes

Run application versions side by side from local branches or completed task runs. Each preview captures an immutable source snapshot, runs its configured tests, and starts with an isolated database and localhost URL. Open **Project Settings → Environments** to configure Kubernetes and build a preview, or use **Preview** beside a task's code branch. Stop preserves preview data; Delete removes it after confirmation. Conductor can prepare previews automatically after successful code tasks.

Docker, kubectl, and a Kubernetes cluster with persistent storage and NetworkPolicy enforcement are required. The included setup script creates a dedicated local kind cluster. `make dev` and `make dev-test` automatically load its default kubeconfig unless you explicitly set `KUBECONFIG`. Project previews let you choose Working tree (including local edits) or Latest commit. See the [Kubernetes setup and verification guide](Documentation/kubernetes-environments.md); the [original research proposal](Documentation/branch-environments-spec.md) records earlier design options.

The [technical architecture](Documentation/kubernetes-architecture.md) explains source capture, orchestration, Kubernetes resources, isolation, and the API. The [installed-system verification report](Documentation/kubernetes-verification.md) records the local setup, tested behavior, and launch instructions.

The [AI redesign review](Documentation/ai-integration-redesign-review.md) explains the replacement architecture. The [implementation and verification notes](Documentation/ai-redesign-implementation.md) record the shipped behavior and remaining validation.

---

## Versioning

`app/package.json`'s `version` field uses a six-digit scheme: `X.X.X.Y.Y.Y`.

- The first three digits (`X.X.X`) track changes Waseem brings into the project.
- The last three digits (`Y.Y.Y`) track Antonio's own changes on top, starting at `0.1.1`.

Example: `0.1.5.0.1.1` is Waseem's `0.1.5` plus Antonio's `0.1.1`.

---

## Install from a pre-built binary (recommended)

A binary is a ready-to-run program — no compiler, no dependencies, no installation wizard. Just download it and run it.

### macOS

1. Go to the [Releases page](../../releases/latest) and download `aycorn-darwin-arm64`.

2. Open Terminal and run the following commands (replace `~/Downloads/aycorn-darwin-arm64` with the actual path to the file you downloaded):

   ```bash
   # Make the file executable
   chmod +x ~/Downloads/aycorn-darwin-arm64

   # Remove the macOS quarantine flag (see Troubleshooting if you skip this)
   xattr -d com.apple.quarantine ~/Downloads/aycorn-darwin-arm64

   # Move it to /usr/local/bin — a folder your system checks when you type a command,
   # so you can run 'aycorn' from anywhere without typing the full path
   sudo mv ~/Downloads/aycorn-darwin-arm64 /usr/local/bin/aycorn
   ```

   `sudo` will ask for your Mac login password.

3. Start Aycorn:
   ```bash
   aycorn
   ```

4. Open the URL shown in the terminal (e.g. **http://localhost:8000**) in your browser.

---

### Linux

1. Go to the [Releases page](../../releases/latest) and download `aycorn-linux-amd64`.

2. In your terminal:

   ```bash
   chmod +x ~/Downloads/aycorn-linux-amd64
   sudo mv ~/Downloads/aycorn-linux-amd64 /usr/local/bin/aycorn
   ```

3. Start Aycorn:
   ```bash
   aycorn
   ```

4. Open the URL shown in the terminal (e.g. **http://localhost:8000**) in your browser.

---

### Windows

1. Go to the [Releases page](../../releases/latest) and download `aycorn-windows-amd64.exe`.

2. Create a folder to keep your personal tools, for example `C:\Users\<YourName>\bin`.

3. Move the downloaded file into that folder and rename it to `aycorn.exe`.

4. Add the folder to your PATH so Windows can find it:
   - Press `Win + R`, type `sysdm.cpl`, press Enter.
   - Click **Advanced** → **Environment Variables**.
   - Under **User variables**, select **Path** and click **Edit**.
   - Click **New** and paste the folder path (e.g. `C:\Users\<YourName>\bin`).
   - Click OK on all dialogs.

5. Open a new Command Prompt or PowerShell window (the PATH change won't apply to windows already open), then run:
   ```
   aycorn
   ```

6. Open the URL shown in the terminal (e.g. **http://localhost:8000**) in your browser.

---

## Build from source

Use this path if you want to contribute, if you're on an unsupported platform, or if you simply prefer to build your own binaries.

**Prerequisites:**
- [Go](https://go.dev/dl/) 1.22 or later
- [Node.js](https://nodejs.org/) 22 or later
- `make` — comes pre-installed on macOS and Linux; Windows users can use [Git Bash](https://gitforwindows.org/) or [WSL](https://learn.microsoft.com/en-us/windows/wsl/)

**Steps:**

1. Clone the repository:
   ```bash
   git clone https://github.com/waseem-polus/aycorn.git
   cd aycorn
   ```

2. Build and install in one step:
   ```bash
   make install
   ```
   This builds the React frontend, bundles it into the Go binary, and copies the result to `/usr/local/bin/aycorn`. It will ask for your password once (needed to write to `/usr/local/bin`).

3. Start Aycorn:
   ```bash
   aycorn
   ```

4. Open the URL shown in the terminal (e.g. **http://localhost:8000**) in your browser.

---

## Running Aycorn

```bash
aycorn
```

Aycorn starts a local web server and prints where your data lives and which port it's on:
```
2025/01/01 12:00:00 Using data directory /Users/you/Library/Application Support/aycorn
2025/01/01 12:00:00 Listening on http://127.0.0.1:8000
```

Open the URL from the `Listening on` line in your browser. Aycorn defaults to port 8000, but automatically tries the next port up if 8000 is already in use. Task storage runs on your machine. Optional AI runs send the selected task and repository context to OpenAI through Codex.

**To stop it:** press `Ctrl-C` in the terminal where it's running. Aycorn waits for any in-progress requests to finish before it exits.

**Accessing Aycorn from another device (Tailscale, LAN):** by default Aycorn binds to `127.0.0.1`, so it's only reachable from the machine it's running on. If you want to reach it from another device (e.g. over [Tailscale](https://tailscale.com/)), set `AYCORN_HOST` to the address you want it to listen on:

```bash
AYCORN_HOST=100.x.x.x aycorn   # bind to your Tailscale IP (find it with `tailscale ip -4`)
```

Or pass it as a flag:
```bash
aycorn --host 100.x.x.x
```

Binding to your Tailscale IP keeps Aycorn reachable only over your private tailnet, rather than opening it to your whole LAN.

**Serving other people.** Everyone signs up with an email and password and gets a personal workspace; organizations invite people by email. These settings matter once other people use your server:

| Variable | What it does |
|---|---|
| `RESEND_API_KEY` | Sends invites, email confirmations, and password resets through [Resend](https://resend.com). Without it, emails are written to the server log, invites show a link to share by hand, and new accounts skip email confirmation. |
| `AYCORN_EMAIL_FROM` | The sender, e.g. `Aycorn <aycorn@yourdomain.com>`. It must be on a domain verified in Resend; Resend's default test sender only reaches your own address. |
| `AYCORN_VERIFY_EMAILS` | With email set up, new accounts must confirm their address before using a workspace. Set to `0` to turn that off. |
| `AYCORN_PUBLIC_URL` | The address used in emailed links, e.g. `https://aycorn.example.com`. Defaults to the address each request came in on. |
| `AYCORN_TLS_CERT`, `AYCORN_TLS_KEY` | Certificate and key files to serve HTTPS directly. |
| `AYCORN_TRUST_PROXY` | Set to `1` when a reverse proxy sits in front of Aycorn, so login rate limits apply per visitor instead of to the proxy. |

Over Tailscale, plain HTTP stays inside your tailnet (and `tailscale serve` can add HTTPS). Anywhere beyond a private network, serve HTTPS: either set the two TLS variables, or put a proxy such as [Caddy](https://caddyserver.com/) in front (`caddy reverse-proxy --from aycorn.example.com --to 127.0.0.1:8000`) and set `AYCORN_TRUST_PROXY=1`.

Login, signup, and password reset attempts are rate limited per address and per account.

**To check the version:**
```bash
aycorn --version
```

**To back up or restore your data:** see [Backup & migrating to a new machine](#backup--migrating-to-a-new-machine).

---

## Upgrading

### From a binary install

1. Download the new binary from the [Releases page](../../releases/latest) (same file you downloaded originally).
2. Stop the running Aycorn (`Ctrl-C` in its terminal, or `pkill -TERM -x aycorn`).
3. Repeat the install step — move the new file to `/usr/local/bin/aycorn` (overwriting the old one).
4. Run `aycorn` to start the new version.

Your database is stored separately from the binary, so your data is never affected by an upgrade. And if a new version needs to update the database schema, Aycorn automatically snapshots your database first — see [Backup & migrating to a new machine](#backup--migrating-to-a-new-machine).

### From a source build

```bash
git pull        # fetch the latest changes (review what's new before this if you like)
make upgrade    # rebuild, reinstall, and stop the old process
aycorn          # start the new version
```

---

## Where your data lives

Aycorn keeps a data directory: `accounts.db` (your accounts, workspaces, and memberships) plus one `workspaces/<id>/app.db` per workspace. Its location depends on your operating system:

| OS | Data directory |
|---|---|
| macOS | `~/Library/Application Support/aycorn/` |
| Linux | `~/.config/aycorn/` |
| Windows | `C:\Users\<YourName>\AppData\Roaming\aycorn\` |

Aycorn prints the exact path on startup — look for the `Using data directory` line.

**To inspect or query your data directly**, you can use the `sqlite3` command-line tool:

```bash
# macOS
sqlite3 "$HOME/Library/Application Support/aycorn/accounts.db"          # accounts, workspaces, memberships
sqlite3 "$HOME/Library/Application Support/aycorn/workspaces/1/app.db"  # one workspace's projects and tasks

# Linux
sqlite3 "$HOME/.config/aycorn/accounts.db"
sqlite3 "$HOME/.config/aycorn/workspaces/1/app.db"
```

> `sqlite3` comes pre-installed on macOS. On Linux, install it with `sudo apt install sqlite3` (Ubuntu/Debian) or `sudo dnf install sqlite` (Fedora). A graphical alternative is [DB Browser for SQLite](https://sqlitebrowser.org/).

**To use a custom data directory** (useful for testing or running multiple instances):
```bash
AYCORN_DATA_DIR=/path/to/data aycorn
```

Tools that act on a single workspace database — `aycorn-mcp` and the `aycorn backup`/`aycorn restore` commands below — take `AYCORN_WORKSPACE=<id>` instead, to pick one workspace out of the data directory, or `AYCORN_DB=/path/to/app.db` to point at an exact file.

---

## Backup & migrating to a new machine

Because all your data lives in one SQLite file, backing up and moving Aycorn is just a matter of snapshotting that file safely. Aycorn does this for you with `VACUUM INTO`, which produces a clean, consistent copy even while the app is running.

### Automatic backups on upgrade

Every time Aycorn starts and finds that a new version needs to update the database schema, it **automatically snapshots your database first** — before applying any change. So an upgrade can never silently lose data; the previous state is always saved.

Snapshots live in a `backups/` folder next to your database, named by timestamp (e.g. `app-20260611-091500-pre-v5.db`). Aycorn keeps the **10 most recent** by default; set `AYCORN_BACKUP_KEEP` to change that (`0` keeps all):

```bash
AYCORN_BACKUP_KEEP=20 aycorn
```

### Manual backup

Make an on-demand snapshot of one workspace at any time (`AYCORN_WORKSPACE` isn't needed for a pre-accounts single-user install):

```bash
AYCORN_WORKSPACE=1 aycorn backup                      # writes a timestamped file into that workspace's backups/ folder
AYCORN_WORKSPACE=1 aycorn backup ~/aycorn-backup.db   # or write to a path you choose
```

### Restore / move to new hardware

To move a workspace to a new machine (or roll back to a snapshot):

```bash
# On the old machine — make a clean snapshot and copy it over
AYCORN_WORKSPACE=1 aycorn backup ~/aycorn-snapshot.db
scp ~/aycorn-snapshot.db newhost:~/

# On the new machine — install Aycorn first, then:
AYCORN_WORKSPACE=1 aycorn restore ~/aycorn-snapshot.db   # validates the snapshot, backs up any existing DB, installs it
aycorn                                                   # start normally; the schema rolls forward automatically
```

`restore` checks the snapshot is a healthy SQLite database, backs up your current database first (so the restore is itself reversible), then swaps the file in. **Stop any running Aycorn before restoring.**

> The version you install on the new machine must be the **same or newer** than the one the snapshot came from — Aycorn only upgrades a database forward, never downgrades it.

---

## Make commands (for source builders)

| Command | What it does |
|---|---|
| `make dev` | Build the frontend and run the Go server using your personal data directory, unless `AYCORN_DATA_DIR` is explicitly set. |
| `make dev-test` | Build and run against the separate, disposable data directory at `server/data`. |
| `make build` | Build the production binary (React + Go bundled together) at `./aycorn`. |
| `make install` | Build and copy the binary to `/usr/local/bin/aycorn` so you can run it from anywhere. |
| `make upgrade` | Rebuild, reinstall, and stop the running instance. Run after `git pull`. Then run `aycorn` to start the new version. |
| `make stop` | Gracefully stop the running `aycorn` process. Does nothing if it isn't running. |
| `make typecheck` | Run the TypeScript type checker on the frontend without building. |
| `make clean` | Delete the built binary and frontend build artifacts. |
| `make backup WORKSPACE=<id>` | Snapshot that workspace's database in your personal data directory, or the path selected by `AYCORN_DB`. Pass `DEST=path` to choose the snapshot destination. |
| `make restore WORKSPACE=<id>` | Restore that workspace's database, or the path selected by `AYCORN_DB`, from `SRC=path`. Stop its server before restoring. |
| `make backup-test WORKSPACE=<id>` / `make restore-test WORKSPACE=<id>` | Snapshot or restore one workspace of the disposable test data at `server/data`. |

---

## Troubleshooting

### Aycorn started on an unexpected port

If the default port (8000) is already in use by something else, Aycorn automatically tries 8001, then 8002, and so on — up to 10 attempts. It always prints the URL it actually landed on:

```
Listening on http://localhost:8001
```

If you want Aycorn to always use a specific port, set it once in your shell profile (e.g. `~/.zshrc` or `~/.bashrc`):

```bash
export AYCORN_PORT=9000
```

Or pass it as a flag each time:

```bash
aycorn --port 9000
```

If all 10 ports in the range are occupied, Aycorn will exit with an error telling you to pick a different starting port using one of the methods above.

---

### macOS: "Apple could not verify this app is free of malware"

This happens because the binary isn't signed with an Apple Developer certificate. It's a standard macOS security prompt for software downloaded from the internet — it doesn't mean the software is harmful.

**Option 1 — Remove the quarantine flag (recommended):**
```bash
xattr -d com.apple.quarantine /usr/local/bin/aycorn
```

**Option 2 — Allow it through System Settings:**
Go to **System Settings → Privacy & Security**, scroll down, and click **Open Anyway** next to the aycorn entry.

**Option 3 — Right-click to open the first time:**
Right-click the file in Finder and choose **Open** (not double-click). A different dialog appears that includes an **Open** button.

After doing any of the above once, macOS will remember and won't ask again.

---

### Windows: "Windows protected your PC" (SmartScreen)

SmartScreen shows this warning for executable files downloaded from the internet that don't have a code-signing certificate. It's the Windows equivalent of the macOS Gatekeeper prompt.

Click **More info**, then click **Run anyway**. Windows will remember your choice for this file.

---

### Windows: "Windows Defender Firewall has blocked some features of this app"

By default Aycorn binds only to `127.0.0.1` (loopback), so it isn't reachable from other devices and shouldn't trigger this prompt. If you're seeing it, you likely set `AYCORN_HOST` (or `--host`) to a wider address like `0.0.0.0` to allow access from other devices. 

If you don't need access from other devices, just leave `AYCORN_HOST` unset — Aycorn will only ever request loopback access.

---

### "My data is empty" / "I don't see my tasks"

Aycorn is probably looking at a different data directory than you expect. Check the startup log:
```
Using data directory /Users/you/Library/Application Support/aycorn
```

If it's pointing at the wrong directory, use `AYCORN_DATA_DIR` to tell it exactly where to look:
```bash
AYCORN_DATA_DIR="/path/to/data" aycorn
```

---

### `make upgrade` says "Already up to date" but the version didn't change

This means git already has the latest code — there's nothing new to pull. Possible reasons:
- You're building from a branch that hasn't been tagged yet. The version will show as something like `v0.1.0-3-gabcdef` (3 commits past the last tag).
- You have local uncommitted changes, which adds `-dirty` to the version string.

Run `aycorn --version` to see exactly what version is installed.
