# Kubernetes branch previews: technical architecture

AICorn turns a local Git branch or finished task worktree into a saved source snapshot, tests that snapshot in Kubernetes, and exposes a separate application instance on localhost. Each preview has its own database, lifecycle, diagnostics, and source identity. Source code and project settings are captured once for that environment; Rebuild creates another environment.

This document describes the implementation. See the [setup and operations guide](kubernetes-environments.md) for installation and daily use, and the [verification report](kubernetes-verification.md) for the installed system's test results.

## Components and execution boundaries

The host AICorn server owns orchestration. Kubernetes is the runtime for test Jobs and application Deployments. Codex continues executing through the existing host worker and Git worktrees; this feature does not move the coding agent, its authentication, or its MCP server into Kubernetes.

| Component | Responsibility | Implementation |
| --- | --- | --- |
| React environment views | Project configuration, task preview creation, status, logs, lifecycle actions | [Frontend feature](../app/src/features/environments/) |
| HTTP handlers | Validate requests, resolve project/task ownership, persist intent, return status | [environmentHandler.go](../server/cmd/web/environmentHandler.go) |
| `Store` | SQLite settings, ownership, lifecycle intent, source metadata, capacity, bounded logs | [store.go](../server/internal/environments/store.go) |
| `Service` | Source capture, automatic previews, reconciliation, cancellation, build serialization | [service.go](../server/internal/environments/service.go) |
| `Runtime` interface / `Kubernetes` adapter | Build, start, inspect, stop, delete, logs, managed port-forwards | [types.go](../server/internal/environments/types.go), [kubernetes.go](../server/internal/environments/kubernetes.go) |
| Manifest builders | Namespaces, policies, quota, volume, Job, Deployment, Service | [manifests.go](../server/internal/environments/manifests.go) |
| Source exporter | Export an exact commit or stable inactive worktree without changing Git | [snapshot.go](../server/internal/worktree/snapshot.go) |

The design uses a **reconciliation loop**: the database records what should exist, and the controller repeatedly compares that intent with runtime observations. A runtime adapter keeps CLI side effects separate from lifecycle decisions and enables fake-runtime tests.

The host invokes Git, Docker, kind, and kubectl as subprocesses. Kubernetes operations always receive the environment's explicit `--context`; AICorn does not change the user's current context. The server process must inherit the correct `PATH`, Docker access, and `KUBECONFIG`.

## From request to running application

1. A request identifies a local branch or an inactive agent run. AICorn validates the project repository, configuration, source, and available project capacity.
2. SQLite receives a `queued` environment with `desired=running`, its resolved Git commit, and a complete settings snapshot. Creation returns HTTP 202; work continues asynchronously.
3. The controller exports source into its private storage directory and records a SHA-256 digest.
4. The host Docker daemon builds the preview target, then the optional test target, using the saved recipe and source directory. Images are loaded into kind or pushed to the configured registry.
5. The controller applies the namespace, quota, deny-all network policy, and persistent data claim. It optionally copies a registry pull secret and permits test internet access.
6. If configured, a Kubernetes Job executes the test command. Only successful tests allow deployment; a skipped test configuration is explicitly recorded as `skipped`.
7. The controller applies a one-replica Deployment and a ClusterIP Service, waits for Kubernetes readiness, opens a loopback port-forward, and checks the application health URL through it.
8. The environment becomes `ready`. The frontend polls environment lists every four seconds and displays its localhost URL.

No step commits or merges the source. Creating a preview does not modify the task's workflow stage or approve the task.

## Source identity and capture

**Project branch previews** resolve a local branch to a commit when the request is created. `includeChanges=false` (the API default) exports that commit using `git archive`. `includeChanges=true` captures the branch's checked-out working tree, including local edits, through the same exporter used for task previews. The UI selects the current branch and defaults to Working tree when a checkout is available; Latest commit remains an explicit choice. Later edits or branch changes do not update either kind of saved environment.

**Task previews** reference an agent job and its latest recorded run branch. The job must be `completed`, `failed`, `canceled`, or `interrupted`; active jobs are rejected, and activity is checked again immediately before capture. The run's original repository must still match the project's linked repository, and its worktree must still exist.

