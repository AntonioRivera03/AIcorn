import { apiFetch } from "@/lib/api";
import { useQuery } from "@tanstack/react-query";
import type { Stage } from "@/types/types";

export function useAllStagesQuery() {
  return useQuery<Stage[]>({
    queryKey: ["allStages"],
    queryFn: async () => {
      const res = await apiFetch("/api/stage");
      if (!res.ok) throw new Error(await res.text());
      return res.json();
    },
  });
}
