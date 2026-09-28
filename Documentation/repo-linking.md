# Repository links

A project's agents and branch environments work from the project's repository. **Project settings → General → Repository** picks how Aycorn reaches it:

- **Personal**: a folder on the server that holds a git checkout you own. You work in it and pull the latest yourself; Aycorn uses what is there. This is how every project worked before, and projects that had a repo folder became Personal links.
- **Official**: a GitHub repository, entered as `https://github.com/owner/repo`. Aycorn clones it, keeps the clone up to date, and builds from it. You keep working on your own machine and pull from GitHub as usual; nothing on the server needs your attention.

Both fields are kept, so switching modes back and forth doesn't lose the other one. A project with neither has no repository: Implement, Review and Edit-files runs, Conductor's "Work in the project repository", and branch environments ask you to link one first.

## Official links

**Accepted URLs.** Only `https://github.com/<owner>/<repo>`, optionally ending in `.git` or `/`. Owner and name are lowercased, as GitHub treats them case-insensitively. Every other form is refused before git runs: `http://`, `ssh://`, `git@github.com:`, `file://`, `ext::`, local paths, ports, credentials in the URL, queries and fragments.

**Credentials.** Aycorn clones and fetches with the server machine's own git login: its credential helper, or `gh auth login` (which installs one). It stores no tokens. This is the same arrangement as the AI harnesses, which use the server's Codex login for every workspace. If that login can't read the repository, the status shows "the server's git login can't read owner/repo"; sign in on the server with an account that can, then press **Fetch now**. Git never prompts (`GIT_TERMINAL_PROMPT=0`), so a missing login fails straight away instead of hanging.

**Where clones live.** `<workspace dir>/repos/<project id>/<owner>/<repo>`, beside the workspace's own database, so a workspace only ever reaches its own clones. Agent runs add their worktrees inside the clone, under `.worktrees/`, as they do in a Personal checkout.

**When Aycorn clones and fetches.**

- It clones when you link the repository, or the first time something needs it.
- It runs `git fetch --prune` before each environment build from a branch, before each agent run, when the Environments tab lists branches, and when you press **Fetch now**. A fetch also follows a change of the repository's default branch.
- Only one clone or fetch runs per project at a time. Callers that arrive while one is running share it. A page request that can't wait gets "still cloning or fetching", and the work finishes in the background.
- Each clone or fetch has a time limit (20 minutes to clone, 5 to fetch). A clone goes into a temporary folder and is moved into place only when it completes.

**Status.** The Repository section shows whether the repository is cloned, when it was last fetched, and the last error (cleared by the next successful fetch).

**Branches and runs.** In Official mode the Environments branch picker lists the repository's branches on GitHub (`origin/*`), with its default branch selected. A preview builds from the branch's latest commit, fetched when you create the preview. There is no working-tree option, because nobody edits the clone. New agent runs start from the repository's default branch as of the fetch, not from whatever the clone has checked out.

**Changing or removing the link.** Changing the URL, or switching to Personal, deletes Aycorn's clone, including agent branches in it. The app asks first, and the server refuses while an agent is working in the clone. Deleting the project deletes its clone too. Changing the link while a run is queued makes that run stop with "start it again" instead of working in the wrong repository. When a workspace starts up on the server, Aycorn removes any clone left behind by an interrupted deletion.

## Aycorn never pushes

Nothing is ever pushed to GitHub automatically. Agent branches (`aycorn/run-…`) stay in Aycorn's clone, where a task's branch and diff views show them. Merging an agent branch from those views updates a branch in Aycorn's clone only. New runs and previews still start from GitHub, so merged work isn't in them until it reaches GitHub another way. Pushing agent work is a follow-up (below).

## Ignored files

Both modes use the same built-in list when snapshotting source for an environment. It leaves out `.git`, `node_modules`, `.env*`, keys, local databases and similar files (`excludedSource` in `server/internal/worktree/snapshot.go`). A repository's own `.dockerignore` or other ignore files are not read yet.

## Implementation

- `server/internal/repolink` is the single resolver. `Locate` returns the checkout without touching the network: the Personal folder's repository root, or where the Official clone lives. Requests record it as the run's repository. `Service.SourceRepo` clones or fetches first and returns the base commit for new runs. The worker, environments, AI requests and Conductor all go through it; nothing reads `project.repoPath` directly.
- Git always runs with an argument list, never a shell, with `--` before URLs and paths. It gets a timeout and `GIT_ALLOW_PROTOCOL=https`, so even a changed git config can't redirect a fetch to another transport. Fetches name the validated URL themselves rather than trusting the clone's configured remote. Tests clone local bare repositories through an unexported hook; production code has no way to accept a local URL.
- Schema: `project.repoMode` (`''`, `personal`, `official`; validated in Go rather than by a `CHECK` constraint), `project.repoUrl`, the existing `project.repoPath`, `repository_sync` (last fetch time and error per project and URL, kept apart from `project` so a fetch doesn't change its modified time) and `task_environment.remote` (a preview of an `origin/*` branch).
- HTTP: `GET` and `PUT /api/project/{id}/settings/repository` (`{"mode", "path", "url"}`; omitted fields stay as they are), and `POST /api/project/{id}/settings/repository/fetch`. The general project `PUT` ignores the link fields. Application previews can read the link but not change or fetch it.

## Follow-ups

- **Per-workspace GitHub credentials** (a GitHub App installation or a connected account), so organizations don't depend on the server machine's login, and a revoked credential is reported per workspace.
- **Pushing agent branches** to GitHub, and opening pull requests from them.
- **Ignore files**: respect or merge the repository's own `.dockerignore` and similar files when building environments.
- **Installing the project's skills** into the environment or the agent's workspace.
