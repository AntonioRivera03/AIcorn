import { useState, type FormEvent } from "react";
import { Link } from "@tanstack/react-router";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { AuthCard } from "@/features/auth/auth-card";
import { FormError } from "@/features/auth/form-error";
import { useRequestPasswordResetMutation } from "@/features/auth/queries/auth-mutations";

export function ForgotPasswordForm() {
  const [email, setEmail] = useState("");
  const request = useRequestPasswordResetMutation();

  const handleSubmit = (event: FormEvent) => {
    event.preventDefault();
    request.mutate(email.trim());
  };

  const backToLogin = (
    <Link to="/login" className="font-medium text-foreground underline-offset-4 hover:underline">
      Back to log in
    </Link>
  );

  // The server answers the same whether or not the address has an account,
  // so neither does this page.
  if (request.data) {
    return (
      <AuthCard
        title={request.data.emailSent ? "Check your email" : "Ask your Aycorn admin"}
        description={
          request.data.emailSent ? (
            <>
              If <span className="font-medium text-foreground">{request.variables}</span>{" "}
              has an account, we sent it a link to choose a new password. The link
              expires in 1 hour.
            </>
          ) : (
            "This server can't send email, so the reset link was written to the server's log instead. Ask whoever runs this Aycorn server for it."
          )
        }
        footer={backToLogin}
      >
        <Button variant="outline" className="w-full" onClick={() => request.reset()}>
          Use a different email
        </Button>
      </AuthCard>
    );
  }

  return (
    <AuthCard
      title="Reset your password"
      description="Enter the email you sign in with and we'll send you a link to choose a new password."
      footer={backToLogin}
    >
      <form onSubmit={handleSubmit} className="flex flex-col gap-4">
        <div className="flex flex-col gap-2">
          <Label htmlFor="email">Email</Label>
          <Input
            id="email"
            type="email"
            autoComplete="email"
            autoFocus
            required
            value={email}
            onChange={(e) => setEmail(e.target.value)}
          />
        </div>
        <FormError error={request.error} />
        <Button type="submit" disabled={request.isPending}>
          {request.isPending ? "Sending…" : "Send reset link"}
        </Button>
      </form>
    </AuthCard>
  );
}
