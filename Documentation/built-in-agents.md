# Built-in agents

Every project uses **Conductor** as its fixed orchestrator, with **Planner**, **Research**, **Coder**, and **Reviewer** for delegated work. **Chatter** handles the persistent project conversation. Projects configure workflow stages, stage-specific requirements, and repository use; they do not select a replacement Conductor or Coder.

On the **AI** page, open an agent to edit its OpenAI model. Changes autosave on blur or Enter. Names, roles, prompts, skill references, and tool permissions are read-only, and creation/deletion controls are unavailable. The details panel renders the actual bundled Markdown and the workflow skill. Older custom agents remain visible with their saved instructions, for compatibility with existing task and Job references, but only their models can be changed.

Models are captured when a new run begins. Conductor retains all role models across planning, implementation, and an independent Job recheck; changing preferences cannot silently change an active cycle. A Job's selected agent supplies the Coder model, while Conductor's and Coder's prompts stay fixed. Chatter uses its own model preference. Manual runs without a selected agent continue to use the default model.

## One definition for the UI and execution

The catalog and canonical Markdown live in `server/internal/harness/fleet/`. The persona read path overlays these bundled definitions onto stored preferences, and the harness generates each role's TOML from that same Markdown. A legacy selected prompt cannot replace the Conductor or Coder system contract. Native configurations include the bundled `aycorn-workflow` skill explicitly; Planner, Research, Reviewer, and Chatter use read-only repository sandboxes. Runtime MCP scope and ownership checks continue to enforce task access.

The checked-in `.codex/agents/` TOML and Markdown files mirror the catalog for interactive work in this repository. `.codex/config.toml` registers them. Research uses the existing `research.toml` filename and the runtime role name `researcher`.

After changing canonical instructions, run:

```sh
make sync-agents
make check-agents
```

The check rejects drift between bundled instructions and repository agents. Rebuild/restart Aycorn to load a new instruction version. Model preferences belong to SQLite, so changing a model in the UI does not rewrite repository files. Managed runs materialize immutable configurations in a content-addressed fleet directory beside the database. See [official OpenAI documentation on Codex subagents](https://learn.chatgpt.com/docs/agent-configuration/subagents) for standalone agent files and per-agent model configuration.

## Migration and API

Migration `00026_fixed_agent_roles.sql` adopts existing named roles, preserves their models and stored prompts, inserts missing roles, and pins project agent IDs to Conductor/Coder. Other agents, job history, workflow choices, and stage prompts are retained. It adds a partial unique index on `persona.builtin_role`; existing priority, type, and stage CHECK constraints are unchanged.

- `GET /api/persona` and `GET /api/persona/{id}` include `BuiltinRole`, `Description`, `Instructions`, `InstructionPath`, and `Skills` for built-in agents.
- `PUT /api/persona/{id}` accepts only `{ "Model": "gpt-5.6-sol" }`. Other fields, malformed requests, and invalid models are rejected.
- Agent creation, deletion, and legacy bulk mutation routes return HTTP 403.
- Project Conductor patches reject `conductorAgentId` and `taskAgentId`; the server resolves these identities even for projects with no saved Conductor configuration.

Regression checks cover migration preservation, default identities, forbidden API mutations, model-only updates, stable models across a Conductor cycle, and the actual per-role configuration passed to Codex. Browser verification checks read-only instructions/skills and keyboard model autosave.
