import { useState } from "react";
import { Link, useNavigate } from "@tanstack/react-router";
import { toast } from "sonner";
import { useMeQuery } from "@/features/auth/queries/me-query";
import { useSetUsageMutation } from "@/features/auth/queries/auth-mutations";
import { OrganizationStep } from "@/features/onboarding/organization-step";
import { UsageStep } from "@/features/onboarding/usage-step";
import type { Workspace } from "@/features/workspaces/types";
import { selectWorkspace } from "@/features/workspaces/workspace-selection";

type Step = "usage" | "organization";

type Props = {
  step?: Step;
  code?: string;
};

// Onboarding asks how the user will use Aycorn. It doubles as the "join or
// create an organization" page for users who are already set up.
export function OnboardingPage({ step: initialStep, code }: Props) {
  const { data: me } = useMeQuery();
  const [step, setStep] = useState<Step>(initialStep ?? "usage");
  const setUsage = useSetUsageMutation();
  const navigate = useNavigate();

  if (!me) return null;
  const onboarded = me.account.usage !== "";

  const continueSolo = () =>
    setUsage.mutate("solo", { onSuccess: () => navigate({ to: "/app" }) });

  const openWorkspace = (workspace: Workspace) => {
    selectWorkspace(workspace.id);
    toast.success(`Welcome to ${workspace.name}.`);
    navigate({ to: "/app" });
  };

  const linkClass = "font-medium text-foreground underline-offset-4 hover:underline";

  if (step === "usage") {
    return (
      <UsageStep
        name={me.account.name}
        pending={setUsage.isPending}
        error={setUsage.error}
        onSolo={continueSolo}
        onOrganization={() => setStep("organization")}
      />
    );
  }

  return (
    <OrganizationStep
      initialCode={code}
      onJoined={openWorkspace}
      footer={
        onboarded ? (
          <Link to="/app" className={linkClass}>
            Back to Aycorn
          </Link>
        ) : (
          <span>
            Not sure yet?{" "}
            <button
              type="button"
              className={linkClass}
              disabled={setUsage.isPending}
              onClick={continueSolo}
            >
              Continue solo
            </button>
          </span>
        )
      }
    />
  );
}
