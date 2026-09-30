import { useState } from "react";
import { Link } from "@tanstack/react-router";
import { Building2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Table,
  TableBody,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  useInvitesQuery,
  useMembersQuery,
} from "@/features/workspaces/queries/workspace-queries";
import { canManageMembers } from "@/features/workspaces/types";
import { useWorkspace } from "@/features/workspaces/workspace-context";
import { DeleteOrganizationDialog } from "@/features/workspaces/members/delete-organization-dialog";
import { InviteMemberForm } from "@/features/workspaces/members/invite-member-form";
import { MemberRow } from "@/features/workspaces/members/member-row";
import { OrganizationName } from "@/features/workspaces/members/organization-name";
import { PendingInviteRow } from "@/features/workspaces/members/pending-invite-row";
import { SectionHeading } from "@/features/settings/section-heading";

export function MembersPanel() {
  const { account, workspace, workspaces } = useWorkspace();
  const manager = canManageMembers(workspace);
  const { data: members = [], isPending } = useMembersQuery(workspace.id);
  const { data: invites = [] } = useInvitesQuery(workspace.id, manager);
  const [deleteOpen, setDeleteOpen] = useState(false);

  if (workspace.kind === "personal") {
    return (
      <section className="flex flex-col items-start gap-3 rounded-lg border border-dashed p-6">
        <Building2 className="size-5 text-muted-foreground" />
        <SectionHeading
          title="Your personal workspace is just you"
          description="To work with other people, create an organization or join one with an invite code. Its projects, workflows, and AI agents are shared with everyone in it."
        />
        <Button asChild variant="outline" size="sm">
          <Link to="/onboarding" search={{ step: "organization" }}>
            Create or join an organization
          </Link>
        </Button>
      </section>
    );
  }

  const fallbackWorkspaceId =
    workspaces.find((w) => w.kind === "personal")?.id ?? workspace.id;

  return (
    <div className="flex flex-col gap-8">
      <section className="flex flex-col gap-1">
        <OrganizationName workspace={workspace} />
      </section>

      {manager && (
        <section className="flex flex-col gap-3">
          <SectionHeading
            title="Invite people"
            description="Invites are emailed with a link and a one-time code, and only work for the address you enter."
          />
          <InviteMemberForm workspaceId={workspace.id} />
        </section>
      )}

      <section className="flex flex-col gap-3">
        <SectionHeading
          title="Members"
          description="Everyone here can see the organization's projects and be assigned its tasks."
        />
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead className="w-36">Role</TableHead>
              <TableHead className="w-24" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {!isPending &&
              members.map((member) => (
                <MemberRow
                  key={member.accountId}
                  member={member}
                  workspace={workspace}
                  isSelf={member.accountId === account.id}
                  fallbackWorkspaceId={fallbackWorkspaceId}
                />
              ))}
          </TableBody>
        </Table>
      </section>

      {manager && invites.length > 0 && (
        <section className="flex flex-col gap-3">
          <SectionHeading
            title="Pending invites"
            description="Invites expire after 7 days. Inviting the same email again replaces its invite."
          />
          <Table>
            <TableBody>
              {invites.map((invite) => (
                <PendingInviteRow key={invite.id} invite={invite} workspaceId={workspace.id} />
              ))}
            </TableBody>
          </Table>
        </section>
      )}

      {workspace.role === "owner" && (
        <section className="flex flex-col items-start gap-3 rounded-lg border border-destructive/40 p-4">
          <SectionHeading
            title="Delete organization"
            description="Removes the organization and all of its work for every member. Only owners can do this."
          />
          <Button variant="destructive" size="sm" onClick={() => setDeleteOpen(true)}>
            Delete organization
          </Button>
          <DeleteOrganizationDialog
            workspace={workspace}
            fallbackWorkspaceId={fallbackWorkspaceId}
            open={deleteOpen}
            onOpenChange={setDeleteOpen}
          />
        </section>
      )}
    </div>
  );
}
