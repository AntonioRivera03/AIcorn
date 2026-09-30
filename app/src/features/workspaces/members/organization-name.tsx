import { toast } from "sonner";
import { EditableHeader } from "@/components/EditableHeader";
import { useRenameWorkspaceMutation } from "@/features/workspaces/queries/workspace-queries";
import { canManageMembers, type Workspace } from "@/features/workspaces/types";

// The organization's name, editable in place by owners and admins.
export function OrganizationName({ workspace }: { workspace: Workspace }) {
  const rename = useRenameWorkspaceMutation(workspace.id);

  if (!canManageMembers(workspace)) {
    return <h2 className="p-1 text-xl">{workspace.name}</h2>;
  }

  return (
    <EditableHeader
      value={workspace.name}
      placeholder="Organization name"
      className="text-xl"
      setValue={(name) => {
        const trimmed = name.trim();
        if (trimmed === "" || trimmed === workspace.name) return;
        rename.mutate(trimmed, {
          onError: (err) => toast.error(err.message || "Failed to rename the organization."),
        });
      }}
    />
  );
}
