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

// Changing an Official link deletes Aycorn's clone, and the agent branches
// that only exist there, so it's confirmed like any other delete.
export function RemoveCloneDialog({
  open,
  repository,
  switching,
  onCancel,
  onConfirm,
}: {
  open: boolean;
  repository: string;
  switching: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  return (
    <AlertDialog open={open} onOpenChange={(next) => !next && onCancel()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>
            Remove Aycorn's copy of {repository}?
          </AlertDialogTitle>
          <AlertDialogDescription>
            {switching
              ? "Switching to Personal deletes the clone Aycorn keeps on the server."
              : "Linking a different repository deletes the clone Aycorn keeps on the server."}{" "}
            Agent branches in it that you haven't copied elsewhere are lost. The
            repository on GitHub isn't touched.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction
            variant="destructive"
            onClick={(event) => {
              // Close through onConfirm, so onOpenChange only reports a cancel.
              event.preventDefault();
              onConfirm();
            }}
          >
            Remove clone
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
