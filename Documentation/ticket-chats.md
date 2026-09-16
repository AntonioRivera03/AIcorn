# Ticket chats

Tasks started through Conductor, Jobs, or Task AI now use [independent task sessions](conductor-mode.md). For those tasks, this view shows the existing task conversation and the question/work composer: follow-up work requires confirmation, server code controls managed stages, and the selected role/model persists. The older chat endpoint cannot bypass that gate. The manual Chat-type flow below applies to tickets that have not entered a task session.

Select the bundled **Chat** task type to replace a ticket's document editor with a persistent conversation. It is enabled for existing and new projects alongside the normal default task type. Renaming Chat does not change its behavior. Switching back to a document type restores the original body; neither conversion deletes the body or conversation history.

The title, priority, stage, assignee, checklist, dates and relationships keep their normal controls. Chat never queues autonomous ticket work or changes these fields. The project Conductor toggle does not trigger chat messages. A ticket currently owned by Conductor must first be released from it before accepting a chat message.

Choose an agent (or the default Codex model), write a message and press **Send message** or **Ctrl/⌘ + Enter**. Draft messages are kept in this browser. **Ask** is read-only. **Edit files** requires a linked project repository and allows explicitly requested edits in an isolated worktree. The selected agent's instructions and current model are resolved for each turn. OpenCode remains unavailable.

Follow-up messages resume the stored native Codex conversation and the same worktree. The timeline retains every submitted message, response, failure or cancellation and the files changed during that turn, with expandable patches. The task's branch panel retains the cumulative branch changes and supports the existing merge and container-preview flows after the active turn finishes. Native conversation IDs can be copied as `codex resume` commands.

**Start a new conversation with this message** opens a separate native session and worktree while retaining earlier messages and branches. Use it when changing repository context or when the prior native conversation/worktree was removed outside Aycorn. Changes made in native Codex remain in its native history; Aycorn shows turns submitted through Aycorn and does not synchronize externally submitted messages into its timeline.

Only one active AI run can own a ticket at a time. Retrying a submission with the same request key returns that same turn. Pending cancellations do not lose the prior session. Interrupted and failed turns retain available output, native IDs and file changes; they never replay automatically. Repository merges and preview snapshots are blocked during an active turn. Cumulative and per-turn patches have a 4 MiB capture limit; a warning points to the preserved workspace when capture is incomplete.

## API

`POST /api/ai/tasks/{taskId}/chat` accepts `key`, `message`, `mode` (`ask` or `edit`), `presetId`, `useRepository` and `newConversation`. It returns the normal queued agent job. Native session IDs, workspace paths and transcript cursors are resolved from that ticket's history on the server; clients cannot supply them. Existing run history, cancel, branch and preview APIs apply.

Migration 20 adds a constrained `task_type.viewMode` (`document` or `chat`), enables the bundled Chat type for projects and adds a per-ticket unique chat request index. Down migration preserves tasks and stored transcripts.

## Verification

`chatHandler_test.go` covers strict input, concurrent idempotency, active-run conflicts, read-only/edit transitions, failure recovery, cancellation continuity, fresh conversations, per-turn versus cumulative changes and unchanged ticket fields. Worktree tests cover resume identity, unchanged staging and merge/preview exclusion. The migration test upgrades existing projects and verifies defaults for new projects. `TestCodexResumesChatWithCurrentContextAndScope` covers resume RPC parameters. Opt-in `TestInstalledCodexTurn` runs two turns against installed Codex and verifies native history survives across app-server processes before archiving its disposable test conversation.
