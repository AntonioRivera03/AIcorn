import { useNavigate } from "@tanstack/react-router";
import { Bot, Cable, Wrench } from "lucide-react";
import type { KeyboardEvent } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { selectedItemClasses, useSharedSelection } from "@/hooks/useSelection";
import { cn } from "@/lib/utils";
import type { Persona } from "@/types/types";

type PersonaCardProps = {
  persona: Persona;
  boundStageCount: number;
};

export function PersonaCard({ persona, boundStageCount }: PersonaCardProps) {
  const navigate = useNavigate();
  const { getItemProps } = useSharedSelection();
  const itemProps = getItemProps(`persona-${persona.ID}`);
  const itemOnClick = itemProps.onClick;

  const openPersona = () => {
    navigate({
      to: "/personas/$personaId",
      params: { personaId: String(persona.ID) },
    });
  };

  const handleKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key !== "Enter") return;
    event.preventDefault();
    openPersona();
  };

  return (
    <Card
      {...itemProps}
      data-task-card=""
      role="link"
      tabIndex={0}
      aria-label={`Open ${persona.Name || "Untitled Persona"}`}
      className={cn(
        "cursor-pointer gap-4 rounded-lg py-4 shadow-none transition-colors hover:bg-accent/30 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
        selectedItemClasses(),
        itemProps.className,
      )}
      onClick={(event) => {
        itemOnClick(event);
        if (!event.defaultPrevented) openPersona();
      }}
      onKeyDown={handleKeyDown}
    >
      <CardHeader className="gap-1 px-4">
        <CardTitle className="min-w-0 text-base font-medium">
          <Tooltip>
            <TooltipTrigger asChild>
              <span className={cn("block truncate", persona.Name === "" && "text-muted-foreground")}>
                {persona.Name || "Untitled Persona"}
              </span>
            </TooltipTrigger>
            <TooltipContent>{persona.Name || "Untitled Persona"}</TooltipContent>
          </Tooltip>
        </CardTitle>
        <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
          <Bot className="size-3.5" />
          <span>{persona.Harness}</span>
          <span aria-hidden="true">·</span>
          <span className="capitalize">{persona.Model}</span>
        </p>
      </CardHeader>
      <CardContent className="flex flex-wrap gap-2 px-4 text-xs text-muted-foreground">
        <span className="inline-flex items-center gap-1 rounded-full border px-2 py-1">
          <Wrench className="size-3" />
          {persona.AllowedTools.length} {persona.AllowedTools.length === 1 ? "tool" : "tools"}
        </span>
        <span className="inline-flex items-center gap-1 rounded-full border px-2 py-1">
          <Cable className="size-3" />
          {boundStageCount} {boundStageCount === 1 ? "stage" : "stages"}
        </span>
      </CardContent>
    </Card>
  );
}
