# Local harness integration

Aycorn connects to the installed Codex CLI through `codex app-server --listen stdio://`. Sign in with `codex login` on the machine running Aycorn. No separate OpenAI API key or SDK is required. The process retains the user's `CODEX_HOME`, authentication and local session store.

The implementation uses the same separation as [T3 Code](https://github.com/pingdotgg/t3code/tree/935c55b3778fdeae0e25b250ce2a9fa7e79c0327/apps/server/src/provider): a provider registry, harness transport, provider session lifecycle and application-owned context. It is implemented in Go for Aycorn; no T3 source was copied. Codex protocol fields are checked against the installed CLI's generated JSON schemas and [official app-server documentation](https://developers.openai.com/codex/app-server).

## Codex sessions

Each ordinary Aycorn run creates a persistent, named Codex conversation. [Ticket chats](ticket-chats.md) resume their conversation for follow-up messages. The run records its native conversation and turn IDs before execution proceeds. Run cards offer a `codex resume <id>` command; conversations with messages appear in Codex's normal local conversation listing. Retrying an ordinary run creates a new conversation, preserving the previous attempt. After an interruption, Aycorn never silently replays a possibly executed turn.

The adapter initializes the JSON-RPC connection, creates the thread, injects developer context separately from ticket data, starts a turn and consumes notifications. Only completion of the root turn completes the run. Child-agent notifications cannot overwrite the parent's final handoff. Partial responses survive failure, cancellation and timeout. Cancellation terminates the app-server process group and its children. Interactive requests stop the unattended run with a message to continue in Codex; they are never automatically approved.

Ask, Plan and Review use a read-only sandbox. Implement uses a separate writable Git worktree. The configured time limit and command network restriction continue to apply. Codex's user configuration and login remain available, with Aycorn overriding its own MCP connection and run policy for the session.

## Context and fleet

`BuildContext` keeps workflow instructions in the developer prompt and ticket title/body/request in the user prompt. The immutable Conductor snapshot supplies project/task IDs, configured planning/working/review stage IDs, selected agent instructions and stage-specific prompts. Names such as “Doing” or “Review” are never used to guess stage IDs.

The server embeds `server/internal/harness/fleet/` and installs a content-addressed copy beside the database in `harness-fleet/`. These persistent files support native session resume and work for projects outside the Aycorn repository. Existing project files and the user's global Codex configuration are not rewritten. The repository's `.codex/config.toml` also registers the fleet for working on Aycorn itself.

- **Conductor** owns ticket handling, delegates work, waits for results and produces the final decision.
- **Planner** checks readiness and proposes a bounded plan.
- **Researcher** resolves technical questions from repository evidence and primary sources.
- **Coder** implements the accepted scope using the selected task agent's frozen model and instructions.
- **Reviewer** independently checks the actual changes against acceptance criteria and test evidence.

The selected Conductor stays the parent through planning and implementation. Up to four subagents can run concurrently. The workflow skill is bundled and injected for every run, and is discoverable in `.agents/skills/aycorn-workflow` for interactive repository work.

The durable Conductor controller applies the root agent's structured decisions: planning at intake, working when execution starts, review only after a successful completed handoff. Blocked, failed, canceled and interrupted runs do not enter review; done still requires the human. Keeping stage writes in the controller makes them transactional and restart-safe. Subagents receive read-only Aycorn tools and cannot move tickets through the managed MCP surface. Manual AI runs do not alter ticket ownership or stages.

## OpenCode

OpenCode is listed in settings as disabled. The backend registry rejects it too, including persisted or forged requests. Its local loopback HTTP client implements session creation, messages, history, abort and SSE framing using [OpenCode's server API](https://opencode.ai/docs/server/). There is no fallback to another provider. Process startup, interactive permissions and production dispatch remain disabled until the follow-up OpenCode work is requested.

## Verification

Run `go test -race ./internal/harness ./internal/worker ./cmd/mcp ./cmd/web ./internal/models/services` from `server/`, and `npm test` plus `npm run build` from `app/`.

The opt-in `TestInstalledCodexProtocol` creates and reads an empty persistent session in a temporary `CODEX_HOME` without a model call. Set `AYCORN_CODEX_INTEGRATION` to the CLI path and `AYCORN_MCP_INTEGRATION` to the built Aycorn MCP binary.

`TestInstalledCodexTurn` additionally requires `AYCORN_CODEX_LIVE=1`. It uses the current local login for two small real turns, a disposable ticket database and work directory, verifies subagent delegation, conversation resume and native listing, then archives only its own test conversation.
