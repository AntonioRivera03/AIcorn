---
name: aycorn-ticket-template
description: >
  Format a ticket body for the Aycorn project management app using its standard
  Acceptance Criteria template, then write it via the aycorn MCP update_task tool.
  Use whenever creating or updating an Aycorn ticket's body/description, or when
  the user asks to "update the ticket", "write up the ticket", "fill in acceptance
  criteria", or similar for a task in Aycorn.
---

# Aycorn Ticket Template

When writing or updating an Aycorn ticket's body, always structure it with this
Markdown template:

```markdown
# Acceptance Criteria

### Type
 - [New Feature/ Existing Feature change/Update/UI Changes]
## Goal
 - Enter goal
## Reason
 - Enter reasons
## Implementation
 - Enter the planned pieces of work
## Solution
 - Enter solution
## Research
 - Enter context for this ticket to help in development
```

`Type`, `Goal`, and `Reason` are always present. `Implementation`, `Solution`,
and `Research` are all conditional — see "The Implementation section is
optional", "The Solution section is optional", and "The Research section is
conditional" below.

## Rules

- **Keep the Markdown formatting intact** — the `#`/`##`/`###` headers and `-`
  bullets must be preserved exactly as shown when the body is written into Aycorn
  via `mcp__aycorn__update_task`. Do not flatten headers into plain text or strip
  the hashtags.
- **Pick one `Type`** value from the bracketed options (New Feature, Existing
  Feature change, Update, UI Changes) — don't leave the brackets literally in
  the final body.
- **Be short.** A few bullets per section, not paragraphs. Someone should grasp
  the ticket in a few seconds, not read a report.

## Stay at capability altitude, not code altitude

A ticket describes what changed for someone using the app — a feature, a
behavior, a UI change — never how it was built. Write every ticket as if the
reader will never open the codebase.

**Never mention, in any section:** file paths, function/variable/type names,
line numbers, table or column names, specific code changes, library names, or
implementation approach. If a sentence would only make sense to someone reading
the diff, cut it or rewrite it at a higher altitude.

**The one exception: bug tickets.** When the ticket exists *because of* a bug,
naming the responsible file/function/line is the point — that precision is what
makes the ticket useful. Everywhere else, that same precision is noise.

### Calibrate technical language to the ticket, not to what you know

You'll usually know far more implementation detail than belongs in the ticket.
Match the *vocabulary*, not the depth, to what kind of thing shipped:

- **User-facing feature or UI change** (e.g. "Collapsible side nav sections"):
  plain language only, zero technical terms. Describe what the user sees and
  can now do. Reason is about user value, not architecture.
  - Goal: "Sidebar sections can be collapsed and expanded to reduce clutter."
  - Reason: "The nav was getting long with more sections added over time."
- **Infrastructure / system-level ticket** (e.g. "MCP to Server websocket
  integration"): technical terms are fine and expected (the audience for this
  ticket is technical), but stay at the level of *what the system now does* —
  never *how the code does it*. Describe the capability and the mechanism in
  plain-engineering terms (e.g. "the MCP server pushes live updates to the web
  app over a websocket instead of polling"), not the implementation (no
  mention of specific handlers, packages, or functions).
- **Mixed ticket** (e.g. "Document Management" — UI work plus backend/schema
  work together): describe each side at its own altitude in the same style —
  what the UI now lets someone do, and what new capability or data the backend
  now supports — without naming the tables, migrations, or files involved.

### The Implementation section is optional

Case-by-case, like Solution — but this one is forward-looking, not a report of
what already happened. Use it when a ticket bundles multiple distinct pieces
of work under one umbrella and it's worth laying out the planned scope up
front, e.g. "Overhaul assignees" listing out the individual features/changes
the overhaul is meant to include (multi-assignee support, an assignee filter,
avatar display, whatever the actual planned pieces are). Omit it for a ticket
that's already a single, atomic piece of work — Goal already covers that case,
and a one-item Implementation list is redundant with it.

Same altitude rules apply: list the planned capabilities/features, not the
technical steps to build them.

### The Solution section is optional

Default to including it — most tickets end with something concrete having
shipped, and "so what shipped?" is worth answering. Omit it (header included)
when there's no direct deliverable to point to: a planning/definition ticket
that only scopes or decides something for future work (e.g. "Define Exact
Controls of MCP Tools"), not a change made against the app itself. If the
ticket didn't ship a capability, there's nothing to put in Solution — don't
force a placeholder.

When present, same altitude rules as everywhere else: describe the capability
that now exists, not the implementation. For a user-facing ticket this reads
like a changelog entry ("Sidebar sections now have a collapse/expand toggle,
state persists per session"). For an infra ticket it can name the mechanism in
plain-engineering terms ("switched from polling to a websocket push") without
naming the code that does it.

### The Research section is conditional

Include `## Research` **only when the ticket is technical in nature** —
infrastructure/system-level tickets, or the technical half of a mixed ticket.
Omit the whole section (header included) for tickets that are purely
user-facing feature/UI work — don't leave it in empty or with a placeholder.

When present, it captures context that helps someone pick up the ticket cold —
prior discussion, decisions made and why, tradeoffs considered, open questions
— at the same capability altitude as the rest of the ticket. This is not a
technical notes dump. If what you want to write is "which files changed and
how," it belongs in the commit, not the ticket.
