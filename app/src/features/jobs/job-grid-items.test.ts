import { describe, expect, it } from "vitest";
import { jobGridItems } from "./job-grid-items";
import type { ScheduledJob, TaskTemplate } from "./use-jobs";

const templates = [
  {
    id: 1,
    name: "Weekly audit",
    title: "Review dependencies",
    body: "Find outdated packages",
  },
  {
    id: 2,
    name: "Release checklist",
    title: "Prepare release",
    body: "Verify migration and tests",
  },
] as TaskTemplate[];
const jobs = [
  { id: 2, templateId: 1, name: "Monday audit", schedule: "0 9 * * 1" },
] as ScheduledJob[];

describe("combined jobs and templates grid", () => {
  it("shows converted templates as jobs without duplicate source cards or ID collisions", () => {
    const items = jobGridItems(templates, jobs, "");
    expect(items.map((item) => item.kind)).toEqual(["jobs", "templates"]);
    expect(items[1].template?.id).toBe(2);
  });
  it("searches across names and task details with case-insensitive terms", () => {
    expect(jobGridItems(templates, jobs, " MONDAY packages ")).toHaveLength(1);
    expect(
      jobGridItems(templates, jobs, "template migration")[0].template?.id,
    ).toBe(2);
    expect(jobGridItems(templates, jobs, "unmatched")).toEqual([]);
  });
  it("restores the source template after conversion back and retains jobs with missing template data", () => {
    expect(jobGridItems(templates, [], "")).toHaveLength(2);
    expect(jobGridItems([], jobs, "Monday")).toHaveLength(1);
  });
});
