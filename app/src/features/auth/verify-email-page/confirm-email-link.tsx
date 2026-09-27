import { useEffect, useRef } from "react";
import { Link, useNavigate } from "@tanstack/react-router";
import { Button } from "@/components/ui/button";
import { AuthCard } from "@/features/auth/auth-card";
import { destinationAfterAuth } from "@/features/auth/after-auth";
import { useVerifyEmailMutation } from "@/features/auth/queries/auth-mutations";
import { useMeQuery } from "@/features/auth/queries/me-query";

// Confirms the email as soon as the link opens. Signed in as that account, it
// carries on into the app; otherwise (say, another browser) it says so and
// offers the way on. The mutation refreshes the account before it succeeds.
export function ConfirmEmailLink({ token }: { token: string }) {
  const verify = useVerifyEmailMutation();
  const { data: me } = useMeQuery();
  const navigate = useNavigate();
  // A link works once, so it must not be submitted twice (e.g. by React's
  // development double-run of effects).
  const submitted = useRef(false);

  useEffect(() => {
    if (submitted.current) return;
    submitted.current = true;
    verify.mutate(token);
  }, [token, verify]);

  useEffect(() => {
    if (verify.isSuccess && me?.account.emailVerified) {
      navigate({ href: destinationAfterAuth(me), replace: true });
    }
  }, [verify.isSuccess, me, navigate]);

  if (verify.isError) {
    return (
      <AuthCard
        title="This link can't be used"
        description="It may have expired, or been replaced by a newer one. Links work once."
      >
        <Button asChild variant="outline" className="w-full">
          <Link to="/verify-email">Get a new link</Link>
        </Button>
      </AuthCard>
    );
  }

  if (verify.isSuccess && !me?.account.emailVerified) {
    return (
      <AuthCard
        title="Email confirmed"
        description={me ? "You can carry on in Aycorn." : "Log in to start using Aycorn."}
      >
        <Button asChild className="w-full" autoFocus>
          {me ? <a href={destinationAfterAuth(me)}>Continue</a> : <Link to="/login">Log in</Link>}
        </Button>
      </AuthCard>
    );
  }

  return (
    <AuthCard title="Confirming your email…">
      <p className="text-sm text-muted-foreground">This only takes a moment.</p>
    </AuthCard>
  );
}
