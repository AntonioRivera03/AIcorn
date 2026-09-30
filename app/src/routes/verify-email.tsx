import { createFileRoute, redirect } from "@tanstack/react-router";
import { meQueryOptions } from "@/features/auth/queries/me-query";
import { destinationAfterAuth } from "@/features/auth/after-auth";
import { PublicLayout } from "@/features/auth/public-layout";
import { optionalString } from "@/features/auth/search";
import { VerifyEmailPage } from "@/features/auth/verify-email-page";

type VerifyEmailSearch = {
  token?: string;
};

// With a token this follows the confirmation link, signed in or not (it may
// be opened in another browser). Without one it's the "check your inbox" page
// for a signed-in, unconfirmed user.
export const Route = createFileRoute("/verify-email")({
  validateSearch: (search: Record<string, unknown>): VerifyEmailSearch => ({
    token: optionalString(search.token),
  }),
  beforeLoad: async ({ context, location, search }) => {
    if (search.token) return;
    const me = await context.queryClient.fetchQuery(meQueryOptions);
    if (!me) {
      throw redirect({ to: "/login", search: { redirect: location.href } });
    }
    if (me.account.emailVerified) throw redirect({ href: destinationAfterAuth(me) });
  },
  component: RouteComponent,
});

function RouteComponent() {
  const { token } = Route.useSearch();
  return (
    <PublicLayout>
      <VerifyEmailPage token={token} />
    </PublicLayout>
  );
}
