# Installed Kubernetes system: verification report

Verified on **7 September 2026**, Linux AMD64, using the permanently installed tools. The local cluster is configured and remains available for AICorn. No additional software installation is required for the tested local preview workflow.

For implementation details, see the [technical architecture](kubernetes-architecture.md). For general installation, configuration, and troubleshooting, see the [operations guide](kubernetes-environments.md).

## Installed setup

| Component | Verified value |
| --- | --- |
| Docker daemon | 29.7.2, running |
| kubectl | 1.37.0, installed through mise |
| kind | 0.33.0, installed through mise |
| Kubernetes | 1.37.0, node Ready |
| Cluster / context | `aycorn` / `kind-aycorn` |
| Kubeconfig | `/home/hal/.config/aycorn/kubeconfig-aycorn`, permissions `0600` |
| Persistent storage | Default `standard` StorageClass, local-path provisioner, `Delete` reclaim policy |
| Network policy controller | Ready; installed from the checksum-verified pinned upstream manifest |
| Host prerequisites | Git, Node, Go, and Docker available to the server process |

The cluster was created with `./scripts/k8s-local.sh`. Its kubeconfig is separate from the default Kubernetes configuration. The previous default current-context remains unchanged; commands must select this kubeconfig explicitly or inherit it through `KUBECONFIG`.

The local production executable and MCP companion were rebuilt successfully with `make build`. Verification used the current working tree on `feat/kubernetes-branch-environments`, based on commit `609be18`, including its uncommitted implementation changes.

## Start your normal AICorn instance

From this repository:

```bash
cd /home/hal/T3/AIcorn
make dev
```

This automatically loads the default local AICorn kubeconfig when `KUBECONFIG` is unset, builds the frontend and companion, then starts AICorn with its normal personal database. An explicit `AYCORN_DB` overrides that database. If an older AICorn instance is already running, stop that instance and launch the current version with this environment. Installing tools or exporting variables does not update an existing server process.

To use the already-built executable without rebuilding:

```bash
cd /home/hal/T3/AIcorn
export KUBECONFIG="$HOME/.config/aycorn/kubeconfig-aycorn"
export AYCORN_MCP_EXECUTABLE="$PWD/server/bin/aycorn-mcp"
./aycorn
```

For each project, link its local repository in **Project Settings → General**, then set **Project Settings → Environments**:

| Setting | Value |
| --- | --- |
| Kubernetes context | `kind-aycorn` |
| Image delivery | Local kind cluster |
| kind cluster name | `aycorn` |

Click **Check connection**, then create a preview. These project-specific settings were configured in the verification database; the personal database was not changed.

Project previews now offer Working tree and Latest commit sources. Working tree includes local uncommitted changes and is selected by default for checked-out branches. Latest commit excludes those edits and requires AICorn's preview support to be committed. Task previews continue capturing inactive agent worktrees. The initial lifecycle test used a synthetic completed-run record; the follow-up below verifies project previews directly through the Environments tab.

## Results

| Check | Result |
| --- | --- |
| Bootstrap script | Passed: new cluster, private kubeconfig, pinned manifest checksum, node/controller readiness, default storage |
| Application connection diagnostics | Passed: Docker, kubectl, kind, context, storage, cluster API, and namespace permission |
| Real Kubernetes lifecycle acceptance | Passed in 171.04 seconds using installed tools and `kind-aycorn` |
| Simultaneous versions | Two source versions served concurrently; uncommitted changes appeared in the correct snapshot |
| Data and network isolation | Separate persistent data; actual cross-preview traffic denied |
| Test gating | Passing tests allowed startup; deliberate test failure prevented application deployment and retained diagnostics |
| Lifecycle | Stop/Start preserved data; dropped port-forwards recovered; deleting one preview preserved the other |
| Default AICorn recipe | Both images built, loaded into kind, and ran as unprivileged workloads |
| Full test Job | All 75 frontend tests and the complete Go suite passed with test-network access disabled; exit code 0 |
| Browser | Full AICorn application and preview banner rendered; no browser console errors in the fresh preview tab |
| Preview source identity | `/api/preview` matched the environment's saved digest and revision |
| Editable preview data | Renaming the sample project persisted without changing the main verification project's data |
| Preview restrictions | AI and nested-environment endpoints returned 403; the main API rejected the preview's browser Origin |
| Main controller restart | The rebuilt `./aycorn` restored access to the existing preview and retained its edited data |
| Cleanup | Passed: all test namespaces and owned image tags removed; temporary server stopped; permanent `aycorn` cluster retained and Ready |

