import { FileText } from "lucide-react";
import type { KeyboardEvent } from "react";
import { Card, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import type { Persona } from "@/types/types";

type PersonaCardProps = {
  persona: Persona;
  onOpen: (personaId: number) => void;
};

export function PersonaCard({ persona, onOpen }: PersonaCardProps) {
  const openPersona = () => {
    onOpen(persona.ID);
  };

  const handleKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key !== "Enter" && event.key !== " ") return;
    event.preventDefault();
    openPersona();
  };

  return (
    <Card
      data-task-card=""
      role="button"
      tabIndex={0}
      aria-label={`Open ${persona.Name || "Untitled agent"}`}
      className={cn(
        "cursor-pointer gap-4 rounded-lg py-4 shadow-none transition-colors hover:bg-accent/30 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
      )}
      onClick={openPersona}
      onKeyDown={handleKeyDown}
    >
      <CardHeader className="gap-1 px-4">
        <CardTitle className="min-w-0 text-base font-medium">
          <Tooltip>
            <TooltipTrigger asChild>
              <span
                className={cn(
                  "block truncate",
                  persona.Name === "" && "text-muted-foreground",
                )}
              >
                {persona.Name || "Untitled agent"}
              </span>
            </TooltipTrigger>
            <TooltipContent>{persona.Name || "Untitled agent"}</TooltipContent>
          </Tooltip>
        </CardTitle>
        <p className="text-xs text-muted-foreground">
          {persona.Description ||
            "Legacy agent · saved instructions are read-only"}
        </p>
        <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
          <FileText className="size-3.5" /> Codex · {persona.Model}
        </p>
      </CardHeader>
    </Card>
  );
}
