import { useMemo, useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import { Plus, Search } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from "@/components/ui/input-group";
import { PersonaCard } from "@/features/persona/persona-card";
import { PersonasBulkActionsToolbar } from "@/features/persona/personas-bulk-actions-toolbar";
import { usePersonaMutations } from "@/features/persona/queries/use-persona-mutations";
import { usePersonasQuery } from "@/features/persona/queries/use-personas-query";
import { useAllWorkflowsQuery } from "@/features/workflows/shared/queries/useAllWorkflowsQuery";

export function PersonasPage() {
  const [search, setSearch] = useState("");
  const navigate = useNavigate();
  const { data: personas = [], isFetching } = usePersonasQuery();
  const { data: workflows = [] } = useAllWorkflowsQuery();
  const { createPersona } = usePersonaMutations();

  const stageCounts = useMemo(() => {
    const counts = new Map<number, number>();
    for (const workflow of workflows) {
      for (const stage of workflow.Stages) {
        if (!stage.Persona) continue;
        counts.set(stage.Persona.ID, (counts.get(stage.Persona.ID) ?? 0) + 1);
      }
    }
    return counts;
  }, [workflows]);

  const filtered = useMemo(() => {
    const query = search.trim().toLowerCase();
    if (query === "") return personas;
    return personas.filter((persona) =>
      [persona.Name, persona.Harness, persona.Model].some((value) =>
        value.toLowerCase().includes(query),
      ),
    );
  }, [personas, search]);

  const handleCreate = () => {
    createPersona.mutate(undefined, {
      onSuccess: (persona) => {
        navigate({
          to: "/personas/$personaId",
          params: { personaId: String(persona.ID) },
          search: { new: true },
        });
      },
      onError: () => toast.error("Failed to create persona."),
    });
  };

  const isLoading = isFetching && personas.length === 0;

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-4">
      <div className="flex flex-col items-center gap-2 md:flex-row">
        <InputGroup>
          <InputGroupInput
            aria-label="Search personas"
            placeholder="Search personas..."
            value={search}
            onChange={(event) => setSearch(event.target.value)}
          />
          <InputGroupAddon><Search /></InputGroupAddon>
          <InputGroupAddon align="inline-end">
            {filtered.length} {filtered.length === 1 ? "persona" : "personas"}
          </InputGroupAddon>
        </InputGroup>
        <Button className="w-full md:w-auto" onClick={handleCreate} disabled={createPersona.isPending}>
          <Plus />
          New Persona
        </Button>
      </div>

      {isLoading ? (
        <div className="flex items-center justify-center rounded-lg border border-dashed py-12 text-sm text-muted-foreground">
          Loading personas...
        </div>
      ) : filtered.length === 0 ? (
        <div className="flex flex-col items-center justify-center gap-3 rounded-lg border border-dashed py-12 text-center">
          <p className="text-sm text-muted-foreground">
            {search ? "No personas match your search." : "No personas yet."}
          </p>
          {!search && (
            <Button variant="outline" onClick={handleCreate} disabled={createPersona.isPending}>
              <Plus />
              Create your first persona
            </Button>
          )}
        </div>
      ) : (
        <div className="grid content-start grid-cols-1 gap-3 p-1 md:grid-cols-2 lg:grid-cols-3">
          {filtered.map((persona) => (
            <PersonaCard
              key={persona.ID}
              persona={persona}
              boundStageCount={stageCounts.get(persona.ID) ?? 0}
            />
          ))}
        </div>
      )}

      <PersonasBulkActionsToolbar personas={personas} stageCounts={stageCounts} />
    </div>
  );
}
