import { createContext, useContext } from "react";
import type { Account } from "@/features/auth/types";
import type { Workspace } from "@/features/workspaces/types";

export type WorkspaceContextValue = {
  account: Account;
  workspace: Workspace;
  workspaces: Workspace[];
  switchWorkspace: (id: number) => Promise<void>;
};

export const WorkspaceContext = createContext<WorkspaceContextValue | null>(null);

// useWorkspace returns the signed-in account and the workspace this tab is
// showing. Only available under /app (see WorkspaceProvider).
export function useWorkspace() {
  const value = useContext(WorkspaceContext);
  if (!value) throw new Error("useWorkspace must be used inside WorkspaceProvider");
  return value;
}
