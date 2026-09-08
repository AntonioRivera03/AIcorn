import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import type { BulkResult } from "@/types/types";

export type EnvironmentSettings = {
  context: string;
  transport: "kind" | "registry";
  kindCluster: string;
  imageRepository: string;
  pullSecretNamespace: string;
  pullSecretName: string;
  platform: string;
  profile: "aycorn" | "custom";
  dockerfile: string;
  previewTarget: string;
  testTarget: string;
  testCommand: string[];
  command: string[];
  port: number;
  healthPath: string;
  cpuMillis: number;
  memoryMiB: number;
  testCpuMillis: number;
  testMemoryMiB: number;
  storageGiB: number;
  storageClass: string;
  timeoutSeconds: number;
  maxRunning: number;
  retentionHours: number;
  autoPreview: boolean;
  allowTestNetwork: boolean;
};
export type Environment = {
  id: number;
  projectId: number;
  taskId: number;
  jobId: number;
  name: string;
  branch: string;
  commit: string;
  includeChanges: boolean;
  digest: string;
  settings: EnvironmentSettings;
  state: string;
  desired: string;
  image: string;
  testImage: string;
  testState: string;
  testExitCode: number | null;
  url: string;
  error: string;
  pinned: boolean;
  expiresAt: number;
  createdAt: number;
  updatedAt: number;
};
export type ConnectionStatus = {
  kubectl: boolean;
  docker: boolean;
  kind: boolean;
  contexts: string[];
  storageClasses: string[];
  connected: boolean;
  error: string;
};
export type EnvironmentList = {
  projectId: number;
  environments: Environment[];
};
export type EnvironmentBranch = {
  name: string;
  current: boolean;
  hasWorktree: boolean;
};

export async function environmentRequest<T>(
  url: string,
  method = "GET",
  body?: unknown,
  signal?: AbortSignal,
): Promise<T> {
  const response = await fetch(url, {
    method,
    signal,
    headers:
      body === undefined ? undefined : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!response.ok) throw new Error((await response.text()).trim());
  return response.json() as Promise<T>;
}

export function useEnvironmentSettings(projectId: number) {
  const client = useQueryClient();
  const query = useQuery({
    queryKey: ["environment-settings", projectId],
    queryFn: () =>
      environmentRequest<EnvironmentSettings>(
        `/api/project/${projectId}/settings/environments`,
      ),
    enabled: projectId > 0,
  });
  const update = useMutation({
    scope: { id: `environment-settings-${projectId}` },
    mutationFn: (patch: Partial<EnvironmentSettings>) =>
      environmentRequest<EnvironmentSettings>(
        `/api/project/${projectId}/settings/environments`,
        "PUT",
        patch,
      ),
    onSuccess: (settings) => {
      client.setQueryData(["environment-settings", projectId], settings);
      void client.invalidateQueries({
        queryKey: ["environment-connection", projectId],
      });
    },
    onError: (error: Error) => toast.error(error.message),
  });
  return { ...query, update };
}
export function useEnvironments(projectId: number, taskId = 0) {
  const url = taskId
    ? `/api/environments/task/${taskId}`
    : `/api/environments/project/${projectId}`;
  return useQuery({
    queryKey: [
      "environments",
      taskId ? "task" : "project",
      taskId || projectId,
    ],
    queryFn: ({ signal }) =>
      environmentRequest<EnvironmentList>(url, "GET", undefined, signal),
    enabled: taskId > 0 || projectId > 0,
    refetchInterval: 4000,
  });
}
export function useEnvironmentMutations(projectId: number, taskId = 0) {
  const client = useQueryClient();
  const invalidate = () => {
    void client.invalidateQueries({ queryKey: ["environments"] });
  };
  const onError = (error: Error) => toast.error(error.message);
  const create = useMutation({
    mutationFn: (input: {
      branch?: string;
      jobId?: number;
      includeChanges?: boolean;
      requestKey: string;
    }) =>
      environmentRequest<Environment>(
        taskId
          ? `/api/environments/task/${taskId}`
          : `/api/environments/project/${projectId}`,
        "POST",
        input,
      ),
    onSuccess: () => {
      invalidate();
      toast.success("Preview queued");
    },
    onError,
  });
  const action = useMutation({
    mutationFn: ({
      id,
      action,
    }: {
      id: number;
      action: "stop" | "start" | "delete" | "rebuild" | "touch";
    }) =>
      environmentRequest<Environment>(
        `/api/environment/${id}${action === "delete" ? "" : `/${action}`}`,
        action === "delete" ? "DELETE" : "POST",
        action === "rebuild" ? { requestKey: crypto.randomUUID() } : undefined,
      ),
    onSuccess: (_, input) => {
      invalidate();
      if (input.action === "rebuild")
        toast.success(
          "Fresh preview queued; the previous environment is retained",
        );
    },
    onError,
  });
  const edit = useMutation({
    mutationFn: ({
      id,
      ...patch
    }: {
      id: number;
      name?: string;
      pinned?: boolean;
    }) =>
      environmentRequest<Environment>(`/api/environment/${id}`, "PUT", patch),
    onSuccess: invalidate,
    onError,
  });
  const bulk = useMutation({
    mutationFn: (input: { ids: number[]; action: "stop" | "delete" }) =>
      environmentRequest<BulkResult>(
        `/api/environments/project/${projectId}/bulk`,
        "POST",
        input,
      ),
    onSuccess: (result) => {
      invalidate();
      toast(
        `${result.success} environments updated.${result.skipped ? ` ${result.skipped} skipped.` : ""}${result.failed ? ` ${result.failed} failed — retry.` : ""}`,
      );
    },
    onError,
  });
  return { create, action, edit, bulk };
}
