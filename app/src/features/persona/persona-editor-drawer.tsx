import { useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import type { Value } from "platejs";
import type { PlateEditor } from "platejs/react";
import { MoreHorizontal, Trash2 } from "lucide-react";
import { Drawer, DrawerContent } from "@/components/ui/drawer";
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
import { EditableHeader } from "@/components/EditableHeader";
import { RichEditor } from "@/features/editor/rich-editor";
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
import { useIsMobile } from "@/hooks/useMobile";
import { Skeleton } from "@/components/ui/skeleton";

type PersonaEditorDrawerProps = {
  personaId: number | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

export function PersonaEditorDrawer({ personaId, open, onOpenChange }: PersonaEditorDrawerProps) {
  const isMobile = useIsMobile();
  const { data: persona, isPending, error } = usePersonaQuery(personaId ?? 0);
  const { data: tools = [], isPending: toolsLoading } = useMcpToolsQuery();
  const { updatePersona } = usePersonaMutations(personaId ?? undefined);

  const [draft, setDraft] = useState<Persona | null>(null);
  const editorRef = useRef<PlateEditor | null>(null);
  const [editorReady, setEditorReady] = useState(false);
  const savedPromptRef = useRef<Value | null>(null);
  const titleRef = useRef<HTMLHeadingElement>(null);
  const hasAutoFocused = useRef(false);
  const [deleteOpen, setDeleteOpen] = useState(false);

  useEffect(() => {
    if (persona) setDraft(persona);
  }, [persona]);

  useEffect(() => {
    if (!open) {
      hasAutoFocused.current = false;
      setEditorReady(false);
      editorRef.current = null;
    }
  }, [open]);

  useEffect(() => {
    if (!open || !draft || hasAutoFocused.current || !titleRef.current) return;
    // Autofocus title on first open for newly created persona (name is empty)
    if (draft.Name === "") {
      hasAutoFocused.current = true;
      titleRef.current.focus();
    }
  }, [open, draft]);

  if (personaId === null) return null;

  const save = (next: Persona, successMessage?: string) => {
    setDraft(next);
    updatePersona.mutate(next, {
      onSuccess: () => {
        if (successMessage) toast.success(successMessage);
      },
      onError: () => {
        if (persona) setDraft(persona);
        toast.error("Failed to update persona.");
      },
    });
  };

  const saveName = (Name: string) => {
    if (!draft || Name === draft.Name) return;
    save({ ...draft, Name });
  };

  const commitPendingPrompt = () => {
    const editor = editorRef.current;
    if (!editor || !draft || personaId === 0) return;
    const body = editor.children as Value;
    if (JSON.stringify(body) !== JSON.stringify(savedPromptRef.current)) {
      save({ ...draft, SystemPrompt: body }, "System prompt updated.");
    }
  };

  const handlePromptChange = (value: Value) => {
    if (!draft) return;
    setDraft({ ...draft, SystemPrompt: value });
    savedPromptRef.current = value;
    // Debounced persist - mirrors task body 250ms behavior via RichEditor + our 250ms
    // RichEditor already debounces, we just forward to API
    if (draft.ID !== 0) {
      // use draft.ID from closure but ensure we send latest SystemPrompt
      // We delay via updatePersona directly; the RichEditor debounce already handled timing
      // So immediate mutate is fine – the debounced value is what we receive here
      updatePersona.mutate(
        { ...draft, SystemPrompt: value },
        {
          onError: () => toast.error("Failed to update system prompt."),
        },
      );
    }
  };

  const saveHarness = (Harness: PersonaHarness) => {
    if (!draft) return;
    save({ ...draft, Harness });
  };
  const saveModel = (Model: PersonaModel) => {
    if (!draft) return;
    save({ ...draft, Model });
  };
  const saveTools = (AllowedTools: string[]) => {
    if (!draft) return;
    save({ ...draft, AllowedTools });
  };

  const handleClose = (nextOpen: boolean) => {
    if (!nextOpen) commitPendingPrompt();
    onOpenChange(nextOpen);
    if (!nextOpen) {
      editorRef.current = null;
      setEditorReady(false);
    }
  };

  return (
    <Drawer
      direction={isMobile ? "bottom" : "right"}
      open={open}
      onOpenChange={handleClose}
      repositionInputs={!isMobile}
      handleOnly={!isMobile}
    >
      <DrawerContent className="md:min-w-3xl p-0 overflow-x-visible box-border rounded-lg data-[vaul-drawer-direction=bottom]:h-[calc(100dvh-var(--header-height))] data-[vaul-drawer-direction=bottom]:max-h-dvh flex flex-col">
        <div className="flex h-12 items-center justify-between border-b px-4">
          <span className="text-sm font-medium">Edit Persona</span>
          <div className="flex items-center gap-1">
            {draft && (
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
            )}
            <Button variant="ghost" size="sm" onClick={() => handleClose(false)}>
              Close
            </Button>
          </div>
        </div>

        <div
          className="flex-1 min-h-0 overflow-y-auto"
          data-vaul-no-drag
          onWheel={(e) => e.stopPropagation()}
        >
          {isPending || !draft ? (
            <div className="flex flex-col gap-4 p-6">
              {error ? (
                <div className="text-sm text-muted-foreground">Persona could not be loaded.</div>
              ) : (
                <>
                  <Skeleton className="h-8 w-3/4" />
                  <Skeleton className="h-10 w-full" />
                  <Skeleton className="h-32 w-full" />
                </>
              )}
            </div>
          ) : (
            <div className="flex flex-col gap-6 p-6">
              <EditableHeader
                ref={titleRef}
                value={draft.Name}
                setValue={saveName}
                placeholder="Untitled Persona"
                className="min-w-0 flex-1"
              />

              <div className="grid gap-4 sm:grid-cols-2">
                <div className="flex flex-col gap-2">
                  <Label htmlFor="persona-drawer-harness">Harness</Label>
                  <Select value={draft.Harness} onValueChange={saveHarness} disabled={updatePersona.isPending}>
                    <SelectTrigger id="persona-drawer-harness" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {PERSONA_HARNESSES.map((harness) => (
                        <SelectItem key={harness} value={harness}>
                          {harness}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                <div className="flex flex-col gap-2">
                  <Label htmlFor="persona-drawer-model">Model</Label>
                  <Select value={draft.Model} onValueChange={saveModel} disabled={updatePersona.isPending}>
                    <SelectTrigger id="persona-drawer-model" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {PERSONA_MODELS.map((model) => (
                        <SelectItem key={model} value={model} className="capitalize">
                          {model}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
              </div>

              <div className="flex flex-col gap-2">
                <Label htmlFor="persona-drawer-tools">Allowed tools</Label>
                <PersonaToolsSelect
                  tools={tools}
                  selected={draft.AllowedTools}
                  loading={toolsLoading}
                  disabled={updatePersona.isPending}
                  onChange={saveTools}
                />
                <p className="text-xs text-muted-foreground">An empty selection means this persona has no tool access.</p>
              </div>

              <div className="flex min-h-0 flex-1 flex-col gap-2">
                <Label>System prompt</Label>
                {open ? (
                  <RichEditor
                    key={`${draft.ID}-${open}`}
                    initialValue={draft.SystemPrompt}
                    debounceDuration={250}
                    onDebounceChange={handlePromptChange}
                    onEditorReady={(editor) => {
                      editorRef.current = editor;
                      savedPromptRef.current = editor.children as Value;
                      setEditorReady(true);
                    }}
                    className="min-h-72"
                  />
                ) : (
                  <Skeleton className="h-32 w-full" />
                )}
                {!editorReady && open && <p className="text-xs text-muted-foreground">Loading editor…</p>}
              </div>
            </div>
          )}
        </div>
        {draft && (
          <DeletePersonaDialog
            persona={draft}
            open={deleteOpen}
            onOpenChange={setDeleteOpen}
            onDeleted={() => {
              setDeleteOpen(false);
              handleClose(false);
            }}
          />
        )}
      </DrawerContent>
    </Drawer>
  );
}

export function NewPersonaDrawer({
  onCreated,
  children,
}: {
  onCreated: (persona: Persona) => void;
  children: React.ReactNode;
}) {
  const { createPersona } = usePersonaMutations();

  const handleCreate = () => {
    createPersona.mutate(undefined, {
      onSuccess: (persona) => onCreated(persona),
      onError: () => toast.error("Failed to create persona."),
    });
  };

  return (
    <span className="contents" onClick={handleCreate}>
      {children}
    </span>
  );
}
