import { AudioLines, ArrowUpRight, RotateCcw, Unplug } from "lucide-react";
import { Button } from "@/components/ui/button";
import { DropdownMenuItem, DropdownMenuSeparator } from "@/components/ui/dropdown-menu";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { useAI } from "@/features/ai/ai-context";
import { useBoardConductor } from "./conductor-context";
import type { ConductorTask } from "./use-conductor";

const labels: Record<ConductorTask["state"], string> = {
  waiting: "Awaiting Conductor", planning: "Conductor planning", needs_context: "Needs context", queued: "Agent queued", working: "Agent working", completed: "Human review", failed: "Conductor failed", held: "Conductor on hold",
};
const canRecheck = (state: string) => ["needs_context", "failed", "held"].includes(state);

export function ConductorTaskMenu({ taskId }: { taskId: number }) {
  const conductor = useBoardConductor();
  if (!conductor?.data || taskId <= 0) return null;
  const task = conductor.data.tasks.find((t) => t.taskId === taskId);
  return <>
    <DropdownMenuSeparator />
    {task && canRecheck(task.state) && <DropdownMenuItem disabled={conductor.manage.isPending} onSelect={() => conductor.manage.mutate({ ids: [taskId], action: "recheck" })}><RotateCcw />Recheck with Conductor</DropdownMenuItem>}
    <DropdownMenuItem disabled={conductor.manage.isPending} onSelect={() => conductor.manage.mutate({ ids: [taskId], action: task ? "release" : "send" })}>
      {task ? <Unplug /> : <AudioLines />}{task ? "Release from Conductor" : "Send to Conductor"}
    </DropdownMenuItem>
  </>;
}

export function ConductorTaskBadge({ taskId, name }: { taskId: number; name: string }) {
  const conductor = useBoardConductor();
  const { openAI } = useAI();
  const task = conductor?.data?.tasks.find((t) => t.taskId === taskId);
  if (!task || !conductor) return null;
  return <Popover>
    <PopoverTrigger asChild>
      <button type="button" onPointerDown={(e) => e.stopPropagation()} onClick={(e) => e.stopPropagation()} className="inline-flex w-fit items-center gap-1.5 rounded-md border border-conductor/25 bg-conductor/5 px-1.5 py-0.5 text-xs text-conductor hover:bg-conductor/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring" aria-label={`${labels[task.state]}. Open Conductor details`}>
        <AudioLines className="size-3" />{labels[task.state]}
      </button>
    </PopoverTrigger>
    <PopoverContent className="w-80 space-y-3" onClick={(e) => e.stopPropagation()} onPointerDown={(e) => e.stopPropagation()}>
      <p className="text-sm font-medium text-conductor">{labels[task.state]}</p>
      <p className="whitespace-pre-wrap text-sm text-muted-foreground">{task.message || "This task will be checked when Conductor starts."}</p>
      {task.state === "needs_context" && <p className="text-xs text-muted-foreground">Add the missing information to the task, then recheck it.</p>}
      <div className="flex flex-wrap gap-2">
        {task.jobId > 0 && <Button size="sm" variant="outline" onClick={() => openAI({ id: taskId, name })}><ArrowUpRight />View session</Button>}
        {canRecheck(task.state) && <Button size="sm" variant="outline" disabled={conductor.manage.isPending} onClick={() => conductor.manage.mutate({ ids: [taskId], action: "recheck" })}><RotateCcw />Recheck</Button>}
        <Button size="sm" variant="ghost" disabled={conductor.manage.isPending} onClick={() => conductor.manage.mutate({ ids: [taskId], action: "release" })}><Unplug />Release</Button>
      </div>
    </PopoverContent>
  </Popover>;
}

export function ConductorBulkActions({ ids, onClear }: { ids: number[]; onClear: () => void }) {
  const conductor = useBoardConductor();
  if (!conductor?.data) return null;
  return <>
    <Button size="sm" variant="outline" disabled={conductor.manage.isPending} onClick={() => conductor.manage.mutate({ ids, action: "send" }, { onSuccess: onClear })}><AudioLines />Send to Conductor</Button>
    <Button size="sm" variant="ghost" disabled={conductor.manage.isPending} onClick={() => conductor.manage.mutate({ ids, action: "release" }, { onSuccess: onClear })}><Unplug />Release</Button>
  </>;
}
