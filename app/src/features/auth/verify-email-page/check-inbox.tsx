import { useEffect } from "react";
import { useNavigate } from "@tanstack/react-router";
import { MailCheck } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { AuthCard } from "@/features/auth/auth-card";
import { destinationAfterAuth } from "@/features/auth/after-auth";
import {
  useLogoutMutation,
  useResendVerificationMutation,
} from "@/features/auth/queries/auth-mutations";
import { useMeQuery } from "@/features/auth/queries/me-query";

// How often to check whether the link was opened elsewhere (another tab, or
// the phone the email was read on).
const POLL_INTERVAL = 5_000;

export function CheckInbox() {
  const { data: me } = useMeQuery({ refetchInterval: POLL_INTERVAL });
  const resend = useResendVerificationMutation();
  const logout = useLogoutMutation();
  const navigate = useNavigate();

  useEffect(() => {
    if (me?.account.emailVerified) navigate({ href: destinationAfterAuth(me) });
  }, [me, navigate]);

  if (!me) return null;

  const resendEmail = () =>
    resend.mutate(undefined, {
      onSuccess: () => toast.success(`Sent a new link to ${me.account.email}.`),
      onError: (err) => toast.error(err.message || "Failed to send the email."),
    });

  const useAnotherAccount = () =>
    logout.mutate(undefined, {
      onSuccess: () => navigate({ to: "/signup" }),
      onError: (err) => toast.error(err.message || "Failed to log out."),
    });

  const linkClass = "font-medium text-foreground underline-offset-4 hover:underline";

  return (
    <AuthCard
      title="Confirm your email"
      description={
        <>
          We sent a link to{" "}
          <span className="font-medium text-foreground">{me.account.email}</span>.
          Open it to start using Aycorn. This page moves on by itself once
          you have.
        </>
      }
      footer={
        <span>
          Wrong address?{" "}
          <button
            type="button"
            className={linkClass}
            disabled={logout.isPending}
            onClick={useAnotherAccount}
          >
            Sign up again
          </button>
        </span>
      }
    >
      <div className="flex flex-col items-center gap-4">
        <MailCheck className="size-8 text-muted-foreground" />
        <Button
          variant="outline"
          className="w-full"
          disabled={resend.isPending}
          onClick={resendEmail}
        >
          {resend.isPending ? "Sending…" : "Send the link again"}
        </Button>
        <p className="text-center text-xs text-muted-foreground">
          The link expires after 48 hours. Check your spam folder if it
          hasn't arrived.
        </p>
      </div>
    </AuthCard>
  );
}
