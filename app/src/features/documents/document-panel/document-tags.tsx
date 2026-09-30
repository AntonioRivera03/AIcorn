import { useState } from "react";
import { Plus, X } from "lucide-react";
import { Badge } from "@/components/ui/badge";
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

const MAX_TAG_LENGTH = 40;

type DocumentTagsProps = {
  tags: string[];
  // Tags used elsewhere in the project, offered first.
  suggestions: string[];
  onChange: (tags: string[]) => void;
};

export function DocumentTags({ tags, suggestions, onChange }: DocumentTagsProps) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const taken = new Set(tags.map((tag) => tag.toLowerCase()));
  const options = suggestions.filter((tag) => !taken.has(tag.toLowerCase()));
  const name = query.trim().replace(/\s+/g, " ");
  const known = taken.has(name.toLowerCase()) || options.some((tag) => tag.toLowerCase() === name.toLowerCase());

  const add = (tag: string) => {
    if (!tag || taken.has(tag.toLowerCase())) return;
    onChange([...tags, tag]);
    setQuery("");
  };

  return (
    <div className="flex flex-wrap items-center gap-1.5">
      {tags.map((tag) => (
        <Badge key={tag} variant="secondary" className="gap-1 pr-0.5">
          {tag}
          <button
            type="button"
            aria-label={`Remove tag ${tag}`}
            className="rounded-sm p-0.5 opacity-60 hover:opacity-100 focus-visible:opacity-100 focus-visible:outline-2 focus-visible:outline-ring"
            onClick={() => onChange(tags.filter((other) => other !== tag))}
          >
            <X className="size-3" />
          </button>
        </Badge>
      ))}
      <Popover
        open={open}
        onOpenChange={(next) => {
          setOpen(next);
          if (!next) setQuery("");
        }}
      >
        <PopoverTrigger asChild>
          <Button variant="ghost" size="sm" className="h-6 px-2 text-muted-foreground">
            <Plus className="size-3.5" />
            Add tag
          </Button>
        </PopoverTrigger>
        <PopoverContent className="w-60 p-0" align="start">
          <Command>
            <CommandInput
              placeholder="Find or create a tag…"
              value={query}
              maxLength={MAX_TAG_LENGTH}
              onValueChange={setQuery}
            />
            {/* Existing tags come first, so Enter picks a match rather than
                creating a near-duplicate. */}
            <CommandList>
              <CommandEmpty>Type a name to create a tag.</CommandEmpty>
              {options.length > 0 && (
                <CommandGroup heading="Tags">
                  {options.map((tag) => (
                    <CommandItem key={tag} value={tag} onSelect={() => add(tag)}>
                      {tag}
                    </CommandItem>
                  ))}
                </CommandGroup>
              )}
              {name && !known && (
                <CommandGroup>
                  <CommandItem value={`create ${name}`} onSelect={() => add(name)}>
                    <Plus />
                    Create “{name}”
                  </CommandItem>
                </CommandGroup>
              )}
            </CommandList>
          </Command>
        </PopoverContent>
      </Popover>
    </div>
  );
}
