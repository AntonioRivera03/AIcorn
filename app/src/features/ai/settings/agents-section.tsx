import { useState } from "react";
import { Button } from "@/components/ui/button";
import { ItemGroup } from "@/components/ui/item";
import { AgentRow } from "@/features/ai/settings/agent-row";
import { useHarnessModels } from "@/features/ai/queries/use-ai";
import type { AISettings } from "@/features/ai/types";
import { AgentInstructionsDrawer } from "@/features/persona/agent-instructions-drawer";
import { usePersonasQuery } from "@/features/persona/queries/use-personas-query";
import { SectionHeading } from "@/features/settings/section-heading";

export function AgentsSection({ settings }: { settings: AISettings }) {
  const personas = usePersonasQuery();
  const models = useHarnessModels(settings.harness);
  const [instructionsFor, setInstructionsFor] = useState<number | null>(null);
  const agents = (personas.data ?? []).filter((persona) => persona.TaskAgent);
  const defaultModelName =
    models.data?.models.find((model) => model.id === settings.model)?.name ??
    settings.model;

  return (
    <section className="flex flex-col gap-4">
      <SectionHeading
        title="Agents"
        description="Conductor hands tasks to these agents, and you can start any of them on a task yourself. Their instructions ship with Aycorn; only the model is yours to pick."
      />
      {personas.isError ? (
        <p role="alert" className="text-sm text-destructive">
          Could not load agents.{" "}
          <Button variant="link" onClick={() => void personas.refetch()}>
            Retry
          </Button>
        </p>
      ) : personas.isPending ? (
        <p className="text-sm text-muted-foreground">Loading agents…</p>
      ) : (
        <ItemGroup className="gap-2">
          {agents.map((agent) => (
            <AgentRow
              key={agent.ID}
              agent={agent}
              harness={settings.harness}
              defaultModelName={defaultModelName}
              onShowInstructions={setInstructionsFor}
            />
          ))}
        </ItemGroup>
      )}
      <AgentInstructionsDrawer
        agentId={instructionsFor}
        onClose={() => setInstructionsFor(null)}
      />
    </section>
  );
}
