import { LockKeyhole } from "lucide-react";
import { useTaskOwnership } from "./queries/use-task-ownership";

export function TaskOwnershipNotice({
  projectId,
  taskId,
}: {
  projectId: number;
  taskId: number;
}) {
  const owners = useTaskOwnership(projectId);
  const owner = owners.data?.find((item) => item.taskId === taskId);
  if (!owner) return null;
  return (
    <p
      role="status"
      className="my-2 flex items-center gap-2 rounded-md border px-3 py-2 text-sm text-muted-foreground"
    >
      <LockKeyhole className="size-4 shrink-0" />
      {owner.name} is managing this task ({owner.state}). Other agents can start
      when it releases the task.
    </p>
  );
}
