import { createFileRoute } from "@tanstack/react-router";
import { ForgotPasswordForm } from "@/features/auth/forgot-password-form";
import { PublicLayout } from "@/features/auth/public-layout";

export const Route = createFileRoute("/forgot-password")({
  component: RouteComponent,
});

function RouteComponent() {
  return (
    <PublicLayout>
      <ForgotPasswordForm />
    </PublicLayout>
  );
}
