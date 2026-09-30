import { describe, expect, it } from "vitest";
import {
  describeTool,
  formatDuration,
  summarizeTools,
  taskChanges,
  thinkingPreview,
  type Names,
} from "@/features/project-chat/activity";
import type { ChatActivity } from "@/features/project-chat/types";

const names: Names = {
  task: (id) => ({ 17: "Export button", 42: "Write docs" })[id],
  stage: (id) => ({ 3: "Review" })[id],
};

const tool = (tool: string, extra: Partial<ChatActivity> = {}): ChatActivity => ({
  id: `${tool}-${Math.random()}`,
  kind: "tool",
  tool,
  status: "completed",
  ...extra,
});

describe("describeTool", () => {
  it("names tasks and stages in finished, live, and failed steps", () => {
    const move = tool("move_task_stage", { arguments: { taskId: 17, fromStage: 1, toStage: 3 } });
    expect(describeTool(move, names)).toEqual({ icon: "move", label: "Moved #17 Export button to Review", taskId: 17 });
    expect(describeTool({ ...move, status: "inProgress" }, names).label).toBe("Moving #17 Export button to Review");
    expect(describeTool({ ...move, status: "failed" }, names).label).toBe("Couldn't move #17 Export button to Review");
  });

  it("uses results for tasks it created or read", () => {
    const create = tool("create_task", { arguments: { name: "New" }, result: { ID: 99, Name: "New" } });
    expect(describeTool(create, names).label).toBe("Created #99 New");
    expect(describeTool({ ...create, status: "inProgress", result: undefined }, names).label).toBe("Creating “New”");
    expect(describeTool(tool("search_tasks", { arguments: { query: "export" } }), names).label).toBe("Searched tasks for “export”");
  });
});

describe("summarizeTools", () => {
  it("counts a run of tool calls in one sentence", () => {
    const run = [tool("read_task"), tool("read_task"), tool("search_tasks"), tool("create_task")];
    expect(summarizeTools(run)).toBe("Read 2 tasks, searched tasks, and created 1 task");
    expect(summarizeTools([tool("project_context"), tool("update_task")])).toBe("Checked the project and updated 1 task");
  });
});

describe("taskChanges", () => {
  it("lists each changed task once and skips reads and failures", () => {
    const changes = taskChanges(
      [
        tool("read_task", { arguments: { taskId: 17 } }),
        tool("create_task", { result: { ID: 42, Name: "Write docs" } }),
        tool("update_task", { arguments: { taskId: 42 } }),
        tool("update_task", { arguments: { taskId: 17 } }),
        tool("update_task", { arguments: { taskId: 17 } }),
        tool("move_task_stage", { arguments: { taskId: 17, toStage: 3 } }),
        tool("move_task_stage", { status: "failed", arguments: { taskId: 42, toStage: 3 } }),
        tool("send_to_conductor", { arguments: { taskIds: [17, 42] }, result: { sent: 1, skipped: 1 } }),
      ],
      names,
    );
    expect(changes.map((change) => change.label)).toEqual([
      "Created #42 Write docs",
      "Updated #17 Export button",
      "Moved #17 Export button to Review",
      "Sent 1 of 2 tasks to Conductor",
    ]);
  });
});

describe("formatting", () => {
  it("formats durations and thinking previews", () => {
    expect(formatDuration(400)).toBe("1s");
    expect(formatDuration(72_000)).toBe("1m 12s");
    expect(formatDuration(120_000)).toBe("2m");
    expect(thinkingPreview("\n**Planning** the launch\nmore")).toBe("Planning the launch");
  });
});