The end-to-end application run used a fresh database under `/tmp/aycorn-installed-ui.bvqMKX`, with a synthetic project/task/run fixture. It did not open, copy, migrate, or modify the personal AICorn database. No Codex/model session was initiated during this verification.

To repeat the infrastructure acceptance test:

```bash
cd /home/hal/T3/AIcorn/server
KUBECONFIG="$HOME/.config/aycorn/kubeconfig-aycorn" \
AYCORN_K8S_LIVE=1 \
AYCORN_K8S_CLUSTER=aycorn \
AYCORN_K8S_CONTEXT=kind-aycorn \
go test ./internal/environments -run '^TestKubernetesLive$' -count=1 -timeout=25m -v
```

This test creates and cleans up its own installation-labelled namespaces and images. It requires the extra pod-exec diagnostic permission used to probe network isolation.

## Follow-up: automatic configuration and project working-tree previews

Verified on **7 September 2026** using a fresh temporary database at `/tmp/aycorn-working-tree-preview.VtWz0a/app.db`, the same branch, and the permanent `aycorn` cluster. The personal database and existing development server were left untouched.

| Check | Result |
| --- | --- |
| `make dev` configuration | Passed: launched with `KUBECONFIG` unset and only a temporary database/port override; application diagnostics connected to `kind-aycorn` using the automatic fallback |
| Development target compatibility | Passed: 20 isolated recipe checks across `dev` and `dev-test`, including XDG paths, missing configuration, spaces, and explicit `KUBECONFIG` overrides |
| Project source selection | Passed in the browser: current branch and Working tree selected by default; New preview created an environment with `includeChanges: true` and `jobId: 0` |
| Uncommitted implementation | Passed: the snapshot included preview support absent from the branch's latest commit; both images built and loaded into kind |
| Kubernetes test gate | Passed: all 75 frontend tests and the full Go suite ran in the test Job, exit code 0; the application reached Ready |
| Running preview | Passed: application and preview banner rendered with no browser console errors; `/api/preview` revision and digest matched the saved environment |
| Latest commit selection | Passed: explicitly selecting Latest commit excluded local edits and reported an actionable message directing the user to Working tree when preview support was absent from that commit |
| Source regressions | Passed: active-agent protection, checked-out branch discovery, staged/unstaged/untracked capture, immutable snapshots, commit pinning, and rebuild source preservation |
| Local checks | Passed: full Go suite, environment/worktree race tests, TypeScript, scoped frontend lint, production frontend build, and diff whitespace checks |
| Cleanup | Passed: both temporary environments deleted, owned namespaces and image tags removed, temporary server stopped; permanent cluster retained and Ready |

Restart an already-running development server with `make dev` to load these changes. Create a **new** preview with **Working tree** selected. Rebuild deliberately preserves an existing environment's source mode, so rebuilding an old Latest commit preview will continue using committed files.

Follow-up diagnostics are under `/tmp/aycorn-working-tree-preview.VtWz0a`, `/tmp/aycorn-source-choice-go.log`, `/tmp/aycorn-source-choice-race.log`, and `/tmp/aycorn-source-choice-build.log`.

## Validation boundaries

This validates local Kubernetes tests and branch previews on this Linux AMD64 machine. It does not certify private registry delivery, ARM64/Docker Desktop, different storage/network providers, optional test internet access, arbitrary custom applications, or long-running automatic Conductor usage. Those still need testing when adopted. Codex execution continues on the host and retains its separate authentication/model requirements.

Development logs are retained under `/tmp/aycorn-installed-k8s-setup.log`, `/tmp/aycorn-installed-k8s-live.log`, `/tmp/aycorn-installed-full-app.log`, and `/tmp/aycorn-installed-build.log`. These are temporary diagnostics; this document records the durable verification summary.
