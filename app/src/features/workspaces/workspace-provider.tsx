import { Fragment, useState, type ReactNode } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { getApiWorkspace } from "@/lib/api";
import { meQueryKey, useMeQuery } from "@/features/auth/queries/me-query";
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
