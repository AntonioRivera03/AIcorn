import { FileText } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  CommandDialog,
  CommandInput,
  CommandList,
  CommandGroup,
  CommandItem,
  CommandEmpty,
} from "@/components/ui/command";
import { useTaskTemplates, type TaskTemplate } from "@/features/jobs/use-jobs";

export function TaskTemplatePicker({
  projectId,
  open,
  onOpenChange,
  onSelect,
}: {
  projectId: number;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSelect: (template: TaskTemplate) => void;
}) {
  const templates = useTaskTemplates(projectId, open);
  return (
    <CommandDialog
      open={open}
      onOpenChange={onOpenChange}
      title="Load a task template"
      description="Choose a template to fill this task locally. Selecting one does not save or run anything."
    >
      <CommandInput
        placeholder="Search templates…"
        aria-label="Search task templates"
      />
      <CommandList>
        {templates.isFetching ? (
          <p role="status" className="p-5 text-sm text-muted-foreground">
            Loading templates…
          </p>
        ) : templates.isError ? (
          <div role="alert" className="p-5 text-sm text-destructive">
            Could not load templates.{" "}
            <Button variant="link" onClick={() => void templates.refetch()}>
              Retry
            </Button>
          </div>
        ) : (
          <>
            <CommandEmpty>
              No templates found. Create one in Jobs & templates.
            </CommandEmpty>
            <CommandGroup>
              {templates.data?.map((template) => (
                <CommandItem
                  key={template.id}
                  value={`${template.id} ${template.name} ${template.title}`}
                  onSelect={() => onSelect(template)}
                  className="gap-3"
                >
                  <FileText className="size-4 shrink-0" />
                  <div className="min-w-0">
                    <div className="truncate font-medium">
                      {template.name || "Untitled template"}
                    </div>
                    <div className="truncate text-xs text-muted-foreground">
                      {template.title || "Untitled task"}
                    </div>
                  </div>
                </CommandItem>
              ))}
            </CommandGroup>
          </>
        )}
      </CommandList>
      <p className="border-t border-border px-4 py-3 text-xs text-muted-foreground">
        Loads a draft. Your template stays unchanged.
      </p>
    </CommandDialog>
  );
}
