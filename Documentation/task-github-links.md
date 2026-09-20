# GitHub references on tasks

Every task page and drawer includes a **GitHub links** section, including tickets using the Chat view. Paste a GitHub pull request or branch URL and press Enter or leave the field to attach it. Labels and URLs edit in place; removing a reference requires confirmation and never deletes anything on GitHub.

Supported references are `https://github.com/owner/repo/pull/123` and `https://github.com/owner/repo/tree/branch-name`. Branch names containing slashes are preserved. Repository/owner casing, URL decorations, and leading zeroes in PR numbers normalize to a canonical URL. Reattaching that URL returns the existing record. Links are stored independently of the ticket body and do not fetch remote metadata.

Writes and deletes check revisions so a stale editor cannot overwrite newer changes. Invalid URL edits remain visible for correction; Escape discards a field's unsaved edit.

## Agent tools

- `list_task_links(taskId)` returns references, IDs, and revisions.
- `add_task_link(taskId, url, label?)` attaches a reference idempotently.
- `remove_task_link(taskId, linkId, revision)` removes a reference.

Task agents can edit links only on their own ticket while their run is active. Conductor can read related tickets' links within its project. External MCP agents can edit only unreserved tickets. Ownership is checked in the same database write transaction as the change; human edits remain available. Completing or canceling a run invalidates its agent identity for later writes.

## HTTP

`GET`/`POST /api/task-links/task/{taskId}` list or attach references. `PUT /api/task-links/task/{taskId}/{id}` accepts `url`, `label`, and `revision`. `DELETE` on that item path requires the current revision in `If-Match`.
