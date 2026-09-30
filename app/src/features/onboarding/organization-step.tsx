import { useState, type FormEvent, type ReactNode } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { AuthCard } from "@/features/auth/auth-card";
import { FormError } from "@/features/auth/form-error";
import {
  useAcceptInviteMutation,
  useCreateOrganizationMutation,
} from "@/features/workspaces/queries/workspace-queries";
import type { Workspace } from "@/features/workspaces/types";

type Props = {
  initialCode?: string;
  footer: ReactNode;
  onJoined: (workspace: Workspace) => void;
};

export function OrganizationStep({ initialCode = "", footer, onJoined }: Props) {
  const [code, setCode] = useState(initialCode);
  const [name, setName] = useState("");
  const accept = useAcceptInviteMutation();
  const create = useCreateOrganizationMutation();
  const pending = accept.isPending || create.isPending;

  const handleJoin = (event: FormEvent) => {
    event.preventDefault();
    create.reset();
    accept.mutate(code, { onSuccess: onJoined });
  };

  const handleCreate = (event: FormEvent) => {
    event.preventDefault();
    accept.reset();
    create.mutate(name, { onSuccess: onJoined });
  };

  return (
    <AuthCard
      title="Join or create an organization"
      description="Organizations share projects, workflows, and AI agents between their members."
      className="max-w-md"
      footer={footer}
    >
      <div className="flex flex-col gap-6">
        <form onSubmit={handleJoin} className="flex flex-col gap-2">
          <Label htmlFor="invite-code">Invite code</Label>
          <div className="flex gap-2">
            <Input
              id="invite-code"
              placeholder="XXXXX-XXXXX"
              autoComplete="off"
              autoFocus
              required
              className="font-mono uppercase placeholder:normal-case"
              value={code}
              onChange={(e) => setCode(e.target.value)}
            />
            <Button type="submit" disabled={pending || code.trim() === ""}>
              Join
            </Button>
          </div>
          <p className="text-xs text-muted-foreground">
            From the invite email an organization sent you.
          </p>
          <FormError error={accept.error} />
        </form>

        <div className="flex items-center gap-3 text-xs text-muted-foreground">
          <span className="h-px flex-1 bg-border" />
          or
          <span className="h-px flex-1 bg-border" />
        </div>

        <form onSubmit={handleCreate} className="flex flex-col gap-2">
          <Label htmlFor="organization-name">Organization name</Label>
          <div className="flex gap-2">
            <Input
              id="organization-name"
              placeholder="Acme Inc."
              autoComplete="organization"
              required
              maxLength={100}
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
            <Button type="submit" variant="outline" disabled={pending || name.trim() === ""}>
              Create
            </Button>
          </div>
          <p className="text-xs text-muted-foreground">
            You'll be its owner and can invite people from Settings.
          </p>
          <FormError error={create.error} />
        </form>
      </div>
    </AuthCard>
  );
}
