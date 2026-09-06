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
import { ProjectContext } from "@/contexts/project/ProjectContext";
import { TaskContext } from "@/contexts/task/TaskContext";
import type { Task } from "@/types/types";
import { cn } from "@/lib/utils";
import { Check, ChevronDown, User, Users, X } from "lucide-react";
import { useContext, useMemo, useState } from "react";
import { SELF_ASSIGNEE } from "@/features/task/stage-move-assignee";
import {
  createAssigneeOptions,
  getAssigneeOptionState,
} from "@/features/task/properties/task-assignee-options";

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
  const isControlled = onValueChange !== undefined;
  const [open, setOpen] = useState(false);
  const [searchValue, setSearchValue] = useState("");

  const currentValue = isControlled ? (value ?? "") : task.Assignee;

  const options = useMemo(() => createAssigneeOptions(Tasks), [Tasks]);

  const { filteredOptions, showCreate, trimmedSearch } = useMemo(
    () => getAssigneeOptionState(options, searchValue),
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
              selectAssignee(SELF_ASSIGNEE);
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
            placeholder="Search assignees..."
            value={searchValue}
            onValueChange={setSearchValue}
          />
          <CommandList>
            <CommandEmpty>No assignees found.</CommandEmpty>
            {currentValue && (
              <CommandGroup>
                <CommandItem value="__clear__" onSelect={() => selectAssignee("")}>
                  <X className="size-4 shrink-0" />
                  <span>Clear assignee</span>
                </CommandItem>
              </CommandGroup>
            )}
            {filteredOptions.length > 0 && (
              <CommandGroup>
                {filteredOptions.map((option) => {
                  const isMe = option === SELF_ASSIGNEE;
                  return (
                    <CommandItem
                      key={option}
                      value={option}
                      onSelect={() => selectAssignee(option)}
                    >
                      {isMe ? (
                        <User className="size-4 shrink-0" />
                      ) : (
                        <Users className="size-4 shrink-0" />
                      )}
                      <span>{isMe ? "Assign to Self" : option}</span>
                      {currentValue === option && <Check className="ml-auto size-4 shrink-0" />}
                    </CommandItem>
                  );
                })}
              </CommandGroup>
            )}
            {showCreate && (
              <CommandGroup>
                <CommandItem
                  value={`__create__${trimmedSearch}`}
                  onSelect={() => selectAssignee(trimmedSearch)}
                >
                  <Users className="size-4 shrink-0" />
                  <span>
                    Assign to <span className="font-medium">"{trimmedSearch}"</span>
                  </span>
                </CommandItem>
              </CommandGroup>
            )}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}
