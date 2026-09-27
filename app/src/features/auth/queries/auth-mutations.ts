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
    mutationFn: (input: { name: string; email: string; password: string }) =>
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
