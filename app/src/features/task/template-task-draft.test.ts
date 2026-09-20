import { describe, expect, it } from "vitest";
import { templateTaskDraft } from "./template-task-draft";
import { defaultTaskContextValue } from "@/contexts/task/TaskContext";
import type { TaskTemplate } from "@/features/jobs/use-jobs";

const task = {
  ...defaultTaskContextValue.state,
  ID: 40,
  Name: "Before",
  Stage: 1,
  Checklist: 2,
  TimePlannedStart: "2026-09-16T12:00:00Z",
};
const template: TaskTemplate = {
  id: 7,
  projectId: 5,
  revision: 1,
  name: "Reusable",
  title: "Loaded title",
  body: "# Heading",
  checklistId: 12,
  stageId: 11,
  typeId: 10,
  priority: "High",
  assignee: "Fabiana",
  prompt: "Only used by jobs",
};
const body = [{ type: "h1", children: [{ text: "Heading" }] }];
const context = {
  projectId: 5,
  checklists: [{ ID: 12, Name: "Product" }],
  stages: [{ ID: 11 }],
  types: [{ ...task.Type, ID: 10, Name: "Feature" }],
};

describe("loading a task template locally", () => {
  it("copies defaults without changing identity, dates, inputs or the template", () => {
    const before = structuredClone(template);
    const draft = templateTaskDraft(task, template, body, context);
    expect(draft).toMatchObject({
      ID: 40,
      Name: "Loaded title",
      Stage: 11,
      Checklist: 12,
      ChecklistName: "Product",
      Type: { ID: 10 },
      Priority: "High",
      Assignee: "Fabiana",
      TimePlannedStart: task.TimePlannedStart,
    });
    draft.Body[0].children = [{ text: "Edited" }];
    expect(body[0].children[0].text).toBe("Heading");
    expect(task.Name).toBe("Before");
    expect(template).toEqual(before);
    expect(draft).not.toHaveProperty("prompt");
  });
  it("keeps the new task's defaults for unspecified template selections", () => {
    expect(
      templateTaskDraft(
        task,
        { ...template, checklistId: 0, stageId: 0, typeId: 0 },
        body,
        context,
      ),
    ).toMatchObject({ Stage: 1, Checklist: 2, Type: task.Type });
  });
  it("rejects cross-project and removed selections rather than silently loading invalid defaults", () => {
    expect(() =>
      templateTaskDraft(task, { ...template, projectId: 99 }, body, context),
    ).toThrow("another project");
    expect(() =>
      templateTaskDraft(task, { ...template, stageId: 99 }, body, context),
    ).toThrow("unavailable");
    expect(() =>
      templateTaskDraft(
        task,
        { ...template, priority: "Unknown" },
        body,
        context,
      ),
    ).toThrow("invalid priority");
  });
});