Working-tree capture reads tracked files and eligible untracked files from that worktree using `git ls-files --cached --others --exclude-standard`. Deleted files are absent from the export. It reads the files directly, without staging, committing, checking out, or invoking checkout filters. Capture and AICorn's branch merge operations share an in-process mutex.

Files are read twice and hashed; a changed digest, branch HEAD, or checkout location rejects the capture. Working-tree previews check for active agent runs on the same repository/branch at creation and before and after capture; committed previews can proceed while an agent runs. Directory-relative reads use Go's `os.OpenRoot`, and symlinks in leaves or parents are rejected. This detects concurrent edits during capture; the worktree is not mounted into the eventual containers.

The digest includes exported file paths, normalized file modes, lengths, and contents. The Git commit identifies the base revision; the digest distinguishes snapshots containing different uncommitted work at that same revision. A source-status request compares the saved commit and, for worktree previews, current filtered contents. This is an on-demand comparison, not a filesystem watcher.

Exports omit Git metadata, nested worktrees, dependencies, common build output, backups, databases, environment files, and named credential locations such as `.ssh`, `.aws`, `.kube`, and `.codex`. The branch's `.dockerignore` is also excluded: the build context is the already-filtered export. Submodules, source links, and Git LFS pointer files are rejected. Limits are 128 MiB total, 16 MiB per file, and 10,000 files. These filename rules are not a general secret scanner.

On the host, artifacts live beside the main database:

```text
<database-directory>/environments-<installation-token>/
  environment-<id>/
    source/          # exported build context
    snapshot.json    # commit, digest, excluded paths
    Dockerfile       # captured project recipe
```

The controller reuses saved snapshot metadata after interruption. An existing environment is not refreshed from the live source when it starts again. Rebuild creates a new row, captures current source and settings, and allocates fresh preview data; the previous environment remains available until stopped or deleted.

## Persistent state and reconciliation

[Migration 00018](../server/internal/migrations/sql/00018_environments.sql) adds three tables:

| Table | Stored data |
| --- | --- |
| `environment_installation` | One random installation token used in ownership labels and names |
| `environment_settings` | Project settings JSON and the time automatic previews were enabled |
| `task_environment` | Project/task/job references, request key, repository, branch, commit, digest, settings snapshot, actual/desired state, images, test result, URL, error, log tail, pin, expiry, timestamps |

The unique `(project, requestKey)` constraint deduplicates creation, including requests repeated after a restart. The API returns the existing environment for that key. A new key is required for a genuinely new capture.

Project deletion first marks its environments for runtime deletion. References then become null, leaving ownership records available for cleanup. Task/job deletion also preserves environment records through nullable references. The migration is forward-only because dropping these records could orphan cluster resources.

`desired` has three values: `running`, `stopped`, and `deleted`. `state` records observed progress:

| State / transition | Meaning and controller behavior |
| --- | --- |
| `queued → snapshotting → building → starting → ready` | Normal creation; captured source or built images allow intermediate work to be reused |
| `starting` with `testState=running` | Tests run before the application is deployed |
| `ready ↔ unavailable` | Readiness, cluster, or connection interruption; continue inspecting without rebuilding or rerunning tests |
| `failed` | Capture/build/startup/test failure; no automatic execution retry while desired state remains running |
| `stopping → stopped` | Cancel pending work, close the forward, stop pods, retain source and persistent data |
| `deleting → deleted` | Cancel pending work, remove owned runtime resources/images, then remove local source storage |

The controller ticks every three seconds, with at most one in-flight operation per environment. A changed desired state cancels the old operation. Database observations use `UPDATE ... WHERE desired=?`, so a late observation cannot overwrite a Stop or Delete request made while an external command was running. Cleanup continues on later ticks; Stop/Delete is not considered complete until the runtime confirms it.

The full capture/build/startup operation has the configured deadline, including time waiting for the build slot. Cleanup has a 90-second attempt budget and remains retryable after failure. Routine ready/unavailable inspection has a 25-second budget. Most kubectl commands additionally have a 15-second request timeout.

