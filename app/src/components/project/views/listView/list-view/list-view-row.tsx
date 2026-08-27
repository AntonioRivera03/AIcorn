import { Badge } from "@/components/ui/badge";
import {
  Item,
  ItemActions,
  ItemContent,
  ItemDescription,
  ItemFooter,
  ItemMedia,
} from "@/components/ui/item";
import { ListViewTaskName } from "@/components/project/views/listView/list-view/list-view-task-name";
import { Bot, ChevronRightIcon, LandPlot, User } from "lucide-react";
import TaskEditorDrawer from "@/features/task/task-editor-drawer";
import { StageIcon } from "@/features/stage/stage-visual";
import TaskTypeBadge from "@/features/task/properties/task-type-badge";
import { TaskProvider } from "@/contexts/task/TaskProvider";
import TaskPriorityIcon from "@/features/task/properties/icons/TaskPriorityIcon";
import { TaskPlannedDates } from "@/features/task/properties/task-planned-dates";
import { selectedItemClasses } from "@/hooks/useSelection";
import { UnresolvedBlockersBadge } from "@/features/task/relationships/unresolved-blockers-badge";
import { SubtaskProgressBar } from "@/features/task/relationships/subtask-progress-bar";
import { cn } from "@/lib/utils";
import type { ChecklistTask, Stage } from "@/types/types";
import { useSubtaskProgress } from "@/features/task/relationships/queries/useSubtaskProgress";
import { usePersonasQuery } from "@/features/persona/queries/use-personas-query";
import { useContext, useMemo } from "react";
import { ProjectContext } from "@/contexts/project/ProjectContext";

export function ListViewRow({
  task,
  stage,
  showChecklist,
  itemProps,
}: {
  task: ChecklistTask;
  stage: Stage | undefined;
  showChecklist: boolean;
  itemProps: Record<string, unknown>;
}) {
  const itemClassName = (itemProps.className as string | undefined) ?? "";
  const subtaskProgress = useSubtaskProgress(task.ID);
  const { Stages } = useContext(ProjectContext);
  const { data: personas = [] } = usePersonasQuery();
  const isPersonaAssignee = useMemo(() => {
    if (!task.Assignee) return false;
    const names = new Set<string>();
    for (const persona of personas) if (persona.Name) names.add(persona.Name);
    for (const s of Stages) if (s.Persona?.Name) names.add(s.Persona.Name);
    return names.has(task.Assignee);
  }, [personas, Stages, task.Assignee]);

  return (
    <TaskProvider defaultState={task}>
      <TaskEditorDrawer>
        <Item asChild>
          <a
            {...itemProps}
            className={cn("px-0 sm:px-4", itemClassName, selectedItemClasses())}
          >
            <ItemMedia className="flex flex-col">
              <StageIcon stage={stage} />
              <TaskPriorityIcon variant={task.Priority} />
            </ItemMedia>
            <ItemContent className="flex-1 min-w-0 overflow-visible">
              <ListViewTaskName />

              <ItemDescription className="line-clamp-none">
                <span className="w-full h-fit flex flex-wrap gap-2">
                  <TaskTypeBadge className="flex sm:hidden" type={task.Type} />

                  <Badge
                    variant={task.Assignee !== "" ? "secondary" : "outline"}
                    className={
                      task.Assignee !== ""
                        ? ""
                        : "bg-background text-muted-foreground"
                    }
                  >
                    {isPersonaAssignee ? (
                      <Bot className="size-2 text-primary" />
                    ) : (
                      <User className="size-2" />
                    )}
                    {task.Assignee === "" ? "Not Assigned" : task.Assignee}
                  </Badge>

                  <TaskPlannedDates
                    className="hidden sm:flex"
                    start={task.TimePlannedStart}
                    end={task.TimePlannedEnd}
                    hasStartTime={task.HasTimePlannedStart}
                    hasEndTime={task.HasTimePlannedEnd}
                    excludeYear
                  />

                  {showChecklist && (
                    <Badge
                      variant="outline"
                      className="bg-background flex sm:hidden"
                    >
                      <LandPlot className="size-2" />
                      {task.ChecklistName !== ""
                        ? task.ChecklistName
                        : "Untitled Checklist"}
                    </Badge>
                  )}

                  <UnresolvedBlockersBadge taskId={task.ID} />
                </span>
              </ItemDescription>
            </ItemContent>
            <ItemActions>
              <span className="hidden sm:flex w-full justify-end gap-2">
                {subtaskProgress && (
                    <SubtaskProgressBar
                        done={subtaskProgress.done}
                        total={subtaskProgress.total}
                        className="min-w-3xs"
                    />
                )}
                <TaskTypeBadge type={task.Type} />
                {showChecklist && (
                  <Badge variant="outline" className="bg-background">
                    <LandPlot className="size-2" />
                    {task.ChecklistName !== ""
                      ? task.ChecklistName
                      : "Untitled Checklist"}
                  </Badge>
                )}
              </span>
              <ChevronRightIcon className="size-4" />
            </ItemActions>
            {subtaskProgress && (
                <ItemFooter className="sm:hidden">
                    <SubtaskProgressBar done={subtaskProgress.done} total={subtaskProgress.total} />
                </ItemFooter>
            )}
          </a>
        </Item>
      </TaskEditorDrawer>
    </TaskProvider>
  );
}
