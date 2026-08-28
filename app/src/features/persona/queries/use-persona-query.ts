import { useQuery } from "@tanstack/react-query";
import { normalizePersona } from "@/features/persona/queries/persona-query-normalization";
import type { PersonaResponse } from "@/features/persona/queries/persona-query-normalization";
import type { Persona } from "@/types/types";

export function usePersonaQuery(personaId: number) {
  return useQuery<Persona>({
    queryKey: ["persona", personaId],
    queryFn: async () => {
      const response = await fetch(`/api/persona/${personaId}`);
      if (!response.ok) throw new Error(await response.text());
      const raw = (await response.json()) as PersonaResponse;
      return normalizePersona(raw);
    },
    enabled: personaId > 0,
  });
}
