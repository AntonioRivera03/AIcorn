# Project Chats and Chatter

The **Chats** project view is a set of conversations with Chatter, the project's knowledge bank and project manager. The chat list sits on the left, newest first, and can be renamed (double-click, or the row's menu) or deleted. A new chat shows one headline and a small, rounded composer in the middle of the view; after the first message the composer moves to the bottom and the conversation fills the space above it. The open chat is in the URL (`?view=chats&chat=12`).

Each chat keeps its own native Codex session, resumed on every turn. Messages, partial output, activity, status, session and turn IDs, and usage are stored in SQLite (`project_chat` and `project_chat_turn`; migration 00031 lifted the one-chat-per-project limit). A client request key prevents duplicate sends, including a retry that doesn't yet know which chat the first attempt created. One turn can be active per chat. Deleting a chat archives it and stops a working turn; tasks keep their record of which chat turn changed them.

## Timeline

The timeline follows T3 Code. The user's message is a right-aligned bubble; Chatter's answer is plain text under a header row. While a turn works, the header counts "Working for 12s" and the work shows live: "Thinking" (from the model's reasoning summaries) and one line per tool call, with a moving highlight on whatever is in progress. When the turn ends, its work folds behind "Worked for 1m 12s", runs of tool calls fold into a sentence ("Read 3 tasks and created 1 task"), and a panel under the answer lists what changed ("Created #42 Export button", "Moved #17 to Review"), linking each task. Rows open to a tool's arguments and result.

The activity comes from Codex's item events (`reasoning`, `mcpToolCall`, …), collected in `internal/harness/activity.go` and saved on the turn as `activityJson`. Labels and the change summary are worked out in the app (`features/project-chat/activity.ts`) from the tool names, arguments, and results.

## Task references

Type `#` to open a task picker above the composer. Digits filter task numbers; `#"` filters titles. Arrow keys select; Enter or Tab inserts `#123`; Escape closes. Enter sends and Shift+Enter starts a new line. The `#` button also opens the picker.

## Chatter scope

Chatter never writes code. Its instructions (`server/internal/harness/fleet/chatter-instructions.md`) say so, and its session enforces it: it runs in a read-only sandbox in an empty folder of its own, never the project repository, with Codex's shell, unified exec, browser, computer use, image generation, plugins, multi-agent tools, and web search turned off. Its only tools are Aycorn's: `project_context`, `read_task`, `search_tasks`, `create_task`, `update_task`, `move_task_stage`, the task-link tools, `read_project_document`, and `send_to_conductor`. To get work done it creates or updates a task and gives it to Conductor, which picks the agent; it can't start agents itself.

On Codex, Chatter runs on GPT-6 Sol (`fleet.Definition.Models`); on other harnesses it uses the workspace's default model.

MCP mutations check project membership and the live chat turn inside the same SQLite write transaction as the edit, so a canceled, finished, or deleted chat can't keep writing. Tasks owned by Conductor or an agent are off limits. Stop revokes writes and cancels the model turn. Startup marks in-flight turns interrupted instead of replaying uncertain writes.

Aycorn sets `approval_mode = "approve"` only for each tool it exposes in that session, which non-interactive sessions need for already-authorized task writes; server-side ownership and scope checks remain authoritative.
