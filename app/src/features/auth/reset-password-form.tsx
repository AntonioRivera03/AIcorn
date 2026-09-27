import { useState, type FormEvent } from "react";
import { Link, useNavigate } from "@tanstack/react-router";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { AuthCard } from "@/features/auth/auth-card";
import { FormError } from "@/features/auth/form-error";
import { destinationAfterAuth } from "@/features/auth/after-auth";
import { useResetPasswordMutation } from "@/features/auth/queries/auth-mutations";
import { MIN_PASSWORD_LENGTH } from "@/features/auth/password";

export function ResetPasswordForm({ token }: { token?: string }) {
  const [password, setPassword] = useState("");
  const reset = useResetPasswordMutation();
  const navigate = useNavigate();

  const linkClass = "font-medium text-foreground underline-offset-4 hover:underline";
  const requestNewLink = (
    <Link to="/forgot-password" className={linkClass}>
      Request a new link
    </Link>
  );

  if (!token) {
    return (
      <AuthCard
        title="This link is incomplete"
        description="Open the link from your password reset email again, or request a new one."
        footer={requestNewLink}
      >
        <Button asChild variant="outline" className="w-full">
          <Link to="/login">Back to log in</Link>
        </Button>
      </AuthCard>
    );
  }

  const handleSubmit = (event: FormEvent) => {
    event.preventDefault();
    reset.mutate(
      { token, password },
      {
        onSuccess: (me) => {
          toast.success("Password changed. You're signed out everywhere else.");
          navigate({ href: destinationAfterAuth(me), replace: true });
        },
      },
    );
  };

  return (
    <AuthCard
      title="Choose a new password"
      description="Once it's set, every other device signed in to your account is signed out."
      footer={reset.isError ? requestNewLink : undefined}
    >
      <form onSubmit={handleSubmit} className="flex flex-col gap-4">
        <div className="flex flex-col gap-2">
          <Label htmlFor="password">New password</Label>
          <Input
            id="password"
            type="password"
            autoComplete="new-password"
            autoFocus
            required
            minLength={MIN_PASSWORD_LENGTH}
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
          <p className="text-xs text-muted-foreground">
            At least {MIN_PASSWORD_LENGTH} characters.
          </p>
        </div>
        <FormError error={reset.error} />
        <Button type="submit" disabled={reset.isPending}>
          {reset.isPending ? "Saving…" : "Set password"}
        </Button>
      </form>
    </AuthCard>
  );
}
