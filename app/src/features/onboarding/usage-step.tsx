import { Building2, User } from "lucide-react";
import { AuthCard } from "@/features/auth/auth-card";
import { FormError } from "@/features/auth/form-error";
import { UsageOption } from "@/features/onboarding/usage-option";

type Props = {
  name: string;
  pending: boolean;
  error: Error | null;
  onSolo: () => void;
  onOrganization: () => void;
};

export function UsageStep({ name, pending, error, onSolo, onOrganization }: Props) {
  return (
    <AuthCard
      title={`Welcome, ${name}`}
      description="How will you use Aycorn?"
      className="max-w-md"
    >
      <div className="flex flex-col gap-3">
        <UsageOption
          icon={User}
          title="Solo"
          description="Just your own projects. You can join an organization anytime."
          autoFocus
          disabled={pending}
          onSelect={onSolo}
        />
        <UsageOption
          icon={Building2}
          title="With an organization"
          description="Create an organization, or join one with an invite code."
          disabled={pending}
          onSelect={onOrganization}
        />
        <FormError error={error} />
      </div>
    </AuthCard>
  );
}
