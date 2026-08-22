import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { useTaskMutation } from "@/queries/useTaskMutation";
import type { Task } from "@/types/types";
import { toast } from "sonner";

type Props = {
  task: Task;
  projectId: number;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onDeleted?: () => void;
};

export function DeleteTaskDialog({
  task,
  projectId,
  open,
  onOpenChange,
  onDeleted,
}: Props) {
  const { deleteTask } = useTaskMutation(projectId);
  const displayName = task.Name === "" ? "Untitled Task" : task.Name;

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Delete task</AlertDialogTitle>
          <AlertDialogDescription>
            Delete <b>{displayName}</b>? This action cannot be undone.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction
            variant="destructive"
            onClick={() =>
              deleteTask.mutate(task.ID, {
                onSuccess: () => {
                  toast(`Deleted ${displayName} successfully.`);
                  onDeleted?.();
                },
                onError: () => toast.error("Failed deleting task."),
              })
            }
          >
            Delete
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
