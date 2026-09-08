import { describe, expect, it } from "vitest";

import {
  createAssigneeOptions,
  getAssigneeOptionState,
} from "@/features/task/properties/task-assignee-options";
const options = ["Me", "Alice", "Researcher", "Stage Builder"];

describe("createAssigneeOptions", () => {
 it("preserves existing free-text owners without injecting personas", () => {
  expect(createAssigneeOptions([{Assignee:"Alice"}, {Assignee:"Researcher"}, {Assignee:""}, {Assignee:"Alice"}])).toEqual(["Me", "Alice", "Researcher"]);
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
