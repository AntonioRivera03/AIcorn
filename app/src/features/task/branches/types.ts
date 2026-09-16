import type { AgentJobStatus } from "@/features/agentJob/queries/useAgentJobs";

export type TaskBranch = {
  jobId: number;
  taskId: number;
  status: AgentJobStatus;
  intent: string;
  repoPath: string;
  branch: string;
  workspace: string;
  baseCommit: string;
  createdAt: string;
  dirty: boolean;
  mergedInto: string[];
  problem?: string;
};
export type MergePreview = {
  target: string;
  targets: string[];
  token: string;
  commits: number;
  uncommitted: boolean;
  files: string[];
  diff: string;
  blocked?: string;
  alreadyMerged: boolean;
};
export type MergeResult = {
  target: string;
  commit: string;
  output: string;
  warning?: string;
};
