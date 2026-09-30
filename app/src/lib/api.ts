// Every request to the Aycorn API goes through apiFetch. It tags the request
// with the workspace open in this tab (the server routes it to that
// workspace's data and checks you're a member), sends a signed-out user back
// to the login page, and reports when the tab's workspace is no longer open
// to them.

import { sentence } from "@/utils/sentence";

export const WORKSPACE_HEADER = "X-Aycorn-Workspace";
// Set by the server on a workspace request refused because the user isn't a
// member (anymore): they were removed, or the organization was deleted.
const WORKSPACE_ACCESS_HEADER = "X-Aycorn-Workspace-Access";

let currentWorkspaceId: number | null = null;

export const setApiWorkspace = (id: number | null) => {
  currentWorkspaceId = id;
};

export const getApiWorkspace = () => currentWorkspaceId;

let workspaceLostHandler: ((workspaceId: number) => void) | null = null;

// onWorkspaceLost registers what happens when a request finds the user has
// lost access to the workspace it was for. Returns an unregister function.
export const onWorkspaceLost = (handler: (workspaceId: number) => void) => {
  workspaceLostHandler = handler;
  return () => {
    if (workspaceLostHandler === handler) workspaceLostHandler = null;
  };
};

const redirectToLogin = () => {
  const { pathname, search } = window.location;
  // Only pages inside the app need a session; public pages handle 401s
  // themselves (e.g. a wrong password on the login page).
  if (!pathname.startsWith("/app")) return;
  const redirect = encodeURIComponent(pathname + search);
  window.location.assign(`/login?redirect=${redirect}`);
};

export const apiFetch = async (
  input: RequestInfo | URL,
  init: RequestInit = {},
): Promise<Response> => {
  const headers = new Headers(init.headers);
  if (currentWorkspaceId !== null && !headers.has(WORKSPACE_HEADER)) {
    headers.set(WORKSPACE_HEADER, String(currentWorkspaceId));
  }
  const response = await fetch(input, { ...init, headers });
  if (response.status === 401) redirectToLogin();
  if (response.headers.get(WORKSPACE_ACCESS_HEADER) === "none") {
    workspaceLostHandler?.(Number(headers.get(WORKSPACE_HEADER)));
  }
  return response;
};

// apiJson fetches and decodes JSON, throwing the server's error text on a
// non-2xx response so callers can show it in a toast.
export const apiJson = async <T>(
  input: RequestInfo | URL,
  init: RequestInit = {},
): Promise<T> => {
  const headers = new Headers(init.headers);
  if (init.body !== undefined && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  const response = await apiFetch(input, { ...init, headers });
  if (!response.ok) {
    const message = (await response.text()).trim();
    throw new Error(sentence(message) || `Request failed (${response.status})`);
  }
  if (response.status === 204) return undefined as T;
  return (await response.json()) as T;
};
