import { createFileRoute, redirect } from "@tanstack/react-router";
import { meQueryOptions } from "@/features/auth/queries/me-query";
import { destinationAfterAuth } from "@/features/auth/after-auth";
import { LoginForm } from "@/features/auth/login-form";
import { PublicLayout } from "@/features/auth/public-layout";
import { optionalString } from "@/features/auth/search";

type LoginSearch = {
  redirect?: string;
  invite?: string;
};

export const Route = createFileRoute("/login")({
  validateSearch: (search: Record<string, unknown>): LoginSearch => ({
    redirect: optionalString(search.redirect),
    invite: optionalString(search.invite),
  }),
  beforeLoad: async ({ context, search }) => {
    const me = await context.queryClient.fetchQuery(meQueryOptions);
    if (me) throw redirect({ href: destinationAfterAuth(me, search) });
  },
  component: RouteComponent,
});

function RouteComponent() {
  const { redirect, invite } = Route.useSearch();
  return (
    <PublicLayout>
      <LoginForm redirect={redirect} invite={invite} />
    </PublicLayout>
  );
}
