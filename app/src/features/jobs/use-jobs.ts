import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { useRef } from "react";
import { useNavigate } from "@tanstack/react-router";

export type TaskTemplate = {
  id: number;
  projectId: number;
  revision: number;
  name: string;
  title: string;
  body: string;
  checklistId: number;
  stageId: number;
  typeId: number;
  priority: string;
  assignee: string;
  prompt: string;
};
export type ScheduledJob = {
  id: number;
  projectId: number;
  templateId: number;
  name: string;
  agentId: number;
  schedule: string;
  timezone: string;
  enabled: boolean;
  nextRun: number;
  revision: number;
  lastError: string;
};
export type JobRun = {
  id: number;
  jobId: number;
  taskId: number;
  trigger: string;
  createdAt: string;
  state: string;
  message: string;
};
export async function jobRequest<T>(
  url: string,
  method = "GET",
  body?: unknown,
): Promise<T> {
  const res = await fetch(url, {
    method,
    headers:
      body === undefined ? undefined : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!res.ok) throw new Error((await res.text()).trim());
  return res.status === 204 ? (undefined as T) : (res.json() as Promise<T>);
}
export function useTaskTemplates(projectId: number, enabled = true) {
  return useQuery({
    queryKey: ["task-templates", projectId],
    queryFn: () =>
      jobRequest<TaskTemplate[]>(
        `/api/project/${projectId}/automation/templates`,
      ),
    enabled: enabled && projectId > 0,
  });
}
export function useJobResources(projectId: number) {
  const url = `/api/project/${projectId}/automation`;
  const templates = useTaskTemplates(projectId);
  const jobs = useQuery({
    queryKey: ["scheduled-jobs", projectId],
    queryFn: () => jobRequest<ScheduledJob[]>(`${url}/jobs`),
    refetchInterval: 10000,
  });
  return { templates, jobs };
}

// Grid cards and the editor share dispatch, retry identity and cache updates.
export function useRunJob(projectId: number, jobId: number) {
  const client = useQueryClient();
  const navigate = useNavigate();
  const key = useRef<string | null>(null);
  return useMutation({
    mutationKey: ["run-scheduled-job", jobId],
    mutationFn: () => {
      key.current ??= crypto.randomUUID();
      return jobRequest<{ taskId: number }>(
        `/api/project/${projectId}/automation/jobs/${jobId}/run`,
        "POST",
        { key: key.current },
      );
    },
    onSuccess: (result) => {
      key.current = null;
      toast.success("Job queued", {
        description: `Task #${result.taskId} is ready for Conductor.`,
        action: {
          label: "Open task",
          onClick: () =>
            void navigate({
              to: "/task/$taskId",
              params: { taskId: String(result.taskId) },
            }),
        },
      });
      for (const queryKey of [
        ["scheduled-job-runs", jobId],
        ["scheduled-jobs", projectId],
        ["projectDetails", projectId],
        ["conductor", projectId],
      ]) {
        void client.invalidateQueries({ queryKey });
      }
    },
    onError: (error: Error) => toast.error(error.message),
  });
}

// Serialize edits for each entity, reading the latest revision when execution
// starts. Rapid edits to different fields therefore cannot overwrite each other.
export function useJobEdit<T extends TaskTemplate | ScheduledJob>(
  projectId: number,
  id: number,
  kind: "templates" | "jobs",
) {
  const client = useQueryClient();
  const key = [
    kind === "templates" ? "task-templates" : "scheduled-jobs",
    projectId,
  ];
  return useMutation({
    scope: { id: `${kind}-${projectId}-${id}` },
    mutationFn: async (patch: Partial<T>) => {
      await client.cancelQueries({ queryKey: key });
      const current = client
        .getQueryData<T[]>(key)
        ?.find((item) => item.id === id);
      if (!current)
        throw new Error("This item is no longer available. Refresh the list.");
      return jobRequest<T>(
        `/api/project/${projectId}/automation/${kind}/${id}`,
        "PUT",
        { ...current, ...patch },
      );
    },
    onSuccess: (saved) =>
      client.setQueryData<T[]>(key, (items) =>
        items?.map((item) => (item.id === id ? saved : item)),
      ),
    onError: (error: Error) => {
      toast.error(error.message);
      void client.invalidateQueries({ queryKey: key });
    },
  });
}
