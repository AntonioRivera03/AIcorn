import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { isAgentWorking } from "@/features/agentJob/queries/useAgentJobs";
import type {
  MergePreview,
  MergeResult,
  TaskBranch,
} from "@/features/task/branches/types";

const request = async <T>(url: string, init?: RequestInit): Promise<T> => {
  const response = await fetch(url, init);
  if (!response.ok) throw new Error((await response.text()).trim());
  return response.json() as Promise<T>;
};
const branchURL = (taskId: number, jobId: number) =>
  `/api/ai/tasks/${taskId}/branches/${jobId}/merge`;

export const useTaskBranches = (taskId: number) =>
  useQuery({
    queryKey: ["task-branches", taskId],
    queryFn: () => request<TaskBranch[]>(`/api/ai/tasks/${taskId}/branches`),
    enabled: taskId > 0,
    staleTime: 5_000,
    refetchInterval: (query) =>
      isAgentWorking(query.state.data) ? 3_000 : 15_000,
  });

export const useMergePreview = (
  taskId: number,
  jobId: number,
  target: string,
) =>
  useQuery({
    queryKey: ["branch-merge-preview", taskId, jobId, target],
    queryFn: ({ signal }) =>
      request<MergePreview>(
        `${branchURL(taskId, jobId)}?target=${encodeURIComponent(target)}`,
        { signal },
      ),
    staleTime: 0,
    retry: false,
  });

export const useMergeBranch = (taskId: number, jobId: number) => {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (preview: MergePreview) =>
      request<MergeResult>(branchURL(taskId, jobId), {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ target: preview.target, token: preview.token }),
      }),
    onSuccess: (result) => {
      toast.success(`Merged into ${result.target}`);
      if (result.warning) toast.warning(result.warning);
    },
    onError: (error: Error) => toast.error(error.message),
    onSettled: () => {
      void client.invalidateQueries({ queryKey: ["task-branches"] });
      void client.invalidateQueries({ queryKey: ["branch-merge-preview"] });
    },
  });
};
