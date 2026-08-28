import { useQuery } from "@tanstack/react-query";

import {
  AGENT_JOB_WORKING_STATUSES,
  type AgentJob,
  type AgentJobsResponse,
} from "@/features/agentJob/queries/useAgentJobs";

export type { AgentJob, AgentRun, AgentJobsResponse } from "@/features/agentJob/queries/useAgentJobs";
export { AGENT_JOB_WORKING_STATUSES, isAgentWorking } from "@/features/agentJob/queries/useAgentJobs";

const WORKING_SET: ReadonlySet<string> = new Set(
  AGENT_JOB_WORKING_STATUSES as readonly string[],
);

function isWorkingStatus(status: string): boolean {
  return WORKING_SET.has(status);
}

/**
 * Drawer-scoped fetch for agent jobs + runs for a single task.
 * - Key: ["agentJobs", taskId] (spec)
 * - Fetch: GET /api/agent-jobs/{taskId}
 * - Polling: only when drawer is open (`enabled`) and a job is still pending/claimed/running.
 *   Avoids the per-card 10s fan-out fixed in 46f6f54 — idle drawer does not poll.
 */
export function useAgentJobs(taskId: number, enabled = true) {
  return useQuery<AgentJobsResponse>({
    queryKey: ["agentJobs", taskId],
    enabled: enabled && !!taskId,
    staleTime: 5_000,
    refetchOnWindowFocus: true,
    refetchInterval: (q) => {
      if (!enabled) return false;
      const jobs = (q.state.data as AgentJobsResponse | undefined)?.jobs as
        | AgentJob[]
        | undefined;
      const isAnyWorking = jobs ? jobs.some((j) => isWorkingStatus(j.status)) : false;
      return isAnyWorking ? 5_000 : false;
    },
    queryFn: async () => {
      const response = await fetch(`/api/agent-jobs/${taskId}`);
      if (!response.ok) throw new Error(await response.text());
      return (await response.json()) as AgentJobsResponse;
    },
  });
}
