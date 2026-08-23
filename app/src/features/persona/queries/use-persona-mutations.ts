import { useMutation, useQueryClient } from "@tanstack/react-query";
import type { BulkResult, Persona } from "@/types/types";

export function usePersonaMutations(personaId?: number) {
  const queryClient = useQueryClient();

  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: ["personas"] });
    queryClient.invalidateQueries({ queryKey: ["allWorkflows"] });
    if (personaId !== undefined) {
      queryClient.invalidateQueries({ queryKey: ["persona", personaId] });
    }
  };

  const createPersona = useMutation({
    mutationFn: async () => {
      const response = await fetch("/api/persona", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({}),
      });
      if (!response.ok) throw new Error(await response.text());
      return response.json() as Promise<Persona>;
    },
    onSuccess: invalidate,
  });

  const updatePersona = useMutation({
    mutationFn: async (persona: Persona) => {
      const response = await fetch(`/api/persona/${persona.ID}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(persona),
      });
      if (!response.ok) throw new Error(await response.text());
      return response.json() as Promise<boolean>;
    },
    onSuccess: invalidate,
  });

  const deletePersona = useMutation({
    mutationFn: async (id: number) => {
      const response = await fetch(`/api/persona/${id}`, { method: "DELETE" });
      if (!response.ok) throw new Error(await response.text());
      return response.json() as Promise<boolean>;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["personas"] });
      queryClient.invalidateQueries({ queryKey: ["allWorkflows"] });
      if (personaId !== undefined) {
        queryClient.removeQueries({ queryKey: ["persona", personaId] });
      }
    },
  });

  const bulkDeletePersonas = useMutation({
    mutationFn: async (ids: number[]) => {
      const response = await fetch("/api/persona/bulk/delete", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(ids),
      });
      if (!response.ok) throw new Error(await response.text());
      return response.json() as Promise<BulkResult>;
    },
    onSuccess: invalidate,
  });

  return { createPersona, updatePersona, deletePersona, bulkDeletePersonas };
}
