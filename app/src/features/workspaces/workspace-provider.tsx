import { Fragment, useEffect, useRef, useState, type ReactNode } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { toast } from "sonner";
import { getApiWorkspace, onWorkspaceLost } from "@/lib/api";
import {
  meQueryKey,
  meQueryOptions,
  useMeQuery,
} from "@/features/auth/queries/me-query";
import { WorkspaceContext } from "@/features/workspaces/workspace-context";
import {
  resolveWorkspace,
  selectWorkspace,
} from "@/features/workspaces/workspace-selection";

// WorkspaceProvider holds the workspace this tab is showing. Its children are
// keyed by workspace, so switching remounts the app against the new
// workspace's data instead of briefly showing the old one's.
export function WorkspaceProvider({ children }: { children: ReactNode }) {
  const { data: me } = useMeQuery();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const [workspaceId, setWorkspaceId] = useState(() => getApiWorkspace());
  // Names of every workspace seen, so a notice can still name one the user
  // has just lost.
  const workspaceNames = useRef(new Map<number, string>());
  const lostWorkspace = useRef<number | null>(null);
  // Shown by an effect once the next workspace has mounted: toasts need a
  // mounted Toaster, and the old one unmounts with the old workspace.
  const pendingNotice = useRef<string | null>(null);
  const [noticeCount, setNoticeCount] = useState(0);

  // Compares against the tab's actual workspace, not the displayed fallback:
  // after leaving an organization the fallback is already the target.
  const switchWorkspace = async (id: number) => {
    if (id === workspaceId && id === getApiWorkspace()) return;
    // Leave the current page first: its IDs mean nothing in the new workspace.
    await navigate({ to: "/app" });
    selectWorkspace(id);
    queryClient.removeQueries({
      predicate: (query) => query.queryKey[0] !== meQueryKey[0],
    });
    setWorkspaceId(id);
  };

  // Removed from the open organization, or it was deleted: move the tab to the
  // personal workspace rather than leave every request failing.
  const leaveLostWorkspace = async (lostId: number) => {
    if (lostId !== getApiWorkspace() || lostWorkspace.current === lostId) return;
    lostWorkspace.current = lostId;
    const fresh = await queryClient.fetchQuery({ ...meQueryOptions, staleTime: 0 });
    if (!fresh) return;
    const fallback =
      fresh.workspaces.find((w) => w.kind === "personal") ?? fresh.workspaces[0];
    const name = workspaceNames.current.get(lostId) ?? "that workspace";
    await switchWorkspace(fallback.id);
    pendingNotice.current = `You no longer have access to ${name}.`;
    setNoticeCount((count) => count + 1);
  };

  useEffect(() => onWorkspaceLost((id) => void leaveLostWorkspace(id)));

  // A refreshed list that no longer has the tab's workspace needs no check
  // here: the app remounts on the fallback, and its first requests, still
  // tagged with the lost workspace, come back flagged.
  useEffect(() => {
    for (const w of me?.workspaces ?? []) workspaceNames.current.set(w.id, w.name);
  }, [me]);

  useEffect(() => {
    if (!pendingNotice.current) return;
    toast.info(pendingNotice.current);
    pendingNotice.current = null;
  }, [noticeCount]);

  if (!me) return null;
  const workspace =
    me.workspaces.find((w) => w.id === workspaceId) ??
    resolveWorkspace(me.workspaces);

  return (
    <WorkspaceContext.Provider
      value={{ account: me.account, workspace, workspaces: me.workspaces, switchWorkspace }}
    >
      <Fragment key={workspace.id}>{children}</Fragment>
    </WorkspaceContext.Provider>
  );
}
