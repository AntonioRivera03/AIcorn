import { describe, expect, it } from "vitest";

import {
  createAssigneeOptions,
  filterAssigneeOptions,
} from "@/features/task/properties/task-assignee-options";

describe("createAssigneeOptions", () => {
  it("lists members first and keeps other existing assignees separately", () => {
    // Given workspace members and tasks assigned to members and non-members
    const tasks = [
      { Assignee: "Alice" },
      { Assignee: "AI · Researcher" },
      { Assignee: "" },
      { Assignee: "AI · Researcher" },
      { Assignee: "Former Member" },
    ];

    // When the options are built
    const result = createAssigneeOptions(["Alice", "Bob"], tasks);

    // Then members come from the workspace and others are deduplicated
    expect(result).toEqual({
      members: ["Alice", "Bob"],
      others: ["AI · Researcher", "Former Member"],
    });
  });

  it("offers members even when no task is assigned yet", () => {
    expect(createAssigneeOptions(["Alice"], [])).toEqual({ members: ["Alice"], others: [] });
  });
});

describe("filterAssigneeOptions", () => {
  const options = { members: ["Alice", "Bob"], others: ["AI · Researcher"] };

  it("returns everything for an empty or whitespace search", () => {
    expect(filterAssigneeOptions(options, "   ")).toBe(options);
  });

  it("filters both groups by a trimmed case-insensitive substring", () => {
    expect(filterAssigneeOptions(options, "  sEaR ")).toEqual({
      members: [],
      others: ["AI · Researcher"],
    });
    expect(filterAssigneeOptions(options, "b")).toEqual({ members: ["Bob"], others: [] });
  });
});
