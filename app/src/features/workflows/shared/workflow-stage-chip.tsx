import type { Stage } from "@/types/types";
import { StageIcon, stageTintClass } from "@/features/stage/stage-visual";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import { Bot } from "lucide-react";

export function WorkflowStageChip({
  stage,
  className = "",
}: {
  stage: Stage;
  className?: string;
}) {
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1.5 rounded-md px-2 py-0.5 text-xs",
        stageTintClass(stage.Color),
        className,
      )}
    >
      <StageIcon stage={stage} className="size-3.5" />
      {stage.Name}
      {stage.Persona && (
        <Tooltip>
          <TooltipTrigger asChild>
            <span
              tabIndex={0}
              aria-label={`Persona: ${stage.Persona.Name}`}
              className="inline-flex rounded-sm text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
            >
              <Bot aria-hidden="true" className="size-3" />
            </span>
          </TooltipTrigger>
          <TooltipContent>Persona: {stage.Persona.Name}</TooltipContent>
        </Tooltip>
      )}
    </span>
  );
}
