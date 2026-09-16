import { TaskOwnershipNotice } from "@/features/ai/task-ownership";
import { TaskGitHubLinks } from "@/features/task/links/task-github-links";
import { TicketChat } from "@/features/chat/ticket-chat";
import { TaskBranches } from "@/features/task/branches/task-branches";
import { Drawer, DrawerContent, DrawerTrigger } from "@/components/ui/drawer";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { SelectTaskStage } from "@/features/stage/select-task-stage";
import { SelectTaskType } from "@/features/task/properties/select-task-type";
import { SelectTaskPriority } from "@/features/task/properties/select-task-priority";
import { DatePickerInput } from "@/components/DatePickerInput";
import { useContext, useRef, useState } from "react";
import { TaskContext } from "@/contexts/task/TaskContext";
import {
  EditableTaskName,
  type EditableTaskNameHandle,
} from "@/features/task/header/editable-task-name";
import { SelectChecklist } from "@/features/task/properties/select-checklist";
import { useTaskMutation } from "@/queries/useTaskMutation";
import { ProjectContext } from "@/contexts/project/ProjectContext";
import type { Task } from "@/types/types";
import { useIsMobile } from "@/hooks/useMobile";
import { RichEditor } from "@/features/editor/rich-editor";
import { TaskEditorHeader } from "@/features/task/header/task-editor-header";
import { TaskProperty } from "@/features/task/properties/task-property";
import { TaskAssignee } from "@/features/task/properties/task-assignee";
import type { Value } from "platejs";
import type { PlateEditor } from "platejs/react";
import { MarkdownPlugin, serializeMd } from "@platejs/markdown";
import { toast } from "sonner";
import { useTaskBodyQuery } from "@/queries/useTaskQuery";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { ChevronDown, LandPlotIcon, User } from "lucide-react";
import { cn } from "@/lib/utils";
import { extractPlainText } from "@/features/task/task-utils";
import { TaskRelationshipsCard } from "@/features/task/relationships/task-relationships-card";
import { TaskRelationshipBadges } from "@/features/task/relationships/task-relationship-badges";
import { Separator } from "@/components/ui/separator";
import RelativePlannedDateBadge from "./properties/relative-planned-date-badge";
import { TaskTemplatePicker } from "./task-template-picker";
import { templateTaskDraft } from "./template-task-draft";
import type { TaskTemplate } from "@/features/jobs/use-jobs";
import { useProjectTaskTypesQuery } from "@/features/task-types/queries/useProjectTaskTypesQuery";
import { toValidBody } from "@/lib/plate";
import { createMarkdownEditor } from "@/features/editor/markdown-editor";

