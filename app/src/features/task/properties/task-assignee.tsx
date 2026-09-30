import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Button } from "@/components/ui/button";
import { TruncatedText } from "@/components/truncated-text";
import { ProjectContext } from "@/contexts/project/ProjectContext";
import { TaskContext } from "@/contexts/task/TaskContext";
import type { Task } from "@/types/types";
import { cn } from "@/lib/utils";
import { Check, ChevronDown, User, Users, X } from "lucide-react";
import { useContext, useMemo, useState } from "react";
import {
  createAssigneeOptions,
  filterAssigneeOptions,
} from "@/features/task/properties/task-assignee-options";
import { useMembersQuery } from "@/features/workspaces/queries/workspace-queries";
import { useWorkspace } from "@/features/workspaces/workspace-context";

type Props = {
  onChange?: (task: Task) => void;
  value?: string;
  onValueChange?: (value: string) => void;
  placeholder?: string;
};

export function TaskAssignee({
  onChange = () => {},
  value,
  onValueChange,
  placeholder = "Select an assignee",
}: Props) {
  const { state: task, setState: setTask } = useContext(TaskContext);
  const { Tasks } = useContext(ProjectContext);
  const { account, workspace } = useWorkspace();
  const { data: members = [] } = useMembersQuery(workspace.id);
  const isControlled = onValueChange !== undefined;
  const [open, setOpen] = useState(false);
  const [searchValue, setSearchValue] = useState("");

  const currentValue = isControlled ? (value ?? "") : task.Assignee;

  const options = useMemo(
    () => createAssigneeOptions(members.map((m) => m.name), Tasks),
    [members, Tasks],
  );
  const filtered = useMemo(
    () => filterAssigneeOptions(options, searchValue),
    [options, searchValue],
  );

  const selectAssignee = (assignee: string) => {
    setOpen(false);
    setSearchValue("");
    if (isControlled) {
      onValueChange(assignee);
      return;
    }
    const updated = { ...task, Assignee: assignee };
    setTask(updated);
    onChange(updated);
  };

  const handleOpenChange = (nextOpen: boolean) => {
    setOpen(nextOpen);
    if (!nextOpen) setSearchValue("");
  };

  const renderOption = (option: string, isMember: boolean) => (
    <CommandItem key={option} value={option} onSelect={() => selectAssignee(option)}>
      {isMember ? <User className="size-4 shrink-0" /> : <Users className="size-4 shrink-0" />}
      <TruncatedText text={isMember && option === account.name ? `${option} (you)` : option} />
      {currentValue === option && <Check className="ml-auto size-4 shrink-0" />}
    </CommandItem>
  );

  return (
    <Popover open={open} onOpenChange={handleOpenChange}>
      <PopoverTrigger asChild>
        <Button
          id="assignee"
          variant="outline"
          role="combobox"
          aria-expanded={open}
          className="w-full justify-between font-normal"
          onKeyDown={(event) => {
            if ((event.ctrlKey || event.metaKey) && event.key === "Enter") {
              event.preventDefault();
              selectAssignee(account.name);
            }
          }}
        >
          <span className="flex items-center gap-2 min-w-0">
            {currentValue ? (
              <User className="size-4 shrink-0" />
            ) : (
              <User className="size-4 shrink-0 text-muted-foreground" />
            )}
            <span className={cn("truncate", !currentValue && "text-muted-foreground")}>
              {currentValue || placeholder}
            </span>
          </span>
          <ChevronDown className="size-4 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-60 p-0" align="start">
        <Command shouldFilter={false}>
          <CommandInput
            placeholder="Search members..."
            value={searchValue}
            onValueChange={setSearchValue}
          />
          <CommandList>
            <CommandEmpty>No members found.</CommandEmpty>
            {currentValue && (
              <CommandGroup>
                <CommandItem value="__clear__" onSelect={() => selectAssignee("")}>
                  <X className="size-4 shrink-0" />
                  <span>Clear assignee</span>
                </CommandItem>
              </CommandGroup>
            )}
            {filtered.members.length > 0 && (
              <CommandGroup heading="Members">
                {filtered.members.map((option) => renderOption(option, true))}
              </CommandGroup>
            )}
            {filtered.others.length > 0 && (
              <CommandGroup heading="Other">
                {filtered.others.map((option) => renderOption(option, false))}
              </CommandGroup>
            )}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}
