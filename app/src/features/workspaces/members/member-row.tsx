import { useState } from "react";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { TableCell, TableRow } from "@/components/ui/table";
import { TruncatedText } from "@/components/truncated-text";
import {
  useRemoveMemberMutation,
  useSetMemberRoleMutation,
} from "@/features/workspaces/queries/workspace-queries";
import type { Member, Role, Workspace } from "@/features/workspaces/types";
import { ConfirmDialog } from "@/features/workspaces/members/confirm-dialog";
import { ROLE_LABELS } from "@/features/workspaces/members/role-labels";
import { RoleSelect } from "@/features/workspaces/members/role-select";

type Props = {
  member: Member;
  workspace: Workspace;
  isSelf: boolean;
  onLeft: () => void;
};

// Mirrors the server's rules: owners change roles and remove anyone, admins
// remove members, and anyone can leave.
const canRemove = (workspace: Workspace, member: Member, isSelf: boolean) =>
  isSelf ||
  workspace.role === "owner" ||
  (workspace.role === "admin" && member.role === "member");

export function MemberRow({ member, workspace, isSelf, onLeft }: Props) {
  const [confirmOpen, setConfirmOpen] = useState(false);
  const setRole = useSetMemberRoleMutation(workspace.id);
  const remove = useRemoveMemberMutation(workspace.id);

  const changeRole = (role: Role) =>
    setRole.mutate(
      { accountId: member.accountId, role },
      { onError: (err) => toast.error(err.message || "Failed to change the role.") },
    );

  const confirmRemove = () =>
    remove.mutate(member.accountId, {
      onSuccess: () => {
        if (isSelf) onLeft();
        else toast.success(`Removed ${member.name}.`);
      },
      onError: (err) => toast.error(err.message || "Failed to remove the member."),
    });

  return (
    <>
      <TableRow>
        <TableCell className="max-w-48">
          <div className="flex min-w-0 flex-col">
            <TruncatedText text={isSelf ? `${member.name} (you)` : member.name} className="font-medium" />
            <TruncatedText text={member.email} className="text-xs text-muted-foreground" />
          </div>
        </TableCell>
        <TableCell>
          {workspace.role === "owner" ? (
            <RoleSelect
              value={member.role}
              roles={["owner", "admin", "member"]}
              onChange={changeRole}
              disabled={setRole.isPending}
            />
          ) : (
            <Badge variant="outline">{ROLE_LABELS[member.role]}</Badge>
          )}
        </TableCell>
        <TableCell className="text-right">
          {canRemove(workspace, member, isSelf) && (
            <Button
              size="sm"
              variant="ghost"
              className="text-destructive hover:text-destructive"
              disabled={remove.isPending}
              onClick={() => setConfirmOpen(true)}
            >
              {isSelf ? "Leave" : "Remove"}
            </Button>
          )}
        </TableCell>
      </TableRow>
      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title={isSelf ? `Leave ${workspace.name}?` : `Remove ${member.name}?`}
        description={
          isSelf
            ? "You'll lose access to its projects until someone invites you again."
            : `${member.name} will lose access to ${workspace.name}'s projects. Tasks assigned to them stay assigned.`
        }
        confirmLabel={isSelf ? "Leave" : "Remove"}
        onConfirm={confirmRemove}
      />
    </>
  );
}
