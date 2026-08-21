import { StageIcon, stageTintClass } from "@/features/stage/stage-visual";
import { Badge } from "@/components/ui/badge";
import { ItemGroup } from "@/components/ui/item";
import { ProjectContext } from "@/contexts/project/ProjectContext";
import { TaskContext, defaultTaskContextValue } from "@/contexts/task/TaskContext";
import { useDropZone } from "@/hooks/useDropZone";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import type { ChecklistTask, Stage } from "@/types/types";
import { useCallback, useContext, useMemo, type SyntheticEvent } from "react";
import { KanbanItem } from "./kanban-item";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { PlusIcon } from "lucide-react";
import { useTaskMutation } from "@/queries/useTaskMutation";
import TaskEditorDrawer from "@/features/task/task-editor-drawer";

type DragListeners = Record<string, (e: SyntheticEvent) => void>;

export function KanbanColumn({
  stage,
  getItemProps,
  lastDrop,
  setTaskDrawerOpen,
}: {
  stage: Stage;
  getItemProps?: (
    id: string,
    opts?: { listeners?: DragListeners },
  ) => Record<string, unknown>;
  lastDrop?: { taskIds: Set<number>; animClass: string } | null;
  setTaskDrawerOpen: (open: boolean) => void;
}) {
  const { setState: setTask } = useContext(TaskContext);
  const { Project, Checklists } = useContext(ProjectContext);
  const { create } = useTaskMutation(Project.ID);

  // Always build from the defaults, never from whatever the (long-lived)
  // context happens to hold — a leftover value would be baked into the new task.
  const handleAddTask = useCallback(
    () =>
      create.mutate(
        {
          ...defaultTaskContextValue.state,
          Checklist: Checklists[0]?.ID,
          Stage: stage.ID,
        },
        {
          onSuccess: (newTask: ChecklistTask) => {
            setTask(newTask);
          },
        },
      ),
    [create, Checklists, setTask, stage.ID],
  );

  const { setNodeRef, isOver } = useDropZone(stage.ID);

  const { Tasks } = useContext(ProjectContext);
  const filteredTasks = useMemo(
    () => Tasks.filter((task) => task.Stage === stage.ID),
    [Tasks, stage.ID],
  );

  const tint = stageTintClass(stage.Color);

  return (
    <div className="min-w-64 w-full overflow-hidden flex flex-col gap-2 p-1 h-full min-h-full">
        <div className="flex justify-between items-center shrink">
            <div className="p-2 flex-1">
                <span className="flex items-center gap-2 text-foreground">
                    <StageIcon stage={stage} />
                    {stage.Name}
                    <Badge variant="outline" className="size-5">
                        {filteredTasks.length}
                    </Badge>
                </span>

                <Tooltip>
                    <TooltipTrigger asChild>
                        <span className="text-muted-foreground text-sm h-5 block truncate">
                        {stage.Description}
                        </span>
                    </TooltipTrigger>
                    {stage.Description && (
                        <TooltipContent>{stage.Description}</TooltipContent>
                    )}
                </Tooltip>
            </div>

            <TaskEditorDrawer
                onOpenChange={(open) => {
                    setTaskDrawerOpen(open);
                    if (!open) {
                        setTask(defaultTaskContextValue.state);
                    }
                }}
            >
                <Button variant="ghost" size="icon-sm" onClick={handleAddTask}>
                    <PlusIcon />
                </Button>
            </TaskEditorDrawer>
          </div>
      <ItemGroup
        className={cn(
          "h-full overflow-y-scroll w-full min-w-0 overflow-x-visible flex flex-col gap-2 p-2 rounded-xl",
          isOver && tint,
        )}
        ref={setNodeRef}
      >
        {filteredTasks.map((task) => (
          <KanbanItem
            key={task.ID}
            task={task}
            getItemProps={getItemProps}
            animClass={lastDrop?.taskIds.has(task.ID) ? lastDrop.animClass : undefined}
          />
        ))}
      </ItemGroup>
    </div>
  );
}
