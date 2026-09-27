import { useState, type FormEvent } from "react";
import { Link, useNavigate } from "@tanstack/react-router";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { AuthCard } from "@/features/auth/auth-card";
import { FormError } from "@/features/auth/form-error";
import { destinationAfterAuth } from "@/features/auth/after-auth";
import { useSignupMutation } from "@/features/auth/queries/auth-mutations";
import { MIN_PASSWORD_LENGTH } from "@/features/auth/password";

type Props = {
  invite?: string;
  email?: string;
};

export function SignupForm({ invite, email: invitedEmail = "" }: Props) {
  const [name, setName] = useState("");
  const [email, setEmail] = useState(invitedEmail);
  const [password, setPassword] = useState("");
  const signup = useSignupMutation();
  const navigate = useNavigate();

  const handleSubmit = (event: FormEvent) => {
    event.preventDefault();
    signup.mutate(
      { name, email, password, invite },
      { onSuccess: (me) => navigate({ href: destinationAfterAuth(me, { invite }) }) },
    );
  };

  return (
    <AuthCard
      title="Create your account"
      description={
        invite
          ? "Sign up with the email your invite was sent to."
          : "Start with your own space. You can join or create an organization anytime."
      }
      footer={
        <span>
          Already have an account?{" "}
          <Link
            to="/login"
            search={{ invite }}
            className="font-medium text-foreground underline-offset-4 hover:underline"
          >
            Log in
          </Link>
        </span>
      }
    >
      <form onSubmit={handleSubmit} className="flex flex-col gap-4">
        <div className="flex flex-col gap-2">
          <Label htmlFor="name">Name</Label>
          <Input
            id="name"
            autoComplete="name"
            autoFocus
            required
            maxLength={100}
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
        </div>
        <div className="flex flex-col gap-2">
          <Label htmlFor="email">Email</Label>
          <Input
            id="email"
            type="email"
            autoComplete="email"
            required
            value={email}
            onChange={(e) => setEmail(e.target.value)}
          />
        </div>
        <div className="flex flex-col gap-2">
          <Label htmlFor="password">Password</Label>
          <Input
            id="password"
            type="password"
            autoComplete="new-password"
            required
            minLength={MIN_PASSWORD_LENGTH}
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
          <p className="text-xs text-muted-foreground">
            At least {MIN_PASSWORD_LENGTH} characters.
          </p>
        </div>
        <FormError error={signup.error} />
        <Button type="submit" disabled={signup.isPending}>
          {signup.isPending ? "Creating account…" : "Create account"}
        </Button>
      </form>
    </AuthCard>
  );
}