A server restart reconstructs work from SQLite. Shutdown cancels host work and closes port-forwards; it does not deliberately delete running previews. Subsequent inspection recreates their local connections. Failed-test Jobs are not silently retried during ordinary reconciliation. Stop deletes the Job, so a later Start reruns configured tests against the saved images before restoring the application. A recorded failed test result blocks Start and requires a fresh preview.

Capacity counts every environment not confirmed `stopped` or `deleted`, including failed, unavailable, and stopping environments that might still own live pods. Creation checks capacity inside its insert transaction; Start checks it in the update statement. The default allows two environments per project. One Docker build pipeline runs at a time across the host controller; tests and previews can run concurrently.

Unpinned environments automatically request Stop after their deadline. The default is 24 hours. Opening a preview sends `touch`, extending that deadline; Start also resets it. Pinning exempts the environment from expiry. Expiry preserves data, snapshots, and the record—it does not perform deletion.

## Settings contract

Project settings support incremental auto-save. Unknown keys and invalid values are rejected; coupled fields may be filled incrementally while automatic previews are off. Creating an environment or enabling automatic previews requires a complete valid configuration. The frontend serializes settings mutations for a project.

Every new environment stores the full validated configuration. Updating the project does not redirect existing environments to another context or change their image recipe, limits, storage, or retention duration.

| Group | JSON fields and defaults |
| --- | --- |
| Cluster and delivery | `context` must be selected; `transport=kind`, `kindCluster=aycorn`; registry uses `imageRepository`, optional `pullSecretNamespace` / `pullSecretName` |
| Build | `platform` is host-native Linux amd64/arm64; `profile=aycorn`; `dockerfile` contains the embedded default recipe; targets `preview` / `test` |
| Commands and health | `command=[]` preserves the image entrypoint; `testCommand` runs frontend then Go tests; `port=8000`, `healthPath=/api/health/ready` |
| Runtime limits | `cpuMillis=1000`, `memoryMiB=768`, `testCpuMillis=2000`, `testMemoryMiB=4096` |
| Storage | `storageGiB=1`; empty `storageClass` uses the cluster default |
| Lifecycle | `timeoutSeconds=1200`, `maxRunning=2`, `retentionHours=24` |
| Opt-ins | `autoPreview=false`, `allowTestNetwork=false` |

Commands are argument arrays, not implicitly evaluated shell strings. An explicit shell command is possible through `["/bin/sh", "-c", "..."]`. Clearing both `testTarget` and `testCommand` disables the test Job. Changing to the custom profile does not generate a new recipe automatically.

## Images, tests, workloads, and storage

The [default Dockerfile](../server/internal/environments/aycorn.Dockerfile) uses Node 24.13.0 and Go 1.25.7. It installs locked npm dependencies and Go modules, builds the Markdown converter and frontend, then compiles the Go executable with the frontend embedded. The test target contains source and dependencies for offline test execution. The final preview image retains Node for rich-text conversion and runs as UID 1000.

Image tags are `<repository>:<installation-token>-e<id>` and the corresponding `-test` tag. The local repository name is `aycorn-preview`; registry delivery uses the configured repository. Docker builds receive the saved target, recipe, platform, and ownership labels. kind delivery loads images into the named cluster; registry delivery uses the host Docker login to push them. Pod image pull policy is `IfNotPresent`.

Each environment owns namespace `aycorn-<installation-token>-e<id>`, labelled with the installation token and environment ID. The adapter checks those labels before managing or deleting an existing namespace. Its resources are:

| Resource | Behavior |
| --- | --- |
| `ResourceQuota/budget` | Bounds object counts, combined CPU/memory limits, storage requests, and ephemeral storage |
| `NetworkPolicy/isolate` | Denies pod ingress and egress by default |
| `PersistentVolumeClaim/data` | One ReadWriteOnce claim, mounted only into the preview at `/data` |
| `Job/tests` | Optional test container, `restartPolicy=Never`, `backoffLimit=0`, configured deadline |
| `Deployment/app` | One replica with `Recreate` strategy and startup/readiness/liveness HTTP probes |
| `Service/app` | ClusterIP endpoint selected only for the preview pod |
| `Secret/registry` | Optional copy of the configured image-pull secret |

