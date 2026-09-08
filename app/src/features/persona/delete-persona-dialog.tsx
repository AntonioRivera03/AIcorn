import { toast } from "sonner";
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
import { usePersonaMutations } from "@/features/persona/queries/use-persona-mutations";
import type { Persona } from "@/types/types";

type DeletePersonaDialogProps = {
  persona: Persona;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onDeleted: () => void;
};

export function DeletePersonaDialog({
  persona,
  open,
  onOpenChange,
  onDeleted,
}: DeletePersonaDialogProps) {
  const { deletePersona } = usePersonaMutations(persona.ID);

  const handleDelete = () => {
    deletePersona.mutate(persona.ID, {
      onSuccess: () => {
        toast.success(`Deleted ${persona.Name || "Untitled Persona"}.`);
        onDeleted();
      },
      onError: () => toast.error("Failed to delete persona."),
    });
  };

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Delete {persona.Name || "Untitled Persona"}?</AlertDialogTitle>
          <AlertDialogDescription>
            The saved instructions will be deleted. Previous AI runs will remain available.
            This action cannot be undone.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction
            variant="destructive"
            disabled={deletePersona.isPending}
            onClick={handleDelete}
          >
            Delete agent
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
