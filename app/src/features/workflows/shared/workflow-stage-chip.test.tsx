import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { WorkflowStageChip } from "@/features/workflows/shared/workflow-stage-chip";
import type { Stage } from "@/types/types";

const stage: Stage = {
  ID: 11,
  Workflow: 2,
  Name: "Ready",
  Description: "Ready for work",
  Color: "green",
  Icon: "circle-dashed",
  Position: 1,
  Type: "todo",
  TaskCount: 3,
  TimeCreated: "2026-08-22T00:00:00Z",
  TimeModified: "2026-08-22T00:00:00Z",
};

describe("WorkflowStageChip persona marker", () => {
  it("exposes the bound persona name on a focusable marker", () => {
    const html = renderToStaticMarkup(
      <WorkflowStageChip
        stage={{
          ...stage,
          Persona: {
            ID: 4,
            Name: "Read-only Researcher",
            Harness: "codex",
            Model: "opencode-go/muse-spark-1.2-contributor",
            Agent: "",
          },
        }}
      />,
    );

    expect(html).toContain('aria-label="Persona: Read-only Researcher"');
    expect(html).toContain('tabindex="0"');
  });

  it("omits the marker when the stage has no persona", () => {
    const html = renderToStaticMarkup(<WorkflowStageChip stage={stage} />);

    expect(html).not.toContain("Persona:");
    expect(html).not.toContain('tabindex="0"');
  });
});
