import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import type {
  AIRunInput,
  AISettings,
  AISettingsResponse,
} from "@/features/ai/types";
import type { AgentJob } from "@/features/agentJob/queries/useAgentJobs";
import type { Project, Task } from "@/types/types";

const request = async <T>(url: string, init?: RequestInit): Promise<T> => {
  const response = await fetch(url, init);
  if (!response.ok) throw new Error((await response.text()).trim());
  return response.json() as Promise<T>;
};
export const useAISettings = () =>
  useQuery({
    queryKey: ["ai-settings"],
    queryFn: () => request<AISettingsResponse>("/api/ai/settings"),
    staleTime: 30_000,
    refetchOnWindowFocus: false,
  });
export const useAIContext = (taskId: number) => {
  const task = useQuery({
    queryKey: ["ai-task", taskId],
    refetchInterval: 2000,
    queryFn: () => request<Task & { ProjectID: number }>(`/api/task/${taskId}`),
    enabled: taskId > 0,
  });
  const projects = useQuery({
    queryKey: ["ai-projects"],
    queryFn: () => request<Project[]>("/api/project"),
    enabled: taskId > 0,
  });
  return {
    task,
    projects,
    project: projects.data?.find((p) => p.ID === task.data?.ProjectID),
  };
};
export const useAIMutations = (taskId: number) => {
  const client = useQueryClient();
  const invalidate = () => {
    void client.invalidateQueries({ queryKey: ["task-ownership"] });
    void client.invalidateQueries({ queryKey: ["agent-jobs"] });
    void client.invalidateQueries({ queryKey: ["task-branches", taskId] });
    void client.invalidateQueries({ queryKey: ["active-agent-jobs"] });
  };
  const start = useMutation({
    mutationFn: (input: AIRunInput) =>
      request<AgentJob>(`/api/ai/tasks/${taskId}/runs`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(input),
      }),
    onSuccess: () => {
      invalidate();
      toast.success("AI run queued");
    },
    onError: (error: Error) => toast.error(error.message),
  });
  const cancel = useMutation({
    mutationFn: (jobId: number) =>
      request<boolean>(`/api/ai/runs/${jobId}/cancel`, { method: "POST" }),
    onSuccess: invalidate,
    onError: (error: Error) => toast.error(error.message),
  });
  return { start, cancel };
};
export const useUpdateAISettings = () => {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (settings: AISettings) =>
      request<AISettings>("/api/ai/settings", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(settings),
      }),
    onSuccess: (settings) => {
      client.setQueryData<AISettingsResponse>(["ai-settings"], (old) =>
        old ? { ...old, settings } : old,
      );
      void client.invalidateQueries({ queryKey: ["ai-settings"] });
    },
    onError: (error: Error) => toast.error(error.message),
  });
};
