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
exact Markdown template:

```markdown
# Acceptance Criteria

### Type
 - [New Feature/ Existing Feature change/Update/UI Changes]
## Goal
 - Enter goal
## Reason
 - Enter reasons
## Research
 - Enter context for this ticket to help in development
```

## Rules

- **Keep the Markdown formatting intact** — the `#`/`##`/`###` headers and `-`
  bullets must be preserved exactly as shown when the body is written into Aycorn
  via `mcp__aycorn__update_task`. Do not flatten headers into plain text or strip
  the hashtags.
- **Pick one `Type`** value from the bracketed options (New Feature, Existing
  Feature change, Update, UI Changes) — don't leave the brackets literally in
  the final body.
- **Be concise.** Each section should be short and easy to scan — a few bullets
  or short sentences, not paragraphs. The reader should be able to grasp the
  ticket's intent in a few seconds, not have to parse a wall of text.
- **Research section** should capture whatever context/findings are relevant
  (prior discussion, tradeoffs considered, links to relevant code) — enough for
  someone picking up the ticket cold to start work without re-deriving it, but
  still trimmed to the essentials.
