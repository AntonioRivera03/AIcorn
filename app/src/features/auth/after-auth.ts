import type { Me } from "@/features/auth/types";

// safeRedirect only follows same-site paths, so a crafted ?redirect= can't
// send someone off to another site after they sign in.
export const safeRedirect = (path: string | undefined) =>
  path && path.startsWith("/") && !path.startsWith("//") ? path : "/app";

// destinationAfterAuth is where a user goes once signed in: an invite they
// arrived with, then onboarding if unfinished, then where they were headed.
export const destinationAfterAuth = (
  me: Me,
  options: { redirect?: string; invite?: string } = {},
) => {
  if (options.invite) {
    return `/onboarding?step=organization&code=${encodeURIComponent(options.invite)}`;
  }
  if (!me.account.usage) return "/onboarding";
  return safeRedirect(options.redirect);
};
