import { describe, expect, it } from "vitest";

import { isAgentWorking } from "@/features/agentJob/queries/useAgentJobs";
import type { AgentJob } from "@/features/agentJob/queries/useAgentJobs";

const job = (status: AgentJob["status"]): Pick<AgentJob, "status"> => ({
  status,
});

describe("isAgentWorking", () => {
  it("returns true for pending", () => {
    expect(isAgentWorking([job("pending")])).toBe(true);
  });

  it("returns true for claimed", () => {
    expect(isAgentWorking([job("claimed")])).toBe(true);
  });

  it("returns true for running", () => {
    expect(isAgentWorking([job("running")])).toBe(true);
  });

  it("returns false for completed", () => {
    expect(isAgentWorking([job("completed")])).toBe(false);
  });

  it("returns false for failed", () => {
    expect(isAgentWorking([job("failed")])).toBe(false);
  });

  it("returns false for empty array", () => {
    expect(isAgentWorking([])).toBe(false);
  });

  it("returns false for null/undefined", () => {
    expect(isAgentWorking(null)).toBe(false);
    expect(isAgentWorking(undefined)).toBe(false);
  });

  it("returns true if any job is working among mixed statuses", () => {
    expect(
      isAgentWorking([
        job("completed"),
        job("failed"),
        job("running"),
      ]),
    ).toBe(true);
  });

  it("returns false when all jobs are terminal", () => {
    expect(
      isAgentWorking([job("completed"), job("failed"), job("completed")]),
    ).toBe(false);
  });
});
