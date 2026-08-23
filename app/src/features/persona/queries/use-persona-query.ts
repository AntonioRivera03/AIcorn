import { useQuery } from "@tanstack/react-query";
import type { Persona } from "@/types/types";

export function usePersonaQuery(personaId: number) {
  return useQuery<Persona>({
    queryKey: ["persona", personaId],
    queryFn: async () => {
      const response = await fetch(`/api/persona/${personaId}`);
      if (!response.ok) throw new Error(await response.text());
      return response.json() as Promise<Persona>;
    },
    enabled: personaId > 0,
  });
}
