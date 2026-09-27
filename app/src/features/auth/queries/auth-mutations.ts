import { useMutation, useQueryClient } from "@tanstack/react-query";
import { apiJson, setApiWorkspace } from "@/lib/api";
import { meQueryKey } from "@/features/auth/queries/me-query";
import type { Me, Usage } from "@/features/auth/types";

export function useLoginMutation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: { email: string; password: string }) =>
      apiJson<Me>("/api/auth/login", {
        method: "POST",
        body: JSON.stringify(input),
      }),
    onSuccess: (me) => queryClient.setQueryData(meQueryKey, me),
  });
}

export function useSignupMutation() {
  const queryClient = useQueryClient();
  return useMutation({
    // invite is the code of the invite the user signed up from; an invite
    // sent to their address confirms it, so they skip the confirmation email.
    mutationFn: (input: { name: string; email: string; password: string; invite?: string }) =>
      apiJson<Me>("/api/auth/signup", {
        method: "POST",
        body: JSON.stringify(input),
      }),
    onSuccess: (me) => queryClient.setQueryData(meQueryKey, me),
  });
}

export function useLogoutMutation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => apiJson<void>("/api/auth/logout", { method: "POST" }),
    onSuccess: () => {
      setApiWorkspace(null);
      queryClient.clear();
      queryClient.setQueryData(meQueryKey, null);
    },
  });
}

export function useSetUsageMutation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (usage: Exclude<Usage, "">) =>
      apiJson<Me>("/api/auth/me/usage", {
        method: "PUT",
        body: JSON.stringify({ usage }),
      }),
    onSuccess: (me) => queryClient.setQueryData(meQueryKey, me),
  });
}

export function useRenameAccountMutation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (name: string) =>
      apiJson<Me>("/api/auth/me", {
        method: "PUT",
        body: JSON.stringify({ name }),
      }),
    onSuccess: (me) => queryClient.setQueryData(meQueryKey, me),
  });
}

export function useChangePasswordMutation() {
  return useMutation({
    mutationFn: (input: { current: string; new: string }) =>
      apiJson<void>("/api/auth/me/password", {
        method: "PUT",
        body: JSON.stringify(input),
      }),
  });
}

// Follows a confirmation link. Works signed in or not; when signed in, the
// refreshed account lets the app move on.
export function useVerifyEmailMutation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (token: string) =>
      apiJson<void>("/api/auth/verify-email", {
        method: "POST",
        body: JSON.stringify({ token }),
      }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: meQueryKey }),
  });
}

export function useResendVerificationMutation() {
  return useMutation({
    mutationFn: () => apiJson<void>("/api/auth/verify-email/resend", { method: "POST" }),
  });
}

// emailSent is false when the server can't send email: the reset link then
// only reaches the server log.
export function useRequestPasswordResetMutation() {
  return useMutation({
    mutationFn: (email: string) =>
      apiJson<{ emailSent: boolean }>("/api/auth/password-reset", {
        method: "POST",
        body: JSON.stringify({ email }),
      }),
  });
}

// Sets a new password from a reset link, which also signs the user in.
export function useResetPasswordMutation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: { token: string; password: string }) =>
      apiJson<Me>("/api/auth/password-reset/confirm", {
        method: "POST",
        body: JSON.stringify(input),
      }),
    onSuccess: (me) => queryClient.setQueryData(meQueryKey, me),
  });
}
