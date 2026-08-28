import { useQuery } from "@tanstack/react-query";

export const AGENT_JOB_WORKING_STATUSES = [
  "pending",
  "claimed",
  "running",
] as const;

export type AgentJobWorkingStatus =
  (typeof AGENT_JOB_WORKING_STATUSES)[number];

export type AgentJobStatus =
  | AgentJobWorkingStatus
  | "completed"
  | "failed";

export type AgentJob = {
  id: number;
  task: number;
  persona: number;
  status: AgentJobStatus;
  fromStage: number | null;
  toStage: number | null;
  claimedAt: string | null;
  startedAt: string | null;
  finishedAt: string | null;
  attempts: number;
  error: string;
  createdAt: string | null;
};

export type AgentRun = {
  id: number;
  job: number;
  output: string;
  summary: string;
  exitCode: number | null;
  usageJson: string;
  createdAt: string | null;
};

export type AgentJobsResponse = {
  jobs: AgentJob[];
  runs: AgentRun[];
};

const WORKING_SET: ReadonlySet<string> = new Set(
  AGENT_JOB_WORKING_STATUSES as readonly string[],
);

/**
 * Returns true if any job is still in flight (pending | claimed | running).
 * Any view can use this to show a working badge without duplicating status checks.
 */
export function isAgentWorking(
  jobs: Pick<AgentJob, "status">[] | null | undefined,
): boolean {
  if (!jobs || jobs.length === 0) return false;
  return jobs.some((j) => WORKING_SET.has(j.status));
}

export function useAgentJobs(taskId: number) {
  return useQuery<AgentJobsResponse>({
    queryKey: ["agent-jobs", taskId],
    enabled: !!taskId,
    staleTime: 5_000,
    refetchOnWindowFocus: true,
    refetchInterval: false,
    queryFn: async () => {
      const response = await fetch(`/api/agent-jobs/${taskId}`);
      if (!response.ok) throw new Error(await response.text());
      return (await response.json()) as AgentJobsResponse;
    },
  });
}