The application gets writable `/data` and a bounded `/tmp`; its root filesystem is read-only. Test containers have a writable filesystem and separate scratch storage, with no preview database mount. Both use UID/GID 1000, dropped capabilities, no privilege escalation, runtime-default seccomp, and no service-account token. The namespace enforces restricted Pod Security at policy version v1.35.

Pod requests are 100 millicores, 128 MiB memory, and 128 MiB ephemeral storage per container; project settings control CPU and memory **limits**. The preview ephemeral limit is 512 MiB and tests receive 2 GiB. Host Docker builds are outside these Kubernetes limits and use the Docker daemon's own resource and network configuration.

The adapter requires Deployment readiness, then starts `kubectl port-forward --address=127.0.0.1 service/app :<port>`. kubectl chooses an available host port. AICorn retains that process, probes the forwarded health path, and replaces the process when its connection dies. The URL is local to the AICorn server machine; there is no ingress, public URL, TLS provisioning, or sharing service.

Stop scales the application to zero, deletes the test Job, and waits for owned pods to disappear. Delete removes the namespace, exact locally owned image tags, and local snapshot files. Registry artifacts, shared Docker build cache, and images loaded into kind nodes have separate retention. Underlying volume destruction follows the StorageClass reclaim policy; a retained volume can require operator cleanup.

## Preview mode, credentials, and networking

The AICorn profile requires the exported source to contain `PreviewProtocolVersion = 1` in [preview.go](../server/cmd/web/preview.go). This compatibility check prevents accidentally launching an older AICorn branch without preview support; it is not verification against malicious source changes.

The workload receives `AYCORN_PREVIEW=1`, source metadata, the main application URL, and `/data/app.db` as its database. Startup applies migrations and seeds a small sample project only if the preview database contains no projects. It never copies the personal database. Stopping and restarting retains review edits on the PVC.

Preview mode does not start the AI worker or environment controller. Its HTTP guard denies AI/repository endpoints, agent requests, Conductor mutations, and nested environment management. The banner uses `/api/preview` to identify the source and link to the main application. `/api/health/ready` checks database access and migration-table availability.

The main server rejects browser requests carrying an Origin from another host/port unless that exact origin is explicitly configured in `AYCORN_ALLOWED_ORIGINS`. Requests without an Origin remain available to local CLI clients. This is browser-origin protection for a localhost application, not user authentication.

Pods receive no host repository mount, Docker socket, kubeconfig, Codex login, or main database. For private images, the host controller reads the configured Secret and copies only its Docker registry configuration to the environment namespace; Kubernetes uses it for pulling, rather than mounting it into the application. The controller itself retains the privileges of its selected kubeconfig and Docker access.

Network isolation depends on an enforcing network-policy implementation. Creating a policy object alone does not establish isolation. The local bootstrap script installs the policy controller; the live acceptance test checks actual cross-preview traffic denial. The connection check covers tools, API readiness, and namespace-creation permission, but does not prove policy enforcement or every runtime permission.

Optional test networking adds cluster DNS and public IPv4 HTTP/HTTPS egress for test-labelled pods, excluding private, loopback, and link-local ranges. Preview egress remains denied. No arbitrary TCP or IPv6 internet allowance is provided. Trusted repositories and trusted Dockerfile recipes are still required; this is a development environment, not hostile multitenancy isolation.

## Task and Conductor integration

Task branch rows expose previews associated with a finished run. The task endpoint derives its project from the database; the service validates that the selected run belongs to that project and task. Branch and repository values come from recorded run/project data rather than accepting an arbitrary filesystem path from the browser.

Automatic previews are polled independently of the Conductor handoff. Candidates require both the Conductor task and agent job to be completed, the project opt-in to be enabled, and the job to have finished after that opt-in's timestamp. Candidates use `requestKey=conductor-<jobId>`, preventing duplicate environments across repeated polls and restarts.

Capacity or configuration can delay creation; a later poll can try again while no environment exists for that key. Once a record exists, even a failed preview is not recreated automatically. The preview controller never retries the coding agent, rewrites its result, changes task stages, or moves a task to Done.

