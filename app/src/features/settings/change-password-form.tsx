import { useState, type FormEvent } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useChangePasswordMutation } from "@/features/auth/queries/auth-mutations";
import { MIN_PASSWORD_LENGTH } from "@/features/auth/password";

// A password is the one account field that can't save on blur: the change
// needs the current password alongside it.
export function ChangePasswordForm() {
  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");
  const change = useChangePasswordMutation();

  const handleSubmit = (event: FormEvent) => {
    event.preventDefault();
    change.mutate(
      { current, new: next },
      {
        onSuccess: () => {
          setCurrent("");
          setNext("");
          toast.success("Password changed. Other devices were signed out.");
        },
        onError: (err) => toast.error(err.message || "Failed to change your password."),
      },
    );
  };

  return (
    <form onSubmit={handleSubmit} className="flex max-w-80 flex-col gap-3">
      <div className="flex flex-col gap-2">
        <Label htmlFor="current-password">Current password</Label>
        <Input
          id="current-password"
          type="password"
          autoComplete="current-password"
          required
          value={current}
          onChange={(e) => setCurrent(e.target.value)}
        />
      </div>
      <div className="flex flex-col gap-2">
        <Label htmlFor="new-password">New password</Label>
        <Input
          id="new-password"
          type="password"
          autoComplete="new-password"
          required
          minLength={MIN_PASSWORD_LENGTH}
          value={next}
          onChange={(e) => setNext(e.target.value)}
        />
        <p className="text-xs text-muted-foreground">
          At least {MIN_PASSWORD_LENGTH} characters.
        </p>
      </div>
      <Button type="submit" variant="outline" size="sm" className="self-start" disabled={change.isPending}>
        {change.isPending ? "Changing…" : "Change password"}
      </Button>
    </form>
  );
}
