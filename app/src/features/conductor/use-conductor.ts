import { apiFetch } from "@/lib/api";
import { useEffect } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { conductorSendNote } from "@/features/conductor/conductor-send-note";
import type { BulkResult, Stage } from "@/types/types";

export type ConductorSettings = {
  enabled: boolean;
  planningStage: number;
  workingStage: number;
  completionStage: number;
  planningPrompt: string;
  conductorAgentId: number;
  taskAgentId: number;
  useRepository: boolean;
};
export type ConductorTask = {
  taskId: number;
  projectId: number;
  state: "waiting" | "planning" | "needs_context" | "queued" | "working" | "completed" | "failed" | "held";
  jobId: number;
  message: string;
  updatedAt: string;
};
export type ConductorBoard = {
  settings: ConductorSettings;
  tasks: ConductorTask[];
  configurationError?: string;
  // Whether the working/finish stage above came from an automatic pick
  // rather than a saved choice (see stage-select.tsx).
  stagesAutoPicked?: boolean;
  // Whether configurationError's fix is a one-click "Add a Review stage".
  canAddReviewStage?: boolean;
};
export type ConductorAction = "send" | "release" | "recheck";

async function request<T>(url: string, method?: string, body?: unknown): Promise<T> {
  const response = await apiFetch(url, {
    method,
    headers: body ? { "Content-Type": "application/json" } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  });
  if (!response.ok) throw new Error((await response.text()).trim());
  return response.json() as Promise<T>;
}

export function useConductor(projectId: number) {
  const client = useQueryClient();
  const url = `/api/project/${projectId}/settings/conductor`;
  const query = useQuery({
    queryKey: ["conductor", projectId],
    enabled: projectId > 0,
    queryFn: () => request<ConductorBoard>(url),
    refetchInterval: (q) => q.state.data?.settings.enabled || q.state.data?.tasks.some((t) => ["planning", "queued", "working"].includes(t.state)) ? 2000 : 15000,
  });
  // One board poll drives cards and both views. Re-fetch tasks only when the
  // durable Conductor cursor changes, including a completed summary append.
  const revision = JSON.stringify(query.data?.tasks);
  useEffect(() => {
    if (!revision) return;
    void client.invalidateQueries({ queryKey: ["projectDetails", projectId] });
    void client.invalidateQueries({ queryKey: ["active-agent-jobs", projectId] });
    void client.invalidateQueries({ queryKey: ["agent-jobs"] });
    void client.invalidateQueries({ queryKey: ["taskBody"], refetchType: "none" });
    void client.invalidateQueries({ queryKey: ["task"], refetchType: "none" });
    void client.invalidateQueries({ queryKey: ["task-branches"] });
  }, [client, projectId, revision]);
  const update = useMutation({
    scope: { id: `conductor-settings-${projectId}` },
    mutationFn: (patch: Partial<ConductorSettings>) => request<ConductorSettings>(url, "PUT", patch),
    onSuccess: (settings) => {
      client.setQueryData<ConductorBoard>(["conductor", projectId], (old) => old ? { ...old, settings } : old);
      void client.invalidateQueries({ queryKey: ["conductor", projectId] });
    },
    onError: (error: Error) => {
      toast.error(error.message);
      void client.invalidateQueries({ queryKey: ["conductor", projectId] });
    },
  });
  const manage = useMutation({
    mutationFn: (input: { ids: number[]; action: ConductorAction }) => request<BulkResult>(`/api/project/${projectId}/conductor/bulk`, "POST", input),
    onSuccess: (result, input) => {
      void client.invalidateQueries({ queryKey: ["conductor", projectId] });
      const verb = input.action === "release" ? "Released" : input.action === "recheck" ? "Sent for recheck" : "Sent to Conductor";
      const note = input.action === "send" && result.success > 0 ? conductorSendNote(query.data) : "";
      toast(`${verb}: ${result.success}.` + (result.skipped ? ` ${result.skipped} ineligible or already managed.` : "") + (result.failed ? ` ${result.failed} failed — try again.` : "") + note);
    },
    onError: (error: Error) => toast.error(error.message),
  });
  const addReviewStage = useMutation({
    mutationFn: () => request<Stage>(`/api/project/${projectId}/settings/conductor/review-stage`, "POST"),
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: ["conductor", projectId] });
      void client.invalidateQueries({ queryKey: ["projectWorkflowSettings", projectId] });
      void client.invalidateQueries({ queryKey: ["projectDetails", projectId] });
      toast("Added a Review stage.");
    },
    onError: (error: Error) => toast.error(error.message),
  });
  return { ...query, update, manage, addReviewStage };
}
