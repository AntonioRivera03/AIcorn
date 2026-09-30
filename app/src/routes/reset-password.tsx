import { createFileRoute } from "@tanstack/react-router";
import { PublicLayout } from "@/features/auth/public-layout";
import { ResetPasswordForm } from "@/features/auth/reset-password-form";
import { optionalString } from "@/features/auth/search";

type ResetPasswordSearch = {
  token?: string;
};

// Where the password reset email links to.
export const Route = createFileRoute("/reset-password")({
  validateSearch: (search: Record<string, unknown>): ResetPasswordSearch => ({
    token: optionalString(search.token),
  }),
  component: RouteComponent,
});

function RouteComponent() {
  const { token } = Route.useSearch();
  return (
    <PublicLayout>
      <ResetPasswordForm token={token} />
    </PublicLayout>
  );
}
