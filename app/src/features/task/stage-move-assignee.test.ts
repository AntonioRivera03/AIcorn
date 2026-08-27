import { describe, expect, it } from "vitest";

import {
  createPersonaNames,
  getStageMoveAssignee,
  isPersona,
} from "@/features/task/stage-move-assignee";

const TARGET_PERSONA = "Builder";
const CATALOG_PERSONA = "Researcher";
const STAGE_PERSONA = "Stage-only persona";
const personaNames = createPersonaNames(
  [{ Name: CATALOG_PERSONA }],
  [{ Persona: { Name: STAGE_PERSONA } }],
);

const moveAssignee = (currentAssignee: string) =>
  getStageMoveAssignee({
    currentAssignee,
    targetPersonaName: TARGET_PERSONA,
    personaNames,
  });

describe("createPersonaNames", () => {
  it("combines catalog and stage-bound persona names while ignoring empty bindings", () => {
    // Given catalog personas plus bound, empty, and absent stage personas
    const personas = [{ Name: CATALOG_PERSONA }, { Name: "" }];
    const stages = [
      { Persona: { Name: STAGE_PERSONA } },
      { Persona: { Name: "" } },
      { Persona: null },
      {},
    ];

    // When the shared persona-name set is created
    const result = createPersonaNames(personas, stages);

    // Then only usable names from both sources are retained
    expect(result).toEqual(new Set([CATALOG_PERSONA, STAGE_PERSONA]));
  });

  it("deduplicates a persona present in both the catalog and a stage binding", () => {
    // Given the same persona from both hydration paths
    const personas = [{ Name: CATALOG_PERSONA }];
    const stages = [{ Persona: { Name: CATALOG_PERSONA } }];

    // When the shared persona-name set is created
    const result = createPersonaNames(personas, stages);

    // Then the assignee classifier sees one stable name
    expect([...result]).toEqual([CATALOG_PERSONA]);
  });
});

describe("isPersona", () => {
  it("recognizes a persona known only through a stage binding", () => {
    // Given the combined persona-name set
    // When a stage-only assignee is classified
    const result = isPersona(STAGE_PERSONA, personaNames);

    // Then it is eligible for the Bot treatment
    expect(result).toBe(true);
  });

  it("does not classify a human assignee as a persona", () => {
    // Given the combined persona-name set
    // When a human assignee is classified
    const result = isPersona("Alice", personaNames);

    // Then it remains eligible for the Users treatment
    expect(result).toBe(false);
  });

  it("matches persona names case-sensitively", () => {
    // Given a catalog persona with canonical casing
    // When a differently-cased assignee is classified
    const result = isPersona(CATALOG_PERSONA.toLowerCase(), personaNames);

    // Then free-text assignee identity is not conflated
    expect(result).toBe(false);
  });
});

describe("getStageMoveAssignee", () => {
  it("assigns the target persona when the current assignee is empty", () => {
    // Given an unassigned task
    const currentAssignee = "";

    // When it moves into a persona-bound stage
    const result = moveAssignee(currentAssignee);

    // Then the stage persona becomes the assignee
    expect(result).toBe(TARGET_PERSONA);
  });

  it("overwrites a persona sourced from the persona catalog", () => {
    // Given a task assigned to a known persona
    const currentAssignee = "Researcher";

    // When it moves into a different persona-bound stage
    const result = moveAssignee(currentAssignee);

    // Then the target stage persona replaces it
    expect(result).toBe(TARGET_PERSONA);
  });

  it("overwrites a persona known only through a stage binding", () => {
    // Given a task assigned to a stage-hydrated persona
    const currentAssignee = "Stage-only persona";

    // When it moves into a different persona-bound stage
    const result = moveAssignee(currentAssignee);

    // Then the target stage persona replaces it
    expect(result).toBe(TARGET_PERSONA);
  });

  it("preserves the self assignee", () => {
    // Given a task assigned to the current user
    const currentAssignee = "Me";

    // When it moves into a persona-bound stage
    const result = moveAssignee(currentAssignee);

    // Then the human assignment remains unchanged
    expect(result).toBe(currentAssignee);
  });

  it("preserves a human assignee", () => {
    // Given a task assigned to a human who is not a persona
    const currentAssignee = "Alice";

    // When it moves into a persona-bound stage
    const result = moveAssignee(currentAssignee);

    // Then the human assignment remains unchanged
    expect(result).toBe(currentAssignee);
  });

  it.each([null, undefined])(
    "preserves the assignee when the target stage persona is %s",
    (targetPersonaName) => {
      // Given a task moving to a stage without a bound persona
      const currentAssignee = CATALOG_PERSONA;

      // When stage-move assignment is resolved
      const result = getStageMoveAssignee({
        currentAssignee,
        targetPersonaName,
        personaNames,
      });

      // Then the existing assignment is untouched
      expect(result).toBe(currentAssignee);
    },
  );

  it("preserves an assignee when no personas are known", () => {
    // Given a free-text assignee and an empty persona-name set
    const currentAssignee = CATALOG_PERSONA;

    // When the task moves into a persona-bound stage
    const result = getStageMoveAssignee({
      currentAssignee,
      targetPersonaName: TARGET_PERSONA,
      personaNames: new Set(),
    });

    // Then the name is treated as a protected human assignment
    expect(result).toBe(currentAssignee);
  });

  it("does not overwrite a differently-cased persona name", () => {
    // Given a free-text assignee that differs from a persona only by casing
    const currentAssignee = CATALOG_PERSONA.toLowerCase();

    // When the task moves into a persona-bound stage
    const result = moveAssignee(currentAssignee);

    // Then name matching remains case-sensitive
    expect(result).toBe(currentAssignee);
  });
});
