import { describe, expect, it } from "vitest";

import {
  createAssigneeOptions,
  getAssigneeOptionState,
} from "@/features/task/properties/task-assignee-options";
import {
  createPersonaNames,
  isPersona,
} from "@/features/task/stage-move-assignee";

const options = ["Me", "Alice", "Researcher", "Stage Builder"];

describe("createAssigneeOptions", () => {
  it("combines self, task, catalog persona, and stage persona names without duplicates", () => {
    // Given assignee names supplied through every picker data source
    const tasks = [
      { Assignee: "Alice" },
      { Assignee: "Researcher" },
      { Assignee: "" },
      { Assignee: "Alice" },
    ];
    const personas = [{ Name: "Researcher" }, { Name: "" }];
    const stages = [
      { Persona: { Name: "Stage Builder" } },
      { Persona: null },
    ];

    // When TaskAssignee builds its command options
    const result = createAssigneeOptions(tasks, personas, stages);

    // Then every selectable identity appears once with self first
    expect(result).toEqual(options);
  });

  it("classifies a stage persona separately from a human option", () => {
    // Given names sourced from the persona catalog and stage hydration
    const personaNames = createPersonaNames(
      [{ Name: "Researcher" }],
      [{ Persona: { Name: "Stage Builder" } }],
    );

    // When TaskAssignee chooses Bot versus Users treatment
    const result = {
      stagePersona: isPersona("Stage Builder", personaNames),
      human: isPersona("Alice", personaNames),
    };

    // Then the stage persona is distinct while the human remains generic
    expect(result).toEqual({ stagePersona: true, human: false });
  });
});

describe("getAssigneeOptionState", () => {
  it("shows every option and no create action for an empty search", () => {
    // Given the complete assignee option list
    // When the search input is empty
    const result = getAssigneeOptionState(options, "");

    // Then the unfiltered command state is returned
    expect(result).toEqual({
      filteredOptions: options,
      hasExactMatch: false,
      showCreate: false,
      trimmedSearch: "",
    });
  });

  it("filters by a trimmed case-insensitive substring", () => {
    // Given mixed-case search input with surrounding whitespace
    // When the command state is derived
    const result = getAssigneeOptionState(options, "  SeAr  ");

    // Then matching is normalized while the trimmed display value is preserved
    expect(result).toEqual({
      filteredOptions: ["Researcher"],
      hasExactMatch: false,
      showCreate: true,
      trimmedSearch: "SeAr",
    });
  });

  it("hides create when an option matches exactly with different casing", () => {
    // Given a search matching an existing option after normalization
    // When the command state is derived
    const result = getAssigneeOptionState(options, " researcher ");

    // Then selecting the existing option replaces free-text creation
    expect(result).toEqual({
      filteredOptions: ["Researcher"],
      hasExactMatch: true,
      showCreate: false,
      trimmedSearch: "researcher",
    });
  });

  it("offers free-text creation for an unmatched non-empty search", () => {
    // Given a new assignee name
    // When the command state is derived
    const result = getAssigneeOptionState(options, "Bob");

    // Then no existing row matches and the create action is available
    expect(result).toEqual({
      filteredOptions: [],
      hasExactMatch: false,
      showCreate: true,
      trimmedSearch: "Bob",
    });
  });

  it("does not offer free-text creation for whitespace-only input", () => {
    // Given input with no usable assignee name
    // When the command state is derived
    const result = getAssigneeOptionState(options, "   ");

    // Then all options remain visible without a create action
    expect(result).toEqual({
      filteredOptions: options,
      hasExactMatch: false,
      showCreate: false,
      trimmedSearch: "",
    });
  });
});
