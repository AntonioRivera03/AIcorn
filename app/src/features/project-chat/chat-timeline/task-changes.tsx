import { Link } from "@tanstack/react-router";
import { ArrowRight, Link2, Plus, Send, SquarePen } from "lucide-react";
import type { TaskChange } from "@/features/project-chat/activity";

const icons = {
  created: Plus,
  updated: SquarePen,
  moved: ArrowRight,
  linked: Link2,
  sent: Send,
} satisfies Record<TaskChange["kind"], unknown>;

// What the turn changed in the project, under its answer: T3 Code's changed
// files, but for tasks.
export function TaskChanges({ changes }: { changes: TaskChange[] }) {
  if (!changes.length) return null;
  return (
    <section aria-label="Changes" className="mt-3 rounded-lg bg-secondary/60 px-3 py-2">
      <h4 className="mb-1 text-xs font-medium text-muted-foreground">
        {changes.length === 1 ? "1 change" : `${changes.length} changes`}
      </h4>
      <ul className="flex flex-col">
        {changes.map((change) => {
          const Icon = icons[change.kind];
          const single = change.taskIds.length === 1 ? change.taskIds[0] : undefined;
          return (
            <li key={change.key} className="flex min-w-0 items-center gap-2 py-0.5 text-sm">
              <Icon className="size-3.5 shrink-0 text-muted-foreground" aria-hidden />
              {single ? (
                <Link
                  to="/app/task/$taskId"
                  params={{ taskId: String(single) }}
                  className="min-w-0 break-words underline-offset-4 hover:underline"
                >
                  {change.label}
                </Link>
              ) : (
                <span className="flex min-w-0 flex-wrap items-center gap-x-2">
                  {change.label}
                  {change.taskIds.map((id) => (
                    <Link
                      key={id}
                      to="/app/task/$taskId"
                      params={{ taskId: String(id) }}
                      className="font-mono text-xs text-muted-foreground underline-offset-4 hover:text-foreground hover:underline"
                    >
                      #{id}
                    </Link>
                  ))}
                </span>
              )}
            </li>
          );
        })}
      </ul>
    </section>
  );
}
