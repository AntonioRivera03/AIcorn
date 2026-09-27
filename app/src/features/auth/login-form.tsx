import { useState, type FormEvent } from "react";
import { Link, useNavigate } from "@tanstack/react-router";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { AuthCard } from "@/features/auth/auth-card";
import { FormError } from "@/features/auth/form-error";
import { destinationAfterAuth } from "@/features/auth/after-auth";
import { useLoginMutation } from "@/features/auth/queries/auth-mutations";

type Props = {
  redirect?: string;
  invite?: string;
};

export function LoginForm({ redirect, invite }: Props) {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const login = useLoginMutation();
  const navigate = useNavigate();

  const handleSubmit = (event: FormEvent) => {
    event.preventDefault();
    login.mutate(
      { email, password },
      {
        onSuccess: (me) =>
          navigate({ href: destinationAfterAuth(me, { redirect, invite }) }),
      },
    );
  };

  return (
    <AuthCard
      title="Log in"
      description="Welcome back to Aycorn."
      footer={
        <span>
          New here?{" "}
          <Link
            to="/signup"
            search={{ invite }}
            className="font-medium text-foreground underline-offset-4 hover:underline"
          >
            Create an account
          </Link>
        </span>
      }
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
        <div className="flex flex-col gap-2">
          <Label htmlFor="password">Password</Label>
          <Input
            id="password"
            type="password"
            autoComplete="current-password"
            required
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
        </div>
        <FormError error={login.error} />
        <Button type="submit" disabled={login.isPending}>
          {login.isPending ? "Logging in…" : "Log in"}
        </Button>
        <Link
          to="/forgot-password"
          className="self-center text-sm text-muted-foreground underline-offset-4 hover:text-foreground hover:underline"
        >
          Forgot your password?
        </Link>
      </form>
    </AuthCard>
  );
}
