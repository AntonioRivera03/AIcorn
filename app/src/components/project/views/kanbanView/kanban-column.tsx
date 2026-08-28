import { StageIcon, stageTintClass } from "@/features/stage/stage-visual";
import { Badge } from "@/components/ui/badge";
import { ItemGroup } from "@/components/ui/item";
import { ProjectContext } from "@/contexts/project/ProjectContext";
import { TaskProvider } from "@/contexts/task/TaskProvider";
import { useDropZone } from "@/hooks/useDropZone";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import type { Stage } from "@/types/types";
import { useContext, useMemo, type SyntheticEvent } from "react";
import { KanbanItem } from "./kanban-item";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Bot, PlusIcon } from "lucide-react";
import { NewTaskEditorDrawer } from "@/features/task/new-task-editor-drawer";
import { shouldShowPersonaIndicator } from "@/components/project/views/kanbanView/kanban-column-persona";

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
  const { Tasks } = useContext(ProjectContext);
  const filteredTasks = useMemo(
    () => Tasks.filter((task) => task.Stage === stage.ID),
    [Tasks, stage.ID],
  );

  const { setNodeRef, isOver } = useDropZone(stage.ID);

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
                    {shouldShowPersonaIndicator(stage) && (
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <span
                            tabIndex={0}
                            aria-label={`Persona: ${stage.Persona.Name}`}
                            className="inline-flex rounded-sm text-primary focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
                          >
                            <Bot aria-hidden="true" className="size-3.5" />
                          </span>
                        </TooltipTrigger>
                        <TooltipContent>Persona: {stage.Persona.Name}</TooltipContent>
                      </Tooltip>
                    )}
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

            <TaskProvider>
                <NewTaskEditorDrawer
                    stageId={stage.ID}
                    setTaskDrawerOpen={setTaskDrawerOpen}
                >
                    <Button variant="ghost" size="icon-sm">
                        <PlusIcon />
                    </Button>
                </NewTaskEditorDrawer>
            </TaskProvider>
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
