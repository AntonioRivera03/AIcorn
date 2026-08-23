import { useEffect, useRef, useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import { MoreHorizontal, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { EditableHeader } from "@/components/EditableHeader";
import { RelativeTimeWithTooltip } from "@/components/relative-time-with-tooltip";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { DeletePersonaDialog } from "@/features/persona/delete-persona-dialog";
import { PersonaToolsSelect } from "@/features/persona/persona-tools-select";
import { useMcpToolsQuery } from "@/features/persona/queries/use-mcp-tools-query";
import { usePersonaMutations } from "@/features/persona/queries/use-persona-mutations";
import { usePersonaQuery } from "@/features/persona/queries/use-persona-query";
import {
  PERSONA_HARNESSES,
  PERSONA_MODELS,
  type Persona,
  type PersonaHarness,
  type PersonaModel,
} from "@/types/types";

type PersonaEditorPageProps = {
  personaId: number;
  autoFocusName: boolean;
};

export function PersonaEditorPage({ personaId, autoFocusName }: PersonaEditorPageProps) {
  const navigate = useNavigate();
  const { data: persona, isPending, error } = usePersonaQuery(personaId);
  const { data: tools = [], isPending: toolsLoading } = useMcpToolsQuery();
  const { updatePersona } = usePersonaMutations(personaId);
  const [draft, setDraft] = useState<Persona | null>(null);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const titleRef = useRef<HTMLHeadingElement>(null);
  const hasAutoFocused = useRef(false);

  useEffect(() => {
    if (persona) setDraft(persona);
  }, [persona]);

  useEffect(() => {
    if (!autoFocusName || !draft || hasAutoFocused.current || !titleRef.current) return;
    hasAutoFocused.current = true;
    titleRef.current.focus();
    navigate({
      to: "/personas/$personaId",
      params: { personaId: String(personaId) },
      search: {},
      replace: true,
    });
  }, [autoFocusName, draft, navigate, personaId]);

  if (isPending || !draft) {
    return (
      <div className="flex items-center justify-center rounded-lg border border-dashed py-12 text-sm text-muted-foreground">
        {error ? "Persona could not be loaded." : "Loading persona..."}
      </div>
    );
  }

  const save = (next: Persona, successMessage?: string) => {
    setDraft(next);
    updatePersona.mutate(next, {
      onSuccess: () => {
        if (successMessage) toast.success(successMessage);
      },
      onError: () => {
        setDraft(draft);
        toast.error("Failed to update persona.");
      },
    });
  };

  const saveName = (Name: string) => {
    if (Name !== draft.Name) save({ ...draft, Name });
  };

  const savePrompt = () => {
    if (persona && draft.SystemPrompt !== persona.SystemPrompt) save(draft, "System prompt updated.");
  };

  const saveHarness = (Harness: PersonaHarness) => save({ ...draft, Harness });
  const saveModel = (Model: PersonaModel) => save({ ...draft, Model });
  const saveTools = (AllowedTools: string[]) => save({ ...draft, AllowedTools });

  return (
    <div className="mx-auto flex w-full max-w-4xl flex-col gap-6">
      <div className="flex items-start gap-3">
        <EditableHeader
          ref={titleRef}
          value={draft.Name}
          setValue={saveName}
          placeholder="Untitled Persona"
          className="min-w-0 flex-1"
        />
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="icon-sm" aria-label="Persona actions">
              <MoreHorizontal />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem variant="destructive" onClick={() => setDeleteOpen(true)}>
              <Trash2 />
              Delete persona
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      <div className="grid gap-4 sm:grid-cols-2">
        <div className="flex flex-col gap-2">
          <Label htmlFor="persona-harness">Harness</Label>
          <Select value={draft.Harness} onValueChange={saveHarness} disabled={updatePersona.isPending}>
            <SelectTrigger id="persona-harness" className="w-full"><SelectValue /></SelectTrigger>
            <SelectContent>
              {PERSONA_HARNESSES.map((harness) => (
                <SelectItem key={harness} value={harness}>{harness}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="flex flex-col gap-2">
          <Label htmlFor="persona-model">Model</Label>
          <Select value={draft.Model} onValueChange={saveModel} disabled={updatePersona.isPending}>
            <SelectTrigger id="persona-model" className="w-full"><SelectValue /></SelectTrigger>
            <SelectContent>
              {PERSONA_MODELS.map((model) => (
                <SelectItem key={model} value={model} className="capitalize">{model}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>

      <div className="flex flex-col gap-2">
        <Label htmlFor="persona-tools">Allowed tools</Label>
        <PersonaToolsSelect
          tools={tools}
          selected={draft.AllowedTools}
          loading={toolsLoading}
          disabled={updatePersona.isPending}
          onChange={saveTools}
        />
        <p className="text-xs text-muted-foreground">
          An empty selection means this persona has no tool access.
        </p>
      </div>

      <div className="flex min-h-0 flex-1 flex-col gap-2">
        <Label htmlFor="persona-system-prompt">System prompt</Label>
        <Textarea
          id="persona-system-prompt"
          value={draft.SystemPrompt}
          onChange={(event) => setDraft({ ...draft, SystemPrompt: event.target.value })}
          onBlur={savePrompt}
          placeholder="Describe the persona's role, boundaries, and operating instructions..."
          className="min-h-72 resize-y font-mono text-sm leading-relaxed"
        />
        <p className="text-xs text-muted-foreground">Saved when this field loses focus.</p>
      </div>

      <div className="flex flex-wrap gap-x-4 gap-y-1 border-t pt-4">
        <RelativeTimeWithTooltip date={draft.TimeCreated} label="Created" className="text-xs" />
        <RelativeTimeWithTooltip date={draft.TimeModified} label="Modified" className="text-xs" />
      </div>

      <DeletePersonaDialog
        persona={draft}
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        onDeleted={() => navigate({ to: "/personas" })}
      />
    </div>
  );
}
