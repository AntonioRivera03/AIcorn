import { useMemo, useState } from "react";
import { Search } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from "@/components/ui/input-group";
import { PersonaCard } from "@/features/persona/persona-card";
import { PersonaEditorDrawer } from "@/features/persona/persona-editor-drawer";
import { usePersonasQuery } from "@/features/persona/queries/use-personas-query";

export function PersonasPage() {
  const [search, setSearch] = useState("");
  const [drawerPersonaId, setDrawerPersonaId] = useState<number | null>(null);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const {
    data: personas = [],
    isFetching,
    isError,
    refetch,
  } = usePersonasQuery();

  const filtered = useMemo(() => {
    const query = search.trim().toLowerCase();
    const ordered = [...personas].sort(
      (a, b) =>
        Number(Boolean(b.BuiltinRole)) - Number(Boolean(a.BuiltinRole)) ||
        (a.BuiltinRole === "conductor"
          ? -1
          : b.BuiltinRole === "conductor"
            ? 1
            : a.ID - b.ID),
    );
    if (query === "") return ordered;
    return ordered.filter((persona) =>
      [persona.Name].some((value) => value.toLowerCase().includes(query)),
    );
  }, [personas, search]);

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
            aria-label="Search agents"
            placeholder="Search agents..."
            value={search}
            onChange={(event) => setSearch(event.target.value)}
          />
          <InputGroupAddon>
            <Search />
          </InputGroupAddon>
          <InputGroupAddon align="inline-end">
            {filtered.length} {filtered.length === 1 ? "agent" : "agents"}
          </InputGroupAddon>
        </InputGroup>
      </div>

      {isError ? (
        <div role="alert" className="text-sm text-destructive">
          Could not load agents.{" "}
          <Button variant="outline" onClick={() => void refetch()}>
            Retry
          </Button>
        </div>
      ) : isLoading ? (
        <div className="flex items-center justify-center rounded-lg border border-dashed py-12 text-sm text-muted-foreground">
          Loading agents...
        </div>
      ) : filtered.length === 0 ? (
        <div className="flex flex-col items-center justify-center gap-3 rounded-lg border border-dashed py-12 text-center">
          <p className="text-sm text-muted-foreground">
            {search ? "No agents match your search." : "No agents yet."}
          </p>
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
