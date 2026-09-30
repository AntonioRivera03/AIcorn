import { Link } from "@tanstack/react-router";
import { Button } from "@/components/ui/button";
import { AuthCard } from "@/features/auth/auth-card";
import { useInvitePreviewQuery } from "@/features/workspaces/queries/workspace-queries";

// What someone sees when they open an invite link before signing in. Signed-in
// users skip this page and land on the join form with the code filled in.
export function InviteLanding({ code }: { code: string }) {
  const { data: invite, isPending, error } = useInvitePreviewQuery(code);

  if (isPending) return null;

  if (error || !invite) {
    return (
      <AuthCard
        title="This invite can't be used"
        description="It may have expired, been revoked, or already been accepted. Ask the organization to send a new one."
      >
        <Button asChild variant="outline" className="w-full">
          <Link to="/">Go to Aycorn</Link>
        </Button>
      </AuthCard>
    );
  }

  return (
    <AuthCard
      title={`Join ${invite.workspaceName}`}
      description={
        <>
          {invite.invitedBy || "Someone"} invited{" "}
          <span className="font-medium text-foreground">{invite.email}</span>{" "}
          to their organization on Aycorn.
        </>
      }
    >
      <div className="flex flex-col gap-2">
        <Button asChild autoFocus>
          <Link to="/signup" search={{ invite: code, email: invite.email }}>
            Create an account
          </Link>
        </Button>
        <Button asChild variant="outline">
          <Link to="/login" search={{ invite: code }}>
            I already have an account
          </Link>
        </Button>
      </div>
    </AuthCard>
  );
}
