import { useContext } from "react";
import { ProjectContext } from "@/contexts/project/ProjectContext";
import type { Names } from "@/features/project-chat/activity";
import type { MentionTask } from "@/features/project-chat/mentions";

// Task titles and stage names for the ids in Chatter's tool calls.
export function useChatNames(tasks: MentionTask[] | undefined): Names {
  const { Stages } = useContext(ProjectContext);
  const titles = new Map((tasks ?? []).map((task) => [task.id, task.title]));
  const stages = new Map(Stages.map((stage) => [stage.ID, stage.Name]));
  return {
    task: (id) => titles.get(id) || undefined,
    stage: (id) => stages.get(id) || undefined,
  };
}
