import { useQuery } from "@tanstack/react-query";
import type { Persona } from "@/types/types";

export function usePersonasQuery() {
  return useQuery<Persona[]>({
    queryKey: ["personas"],
    queryFn: async () => {
      const response = await fetch("/api/persona");
      if (!response.ok) throw new Error(await response.text());
      return response.json() as Promise<Persona[]>;
    },
  });
}
