import { useState } from "react";
import { toast } from "sonner";
import { LockKeyhole } from "lucide-react";
import {
  Drawer,
  DrawerContent,
  DrawerTitle,
  DrawerDescription,
} from "@/components/ui/drawer";
import { Button } from "@/components/ui/button";
import { OpenAIModelInput } from "@/features/ai/openai-model-input";
import { AIMarkdown } from "@/features/ai/ai-markdown";
import { Label } from "@/components/ui/label";
import { usePersonaMutations } from "@/features/persona/queries/use-persona-mutations";
import { usePersonaQuery } from "@/features/persona/queries/use-persona-query";
import { useIsMobile } from "@/hooks/useMobile";

// Legacy rich-text prompts remain readable without mounting an editable editor.
function textOf(node: unknown): string {
  if (Array.isArray(node)) return node.map(textOf).join("\n");
  if (node && typeof node === "object") {
    if ("text" in node && typeof node.text === "string") return node.text;
    if ("children" in node && Array.isArray(node.children))
      return node.children.map(textOf).join("");
  }
  return "";
}

type Props = {
  personaId: number | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};
export function PersonaEditorDrawer(props: Props) {
  return props.open && props.personaId !== null ? (
    <AgentDetails
      key={props.personaId}
      {...props}
      personaId={props.personaId}
    />
  ) : null;
}

function AgentDetails({
  personaId,
  open,
  onOpenChange,
}: Props & { personaId: number }) {
  const mobile = useIsMobile();
  const {
    data: persona,
    isPending,
    error,
    refetch,
  } = usePersonaQuery(personaId);
  const { updatePersona } = usePersonaMutations(personaId);
  const [model, setModel] = useState<string | null>(null);
  const saveModel = () => {
    if (!persona || updatePersona.isPending) return;
    const Model = (model ?? persona.Model).trim();
    if (Model === persona.Model) {
      setModel(null);
      return;
    }
    updatePersona.mutate(
      { ...persona, Model },
      {
        onSuccess: () => setModel(null),
        onError: (err: Error) =>
          toast.error(
            err.message ||
              "Could not save model. Your value is still here to retry.",
          ),
      },
    );
  };
  return (
    <Drawer
      direction={mobile ? "bottom" : "right"}
      open={open}
      onOpenChange={onOpenChange}
      repositionInputs={!mobile}
      handleOnly={!mobile}
    >
      <DrawerContent className="md:min-w-3xl p-0 rounded-lg data-[vaul-drawer-direction=bottom]:h-[calc(100dvh-var(--header-height))] data-[vaul-drawer-direction=bottom]:max-h-dvh flex flex-col">
        <div className="flex h-12 items-center justify-between border-b px-4">
          <DrawerTitle>Agent details</DrawerTitle>
          <DrawerDescription className="sr-only">
            Change the model and read the agent’s fixed instructions and skills.
          </DrawerDescription>
          <Button variant="ghost" size="sm" onClick={() => onOpenChange(false)}>
            Close
          </Button>
        </div>
        <div
          className="flex-1 min-h-0 overflow-y-auto p-6 space-y-6"
          data-vaul-no-drag
          onWheel={(e) => e.stopPropagation()}
        >
          {error ? (
            <div role="alert" className="text-sm text-destructive">
              Could not load agent.{" "}
              <Button variant="outline" onClick={() => void refetch()}>
                Retry
              </Button>
            </div>
          ) : isPending || !persona ? (
            <p className="text-sm text-muted-foreground">Loading agent…</p>
          ) : (
            <>
              <div className="space-y-2">
                <h2 className="text-2xl font-semibold">
                  {persona.Name || "Untitled agent"}
                </h2>
                <p className="text-sm text-muted-foreground">
                  {persona.Description ||
                    "Legacy agent with preserved instructions."}
                </p>
                <p className="flex items-center gap-2 text-xs text-muted-foreground">
                  <LockKeyhole className="size-3.5" />
                  {persona.BuiltinRole
                    ? "Built-in role · instructions and skills are read-only"
                    : "Saved role · instructions are read-only"}
                </p>
              </div>
              <div className="space-y-2">
                <Label htmlFor="agent-model">OpenAI model</Label>
                <OpenAIModelInput
                  id="agent-model"
                  value={model ?? persona.Model}
                  disabled={updatePersona.isPending}
                  onChange={(e) => setModel(e.target.value)}
                  onBlur={saveModel}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" && !e.nativeEvent.isComposing)
                      e.currentTarget.blur();
                  }}
                />
                <p role="status" className="text-xs text-muted-foreground">
                  {updatePersona.isPending
                    ? "Saving model…"
                    : updatePersona.isError
                      ? "Model was not saved. Check the value and leave the field to retry."
                      : "Changes save automatically and apply to new runs. Active work keeps its model snapshot."}
                </p>
              </div>
              <section aria-label="Read-only agent instructions">
                <h3 className="font-medium">Instructions</h3>
                {persona.InstructionPath && (
                  <p className="mt-1 break-all font-mono text-xs text-muted-foreground">
                    {persona.InstructionPath}
                  </p>
                )}
                <AIMarkdown>
                  {persona.Instructions ||
                    textOf(persona.SystemPrompt) ||
                    "No saved instructions."}
                </AIMarkdown>
              </section>
              {persona.Skills?.map((skill) => (
                <details
                  key={skill.Name}
                  className="rounded-lg border border-border p-4"
                >
                  <summary className="cursor-pointer font-medium">
                    {skill.Name} · read-only skill
                  </summary>
                  <p className="mt-3 break-all font-mono text-xs text-muted-foreground">
                    {skill.Path}
                  </p>
                  <AIMarkdown>{skill.Content}</AIMarkdown>
                </details>
              ))}
            </>
          )}
        </div>
      </DrawerContent>
    </Drawer>
  );
}
