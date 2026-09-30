import { useState, type FormEvent } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useInviteMemberMutation } from "@/features/workspaces/queries/workspace-queries";
import type { CreatedInvite, Role } from "@/features/workspaces/types";
import { InviteLinkNotice } from "@/features/workspaces/members/invite-link-notice";
import { RoleSelect } from "@/features/workspaces/members/role-select";

export function InviteMemberForm({ workspaceId }: { workspaceId: number }) {
  const [email, setEmail] = useState("");
  const [role, setRole] = useState<Role>("member");
  const [unsent, setUnsent] = useState<CreatedInvite | null>(null);
  const invite = useInviteMemberMutation(workspaceId);

  const handleSubmit = (event: FormEvent) => {
    event.preventDefault();
    invite.mutate(
      { email, role },
      {
        onSuccess: (created) => {
          setEmail("");
          if (created.emailSent) {
            setUnsent(null);
            toast.success(`Invite sent to ${created.email}.`);
          } else {
            setUnsent(created);
          }
        },
        onError: (err) => toast.error(err.message || "Failed to send the invite."),
      },
    );
  };

  return (
    <div className="flex flex-col gap-3">
      <form onSubmit={handleSubmit} className="flex flex-col gap-2 sm:flex-row">
        <Input
          type="email"
          placeholder="name@example.com"
          aria-label="Email to invite"
          required
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          className="sm:max-w-80"
        />
        <div className="flex gap-2">
          <RoleSelect value={role} roles={["member", "admin"]} onChange={setRole} />
          <Button type="submit" disabled={invite.isPending}>
            {invite.isPending ? "Inviting…" : "Invite"}
          </Button>
        </div>
      </form>
      {unsent && <InviteLinkNotice invite={unsent} />}
    </div>
  );
}
