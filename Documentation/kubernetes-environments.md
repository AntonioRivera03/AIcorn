# Kubernetes branch environments

AICorn can build a branch, run its tests in Kubernetes, and open a separate copy of the application for review. Each environment has an immutable source snapshot, test results, logs, a localhost URL, and its own persistent database. Creating or rebuilding a preview never commits, merges, or changes the source branch.

See the [technical architecture](kubernetes-architecture.md) for implementation details and the [installed-system verification report](kubernetes-verification.md) for the configured local cluster, test results, and launch commands.

This implements the Kubernetes direction chosen after the [original environment proposal](branch-environments-spec.md). The main AICorn server remains on the host. It captures code and builds images through the host Docker daemon; Kubernetes runs the test Jobs and application Deployments. Codex still runs on the host in its existing worktree sandbox. Moving Codex, its authentication, and scoped MCP access into execution containers is separate work.

## Set up a local cluster

Use a **dedicated development cluster**. The setup script creates a new local kind cluster, installs a network-policy controller, and keeps credentials separate from your existing Kubernetes configuration. It never installs host tools, changes another cluster, or replaces your current context.

1. Install Docker with a running daemon, `kubectl`, and `kind`. Put them on the **AICorn server process's PATH**, along with Git. The script also needs Bash, curl, and `sha256sum` or `shasum`. See the official [Docker installation](https://docs.docker.com/engine/install/), [kubectl installation](https://kubernetes.io/docs/tasks/tools/), and [kind quick start](https://kind.sigs.k8s.io/docs/user/quick-start/). Verification used Docker 29.7.2, kubectl 1.37.0, kind 0.33.0, and Kubernetes 1.37.0 on Linux AMD64.
2. From the repository root, run:

   ```bash
   ./scripts/k8s-local.sh
   ```

   The default cluster is `aycorn`, its context is `kind-aycorn`, and its credentials are written to `${XDG_CONFIG_HOME:-$HOME/.config}/aycorn/kubeconfig-aycorn`. An alternate name and destination are supported:

   ```bash
   ./scripts/k8s-local.sh aycorn-dev /absolute/path/to/aycorn-dev.kubeconfig
   ```

   Existing cluster names and destination files are refused. The script downloads the official `kube-network-policies` v1.1.1 install manifest, verifies SHA-256 `5491e0364d32e74807bc80abcf4264a2e9b9a98c6d4a96cf55dc7770fae1aa19`, applies it only to the new context, and waits for its DaemonSet and nodes to become ready. That upstream manifest references the **v1.1.0 controller image**. It is a privileged, cluster infrastructure component; preview pods are unprivileged. [Pinned upstream manifest](https://raw.githubusercontent.com/kubernetes-sigs/kube-network-policies/v1.1.1/install.yaml)
3. For the default local cluster, run `make dev` (or `make dev-test` for the separate test database). These targets automatically load the local AICorn kubeconfig when it exists and `KUBECONFIG` is unset. Explicit values, including an empty value, are preserved. For an alternate cluster configuration or a directly launched binary, use the `export KUBECONFIG=...` command printed by the script. To inspect the default cluster manually:

   ```bash
   export KUBECONFIG="${XDG_CONFIG_HOME:-$HOME/.config}/aycorn/kubeconfig-aycorn"
   kubectl --context kind-aycorn get nodes
   kubectl --context kind-aycorn get storageclasses
   ```

   If AICorn runs through a service manager or desktop launcher, configure PATH and KUBECONFIG for that process instead. Installing a command in one interactive shell does not change an already-running server's environment.
4. Link the project's local Git repository in **Project Settings → General**. Open **Project Settings → Environments**, select `kind-aycorn`, choose **Local kind cluster**, and enter `aycorn`. Click **Check connection**. Settings save automatically.
5. Select a local branch and click **New preview**. Open **Logs** while the first build installs dependencies. The Source selector defaults to Working tree when the branch has a checkout, including local uncommitted changes. Latest commit uses only committed code. The selected source must contain AICorn's preview-mode support.

The local cluster needs a working default StorageClass. kind normally provisions local storage; a different cluster may require a provisioner or an explicit storage-class setting. Allow sufficient disk space for Docker build caches, source snapshots, and persistent volumes. The default test container alone may use 4 GiB; roughly 8 GiB available to Docker/Kubernetes is a practical starting point, with more needed for simultaneous builds and previews.

The verification cluster used during development was disposable, with a private kubeconfig under `/tmp`. Its environments, owned image tags, cluster, and test servers were removed after validation. It is **not** permanent application setup and was not connected to the personal AICorn database.

## How to use previews

**Project branch previews** offer a Source selector. **Working tree** captures the selected branch's local files, including uncommitted edits and eligible untracked files; it is the default when that branch has a checkout. **Latest commit** captures only the recorded commit and excludes local edits. The current branch is selected initially. Branches without a checkout can use Latest commit. A running agent on a branch blocks working-tree capture until it finishes.

**Task previews** start from **Code branches → Preview branch** on a finished agent run. They include the agent worktree's tracked edits and eligible untracked files, so a task can be reviewed before anyone commits it. A changed-file digest distinguishes two snapshots that share the same Git commit. A run must be finished and its worktree must still exist.

**Automatic previews** are optional per project: enable **Prepare previews when Conductor finishes code tasks**. This applies to future completions. Preview creation is deduplicated across polling and server restarts. A failed image build or test does not restart the coding agent, move the ticket, or approve its work.

Environment cards show build state, test outcome and exit code, branch, commit, source digest, errors, and logs. Names edit in place. **Check for newer code** compares the captured source with the current source; previews do not silently change underneath a review.

| Action | Result |
| --- | --- |
| Open preview | Opens its localhost URL and extends its auto-stop deadline. |
| Stop | Stops application pods and any test Job, closes the local connection, and retains the database and snapshot. |
| Start | Starts that same saved environment and retains its data. Configured tests run again after a stop. |
| Rebuild | After confirmation, creates a **new** environment from the latest source and current project recipe, with fresh data. The old environment is retained. |
| Pin | Exempts that environment from automatic stopping; it still counts toward project capacity. |
| Delete | Requires confirmation and removes the owned namespace, its preview resources, and the local source snapshot. The branch and agent worktree are kept. |

Multi-select supports one bulk Stop or confirmed Delete request. The defaults allow two active environments per project, including failed or stopping environments until Stop/Delete completes, one host Docker build at a time across the application, and automatic stopping after 24 hours unless pinned. Automatic stopping does **not** automatically delete old data.

Failed environments keep their diagnostics. Failed tests require Rebuild; Stop then Start cannot bypass a failed test result. Other failures can be retried with Stop then Start. A temporary readiness or cluster error shows Unavailable and recovers automatically when health checks succeed. Corrected project settings apply to newly created environments, because every existing environment keeps the settings captured when it was created.

## What gets built and isolated

The pipeline is:

```text
Local branch or finished task worktree
  → filtered source snapshot + SHA-256
  → host Docker build (preview and test targets)
  → kind image load or registry push
  → environment namespace, quota, network policy, and data volume
  → Kubernetes test Job
  → application Deployment and ClusterIP Service, if tests pass
  → readiness check and localhost port-forward
```

Snapshots exclude Git metadata, nested worktrees, dependencies, build output, databases and backups, common credential directories/files, and environment files. Symlinks and submodules are rejected. Capture rejects a worktree that changes while it is being read. Limits are 128 MiB total, 16 MiB per file, and 10,000 files. Git LFS export and submodule materialization are not implemented. Exclusions are practical protection for a trusted local repository, not a general secret scanner; do not commit secrets under arbitrary filenames.

The settings-owned Dockerfile is used instead of letting each branch choose the build recipe. The default recipe lives in [`server/internal/environments/aycorn.Dockerfile`](../server/internal/environments/aycorn.Dockerfile). It installs npm and Go dependencies during the image build, builds the frontend and Markdown conversion bundle, embeds the frontend in the Go executable, and retains Node in the runtime image for rich-text conversion. The test image includes the source and dependencies needed for offline testing.

Each preview receives a fresh SQLite database at `/data/app.db`, mounted from its own PersistentVolumeClaim. The AICorn profile seeds a small sample project. Restarting preserves preview edits. The personal AICorn database, repository directory, Codex login, Kubernetes credentials, and Docker socket are not mounted into pods. Copying or sanitizing real project data into previews is not implemented.

AICorn's enforced preview mode disables AI execution, Conductor mutations, repository operations, and nested environment management. A visible banner identifies the preview and links back to the main app. Readiness checks database access and completed migrations. The main app also rejects cross-origin API requests by default so a preview page on another port cannot control it through the browser. Development frontends may be explicitly allowed with `AYCORN_ALLOWED_ORIGINS`; keep that list limited to the intended development origin.

Preview pods run as UID/GID 1000 with a read-only root filesystem, writable `/data` and bounded `/tmp`, dropped Linux capabilities, no privilege escalation, and no service-account token. Test pods use the same identity restrictions but a writable container filesystem so test tools can create files. Each namespace enforces Kubernetes restricted Pod Security, has a quota, and uses private data storage.

**Network policy enforcement is required.** The default policy denies ingress and egress; localhost access goes through `kubectl port-forward`. An optional setting allows test pods to use cluster DNS and public IPv4 HTTP/HTTPS, excluding private, loopback, and link-local ranges. Preview application egress stays denied. This setting does not provide arbitrary TCP access or IPv6 internet access. A NetworkPolicy object alone does not enforce traffic: a supporting network implementation must be running. The connection check confirms tools, API access, and namespace-creation permission; it does not prove network isolation. [Kubernetes NetworkPolicy behavior](https://kubernetes.io/docs/concepts/services-networking/network-policies/)

Namespaces and resource limits are useful isolation for trusted development work, not a hostile multitenancy boundary. Use a dedicated cluster and trusted source/recipes. Docker builds execute on the host's Docker daemon with normal build-network access. Build CPU and memory are **not individually bounded by the preview's Kubernetes settings**; manage Docker's host or VM limits separately.

## Custom applications and resource settings

Expand **Build recipe and resource limits** in project Environments settings. The default profile is **AICorn · enforced preview mode**. For another repository, select **Custom application** and provide a Dockerfile recipe with named preview and test targets. Selecting Custom does not rewrite the recipe for you.

The app must run as UID 1000, listen on `0.0.0.0` at the configured unprivileged port, serve the configured HTTP readiness path, and write persistent data only under `/data` or scratch files under `/tmp`. Use `PORT` if the application supports it. The recipe can choose its image entrypoint; a configured application command replaces it. Custom applications must implement their own preview-specific behavior: AICorn cannot disable another application's outbound integrations by application logic.

Commands are JSON arrays, not shell strings. Example test command:

```json
["node", "--test", "/app/math.test.cjs"]
```

Use `["/bin/sh", "-c", "..."]` only when the command needs shell syntax. To skip Kubernetes tests, clear both the test target and the test command (`[]`). Build dependencies into the image whenever practical so tests can run with network access disabled.

| Setting | Default |
| --- | --- |
| Preview CPU / memory | 1 CPU / 768 MiB |
| Test CPU / memory | 2 CPU / 4 GiB |
| Persistent data | 1 GiB |
| Build and startup timeout | 1,200 seconds, including time waiting for the build slot |
| Preview port / readiness path | 8000 / `/api/health/ready` |
| Platform | Host-native `linux/amd64` or `linux/arm64` |
| Test command | Frontend tests followed by `go test ./...` |
| Test internet | Disabled |

Cold builds on a slower machine may need a longer timeout (up to 3,600 seconds). Cluster nodes must match the selected image architecture, or support the necessary emulation. The feature builds a single-platform image. Cross-platform builds, ARM64 runtime behavior, and mixed-architecture scheduling still need validation on the target hardware.

## Use an existing cluster or registry

This is an alternative to the local setup script. A cluster operator must provide a context, network-policy enforcement, dynamic persistent storage, and enough capacity for the configured workloads. AICorn passes the selected context explicitly to every Kubernetes operation. The server must have access to that context through its own kubeconfig.

Choose **Container registry** and set an image repository without a tag, such as `ghcr.io/your-account/aycorn-preview`. Sign the host Docker daemon into that registry through your usual credential manager. AICorn builds uniquely tagged images and pushes them; do not enter registry passwords into project settings.

For a private registry, create a `kubernetes.io/dockerconfigjson` pull secret in an operator-managed namespace and set its namespace/name in the project. The controller reads that secret and copies only its Docker registry configuration into each preview namespace as `registry`. Do not use credentials that grant broader access than image pulling. [Kubernetes private-image pull secrets](https://kubernetes.io/docs/tasks/configure-pod-container/pull-image-private-registry/)

[`ops/kubernetes/controller-role.yaml`](../ops/kubernetes/controller-role.yaml) is an optional, unbound ClusterRole describing current runtime access. Have your cluster operator review and bind it to the host's configured identity. It includes namespace creation/deletion and cross-namespace secret reads, because namespaces are created dynamically and pull secrets may originate elsewhere. Kubernetes RBAC does not restrict those actions to AICorn's ownership labels; application ownership checks provide the label guard. This role is for a dedicated development cluster, not an isolation policy for a shared production cluster. The acceptance test additionally uses `pods/exec` to probe isolation; that diagnostic permission is intentionally absent from the runtime role.

No ingress, shared public URL, TLS provisioning, registry provisioning, node autoscaling, or multi-host AICorn controller coordination is included. Preview URLs are bound to `127.0.0.1` **on the machine running the AICorn server**. Review them there, or use a separately configured secure tunnel. Kubernetes being remote does not make these URLs public.

## Recovery, cleanup, and troubleshooting

The controller records desired state before acting, resumes outstanding operations after a server restart, and rebuilds dropped localhost port-forwards. It labels resources with an installation token and environment ID, and refuses to modify namespaces carrying another installation's labels. Source snapshots live beside the AICorn database in `environments-<installation-token>`. Bounded build/test logs are retained in the database.

| Symptom | Check |
| --- | --- |
| Missing tools / no contexts | PATH and KUBECONFIG of the running server, then restart it. |
| Context/cluster mismatch | For kind delivery, the context must be exactly `kind-<cluster-name>`. |
| Pending PVC | The StorageClass, provisioner, available storage, and warning events in Logs. |
| ImagePullBackOff | Image platform, registry reachability, pull-secret type/access, or kind image loading. |
| Test Job fails | Saved exit code and test logs; dependencies must be built into the image or permitted by test networking. |
| Readiness never succeeds | Application port/path, UID 1000 compatibility, writable directories, and startup logs. |
| Auto preview does not appear | Settings validity, completed implementation run, capacity, and whether automatic previews were enabled before completion. |
| AICorn preview support is missing | If support exists only in local edits, select Working tree for a new preview. Latest commit requires the support to be committed. |
| Connection was ready but traffic is not isolated | Check the network controller and run the live isolation test; API connectivity is insufficient. |

Delete environments through AICorn while the cluster is reachable. Namespace deletion removes their PVCs; the storage provider's reclaim policy determines whether the underlying volume is also destroyed. A `Retain` policy requires operator cleanup. Local exact-match owned image tags are removed with both ownership labels checked; cleanup failures remain visible and are retried. Registry images, shared Docker build caches, and images already loaded into kind nodes follow their respective runtimes' retention policies. No global image prune is performed. [Kubernetes volume reclaim policies](https://kubernetes.io/docs/concepts/storage/persistent-volumes/#reclaiming)

The setup script retains a partially created cluster on failure and prints its private kubeconfig path for diagnosis. It will refuse to rerun against that existing name. If the cluster is disposable and all its data can be discarded, delete it explicitly with `kind delete cluster --name <exact-name>`; never use an unqualified deletion command. Deleting a kind cluster destroys all its local preview volumes. Keep stopped environments only as long as their data is useful.

## Verification and remaining test areas

The opt-in live acceptance test exercised a real dedicated kind cluster: image build and loading, a Kubernetes test Job, two concurrently accessible source versions, capture of uncommitted changes, separate persistent data, readiness, stop/start data retention, repaired port-forwards, failed-test gating, and owned-resource deletion. Cross-preview network denial was tested by attempting traffic to a second known-working preview service; it was not inferred from the presence of a policy object.

The default AICorn recipe also built and ran successfully in that cluster: all 75 frontend tests and the full Go suite passed in the unprivileged test pod with network access disabled. Browser checks verified the seeded application, preview banner, isolated data, and denied AI/nested-environment and cross-origin main-app requests. Desktop dark and mobile light layouts, inline autosave, keyboard selection, and confirmed bulk deletion were checked. The main controller was restarted while the full AICorn preview was running; it restored a localhost URL and preserved edited preview data. Stop/Start reran both test suites and retained those edits. A deliberate readiness loss moved the preview to Unavailable, and restoring its pod recovered the preview without rerunning tests. Vite development requests were also verified through the same-origin proxy. Host Go tests, race tests for the environment/worktree/web packages, and the production frontend build passed. Full-repository lint has the same 48 existing errors as the Phase 4 baseline; the new environment code has no lint errors.

To repeat it against a disposable cluster, launch from the `server` directory with the setup kubeconfig selected:

```bash
AYCORN_K8S_LIVE=1 \
AYCORN_K8S_CLUSTER=aycorn \
AYCORN_K8S_CONTEXT=kind-aycorn \
go test ./internal/environments -run '^TestKubernetesLive$' -count=1 -timeout=25m -v
```

It creates labelled temporary namespaces and images and cleans up those resources. It does not use the personal AICorn database. The test needs cluster administrative access or its extra `pods/exec` diagnostic permission.

Before relying on a different setup, verify:

- The default AICorn recipe and your real application's full test suite on your machine, including a cold build and adequate timeout/resources.
- ARM64, cross-platform builders, Docker Desktop/WSL/rootless Docker, and mixed-architecture clusters if used.
- Registry pushes, private-image pulls, credential rotation, and storage reclaim behavior on the chosen provider.
- Actual network-policy denial on that cluster, including any custom pod/service network ranges; optional test internet access if enabled.
- Longer-running Conductor auto-preview use, capacity exhaustion, cancellation during builds, server/cluster restarts, and extended retention under realistic workloads.
- Custom application write paths, migrations, health probes, browser behavior, and application-specific integrations.

Moving Codex into Kubernetes, copying selected real data, public/team preview access, and multiple main AICorn servers sharing one environment database remain outside this implementation.
