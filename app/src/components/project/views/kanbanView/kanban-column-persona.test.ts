import { describe, expect, it } from "vitest";

import { shouldShowPersonaIndicator } from "@/components/project/views/kanbanView/kanban-column-persona";
import type { Stage } from "@/types/types";

describe("shouldShowPersonaIndicator", () => {
  it("shows the Bot indicator for a persona-bound stage", () => {
    // Given a hydrated stage persona
    const stage: Pick<Stage, "Persona"> = {
      Persona: {
        ID: 3,
        Name: "Researcher",
        Harness: "codex",
        Model: "opencode-go/muse-spark-1.2-contributor",
        Agent: "",
      },
    };

    // When the kanban header condition is evaluated
    const result = shouldShowPersonaIndicator(stage);

    // Then the persona indicator is visible
    expect(result).toBe(true);
  });

  it("hides the Bot indicator for an explicitly unbound stage", () => {
    // Given a stage hydrated with a null persona
    const stage: Pick<Stage, "Persona"> = { Persona: null };

    // When the kanban header condition is evaluated
    const result = shouldShowPersonaIndicator(stage);

    // Then the persona indicator is absent
    expect(result).toBe(false);
  });

  it("hides the Bot indicator when persona hydration is omitted", () => {
    // Given a stage shape without persona data
    const stage: Pick<Stage, "Persona"> = {};

    // When the kanban header condition is evaluated
    const result = shouldShowPersonaIndicator(stage);

    // Then the persona indicator is absent
    expect(result).toBe(false);
  });
});
