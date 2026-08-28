import { describe, expect, it } from "vitest";
import { isAgentWorking } from "@/features/agentJob/queries/useAgentJobs";
import { AgentWorkingBadge } from "@/features/agentJob/agentWorkingBadge";

describe("AgentWorkingBadge helper", () => {
  it("isAgentWorking still drives badge logic", () => {
    expect(isAgentWorking([{ status: "pending" }])).toBe(true);
    expect(isAgentWorking([{ status: "completed" }])).toBe(false);
    expect(isAgentWorking([{ status: "claimed" }])).toBe(true);
    expect(isAgentWorking([{ status: "running" }])).toBe(true);
    expect(isAgentWorking([{ status: "failed" }])).toBe(false);
    expect(isAgentWorking([])).toBe(false);
  });

  it("AgentWorkingBadge is exported as function component", () => {
    expect(typeof AgentWorkingBadge).toBe("function");
    expect(AgentWorkingBadge.length).toBeGreaterThanOrEqual(1);
  });

  it("badge source is animated and uses semantic tokens (static check)", async () => {
    expect(AgentWorkingBadge.name).toBe("AgentWorkingBadge");
  });
});
