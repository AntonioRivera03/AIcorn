import { useEffect } from "react";
import type { Mention, MentionTask } from "@/features/project-chat/mentions";
import { cn } from "@/lib/utils";

type MentionPickerProps = {
  id: string;
  options: MentionTask[];
  selected: number;
  mode: Mention["mode"];
  loading: boolean;
  owners: Map<number, string>;
  onChoose: (task: MentionTask) => void;
};

// Task suggestions for a # in the composer; the composer's keys move through
// them (↑↓, Enter or Tab to insert, Escape to close).
export function MentionPicker({
  id,
  options,
  selected,
  mode,
  loading,
  owners,
  onChoose,
}: MentionPickerProps) {
  const selectedId = options[selected]?.id;
  useEffect(() => {
    if (selectedId) document.getElementById(`${id}-${selectedId}`)?.scrollIntoView({ block: "nearest" });
  }, [id, selectedId]);

  return (
    <div
      id={id}
      role="listbox"
      aria-label="Project tasks"
      className="absolute right-3 bottom-full left-3 z-30 mb-2 max-h-64 overflow-y-auto rounded-xl border bg-popover p-1 shadow-lg"
    >
      <p className="px-3 py-1.5 text-xs text-muted-foreground">
        {mode === "title" ? "Search titles" : "Task numbers"} · ↑↓ · Enter to insert
      </p>
      {options.map((task, index) => (
        <button
          key={task.id}
          id={`${id}-${task.id}`}
          type="button"
          role="option"
          aria-selected={index === selected}
          className={cn(
            "flex w-full items-center gap-2 rounded-lg px-3 py-1.5 text-left text-sm",
            index === selected && "bg-accent text-accent-foreground",
          )}
          onMouseDown={(event) => event.preventDefault()}
          onClick={() => onChoose(task)}
        >
          <span className="font-mono text-xs text-muted-foreground">#{task.id}</span>
          <span className="min-w-0 flex-1 break-words">{task.title || "Untitled task"}</span>
          {owners.get(task.id) && (
            <span className="shrink-0 text-xs text-muted-foreground">{owners.get(task.id)}</span>
          )}
        </button>
      ))}
      {!options.length && (
        <p className="px-3 py-2 text-sm text-muted-foreground">
          {loading ? "Loading tasks…" : "No matching tasks"}
        </p>
      )}
    </div>
  );
}
