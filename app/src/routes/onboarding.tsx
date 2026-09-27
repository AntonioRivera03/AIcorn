import { createFileRoute, redirect } from "@tanstack/react-router";
import { meQueryOptions } from "@/features/auth/queries/me-query";
import { PublicLayout } from "@/features/auth/public-layout";
import { optionalString } from "@/features/auth/search";
import { OnboardingPage } from "@/features/onboarding/onboarding-page";

type OnboardingSearch = {
  step?: "organization";
  code?: string;
};

export const Route = createFileRoute("/onboarding")({
  validateSearch: (search: Record<string, unknown>): OnboardingSearch => ({
    step: search.step === "organization" ? "organization" : undefined,
    code: optionalString(search.code),
  }),
  beforeLoad: async ({ context, location, search }) => {
    const me = await context.queryClient.fetchQuery(meQueryOptions);
    if (!me) {
      throw redirect({ to: "/login", search: { redirect: location.href } });
    }
    // Onboarded users only come here to join or create an organization.
    if (me.account.usage && !search.step) throw redirect({ to: "/app" });
  },
  component: RouteComponent,
});

function RouteComponent() {
  const { step, code } = Route.useSearch();
  return (
    <PublicLayout>
      <OnboardingPage step={step} code={code} />
    </PublicLayout>
  );
}
