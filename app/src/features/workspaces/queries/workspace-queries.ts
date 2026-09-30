import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiJson } from "@/lib/api";
import { meQueryKey } from "@/features/auth/queries/me-query";
import { selectWorkspace } from "@/features/workspaces/workspace-selection";
import type {
  CreatedInvite,
  Invite,
  InvitePreview,
  Member,
  Role,
  Workspace,
} from "@/features/workspaces/types";

const membersKey = (workspaceId: number) => ["workspace", workspaceId, "members"];
const invitesKey = (workspaceId: number) => ["workspace", workspaceId, "invites"];

export function useMembersQuery(workspaceId: number) {
  return useQuery({
    queryKey: membersKey(workspaceId),
    queryFn: () => apiJson<Member[]>(`/api/workspaces/${workspaceId}/members`),
  });
}

export function useInvitesQuery(workspaceId: number, enabled: boolean) {
  return useQuery({
    queryKey: invitesKey(workspaceId),
    queryFn: () => apiJson<Invite[]>(`/api/workspaces/${workspaceId}/invites`),
    enabled,
  });
}

export function useInvitePreviewQuery(code: string) {
  return useQuery({
    queryKey: ["invite", code],
    queryFn: () => apiJson<InvitePreview>(`/api/invites/${encodeURIComponent(code)}`),
    retry: false,
  });
}

export function useCreateOrganizationMutation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (name: string) =>
      apiJson<Workspace>("/api/workspaces", {
        method: "POST",
        body: JSON.stringify({ name }),
      }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: meQueryKey }),
  });
}

export function useAcceptInviteMutation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (code: string) =>
      apiJson<Workspace>("/api/invites/accept", {
        method: "POST",
        body: JSON.stringify({ code }),
      }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: meQueryKey }),
  });
}

export function useRenameWorkspaceMutation(workspaceId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (name: string) =>
      apiJson<void>(`/api/workspaces/${workspaceId}`, {
        method: "PUT",
        body: JSON.stringify({ name }),
      }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: meQueryKey }),
  });
}

export function useInviteMemberMutation(workspaceId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: { email: string; role: Role }) =>
      apiJson<CreatedInvite>(`/api/workspaces/${workspaceId}/invites`, {
        method: "POST",
        body: JSON.stringify(input),
      }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: invitesKey(workspaceId) }),
  });
}

export function useRevokeInviteMutation(workspaceId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (inviteId: number) =>
      apiJson<void>(`/api/workspaces/${workspaceId}/invites/${inviteId}`, {
        method: "DELETE",
      }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: invitesKey(workspaceId) }),
  });
}

export function useSetMemberRoleMutation(workspaceId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: { accountId: number; role: Role }) =>
      apiJson<void>(`/api/workspaces/${workspaceId}/members/${input.accountId}`, {
        method: "PUT",
        body: JSON.stringify({ role: input.role }),
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: membersKey(workspaceId) });
      queryClient.invalidateQueries({ queryKey: meQueryKey });
    },
  });
}

export function useRemoveMemberMutation(workspaceId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (accountId: number) =>
      apiJson<void>(`/api/workspaces/${workspaceId}/members/${accountId}`, {
        method: "DELETE",
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: membersKey(workspaceId) });
      queryClient.invalidateQueries({ queryKey: meQueryKey });
    },
  });
}

// Leaving reloads the app into another workspace (normally the personal one).
// A full reload is simplest: every cached query and the page itself belong to
// the organization that was just left.
export function useLeaveWorkspaceMutation(workspaceId: number, fallbackWorkspaceId: number) {
  return useMutation({
    mutationFn: (accountId: number) =>
      apiJson<void>(`/api/workspaces/${workspaceId}/members/${accountId}`, {
        method: "DELETE",
      }),
    onSuccess: () => {
      selectWorkspace(fallbackWorkspaceId);
      window.location.assign("/app");
    },
  });
}

// Deleting reloads the app into another workspace (normally the personal
// one), for the same reason as leaving.
export function useDeleteOrganizationMutation(workspaceId: number, fallbackWorkspaceId: number) {
  return useMutation({
    mutationFn: () => apiJson<void>(`/api/workspaces/${workspaceId}`, { method: "DELETE" }),
    onSuccess: () => {
      selectWorkspace(fallbackWorkspaceId);
      window.location.assign("/app");
    },
  });
}
