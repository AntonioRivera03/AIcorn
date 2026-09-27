import { createFileRoute, redirect } from "@tanstack/react-router";
import { meQueryOptions } from "@/features/auth/queries/me-query";
import { destinationAfterAuth } from "@/features/auth/after-auth";
import { PublicLayout } from "@/features/auth/public-layout";
import { InviteLanding } from "@/features/workspaces/invite-landing";

export const Route = createFileRoute("/invite/$code")({
  beforeLoad: async ({ context, params }) => {
    const me = await context.queryClient.fetchQuery(meQueryOptions);
    if (me) throw redirect({ href: destinationAfterAuth(me, { invite: params.code }) });
  },
  component: RouteComponent,
});

function RouteComponent() {
  const { code } = Route.useParams();
  return (
    <PublicLayout>
      <InviteLanding code={code} />
    </PublicLayout>
  );
}
