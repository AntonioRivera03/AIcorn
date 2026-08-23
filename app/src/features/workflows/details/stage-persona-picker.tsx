import { useState } from "react";
import { Bot, Check, ChevronsUpDown, UserRoundX } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
} from "@/components/ui/command";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { usePersonasQuery } from "@/features/persona/queries/use-personas-query";
import { useStagePersonaMutations } from "@/features/workflows/shared/queries/useStagePersonaMutations";
import { cn } from "@/lib/utils";
import type { Persona, Stage } from "@/types/types";

export function StagePersonaPicker({
  stage,
  workflowId,
}: {
  readonly stage: Stage;
  readonly workflowId: number;
}) {
  const [open, setOpen] = useState(false);
  const personasQuery = usePersonasQuery();
  const { bindPersona, unbindPersona } =
    useStagePersonaMutations(workflowId);
  const personas = personasQuery.data ?? [];
  const isMutating = bindPersona.isPending || unbindPersona.isPending;

  const handleBind = (persona: Persona) => {
    if (persona.ID === stage.Persona?.ID) {
      setOpen(false);
      return;
    }
    bindPersona.mutate(
      { stageId: stage.ID, personaId: persona.ID },
      {
        onSuccess: () => setOpen(false),
        onError: () => toast.error("Failed to bind persona to stage."),
      },
    );
  };

  const handleClear = () => {
    unbindPersona.mutate(stage.ID, {
      onSuccess: () => setOpen(false),
      onError: () => toast.error("Failed to clear the stage persona."),
    });
  };

  const trigger = (
    <Button
      type="button"
      variant="ghost"
      size="sm"
      role="combobox"
      aria-expanded={open}
      aria-label={
        stage.Persona
          ? `Change persona. Currently ${stage.Persona.Name}`
          : "Assign a persona"
      }
      disabled={isMutating}
      className="h-8 max-w-44 justify-between gap-1.5 px-2 text-muted-foreground hover:text-foreground lg:max-w-48"
    >
      <Bot className="size-3.5 shrink-0" />
      <span className="truncate text-xs">
        {stage.Persona?.Name ?? "Assign persona"}
      </span>
      <ChevronsUpDown className="size-3 shrink-0 opacity-60" />
    </Button>
  );

  return (
    <div
      className="shrink-0"
      onClick={(event) => event.stopPropagation()}
      onKeyDown={(event) => event.stopPropagation()}
    >
      <Popover open={open} onOpenChange={setOpen}>
        {stage.Persona ? (
          <Tooltip>
            <TooltipTrigger asChild>
              <PopoverTrigger asChild>{trigger}</PopoverTrigger>
            </TooltipTrigger>
            <TooltipContent>{stage.Persona.Name}</TooltipContent>
          </Tooltip>
        ) : (
          <PopoverTrigger asChild>{trigger}</PopoverTrigger>
        )}

        <PopoverContent className="w-64 p-0" align="end">
          {personasQuery.isPending ? (
            <div className="px-3 py-4 text-sm text-muted-foreground">
              Loading personas...
            </div>
          ) : personas.length === 0 ? (
            <div className="space-y-2 px-3 py-4 text-sm">
              <div className="flex items-start gap-2 text-muted-foreground">
                <Bot className="mt-0.5 size-4 shrink-0" />
                <p>No personas exist yet. Create one before binding this stage.</p>
              </div>
              <a
                href="/personas"
                className="inline-flex font-medium text-primary underline-offset-4 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                Go to personas
              </a>
            </div>
          ) : (
            <Command>
              <CommandInput placeholder="Search personas..." />
              <CommandList>
                <CommandEmpty>No personas found.</CommandEmpty>
                <CommandGroup heading="Personas">
                  {personas.map((persona) => (
                    <CommandItem
                      key={persona.ID}
                      value={persona.Name}
                      onSelect={() => handleBind(persona)}
                      disabled={isMutating}
                    >
                      <Bot className="size-4" />
                      <span className="min-w-0 flex-1 break-words">
                        {persona.Name}
                      </span>
                      <Check
                        className={cn(
                          "size-4",
                          persona.ID === stage.Persona?.ID
                            ? "opacity-100"
                            : "opacity-0",
                        )}
                      />
                    </CommandItem>
                  ))}
                </CommandGroup>
                {stage.Persona && (
                  <>
                    <CommandSeparator />
                    <CommandGroup>
                      <CommandItem
                        onSelect={handleClear}
                        disabled={isMutating}
                      >
                        <UserRoundX className="size-4" />
                        Clear persona
                      </CommandItem>
                    </CommandGroup>
                  </>
                )}
              </CommandList>
            </Command>
          )}
        </PopoverContent>
      </Popover>
    </div>
  );
}
