import type { Value } from "platejs";
import type { ChecklistTask, TaskType } from "@/types/types";
import type { TaskTemplate } from "@/features/jobs/use-jobs";

// Loading a template is a local projection. Identity, dates and the template
// itself are preserved; persistence belongs to the user's next ordinary edit.
export function templateTaskDraft(
  task: ChecklistTask,
  template: TaskTemplate,
  body: Value,
  context: {
    projectId: number;
    checklists: { ID: number; Name: string }[];
    stages: { ID: number }[];
    types: TaskType[];
  },
): ChecklistTask {
  if (template.projectId !== context.projectId)
    throw new Error("This template belongs to another project.");
  const priority = (["Urgent", "High", "Medium", "Low"] as const).find(
    (value) => value === template.priority,
  );
  if (!priority) throw new Error("This template has an invalid priority.");
  const checklist = context.checklists.find(
    (item) => item.ID === template.checklistId,
  );
  const stage = context.stages.find((item) => item.ID === template.stageId);
  const type = context.types.find((item) => item.ID === template.typeId);
  if (
    (template.checklistId && !checklist) ||
    (template.stageId && !stage) ||
    (template.typeId && !type)
  ) {
    throw new Error(
      "This template has an unavailable checklist, stage or type. Update it in Jobs & templates first.",
    );
  }
  return {
    ...task,
    Name: template.title,
    Body: structuredClone(body),
    Priority: priority,
    Assignee: template.assignee,
    Checklist: checklist?.ID ?? task.Checklist,
    ChecklistName: checklist?.Name ?? task.ChecklistName,
    Stage: stage?.ID ?? task.Stage,
    Type: type ? { ...type } : task.Type,
  };
}
