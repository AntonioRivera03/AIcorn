import { FileText } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  Item,
  ItemActions,
  ItemContent,
  ItemDescription,
  ItemTitle,
} from "@/components/ui/item";
import { ModelSelect } from "@/features/ai/settings/model-select";
import { usePersonaMutations } from "@/features/persona/queries/use-persona-mutations";
import type { Persona, PersonaHarness } from "@/types/types";

type AgentRowProps = {
  agent: Persona;
  harness: PersonaHarness;
  defaultModelName: string;
  onShowInstructions: (agentId: number) => void;
};

export function AgentRow({
  agent,
  harness,
  defaultModelName,
  onShowInstructions,
}: AgentRowProps) {
  const { updatePersona } = usePersonaMutations(agent.ID);

  const changeModel = (Model: string) => {
    if (Model === agent.Model) return;
    updatePersona.mutate(
      { ...agent, Model },
      {
        onError: (err: Error) =>
          toast.error(err.message || `Could not change ${agent.Name}'s model.`),
      },
    );
  };

  return (
    <Item variant="outline" size="sm" role="listitem">
      <ItemContent className="min-w-48">
        <ItemTitle>{agent.Name}</ItemTitle>
        <ItemDescription>{agent.Description}</ItemDescription>
      </ItemContent>
      <ItemActions className="w-full sm:w-auto">
        <ModelSelect
          ariaLabel={`${agent.Name} model`}
          className="w-full sm:w-56"
          harness={harness}
          value={agent.Model}
          onChange={changeModel}
          disabled={updatePersona.isPending}
          defaultLabel={`Default · ${defaultModelName}`}
        />
        <Button
          variant="ghost"
          size="icon"
          aria-label={`${agent.Name} instructions`}
          title="Instructions"
          onClick={() => onShowInstructions(agent.ID)}
        >
          <FileText />
        </Button>
      </ItemActions>
    </Item>
  );
}