export default function TaskEditorDrawer({
  children,
  onOpenChange = () => {},
  allowTemplate = false,
}: {
  children: React.ReactNode;
  onOpenChange?: (open: boolean) => void;
  allowTemplate?: boolean;
}) {
  const isMobile = useIsMobile();
  const [open, setOpen] = useState(false);
  const [propertiesOpen, setPropertiesOpen] = useState(true);
  const editorRef = useRef<PlateEditor | null>(null);
  const [editorReady, setEditorReady] = useState(false);
  const [templatePickerOpen, setTemplatePickerOpen] = useState(false);
  const [templateName, setTemplateName] = useState<string | null>(null);
  const [templateSaveFailed, setTemplateSaveFailed] = useState(false);
  const [templateBody, setTemplateBody] = useState<Value | null>(null);
  const [editorVersion, setEditorVersion] = useState(0);
  const templateDraftPending = useRef(false);
  const templateGeneration = useRef(0);
  const nameRef = useRef<EditableTaskNameHandle>(null);
  // The body as it was last persisted (or as the editor normalised it on mount).
  // Only used to decide whether the close-flush has anything to write.
  const savedBodyRef = useRef<Value | null>(null);

  const { state: task, setState: setTask } = useContext(TaskContext);
  const { Project, Stages, Checklists } = useContext(ProjectContext);
  const types = useProjectTaskTypesQuery(Project.ID);
  const { update, updateBody } = useTaskMutation(Project.ID);

  const persistTemplateDraft = (updatedTask: Task, body: Value) => {
    const generation = templateGeneration.current;
    const name = templateName;
    const retryDraft = () => {
      if (templateGeneration.current !== generation) return;
      templateDraftPending.current = true;
      setTemplateName(name);
      setTemplateSaveFailed(true);
      savedBodyRef.current = null;
    };
    templateDraftPending.current = false;
    setTemplateName(null);
    setTemplateSaveFailed(false);
    setTemplateBody(body);
    savedBodyRef.current = body;
    update.mutate(updatedTask, { onError: retryDraft });
    updateBody.mutate(
      { taskId: updatedTask.ID, body },
      { onError: retryDraft },
    );
  };

  const loadTemplate = (template: TaskTemplate) => {
    if (!task.ID) return;
    if (!types.data || types.isFetching) {
      toast.error("Task types are still loading. Try again in a moment.");
      return;
    }
    try {
      const body = toValidBody(
        createMarkdownEditor()
          .getApi(MarkdownPlugin)
          .markdown.deserialize(template.body),
      );
      const draft = templateTaskDraft(task, template, body, {
        projectId: Project.ID,
        stages: Stages,
        checklists: Checklists,
        types: types.data,
      });
      setTask(draft);
      templateGeneration.current += 1;
      templateDraftPending.current = true;
      setTemplateSaveFailed(false);
      setTemplateName(template.name || "Untitled template");
      setTemplateBody(body);
      setEditorVersion((value) => value + 1);
      setEditorReady(false);
      editorRef.current = null;
      savedBodyRef.current = body;
      setTemplatePickerOpen(false);
      // Remount from the copied body instead of a programmatic editor change:
      // selection itself must not trigger the editor's autosave callback.
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Could not load template",
      );
    }
  };

  const handleTaskChanges = (updatedTask: Task) => {
    // The task row doesn't exist yet (create still in flight), or the context
    // has already been reset by a close. Either way a PUT would target id 0 and
    // silently update nothing — the local state change is enough.
    if (updatedTask.ID === 0) return;
    if (templateDraftPending.current) {
      persistTemplateDraft(
        updatedTask,
        editorRef.current?.children ?? updatedTask.Body,
      );
      return;
    }
    update.mutate(updatedTask);
  };

  const handleCopyAsMarkdown = () => {
    if (!editorRef.current) return;
    try {
      const bodyMd = serializeMd(editorRef.current);
      const content = task.Name ? `# ${task.Name}\n\n${bodyMd}` : bodyMd;
      navigator.clipboard.writeText(content);
      toast("Copied as markdown");
    } catch {
      toast.error("Failed to copy as markdown");
    }
  };

  const handleCopyAsPlainText = () => {
    const bodyText = extractPlainText(task.Body);
    const content = task.Name ? `${task.Name}\n\n${bodyText}` : bodyText;
    navigator.clipboard.writeText(content);
    toast("Copied as plain text");
  };

  const { isPending, isFetching, data } = useTaskBodyQuery(task.ID, open);
  const handleEditorValueChange = (value: Value) => {
    if (JSON.stringify(value) === JSON.stringify(savedBodyRef.current)) return;
    if (templateBody) setTemplateBody(value);
    setTask({ ...task, Body: value });
    if (task.ID !== 0) {
      if (templateDraftPending.current) {
        persistTemplateDraft({ ...task, Body: value }, value);
        return;
      }
      savedBodyRef.current = value;
      updateBody.mutate({ taskId: task.ID, body: value });
    }
  };

  // Closing the drawer is the commit point for anything still pending: the
  // title (which otherwise only saves on a blur that fires too late) and the
  // body (whose 250ms debounce would never fire). Must run before the wrapper's
  // onOpenChange, which is what resets the task context on the new-task drawer.
  const commitPendingEdits = () => {
    nameRef.current?.commit();

    const editor = editorRef.current;
    if (!editor || task.ID === 0 || task.Type.ViewMode === "chat") return;
    // Compare against the editor's own baseline rather than the fetched body:
    // the plugins normalise on mount (an empty body gains a trailing
    // paragraph), so comparing to the raw fetched value would write on every
    // close even when the body was never touched.
    const body = editor.children;
    if (JSON.stringify(body) !== JSON.stringify(savedBodyRef.current)) {
      if (templateDraftPending.current) {
        persistTemplateDraft(task, body);
        return;
      }
      updateBody.mutate({ taskId: task.ID, body });
    }
  };

  const stage = Stages.find((s) => s.ID === task.Stage)!;

  return (
    <Drawer
      handleOnly={!isMobile}
      repositionInputs={!isMobile}
      direction={isMobile ? "bottom" : "right"}
      open={open}
      onOpenChange={(open) => {
        if (!open) commitPendingEdits();
        setOpen(open);
        onOpenChange(open);
        setPropertiesOpen(!isMobile || task.ID === 0);
        if (!open) {
          editorRef.current = null;
          setEditorReady(false);
          setTemplateBody(null);
          setTemplateName(null);
          setTemplateSaveFailed(false);
          templateGeneration.current += 1;
          templateDraftPending.current = false;
          setTemplatePickerOpen(false);
        }
      }}
    >
      <DrawerTrigger asChild>{children}</DrawerTrigger>
      <DrawerContent className="md:min-w-3xl p-0 overflow-x-visible box-border rounded-lg data-[vaul-drawer-direction=bottom]:h-[calc(100dvh-var(--header-height))] data-[vaul-drawer-direction=bottom]:max-h-dvh">
        <TaskEditorHeader
          setOpen={setOpen}
          onCopyAsMarkdown={handleCopyAsMarkdown}
          onCopyAsPlainText={handleCopyAsPlainText}
          isEditorReady={editorReady && task.Type.ViewMode !== "chat"}
          taskStage={stage}
          onLoadTemplate={
            allowTemplate ? () => setTemplatePickerOpen(true) : undefined
          }
        />
        {allowTemplate && templatePickerOpen && (
          <TaskTemplatePicker
            projectId={Project.ID}
            open={templatePickerOpen}
            onOpenChange={setTemplatePickerOpen}
            onSelect={loadTemplate}
          />
        )}
        {templateName && (
          <p
            role="status"
            className="border-b border-border bg-muted/40 px-6 py-3 text-sm text-muted-foreground"
          >
            Loaded{" "}
            <span className="font-medium text-foreground">{templateName}</span>.
            {templateSaveFailed
              ? " Changes could not be saved. Edit a field to retry."
              : " Edit a field to save this task with these defaults."}
          </p>
        )}
        <div
          className="flex-1 min-h-0 overflow-y-auto"
          data-vaul-no-drag
          onWheel={(e) => e.stopPropagation()}
        >
          <Collapsible open={propertiesOpen} onOpenChange={setPropertiesOpen}>
            <div className="flex items-start gap-1 mx-3 sm:mx-6 mt-3 sm:mt-6">
              <EditableTaskName ref={nameRef} onChange={handleTaskChanges} />
              <CollapsibleTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon"
                  className="shrink-0 self-start"
                >
                  <ChevronDown
                    className={cn(
                      "transition-transform duration-200",
                      propertiesOpen && "rotate-180",
                    )}
                  />
                </Button>
              </CollapsibleTrigger>
            </div>
            {!propertiesOpen && (
              <div className="flex flex-wrap items-center gap-1.5 mx-3 sm:mx-6 pb-2">
                <Badge variant="secondary">
                  <LandPlotIcon className="size-2" />
                  {task.ChecklistName}
                </Badge>

                <Badge
                  variant={task.Assignee !== "" ? "secondary" : "outline"}
                  className={
                    task.Assignee !== "" ? "" : "text-muted-foreground"
                  }
                >
                  <User className="size-2" />
                  {task.Assignee !== "" ? task.Assignee : "—"}
                </Badge>

                <RelativePlannedDateBadge
                  start={task.TimePlannedStart}
                  end={task.TimePlannedEnd}
                  overdue={
                    task.TimePlannedStart !== null &&
                    new Date(task.TimePlannedEnd ?? task.TimePlannedStart) <
                      new Date() &&
                    stage.Type !== "done"
                  }
                />

                <Separator orientation="vertical" className="h-4! sm:mx-2" />

                <TaskRelationshipBadges taskId={task.ID} />
              </div>
            )}
            <CollapsibleContent className="border mx-3 sm:mx-6 pl-3 p-2 sm:p-5 rounded-lg overflow-hidden data-[state=open]:animate-collapsible-down data-[state=closed]:animate-collapsible-up mb-2">
              <section className="flex flex-col gap-2">
                <TaskProperty label="Priority" htmlFor="priority">
                  <SelectTaskPriority onChange={handleTaskChanges} />
                </TaskProperty>

                <TaskProperty label="Type" htmlFor="type">
                  <SelectTaskType onChange={handleTaskChanges} />
                </TaskProperty>

                <TaskProperty label="Stage" htmlFor="stage">
                  <SelectTaskStage onChange={handleTaskChanges} />
                </TaskProperty>

                <TaskProperty label="Checklist" htmlFor="checklist">
                  <SelectChecklist onChange={handleTaskChanges} />
                </TaskProperty>

                <TaskProperty label="Assignee" htmlFor="assignee">
                  <TaskAssignee onChange={handleTaskChanges} />
                </TaskProperty>

                <TaskProperty label="Date" htmlFor="date">
                  <DatePickerInput onChange={handleTaskChanges} />
                </TaskProperty>

                <Separator orientation="horizontal" className="my-2" />

                <TaskRelationshipsCard />
              </section>
            </CollapsibleContent>
          </Collapsible>

          {open && (
            <div className="mx-3 sm:mx-6">
              <TaskOwnershipNotice projectId={Project.ID} taskId={task.ID} />
              <TaskGitHubLinks key={`links-${task.ID}`} taskId={task.ID} />
              <TaskBranches key={task.ID} taskId={task.ID} />
            </div>
          )}

          {open && task.Type.ViewMode === "chat" ? (
            <div className="m-3 sm:m-6">
              <TicketChat key={task.ID} taskId={task.ID} />
            </div>
          ) : open &&
            (task.ID === 0 ||
              (data !== undefined && !isFetching && !isPending)) ? (
            <div data-vaul-no-drag>
              <RichEditor
                key={`${task.ID}-${open}-${editorVersion}`}
                onDebounceChange={handleEditorValueChange}
                onEditorReady={(editor) => {
                  editorRef.current = editor;
                  savedBodyRef.current = editor.children;
                  setEditorReady(true);
                }}
                debounceDuration={250}
                initialValue={templateBody ?? data}
              />
            </div>
          ) : (
            <div className="flex w-full flex-col gap-2 py-4 px-8">
              <Skeleton className="h-4 w-full" />
              <Skeleton className="h-4 w-full" />
              <Skeleton className="h-4 w-3/4" />
            </div>
          )}
        </div>
      </DrawerContent>
    </Drawer>
  );
}
