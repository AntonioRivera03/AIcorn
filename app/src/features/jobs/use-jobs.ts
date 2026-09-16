import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";

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
export function useJobResources(projectId: number) {
  const url = `/api/project/${projectId}/automation`;
  const templates = useQuery({
    queryKey: ["task-templates", projectId],
    queryFn: () => jobRequest<TaskTemplate[]>(`${url}/templates`),
  });
  const jobs = useQuery({
    queryKey: ["scheduled-jobs", projectId],
    queryFn: () => jobRequest<ScheduledJob[]>(`${url}/jobs`),
    refetchInterval: 10000,
  });
  return { templates, jobs };
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
