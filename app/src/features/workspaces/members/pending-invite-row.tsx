import { useState } from "react";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { TableCell, TableRow } from "@/components/ui/table";
import { TruncatedText } from "@/components/truncated-text";
import { useRevokeInviteMutation } from "@/features/workspaces/queries/workspace-queries";
import type { Invite } from "@/features/workspaces/types";
import { ConfirmDialog } from "@/features/workspaces/members/confirm-dialog";
import { ROLE_LABELS } from "@/features/workspaces/members/role-labels";

const formatDate = (value: string) =>
  new Date(value).toLocaleDateString(undefined, { month: "short", day: "numeric" });

export function PendingInviteRow({ invite, workspaceId }: { invite: Invite; workspaceId: number }) {
  const [confirmOpen, setConfirmOpen] = useState(false);
  const revoke = useRevokeInviteMutation(workspaceId);

  const confirmRevoke = () =>
    revoke.mutate(invite.id, {
      onSuccess: () => toast.success(`Revoked the invite for ${invite.email}.`),
      onError: (err) => toast.error(err.message || "Failed to revoke the invite."),
    });

  return (
    <>
      <TableRow>
        <TableCell className="max-w-48">
          <div className="flex min-w-0 flex-col">
            <TruncatedText text={invite.email} className="font-medium" />
            <span className="text-xs text-muted-foreground">
              Invited by {invite.invitedBy || "a former member"} · expires {formatDate(invite.timeExpires)}
            </span>
          </div>
        </TableCell>
        <TableCell>
          <Badge variant="outline">{ROLE_LABELS[invite.role]}</Badge>
        </TableCell>
        <TableCell className="text-right">
          <Button
            size="sm"
            variant="ghost"
            className="text-destructive hover:text-destructive"
            disabled={revoke.isPending}
            onClick={() => setConfirmOpen(true)}
          >
            Revoke
          </Button>
        </TableCell>
      </TableRow>
      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title={`Revoke the invite for ${invite.email}?`}
        description="The link and code in their email will stop working. You can invite them again later."
        confirmLabel="Revoke"
        onConfirm={confirmRevoke}
      />
    </>
  );
}
