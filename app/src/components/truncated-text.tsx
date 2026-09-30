import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";

type Props = {
  text: string;
  className?: string;
  side?: "top" | "right" | "bottom" | "left";
};

// A single line that truncates, with the full text in a tooltip.
export function TruncatedText({ text, className, side }: Props) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className={cn("truncate", className)}>{text}</span>
      </TooltipTrigger>
      {text !== "" && <TooltipContent side={side}>{text}</TooltipContent>}
    </Tooltip>
  );
}
