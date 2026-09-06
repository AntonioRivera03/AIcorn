import { useMemo, useState } from "react";
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
import { PersonaEditorDrawer } from "@/features/persona/persona-editor-drawer";
import { usePersonaMutations } from "@/features/persona/queries/use-persona-mutations";
import { usePersonasQuery } from "@/features/persona/queries/use-personas-query";

export function PersonasPage() {
  const [search, setSearch] = useState("");
  const [drawerPersonaId, setDrawerPersonaId] = useState<number | null>(null);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const { data: personas = [], isFetching } = usePersonasQuery();
  const { createPersona } = usePersonaMutations();

  const filtered = useMemo(() => {
    const query = search.trim().toLowerCase();
    if (query === "") return personas;
    return personas.filter((persona) =>
      [persona.Name].some((value) => value.toLowerCase().includes(query)),
    );
  }, [personas, search]);

  const handleCreate = () => {
    createPersona.mutate(undefined, {
      onSuccess: (persona) => {
        setDrawerPersonaId(persona.ID);
        setDrawerOpen(true);
      },
      onError: () => toast.error("Failed to create persona."),
    });
  };

  const handleOpenPersona = (personaId: number) => {
    setDrawerPersonaId(personaId);
    setDrawerOpen(true);
  };

  const isLoading = isFetching && personas.length === 0;

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-4">
      <div className="flex flex-col items-center gap-2 md:flex-row">
        <InputGroup>
          <InputGroupInput
            aria-label="Search presets"
            placeholder="Search presets..."
            value={search}
            onChange={(event) => setSearch(event.target.value)}
          />
          <InputGroupAddon>
            <Search />
          </InputGroupAddon>
          <InputGroupAddon align="inline-end">
            {filtered.length} {filtered.length === 1 ? "preset" : "presets"}
          </InputGroupAddon>
        </InputGroup>
        <Button
          className="w-full md:w-auto"
          onClick={handleCreate}
          disabled={createPersona.isPending}
        >
          <Plus />
          New preset
        </Button>
      </div>

      {isLoading ? (
        <div className="flex items-center justify-center rounded-lg border border-dashed py-12 text-sm text-muted-foreground">
          Loading presets...
        </div>
      ) : filtered.length === 0 ? (
        <div className="flex flex-col items-center justify-center gap-3 rounded-lg border border-dashed py-12 text-center">
          <p className="text-sm text-muted-foreground">
            {search ? "No presets match your search." : "No presets yet."}
          </p>
          {!search && (
            <Button
              variant="outline"
              onClick={handleCreate}
              disabled={createPersona.isPending}
            >
              <Plus />
              Create your first preset
            </Button>
          )}
        </div>
      ) : (
        <div className="grid content-start grid-cols-1 gap-3 p-1 md:grid-cols-2 lg:grid-cols-3">
          {filtered.map((persona) => (
            <PersonaCard
              key={persona.ID}
              persona={persona}
              onOpen={handleOpenPersona}
            />
          ))}
        </div>
      )}

      <PersonasBulkActionsToolbar personas={personas} />

      <PersonaEditorDrawer
        personaId={drawerPersonaId}
        open={drawerOpen}
        onOpenChange={(open) => {
          setDrawerOpen(open);
          if (!open) setDrawerPersonaId(null);
        }}
      />
    </div>
  );
}
