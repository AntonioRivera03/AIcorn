import { CheckInbox } from "@/features/auth/verify-email-page/check-inbox";
import { ConfirmEmailLink } from "@/features/auth/verify-email-page/confirm-email-link";

// The confirmation email links here with a token; without one this is the
// page an unconfirmed user waits on.
export function VerifyEmailPage({ token }: { token?: string }) {
  return token ? <ConfirmEmailLink token={token} /> : <CheckInbox />;
}
