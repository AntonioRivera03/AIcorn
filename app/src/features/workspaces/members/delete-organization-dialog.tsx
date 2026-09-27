import { useState, type FormEvent } from "react";
import { toast } from "sonner";
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useDeleteOrganizationMutation } from "@/features/workspaces/queries/workspace-queries";
import type { Workspace } from "@/features/workspaces/types";

type Props = {
  workspace: Workspace;
  fallbackWorkspaceId: number;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

// Deleting an organization takes everyone's work with it, so the name must be
// typed out, not just confirmed with a click.
export function DeleteOrganizationDialog({ workspace, fallbackWorkspaceId, open, onOpenChange }: Props) {
  const [typedName, setTypedName] = useState("");
  const remove = useDeleteOrganizationMutation(workspace.id, fallbackWorkspaceId);
  const confirmed = typedName.trim() === workspace.name;

  const handleSubmit = (event: FormEvent) => {
    event.preventDefault();
    if (!confirmed) return;
    remove.mutate(undefined, {
      onError: (err) => toast.error(err.message || "Failed to delete the organization."),
    });
  };

  const handleOpenChange = (next: boolean) => {
    if (!next) setTypedName("");
    onOpenChange(next);
  };

  return (
    <AlertDialog open={open} onOpenChange={handleOpenChange}>
      <AlertDialogContent>
        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          <AlertDialogHeader>
            <AlertDialogTitle>Delete {workspace.name}?</AlertDialogTitle>
            <AlertDialogDescription>
              Its projects, tasks, workflows, and agents are removed for every
              member, and its branch previews are shut down. This can't be
              undone from Aycorn.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <div className="flex flex-col gap-2">
            <Label htmlFor="confirm-organization-name" className="block">
              Type <span className="font-semibold">{workspace.name}</span> to confirm
            </Label>
            <Input
              id="confirm-organization-name"
              autoComplete="off"
              autoFocus
              value={typedName}
              onChange={(e) => setTypedName(e.target.value)}
            />
          </div>
          <AlertDialogFooter>
            <AlertDialogCancel type="button">Cancel</AlertDialogCancel>
            <Button type="submit" variant="destructive" disabled={!confirmed || remove.isPending}>
              {remove.isPending ? "Deleting…" : "Delete organization"}
            </Button>
          </AlertDialogFooter>
        </form>
      </AlertDialogContent>
    </AlertDialog>
  );
}
