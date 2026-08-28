import { describe, expect, it } from "vitest";

import { isAgentWorking } from "@/features/agentJob/queries/useAgentJobs";
import type { AgentJob } from "@/features/agentJob/queries/useAgentJobs";

const job = (status: AgentJob["status"], task = 1): Pick<AgentJob, "status" | "task"> => ({
  status,
  task,
});

function activeTaskIds(jobs: Pick<AgentJob, "status" | "task">[]): Set<number> {
  const working = new Set(["pending", "claimed", "running"]);
  const s = new Set<number>();
  for (const j of jobs) if (working.has(j.status)) s.add(j.task);
  return s;
}

describe("useActiveAgentJobs logic", () => {
  it("pending/claimed/running are working, completed/failed are not", () => {
    expect(isAgentWorking([job("pending")])).toBe(true);
    expect(isAgentWorking([job("claimed")])).toBe(true);
    expect(isAgentWorking([job("running")])).toBe(true);
    expect(isAgentWorking([job("completed")])).toBe(false);
    expect(isAgentWorking([job("failed")])).toBe(false);
  });

  it("activeTaskIds contains only working task ids", () => {
    const ids = activeTaskIds([
      job("pending", 10),
      job("completed", 20),
      job("running", 30),
      job("failed", 40),
      job("claimed", 50),
    ]);
    expect(ids.has(10)).toBe(true);
    expect(ids.has(20)).toBe(false);
    expect(ids.has(30)).toBe(true);
    expect(ids.has(40)).toBe(false);
    expect(ids.has(50)).toBe(true);
    expect(ids.size).toBe(3);
  });

  it("queryKey includes projectId", () => {
    const projectId = 42;
    const queryKey = ["active-agent-jobs", projectId] as const;
    expect(queryKey[0]).toBe("active-agent-jobs");
    expect(queryKey[1]).toBe(projectId);
  });

  it("active set deduplicates same task", () => {
    const ids = activeTaskIds([job("pending", 7), job("running", 7)]);
    expect(ids.size).toBe(1);
    expect(ids.has(7)).toBe(true);
  });

  it("empty jobs gives empty set", () => {
    expect(activeTaskIds([]).size).toBe(0);
  });
});
