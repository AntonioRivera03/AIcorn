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
import { EditableHeader } from "@/components/EditableHeader";
import { RichEditor } from "@/features/editor/rich-editor";
import { DeletePersonaDialog } from "@/features/persona/delete-persona-dialog";
import { usePersonaMutations } from "@/features/persona/queries/use-persona-mutations";
import { usePersonaQuery } from "@/features/persona/queries/use-persona-query";
import type { Persona } from "@/types/types";
import { useIsMobile } from "@/hooks/useMobile";
import { Skeleton } from "@/components/ui/skeleton";

type PersonaEditorDrawerProps = {
  personaId: number | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

export function PersonaEditorDrawer(props: PersonaEditorDrawerProps) {
  return props.open && props.personaId !== null ? (
    <PersonaEditorSession key={props.personaId} {...props} />
  ) : null;
}

function PersonaEditorSession({
  personaId,
  open,
  onOpenChange,
}: PersonaEditorDrawerProps) {
  const isMobile = useIsMobile();
  const { data: persona, isPending, error } = usePersonaQuery(personaId ?? 0);
  const { updatePersona } = usePersonaMutations(personaId ?? undefined);

  const [changes, setDraft] = useState<Persona | null>(null);
  const draft = changes ?? persona ?? null;
  const editorRef = useRef<PlateEditor | null>(null);
  const [editorReady, setEditorReady] = useState(false);
  const savedPromptRef = useRef<Value | null>(null);
  const titleRef = useRef<HTMLHeadingElement>(null);
  const hasAutoFocused = useRef(false);
  const [deleteOpen, setDeleteOpen] = useState(false);

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
        toast.error("Failed to update preset.");
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
          <span className="text-sm font-medium">Instruction preset</span>
          <div className="flex items-center gap-1">
            {draft && (
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label="Preset actions"
                  >
                    <MoreHorizontal />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  <DropdownMenuItem
                    variant="destructive"
                    onClick={() => setDeleteOpen(true)}
                  >
                    <Trash2 />
                    Delete preset
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            )}
            <Button
              variant="ghost"
              size="sm"
              onClick={() => handleClose(false)}
            >
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
                <div className="text-sm text-muted-foreground">
                  Preset could not be loaded.
                </div>
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
                placeholder="Untitled preset"
                className="min-w-0 flex-1"
              />

              <div className="flex min-h-0 flex-1 flex-col gap-2">
                <Label>Instructions</Label>
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
                {!editorReady && open && (
                  <p className="text-xs text-muted-foreground">
                    Loading editor…
                  </p>
                )}
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
              onOpenChange(false);
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
