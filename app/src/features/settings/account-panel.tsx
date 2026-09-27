import { useNavigate } from "@tanstack/react-router";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { EditableHeader } from "@/components/EditableHeader";
import {
  useLogoutMutation,
  useRenameAccountMutation,
} from "@/features/auth/queries/auth-mutations";
import { ChangePasswordForm } from "@/features/settings/change-password-form";
import { SectionHeading } from "@/features/settings/section-heading";
import { useWorkspace } from "@/features/workspaces/workspace-context";

export function AccountPanel() {
  const { account } = useWorkspace();
  const rename = useRenameAccountMutation();
  const logout = useLogoutMutation();
  const navigate = useNavigate();

  const saveName = (name: string) => {
    const trimmed = name.trim();
    if (trimmed === "" || trimmed === account.name) return;
    rename.mutate(trimmed, {
      onError: (err) => toast.error(err.message || "Failed to update your name."),
    });
  };

  return (
    <div className="flex flex-col gap-8">
      <section className="flex flex-col gap-2">
        <SectionHeading
          title="Name"
          description="How you appear to other members, including as a task assignee."
        />
        <EditableHeader
          value={account.name}
          placeholder="Your name"
          className="text-base w-fit min-w-48"
          setValue={saveName}
        />
      </section>

      <section className="flex flex-col gap-2">
        <SectionHeading title="Email" description="You sign in with this address, and invites are sent to it." />
        <p className="px-1 text-sm">{account.email}</p>
      </section>

      <section className="flex flex-col gap-3">
        <SectionHeading
          title="Password"
          description="Changing it signs you out on every other device."
        />
        <ChangePasswordForm />
      </section>

      <section className="flex flex-col items-start gap-3">
        <SectionHeading title="Session" description="Sign out of Aycorn on this browser." />
        <Button
          variant="outline"
          size="sm"
          disabled={logout.isPending}
          onClick={() =>
            logout.mutate(undefined, {
              onSuccess: () => navigate({ to: "/login" }),
              onError: (err) => toast.error(err.message || "Failed to log out."),
            })
          }
        >
          Log out
        </Button>
      </section>
    </div>
  );
}
