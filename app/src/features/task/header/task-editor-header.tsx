import { AskAIButton } from "@/features/ai/ask-ai-button";
import { RelativeTimeWithTooltip } from "@/components/relative-time-with-tooltip";
import { Button } from "@/components/ui/button";
import {
  DrawerClose,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
} from "@/components/ui/drawer";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { ProjectContext } from "@/contexts/project/ProjectContext";
import { TaskContext } from "@/contexts/task/TaskContext";
import { WorkflowStageChip } from "@/features/workflows/shared/workflow-stage-chip";
import { useIsMobile } from "@/hooks/useMobile";
import {
  Bot,
  ChevronsRightIcon,
  ClipboardIcon,
  CopyCheckIcon,
  Ellipsis,
  FileText,
  Maximize2,
  PinIcon,
  Trash2Icon,
} from "lucide-react";
import { useContext, useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import type { Stage } from "@/types/types";
import { Badge } from "@/components/ui/badge";
import { priorityOutlineBadgeClass } from "../properties/task-priority-palette";
import TaskPriorityIcon from "../properties/icons/TaskPriorityIcon";
import { cn } from "@/lib/utils";
import TaskTypeBadge from "../properties/task-type-badge";
import { DeleteTaskDialog } from "@/features/task/delete-task-dialog";
import { useAI } from "@/features/ai/ai-context";

export function TaskEditorHeader({
  setOpen = () => {},
  onCopyAsMarkdown,
  onCopyAsPlainText,
  taskStage,
  isEditorReady = false,
  onLoadTemplate,
}: {
  setOpen: (open: boolean) => void;
  onCopyAsMarkdown?: () => void;
  onCopyAsPlainText?: () => void;
  taskStage: Stage;
  isEditorReady?: boolean;
  onLoadTemplate?: () => void;
}) {
  const { state: task } = useContext(TaskContext);
  const { Project } = useContext(ProjectContext);
  const navigate = useNavigate();

  const isMobile = useIsMobile();
  const [deleteOpen, setDeleteOpen] = useState(false);
  const { openAI } = useAI();

  return (
    <DrawerHeader className="p-2 sm:border-b">
      <DrawerTitle hidden>{task.Name}</DrawerTitle>
      <DrawerDescription hidden>
        Task editor for task "{task.Name}"
      </DrawerDescription>
      <div className="flex justify-between">
        <div className="flex items-center">
          <DrawerClose asChild>
            <Button
              variant="ghost"
              size="icon-sm"
              className="text-muted-foreground hidden sm:flex"
              aria-label="Close task"
            >
              <ChevronsRightIcon className={isMobile ? "rotate-90" : ""} />
            </Button>
          </DrawerClose>

          <Button
            variant="ghost"
            size="icon-sm"
            className="text-muted-foreground"
            aria-label="Open full task"
            onClick={() => {
              setOpen(false);
              navigate({
                to: "/task/$taskId",
                params: { taskId: String(task.ID) },
              });
            }}
          >
            <Maximize2 className="size-3.5" />
          </Button>

          <RelativeTimeWithTooltip
            date={task.TimeModified}
            label="Modified"
            className="hidden sm:flex text-xs ml-1"
          />
        </div>

        <div className="flex gap-1 sm:gap-2 items-center">
          <Badge
            variant="outline"
            className={cn(
              "sm:hidden",
              priorityOutlineBadgeClass(task.Priority),
            )}
          >
            <TaskPriorityIcon variant={task.Priority} />
            {task.Priority}
          </Badge>
          <TaskTypeBadge type={task.Type} />
          {taskStage && (
            <WorkflowStageChip className="rounded-full" stage={taskStage} />
          )}
          {task.Type.ViewMode !== "chat" && (
            <AskAIButton taskId={task.ID} taskName={task.Name} />
          )}
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="ghost"
                size="icon"
                className="size-7 data-[state=open]:bg-muted text-muted-foreground flex"
                aria-label="Task actions"
              >
                <Ellipsis className="size-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent className="mr-2">
              <DropdownMenuGroup>
                {onLoadTemplate && (
                  <DropdownMenuItem
                    onSelect={onLoadTemplate}
                    disabled={task.ID === 0}
                  >
                    <FileText className="text-muted-foreground" />
                    Template…
                  </DropdownMenuItem>
                )}
                <DropdownMenuItem>
                  <PinIcon className="text-muted-foreground" />
                  Pin
                </DropdownMenuItem>
                <DropdownMenuItem>
                  <CopyCheckIcon className="text-muted-foreground" />
                  Duplicate
                </DropdownMenuItem>
                <DropdownMenuSub>
                  <DropdownMenuSubTrigger
                    disabled={task.Type.ViewMode === "chat"}
                  >
                    <ClipboardIcon className="text-muted-foreground" />
                    Copy as
                  </DropdownMenuSubTrigger>
                  <DropdownMenuSubContent>
                    <DropdownMenuItem
                      disabled={!isEditorReady}
                      onClick={onCopyAsMarkdown}
                    >
                      Markdown
                    </DropdownMenuItem>
                    <DropdownMenuItem onClick={onCopyAsPlainText}>
                      Plain text
                    </DropdownMenuItem>
                  </DropdownMenuSubContent>
                </DropdownMenuSub>
              </DropdownMenuGroup>
              <DropdownMenuSeparator />
              <DropdownMenuGroup>
                <DropdownMenuItem
                  onClick={() => openAI({ id: task.ID, name: task.Name })}
                  disabled={task.ID === 0 || task.Type.ViewMode === "chat"}
                >
                  <Bot className="size-4" />
                  Ask AI
                </DropdownMenuItem>
              </DropdownMenuGroup>
              <DropdownMenuSeparator />
              <DropdownMenuGroup>
                <DropdownMenuItem
                  onClick={() => setDeleteOpen(true)}
                  variant="destructive"
                >
                  <Trash2Icon className="text-muted-foreground" />
                  Delete
                </DropdownMenuItem>
              </DropdownMenuGroup>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>

      <DeleteTaskDialog
        task={task}
        projectId={Project.ID}
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        onDeleted={() => setOpen(false)}
      />
    </DrawerHeader>
  );
}
