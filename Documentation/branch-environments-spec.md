# Branch environments and task execution containers

Status: researched proposal; container infrastructure is not implemented by the Conductor branch.
Researched: 7 September 2026.

## Recommendation

Start with **Docker Compose on the local machine**, with a separate Compose project for every preview. Each instance gets its own image, database volume, network, and localhost URL. Aycorn remains the application that starts, observes, and stops these environments. Docker explicitly describes distinct Compose project names as a way to run stable copies for different feature branches. [Compose project names](https://docs.docker.com/compose/how-tos/project-name/)

Kubernetes manages container workloads across a cluster, including scheduling, replacement, scaling, and service discovery. Those capabilities become useful if environments must run across several machines or serve a team. For this personal, local application, my assessment is that operating a cluster would add more work than value. Preserve a small runtime interface so a Kubernetes driver can be added when that need exists. [Kubernetes overview](https://kubernetes.io/docs/concepts/overview/)

Two related capabilities should have separate lifetimes:

| Capability | Purpose | Lifetime |
| --- | --- | --- |
| Task execution environment | Supplies the toolchain, dependencies, commands, and tests an agent needs | One agent run; retain files and logs afterward |
| Application preview | Runs a specific version of the application for human review | Until stopped or its retention period expires |

Ship application previews first. Add execution containers afterward; Conductor's current Codex worktree runner can continue to produce the code that previews display.

## What exists in this repository

- The Go backend embeds the built React application, so a review preview can use a single application container. A separate Vite container is unnecessary for the first version.
- `app/package-lock.json` and `server/go.mod` / `go.sum` provide dependency inputs. The current Go directive is 1.25.7. Builds also generate the Node-based Markdown conversion bundle before compiling Go.
- `server/internal/worktree` already creates local agent branches and records the branch, base commit, workspace, and diff on an AI run.
- `server/internal/appdb` supports an explicit `AYCORN_DB`, startup migrations, and consistent backups using `VACUUM INTO`.
- The server supports `AYCORN_HOST` and `--port`. It currently starts its job worker automatically; a preview-only switch and readiness endpoint must be added before container previews ship.
- No Dockerfile or Compose configuration is currently checked in.

## User experience

Add a **Preview** action beside each completed run in the task's Code branches section. Also allow choosing a local branch from a project's Environments panel. Creating a preview immediately creates an empty environment record and shows its progress, following Aycorn's create-then-edit convention.

The environment card shows the branch, exact source revision, task/run link, state, age, and resource usage when available. Actions are **Open preview**, **View logs**, **Rebuild**, **Stop**, and **Delete environment**. Stop retains data; deleting data requires confirmation. All controls must have keyboard access. Project defaults and environment names edit in place and autosave.

The preview itself has a persistent “Preview · branch · revision” banner and an “Open main Aycorn” link. Test data and preview edits must never appear in the primary application's database. Opening a preview does not approve a task or merge its branch.

Optional Conductor setting: **Prepare a preview for completed code tasks**, off initially. When enabled, successful implementation requests a preview through the environment service. Review can begin while the build runs. A preview build failure appears beside the handoff; it does not silently reassign or restart the coding agent. The user can inspect logs and explicitly send the task back for another cycle.

## Source identity: a branch name is insufficient

Agent changes may still be uncommitted when the run finishes. Building only the branch's HEAD would then preview the old code. This must be handled explicitly.

1. For a normal branch preview, resolve its name to a commit and create a detached worktree at that commit. Record both the display name and resolved SHA. Git worktrees provide independent checkouts while sharing the object store. [Git worktree](https://git-scm.com/docs/git-worktree)
2. For an agent-run preview, require the run to be inactive. Capture an immutable source snapshot containing tracked modifications and relevant untracked files. Record HEAD, a content digest, and the run ID. Snapshot creation and mutation of that run's workspace must share the existing per-repository coordination.
3. Copy/export the selected source into a managed build context. Never mount the original repository or shared `.git` store inside a preview.
4. Exclude databases, backups, credentials, `.env` files, dependency directories, and nested worktrees. Validate symlinks and paths so an export cannot reach outside its source. Report excluded files when they affect reproducibility. Git LFS and submodules need an explicit supported export path or a clear unsupported result.
5. A running environment always corresponds to its recorded snapshot. Rebuild resolves and records a new revision; it cannot silently replace the revision displayed on an existing environment.

Do not auto-commit to the task branch merely to create a preview. The existing human review/merge flow keeps control of commits and merges.

## Build and execution profiles

An environment profile is a trusted, project-level recipe: display name, base image/toolchain version, setup steps, test command, start command, container port, readiness path, resource limits, data seed strategy, and allowed environment variables. Store commands as executable plus argument arrays where possible; shell scripts are deliberate recipe content, not interpolated branch names or model output.

Initial profiles:

| Profile | Contents and use |
| --- | --- |
| Aycorn preview | Node build stage, Go build stage, slim non-root runtime with Node and the compiled app |
| Node task runner | Pinned Node toolchain, lockfile-based install, unit tests and build commands |
| Go task runner | Toolchain matching the repository directive, module cache, Go tests |
| Browser validation | Separate optional test container with browser dependencies; uses the preview URL |

Use a multi-stage Dockerfile so build toolchains and intermediate files stay out of the preview image. Copy the built frontend and Markdown conversion bundle into the Go build stage, then copy the resulting app into a runtime containing Node and CA certificates. Pin base-image versions/digests when implementing. [Docker multi-stage builds](https://docs.docker.com/build/building/multi-stage/)

Order dependency manifests before application sources in build layers. Use BuildKit caches for downloaded packages and Go build output, keyed by relevant toolchain/lockfile inputs. Cache downloaded dependencies, not mutable shared `node_modules` folders between branches. Validate cache behavior on the host architecture; ARM and x86 builds must not share incompatible artifacts. [Docker build cache](https://docs.docker.com/build/cache/optimize/)

Aycorn's trusted controller generates the Compose file outside the branch checkout. Do not automatically execute a branch-supplied Compose file: it could introduce host mounts, privileged mode, or a Docker socket. Repository Dockerfiles and package scripts still execute code during a build, so build inputs must be the user's selected project and run without host credentials.

## Instance isolation and data

Use a generated identifier such as `aycorn-preview-p12-e81` as the Compose project name. Branch names are display metadata, never shell syntax or globally unique identifiers. Avoid fixed `container_name`, externally named volumes, and shared external networks, which would bypass instance naming.

Every preview receives a separate writable volume at `/data`, with `AYCORN_DB=/data/app.db`. SQLite's database, WAL/SHM files, and backups belong together in that volume. Never bind-mount the primary database. One preview serves one application process; scaling replicas against that SQLite file is out of scope.

Default data policy: create a fresh database, migrate it with the preview binary, then seed deterministic sample data. An optional “copy my project data” flow uses the existing backup operation, sanitizes the copy, and imports only into the preview. SQLite documents `VACUUM INTO` as producing a consistent snapshot without changing the original database. An interrupted snapshot must be discarded. [SQLite VACUUM INTO](https://sqlite.org/lang_vacuum.html)

Before starting a copied database, disable all Conductor projects, prevent all job execution, and remove active job/automation state from the copy. **Pausing Conductor alone is insufficient**, because manually queued jobs could still run. Add `AYCORN_PREVIEW=1` to make the application skip worker startup and reject new AI execution and host-repository operations. This is a proposed setting, not an existing one. Preview builds predating that capability should be refused initially.

A preview does not receive Codex credentials, the MCP executable, SSH keys, the Docker socket, or a host repository path. Older branch schemas should default to fresh seeds; never attempt to downgrade the primary database or reuse a newer preview's volume on an older branch. Rebuilds that require resetting preview data must explain and confirm the reset.

## Ports and readiness

The application listens on `0.0.0.0:8000` **inside** its container. Docker publishes it on an automatically assigned **127.0.0.1 host port**. Discover the allocated port from Docker after startup rather than scanning for an unused port and racing another process. Localhost binding must be explicit; Docker otherwise publishes ports on all host addresses. Require Docker Engine 28 or newer for the documented localhost publishing behavior. [Docker port publishing](https://docs.docker.com/engine/network/port-publishing/)

Add `GET /api/health/ready` that confirms database migrations and a lightweight database query succeeded. A static `GET /` is not sufficient because it could serve the UI while the API is broken. Use a container healthcheck, then `docker compose up --wait --wait-timeout …`; show Open preview only after readiness succeeds. Reconcile actual health after startup, including crashes and OOM exits. [Compose startup readiness](https://docs.docker.com/reference/cli/docker/compose/up/)

First release uses direct localhost URLs, for example `http://127.0.0.1:49172`. A reverse proxy with separate hostnames is a later usability improvement if stable URLs become necessary. Different ports share browser origin-adjacent state such as cookies' host scope; namespace any cookie/session keys or use distinct hostnames before adding authentication to previews. Public or team sharing requires a separate authentication and networking design.

Illustrative generated Compose configuration, to validate during implementation:

```yaml
services:
  app:
    image: aycorn-preview:${AYCORN_PREVIEW_IMAGE_ID}
    environment:
      AYCORN_HOST: 0.0.0.0
      AYCORN_DB: /data/app.db
      AYCORN_PREVIEW: "1" # proposed application capability
    command: ["--port", "8000"]
    ports:
      - target: 8000
        host_ip: 127.0.0.1
        protocol: tcp
        # Omit published: Docker allocates the host port.
    volumes:
      - data:/data
    read_only: true
    tmpfs: ["/tmp"]
    init: true
    cap_drop: ["ALL"]
    security_opt: ["no-new-privileges:true"]
    mem_limit: 768m
    cpus: 1.0
    pids_limit: 128
    restart: "no"
    # Add a readiness healthcheck supplied by the image.
volumes:
  data: {}
```

The image must create a non-root user with ownership of its writable data paths. Resource, filesystem, and port settings above use the Compose service model; they remain proposed defaults until runtime validation. [Compose service reference](https://docs.docker.com/reference/compose-file/services/)

## Controller, persistence, and recovery

Add an `EnvironmentService` with a small runtime interface: build, start, inspect, stop, and destroy. Implement one Compose driver first. This is the adapter pattern already used for the AI runner, without introducing a generic scheduling framework.

Proposed table `task_environment`:

- `id`, `project`, optional `task` and `agentRun`, profile ID/version;
- source kind (`branch_commit` or `run_snapshot`), branch label, commit SHA, snapshot digest;
- state, desired state, operation generation, runtime project name, image ID/digest, allocated URL;
- created/updated/last-opened timestamps, expiration, pinned flag, last error, bounded log location.

States: `queued → snapshotting → building → starting → ready → stopping → stopped`; operations can enter `failed`, with `deleting → deleted` for cleanup. Database state and container side effects cannot share a transaction. Persist intent before the side effect, label resources with environment ID and generation, and reconcile observed runtime state afterward. Repeated clicks/restarts must converge on one instance per generation. Use the labels to find partial builds and orphaned containers after an application restart.

Proposed API:

- `POST /api/project/{id}/environments` with source task/run or branch and profile; creates the environment immediately, returns its ID.
- `GET /api/project/{id}/environments` and `GET /api/environment/{id}` for status.
- `PUT /api/environment/{id}` for name, pin, and retention edits.
- `POST /api/environment/{id}/rebuild` and `/stop` for explicit lifecycle actions.
- `GET /api/environment/{id}/logs` for bounded logs.
- `DELETE /api/environment/{id}` after confirmation for preview data deletion.
- Multi-selection actions use transactional bulk intent endpoints returning `BulkResult`, followed by background reconciliation; no client fan-out.

Use TanStack Query polling initially, following Conductor's existing query pattern. SSE can be added if log volume warrants it. Serialize environment operations by ID and limit concurrent builds; don't create a second coding agent queue implicitly.

## Limits, cleanup, and host support

Proposed defaults: two ready previews, one build at a time, a ten-minute build limit, 768 MiB/one CPU for each preview, and larger separately bounded build resources. Stop unpinned previews after 24 hours without explicit use; show expiry in the UI. Keep stopped data for seven days only if the user has enabled that retention policy. Never remove a task branch or run worktree as a side effect of environment cleanup.

Stop retains the volume. Confirmed deletion removes only that environment's containers, network, data volume, and unreferenced image/snapshot. Compose `down` removes containers and networks; volume removal is an additional operation, so stopping and deleting must remain distinct. Avoid global Docker prune commands. [Compose teardown](https://docs.docker.com/reference/cli/docker/compose/down/)

Prefer rootless Docker on a supported Linux host; Docker documents it as running the daemon and containers without root privileges. Check whether memory, PID, and CPU limits are actually enforced: rootless cgroup limits require suitable cgroup v2/systemd support. Do not claim a limit exists merely because the Compose file contains it. macOS/Windows support can use Docker Desktop, with host-path and architecture tests. Installing or changing Docker/desktop configuration is separate from this proposal. [Rootless Docker](https://docs.docker.com/engine/security/rootless/), [rootless resource limits](https://docs.docker.com/engine/security/rootless/tips/)

## Task containers: second milestone

An execution container mounts only its assigned source workspace and an artifact directory, with a prepared toolchain and explicit setup/test commands. Dependency installation happens during a bounded preparation phase with the required registry access; execution should not depend on packages already installed in the user's working tree. Preserve exit codes, stdout/stderr, duration, and test artifacts separately from the model's written summary.

Keep the durable Conductor scheduler and primary database outside execution containers. If Codex itself moves into the container, introduce a task-scoped MCP bridge and a credential broker rather than mounting the main database or the user's entire Codex home. The current stdio MCP process reads SQLite directly, so that bridge is a real prerequisite. Authentication, environment setup, writable Git metadata, ownership mapping, and sandbox compatibility need a dedicated spike before choosing the final containerized runner design.

A task agent may request a supported environment/profile through a structured request, but the service owns allocation and permissions. It never receives a general-purpose Docker or Kubernetes administration tool. Test-only jobs can finish and be removed while a separate application preview remains available for review.

## When Kubernetes becomes worthwhile

Revisit after there is a demonstrated need for multiple hosts, shared/team previews, remote build workers, or centralized resource scheduling. The corresponding implementation could use a namespace per environment, a Deployment/Service for previews, Jobs for builds/tests, and separate persistent storage. Namespace names alone are not a security boundary: RBAC, network policy, resource quotas, and storage isolation remain required. NetworkPolicy enforcement depends on the cluster's network implementation. [Kubernetes namespaces](https://kubernetes.io/docs/concepts/overview/working-with-objects/namespaces/), [Kubernetes workloads](https://kubernetes.io/docs/concepts/workloads/), [Kubernetes network policies](https://kubernetes.io/docs/concepts/services-networking/network-policies/)

Kubernetes would replace the runtime driver. It would not replace Aycorn's task readiness rules, queue ownership, branch snapshotting, or human review gate.

## Delivery order and acceptance criteria

1. **Container foundation:** trusted Aycorn image/recipe, readiness endpoint, enforced preview mode, fresh seed, and separate localhost instance. Confirm AI cannot start inside a preview.
2. **Manual previews:** run/branch source selection, immutable snapshot capture, status/logs/URL, stop/delete and restart reconciliation. Start two branches with different visible behavior simultaneously; both retain their own database and the main application remains unchanged.
3. **Review integration:** preview cards on task branches, optional Conductor-triggered builds, revision-staleness indicator and retention controls.
4. **Execution profiles:** prepared Node/Go runner environments, actual test artifacts, bounded preparation, scoped MCP bridge and credentials design.
5. **Optional remote driver:** only after local usage establishes requirements.

Required verification includes uncommitted agent changes appearing in previews; exact revision/digest matching; distinct SQLite data; incompatible schema handling; localhost-only publishing; process and resource limits; missing dependencies; build/test failures; disk exhaustion; stop/delete during a build; daemon restart; duplicate create requests; and cleanup restricted to labeled resources. An unhealthy preview must not appear ready, and a preview failure must not advance a task to Done.

Open choices for the implementation phase: whether previews should default to fresh sample data or a sanitized project copy, how many instances this machine should keep, and whether remote sharing is ever needed. The defaults above allow a local first version without deciding those future features now.
