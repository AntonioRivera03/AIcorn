import { useQuery } from "@tanstack/react-query";
import { useMemo } from "react";

import type { AgentJob } from "@/features/agentJob/queries/useAgentJobs";
import { AGENT_JOB_WORKING_STATUSES } from "@/features/agentJob/queries/useAgentJobs";

const WORKING_SET: ReadonlySet<string> = new Set(
  AGENT_JOB_WORKING_STATUSES as readonly string[],
);

function isWorkingStatus(status: string): boolean {
  return WORKING_SET.has(status);
}

/**
 * Consolidated poll for all active agent jobs in a project.
 * Single query per board instead of N per-card polls.
 */
export function useActiveAgentJobs(projectId: number) {
  const query = useQuery<AgentJob[]>({
    queryKey: ["active-agent-jobs", projectId],
    enabled: !!projectId,
    staleTime: 5_000,
    refetchOnWindowFocus: true,
    refetchInterval: (q) => {
      const jobs = q.state.data as AgentJob[] | undefined;
      const isAnyWorking = jobs ? jobs.some((j) => isWorkingStatus(j.status)) : false;
      return isAnyWorking ? 5_000 : 15_000;
    },
    queryFn: async () => {
      const response = await fetch(
        `/api/agent-jobs?projectId=${projectId}&status=active`,
      );
      if (!response.ok) throw new Error(await response.text());
      const data = await response.json();
      // Backend returns AgentJob[] directly for project-filtered queries,
      // but handle AgentJobsResponse shape defensively.
      if (Array.isArray(data)) return data as AgentJob[];
      if (data && Array.isArray((data as { jobs?: unknown }).jobs)) {
        return (data as { jobs: AgentJob[] }).jobs;
      }
      return [] as AgentJob[];
    },
  });

  const activeTaskIds = useMemo(() => {
    const jobs = query.data;
    if (!jobs || jobs.length === 0) return new Set<number>();
    const s = new Set<number>();
    for (const j of jobs) {
      if (isWorkingStatus(j.status)) s.add(j.task);
    }
    return s;
  }, [query.data]);

  const isTaskWorking = (taskId: number): boolean => activeTaskIds.has(taskId);

  return {
    ...query,
    activeTaskIds,
    isTaskWorking,
  };
}

export function isActiveAgentJobsWorking(jobs: Pick<AgentJob, "status">[] | null | undefined): boolean {
  if (!jobs || jobs.length === 0) return false;
  return jobs.some((j) => WORKING_SET.has(j.status));
}
