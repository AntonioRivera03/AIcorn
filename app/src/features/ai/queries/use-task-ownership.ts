import { useQuery } from "@tanstack/react-query";

export type TaskOwner = {
  taskId: number;
  kind: "conductor" | "agent";
  jobId?: number;
  name: string;
  state: string;
};

export function useTaskOwnership(projectId: number | undefined) {
  return useQuery({
    queryKey: ["task-ownership", projectId],
    queryFn: async () => {
      const response = await fetch(`/api/task-ownership/project/${projectId}`);
      if (!response.ok)
        throw new Error("Could not check which tasks are being worked on.");
      return response.json() as Promise<TaskOwner[]>;
    },
    enabled: !!projectId,
    refetchInterval: 3000,
  });
}
