import { createFileRoute, redirect } from "@tanstack/react-router";
import { meQueryOptions } from "@/features/auth/queries/me-query";
import { destinationAfterAuth } from "@/features/auth/after-auth";
import { PublicLayout } from "@/features/auth/public-layout";
import { SignupForm } from "@/features/auth/signup-form";
import { optionalString } from "@/features/auth/search";

type SignupSearch = {
  invite?: string;
  email?: string;
};

export const Route = createFileRoute("/signup")({
  validateSearch: (search: Record<string, unknown>): SignupSearch => ({
    invite: optionalString(search.invite),
    email: optionalString(search.email),
  }),
  beforeLoad: async ({ context, search }) => {
    const me = await context.queryClient.fetchQuery(meQueryOptions);
    if (me) throw redirect({ href: destinationAfterAuth(me, search) });
  },
  component: RouteComponent,
});

function RouteComponent() {
  const { invite, email } = Route.useSearch();
  return (
    <PublicLayout>
      <SignupForm invite={invite} email={email} />
    </PublicLayout>
  );
}
