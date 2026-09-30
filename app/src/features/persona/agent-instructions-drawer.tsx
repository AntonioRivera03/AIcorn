import { LockKeyhole } from "lucide-react";
import {
  Drawer,
  DrawerContent,
  DrawerTitle,
  DrawerDescription,
} from "@/components/ui/drawer";
import { Button } from "@/components/ui/button";
import { AIMarkdown } from "@/features/ai/ai-markdown";
import { usePersonaQuery } from "@/features/persona/queries/use-persona-query";
import { useIsMobile } from "@/hooks/useMobile";

type Props = {
  agentId: number | null;
  onClose: () => void;
};

// Read-only view of an agent's bundled instructions and skills. The model is
// chosen in the agent's row in AI settings.
export function AgentInstructionsDrawer({ agentId, onClose }: Props) {
  const mobile = useIsMobile();
  return (
    <Drawer
      direction={mobile ? "bottom" : "right"}
      open={agentId !== null}
      onOpenChange={(open) => !open && onClose()}
      handleOnly={!mobile}
    >
      <DrawerContent className="md:min-w-3xl p-0 rounded-lg data-[vaul-drawer-direction=bottom]:h-[calc(100dvh-var(--header-height))] data-[vaul-drawer-direction=bottom]:max-h-dvh flex flex-col">
        <div className="flex h-12 items-center justify-between border-b px-4">
          <DrawerTitle>Agent instructions</DrawerTitle>
          <DrawerDescription className="sr-only">
            The agent's fixed instructions and skills.
          </DrawerDescription>
          <Button variant="ghost" size="sm" onClick={onClose}>
            Close
          </Button>
        </div>
        <div
          className="flex-1 min-h-0 overflow-y-auto p-6 space-y-6"
          data-vaul-no-drag
          onWheel={(e) => e.stopPropagation()}
        >
          {agentId !== null && <AgentInstructions agentId={agentId} />}
        </div>
      </DrawerContent>
    </Drawer>
  );
}

function AgentInstructions({ agentId }: { agentId: number }) {
  const { data: agent, isPending, error, refetch } = usePersonaQuery(agentId);

  if (error)
    return (
      <div role="alert" className="text-sm text-destructive">
        Could not load agent.{" "}
        <Button variant="outline" onClick={() => void refetch()}>
          Retry
        </Button>
      </div>
    );
  if (isPending || !agent)
    return <p className="text-sm text-muted-foreground">Loading agent…</p>;

  return (
    <>
      <div className="space-y-2">
        <h2 className="text-2xl font-semibold">{agent.Name}</h2>
        <p className="text-sm text-muted-foreground">{agent.Description}</p>
        <p className="flex items-center gap-2 text-xs text-muted-foreground">
          <LockKeyhole className="size-3.5" />
          Built into Aycorn · instructions and skills are read-only
        </p>
      </div>
      <section aria-label="Agent instructions">
        {agent.InstructionPath && (
          <p className="break-all font-mono text-xs text-muted-foreground">
            {agent.InstructionPath}
          </p>
        )}
        <AIMarkdown>{agent.Instructions || "No instructions."}</AIMarkdown>
      </section>
      {agent.Skills?.map((skill) => (
        <details key={skill.Name} className="rounded-lg border border-border p-4">
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
  );
}
