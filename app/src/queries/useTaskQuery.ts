import { useQuery } from "@tanstack/react-query";
import type { TaskWithProject } from "@/types/types";
import { toValidBody } from "@/lib/plate";
import { rememberTaskBodyRevision } from "@/lib/task-body-revision";

const tryParse = (value: string): unknown => {
  try {
    return JSON.parse(value);
  } catch {
    return null;
  }
};

export function useTaskQuery(taskId: number) {
  return useQuery({
    queryKey: ["task", taskId],
    queryFn: async () => {
      const response = await fetch(`/api/task/${taskId}`);
      if (!response.ok) throw new Error(await response.text());
      rememberTaskBodyRevision(taskId, response);
      const raw = await response.json();
      const parsedBody =
        typeof raw.Body === "string" ? tryParse(raw.Body) : raw.Body;
      return {
        ...raw,
        Body: toValidBody(parsedBody),
      } as TaskWithProject;
    },
    enabled: taskId !== 0,
    refetchOnWindowFocus: false,
  });
}

export function useTaskBodyQuery(taskId: number, enabled: boolean) {
  const { isPending, error, data, isFetching, refetch } = useQuery({
    queryKey: ["taskBody", taskId],
    queryFn: async () => {
      const response = await fetch(`/api/task/body/${taskId}`);
      if (!response.ok) throw new Error(await response.text());
      rememberTaskBodyRevision(taskId, response);
      const res = await response.json();
      // Always hand the editor a valid Plate document (an array). Guard against
      // an empty/invalid body (e.g. a row damaged by the historical body-clobber
      // bug) — JSON.parse("") throws, and a damaged row can parse to "".
      const parsed = typeof res === "string" ? tryParse(res) : res;
      return toValidBody(parsed);
    },
    enabled: enabled && taskId !== 0,
    refetchOnWindowFocus: false,
  });

  return { isPending, error, data, isFetching, refetch };
}
