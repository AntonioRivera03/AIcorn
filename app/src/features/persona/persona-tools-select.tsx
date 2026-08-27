import { useState } from "react";
import { Check, ChevronsUpDown, Wrench } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { cn } from "@/lib/utils";
import type { McpTool } from "@/features/persona/queries/use-mcp-tools-query";

type PersonaToolsSelectProps = {
  tools: McpTool[];
  selected: string[];
  loading: boolean;
  disabled: boolean;
  onChange: (tools: string[]) => void;
};

export function PersonaToolsSelect({
  tools,
  selected,
  loading,
  disabled,
  onChange,
}: PersonaToolsSelectProps) {
  const [open, setOpen] = useState(false);

  const toggleTool = (name: string) => {
    const next = selected.includes(name)
      ? selected.filter((tool) => tool !== name)
      : [...selected, name];
    onChange(next);
  };

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          id="persona-tools"
          variant="outline"
          role="combobox"
          aria-expanded={open}
          disabled={disabled || loading}
          className="h-auto min-h-9 w-full justify-between whitespace-normal text-left font-normal"
        >
          <span className="flex items-center gap-2">
            <Wrench className="size-4 text-muted-foreground" />
            {loading
              ? "Loading tools..."
              : selected.length === 0
                ? "No tools allowed"
                : `${selected.length} tool${selected.length === 1 ? "" : "s"} allowed`}
          </span>
          <ChevronsUpDown className="size-4 shrink-0 text-muted-foreground" />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        align="start"
        className="w-(--radix-popover-trigger-width) p-0"
        data-vaul-no-drag
        onWheel={(e) => e.stopPropagation()}
        onPointerMove={(e) => e.stopPropagation()}
      >
        <Command>
          <CommandInput placeholder="Search tools..." />
          <CommandList
            data-vaul-no-drag
            onWheel={(e) => e.stopPropagation()}
            className="max-h-[min(320px,50vh)]"
          >
            <CommandEmpty>No tools found.</CommandEmpty>
            <CommandGroup>
              {tools.map((tool) => {
                const isSelected = selected.includes(tool.name);
                return (
                  <CommandItem
                    key={tool.name}
                    value={`${tool.name} ${tool.description}`}
                    onSelect={() => toggleTool(tool.name)}
                  >
                    <Check className={cn("size-4", !isSelected && "opacity-0")} />
                    <span className="flex min-w-0 flex-col gap-0.5">
                      <span className="font-mono text-xs">{tool.name}</span>
                      <span className="text-xs text-muted-foreground">{tool.description}</span>
                    </span>
                  </CommandItem>
                );
              })}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}
