import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from "@/components/ui/input-group";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { ProjectContext } from "@/contexts/project/ProjectContext";
import { TaskContext } from "@/contexts/task/TaskContext";
import { usePersonasQuery } from "@/features/persona/queries/use-personas-query";
import type { Task } from "@/types/types";
import { Check, ChevronDown, User, Users, X } from "lucide-react";
import { useContext, useMemo, useState } from "react";

export const SELF_ASSIGNEE = "Me";

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
  placeholder = "Assignee (optional)",
}: Props) {
  const { state: task, setState: setTask } = useContext(TaskContext);
  const { Tasks, Stages } = useContext(ProjectContext);
  const { data: personas = [] } = usePersonasQuery();
  const isControlled = onValueChange !== undefined;
  const [localValue, setLocalValue] = useState(value ?? "");
  const [prevPropValue, setPrevPropValue] = useState(value);
  const [open, setOpen] = useState(false);

  if (isControlled && value !== prevPropValue) {
    setPrevPropValue(value);
    setLocalValue(value ?? "");
  }

  const currentValue = isControlled ? localValue : task.Assignee;
  const options = useMemo(() => {
    const names = new Set<string>([SELF_ASSIGNEE]);
    for (const candidate of Tasks) if (candidate.Assignee) names.add(candidate.Assignee);
    for (const persona of personas) names.add(persona.Name);
    for (const stage of Stages) if (stage.Persona?.Name) names.add(stage.Persona.Name);
    return [...names];
  }, [personas, Stages, Tasks]);

  const selectAssignee = (assignee: string) => {
    setOpen(false);
    if (isControlled) {
      setLocalValue(assignee);
      onValueChange(assignee);
      return;
    }
    const updated = { ...task, Assignee: assignee };
    setTask(updated);
    onChange(updated);
  };

  const handleChange = (assignee: string) => {
    if (isControlled) {
      setLocalValue(assignee);
      return;
    }
    setTask({ ...task, Assignee: assignee });
  };

  const commit = () => {
    if (isControlled) {
      if (currentValue !== (value ?? "")) onValueChange(currentValue);
      return;
    }
    onChange({ ...task, Assignee: task.Assignee });
  };

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <InputGroup>
        <InputGroupAddon>
          <User />
        </InputGroupAddon>
        <InputGroupInput
          id="assignee"
          value={currentValue}
          placeholder={placeholder}
          className="placeholder:text-muted-foreground text-sm"
          aria-keyshortcuts="Control+Enter Meta+Enter"
          onChange={(event) => handleChange(event.target.value)}
          onBlur={commit}
          onKeyDown={(event) => {
            if ((event.ctrlKey || event.metaKey) && event.key === "Enter") {
              event.preventDefault();
              selectAssignee(SELF_ASSIGNEE);
            }
          }}
        />
        <InputGroupAddon align="inline-end">
          {currentValue && (
            <button
              type="button"
              aria-label="Clear assignee"
              className="rounded-full p-0.5 hover:bg-muted-foreground/20 cursor-pointer"
              onClick={() => selectAssignee("")}
            >
              <X className="size-3.5" />
            </button>
          )}
          <PopoverTrigger asChild>
            <InputGroupButton size="icon-xs" aria-label="Choose assignee">
              <ChevronDown className="size-3.5" />
            </InputGroupButton>
          </PopoverTrigger>
        </InputGroupAddon>
      </InputGroup>
      <PopoverContent className="w-64 p-0" align="end">
        <Command>
          <CommandInput placeholder="Search people or personas..." />
          <CommandList>
            <CommandEmpty>No assignees found.</CommandEmpty>
            <CommandGroup>
              <CommandItem value={SELF_ASSIGNEE} onSelect={() => selectAssignee(SELF_ASSIGNEE)}>
                <User />
                <span>Assign to Self</span>
                {currentValue === SELF_ASSIGNEE && <Check className="ml-auto" />}
              </CommandItem>
              {options.filter((option) => option !== SELF_ASSIGNEE).map((option) => (
                <CommandItem key={option} value={option} onSelect={() => selectAssignee(option)}>
                  <Users />
                  <span>{option}</span>
                  {currentValue === option && <Check className="ml-auto" />}
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}
