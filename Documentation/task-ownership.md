# Exclusive task ownership

A task can have one active AI owner. Pending, claimed, running, and canceling jobs reserve it. Conductor also retains a reservation while the task is waiting, planning, queued, or working, including the interval between planning and implementation runs. Pausing Conductor keeps those reservations; release the task from Conductor to hand it back.

Ordinary AI starts, ticket-chat turns, and agent MCP mutations check the same ownership rules. Checks and writes run under the same SQLite writer transaction, so a manual run and Conductor cannot both claim a free task. A database trigger also protects Conductor reservations from legacy queue insertion paths.

Agent property edits and stage moves are atomic, validate project/workflow references, and change only the supplied fields. Reads remain available. Human ticket edits remain available; existing Conductor content/stage comparisons stop it from applying a stale handoff after a human changes the ticket.

An agent may attach GitHub links to its own ticket only while its run is claimed or running. A canceled, interrupted, failed, or finished run cannot keep writing using an old job identity. Canceling runs retain the reservation until execution stops. Conductor reconciles failed/interrupted/canceled jobs into a non-active state before releasing its lifecycle reservation.

The task page, drawer, ticket chat, and Ask AI panel show active ownership. Competing starts are disabled in the UI and rejected independently on the server with an actionable conflict message. `GET /api/task-ownership/project/{projectId}` returns the project's current owners through one shared query.