## HTTP and frontend integration

Routes are registered by [routes.go](../server/cmd/web/routes.go) only when the host environment service exists. Preview mode blocks environment requests before dispatch.

| Method and path | Contract |
| --- | --- |
| `GET /api/project/{projectId}/settings/environments` | Effective project settings with defaults |
| `PUT /api/project/{projectId}/settings/environments` | Partial settings update; returns resulting settings |
| `GET /api/environments/project/{projectId}/check` | Host tools, contexts, storage classes, connectivity diagnostics |
| `GET /api/environments/project/{projectId}/branches` | Local Git branch names; `?details=true` returns `{name, current, hasWorktree}` entries for the source selector |
| `GET /api/environments/project/{projectId}` | Project ID and non-deleted environment list |
| `POST /api/environments/project/{projectId}` | Create from branch or run; returns 202 and environment |
| `GET /api/environments/task/{taskId}` | Task-scoped environment list |
| `POST /api/environments/task/{taskId}` | Create from run; task ID is supplied by route |
| `GET /api/environment/{id}` | Single persisted environment |
| `PUT /api/environment/{id}` | Edit `name` and/or `pinned` |
| `POST /api/environment/{id}/{action}` | `start`, `stop`, `touch`, or `rebuild`; Rebuild returns a new environment with 202 |
| `DELETE /api/environment/{id}` | Persist asynchronous deletion intent |
| `GET /api/environment/{id}/source` | Compare captured and current source |
| `GET /api/environment/{id}/logs` | Saved output plus available runtime logs and warning events |
| `POST /api/environments/project/{projectId}/bulk` | `{ids, action}` for Stop/Delete; one set update returning `{success, failed, skipped}` |

Create bodies accept `branch`, `jobId`, `taskId`, `includeChanges`, and `requestKey`; callers should retain the same key when retrying a creation request. The frontend generates a fresh UUID for a new user action. Invalid configuration maps to 400, missing records to 404, and lifecycle/capacity conflicts to 409. Accepted lifecycle actions report persisted intent, not synchronous completion.

The project tab supports source selection, auto-save, and shared multi-selection. Rebuild retains the environment's source mode while capturing a fresh snapshot. Environment cards provide lifecycle buttons, inline name editing, source comparison, pinning, and a log dialog. Rebuild and Delete require UI confirmation. The frontend uses TanStack Query invalidation and polling rather than opening a dedicated event stream.

## Diagnostics and implementation limits

Start with the environment's state, test result, and Logs. The database keeps the latest 262,144 log characters. Runtime log requests add bounded test/application tails and warning events; command error summaries are bounded separately. Logs are diagnostics, not a complete exported test-artifact archive.

If the UI cannot explain a failure, inspect the selected context and exact owned namespace:

```bash
kubectl --context <context> get nodes
kubectl --context <context> -n <namespace> get pods,jobs,deployments,services,pvc
kubectl --context <context> -n <namespace> get events --sort-by=.lastTimestamp
kubectl --context <context> -n <namespace> logs job/tests
kubectl --context <context> -n <namespace> logs deployment/app
```

Check failures in order: host tool access and context; image build/load/pull; PVC provisioning and scheduling; test exit; application health; localhost forwarding. Restore a temporarily unavailable cluster before retrying cleanup. Corrected settings require a new environment because existing records retain their original configuration.

The implementation assumes one main AICorn controller per database. Multiple replicas sharing ownership are unsupported. It does not provision registries, ingress, autoscaling, remote reviewer access, production data copies, or containerized Codex execution. Registry delivery, other storage providers, architecture combinations, and custom applications require validation in their actual deployment environment.

Relevant checks are [controller/store tests](../server/internal/environments/service_test.go), [CLI adapter tests](../server/internal/environments/kubernetes_test.go), [source-export tests](../server/internal/worktree/snapshot_test.go), [HTTP/preview tests](../server/cmd/web/environmentHandler_test.go), and the opt-in [real Kubernetes lifecycle test](../server/internal/environments/kubernetes_live_test.go). Setup commands, access requirements, and troubleshooting details are maintained in the [operations guide](kubernetes-environments.md).
