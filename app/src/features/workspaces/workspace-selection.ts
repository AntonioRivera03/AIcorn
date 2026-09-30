import { getApiWorkspace, setApiWorkspace } from "@/lib/api";
import type { Workspace } from "@/features/workspaces/types";

// Which workspace a tab shows. sessionStorage keeps each tab on its own
// workspace (two tabs can show two organizations); localStorage remembers the
// last one picked so a new tab opens where you left off.
const TAB_KEY = "aycorn.workspace";
const LAST_KEY = "aycorn.lastWorkspace";

const readStored = (storage: () => Storage, key: string): number | null => {
  try {
    const value = Number(storage().getItem(key));
    return Number.isInteger(value) && value > 0 ? value : null;
  } catch {
    return null;
  }
};

const writeStored = (storage: () => Storage, key: string, id: number) => {
  try {
    storage().setItem(key, String(id));
  } catch {
    // Storage can be unavailable (private mode); the tab still works.
  }
};

// selectWorkspace makes id the workspace every API call from this tab uses.
export const selectWorkspace = (id: number) => {
  setApiWorkspace(id);
  writeStored(() => sessionStorage, TAB_KEY, id);
  writeStored(() => localStorage, LAST_KEY, id);
};

// resolveWorkspace picks the tab's workspace from the ones the user belongs
// to: the one already open, then this tab's, then the last used, then their
// personal workspace.
export const resolveWorkspace = (workspaces: Workspace[]): Workspace => {
  const candidates = [
    getApiWorkspace(),
    readStored(() => sessionStorage, TAB_KEY),
    readStored(() => localStorage, LAST_KEY),
  ];
  for (const id of candidates) {
    const match = workspaces.find((w) => w.id === id);
    if (match) return match;
  }
  return workspaces.find((w) => w.kind === "personal") ?? workspaces[0];
};
