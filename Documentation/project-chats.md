# Project Chats and Chatter

The **Chats** project view holds one durable conversation per project. It uses the installed Codex login and AI settings, streams the current response, retains previous turns, and resumes the same native session after navigation or restart. Messages, partial output, status, session/turn IDs, and native usage are stored in SQLite. A client request key prevents duplicate sends. Only one turn can be active in a conversation.

The schema separates `project_chat` from `project_chat_turn`. Conversations already have `archivedAt` and a unique active-per-project index; archiving/new-conversation controls can be added later without replacing the history model. This release exposes one persistent chat only.

## Task references

Type `#` to open a project task picker above the composer. Digits filter task numbers; `#"` filters titles case-insensitively, including spaces. Arrow keys select; Enter or Tab inserts `#123`; Escape dismisses. Ctrl/Cmd+Enter sends when no picker selection is being inserted. The Reference a task button also opens the picker. Referenced tasks appear as navigable links under messages. The picker uses the entire project, independently of board filters, and shows active task owners.

## Chatter scope

`server/internal/harness/fleet/chatter.toml` registers the bundled agent; `chatter-instructions.md` is the trusted root conversation contract. Chatter reads current workflow stages, Conductor settings/prompts, enabled task types, checklists, active owners, and document metadata through `project_context`. It can read written document notes; binary originals are not automatically parsed/OCRed.

Chatter can create and refine free tasks, move them using the actual configured stage IDs, attach verified GitHub references, and request task work. Repository access in the conversation is read-only. `request_task_work` prepares and enqueues the normal task agent; implementation runs use the existing isolated-worktree worker. A queue acknowledgment is not a completion claim. Task agents and Conductor retain their existing narrower tool sets.

MCP mutations check both project membership and live chat-turn identity inside the same SQLite writer transaction as the edit or dispatch. A canceled/completed turn cannot keep writing. Active Conductor reservations and pending/claimed/running/canceling task jobs block competing chat intervention. Humans retain control of normal task edits.

The conversation queue runs alongside the existing task worker, under the same application database process lock. Stop immediately revokes writes and cancels the model turn. Startup marks in-flight turns interrupted instead of replaying uncertain writes. Pending turns can still start. Preview environments block chat execution.

Aycorn supplies `approval_mode = "approve"` only for each tool it exposes in that scoped session. This is required for already-authorized local task writes under a non-interactive, never-prompt session; server-side ownership and scope checks remain authoritative. It does not change the user's global Codex settings or grant external GitHub actions. See the [official MCP configuration documentation](https://learn.chatgpt.com/docs/extend/mcp?surface=cli).

## Verification

Backend tests cover idempotent retries, conflicting keys, concurrent enqueue, checkpoint persistence/native-session resume, canceled turns, restart recovery, unknown client fields, cross-project references, scoped tool registration, and owner-protected mutations. Frontend tests cover numeric/title filtering, caret-aware insertion, and reference links. Live browser verification exercises native task creation through MCP, following up in the same session, reload persistence, and keyboard task insertion.
