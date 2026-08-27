import { useQuery } from "@tanstack/react-query";
import { normalizePersona } from "@/features/persona/queries/persona-query-normalization";
import type { PersonaResponse } from "@/features/persona/queries/persona-query-normalization";
import type { Persona } from "@/types/types";

export function usePersonasQuery() {
  return useQuery<Persona[]>({
    queryKey: ["personas"],
    queryFn: async () => {
      const response = await fetch("/api/persona");
      if (!response.ok) throw new Error(await response.text());
      const raw = (await response.json()) as PersonaResponse[];
      return raw.map(normalizePersona);
    },
  });
}
