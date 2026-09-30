// How a project reaches its code. Personal works from a local checkout on the
// server; Official from a GitHub repository Aycorn clones and fetches with the
// server's git login. See Documentation/repo-linking.md.

import type { Project, RepoMode } from "@/types/types";

export type RepositoryStatus = {
  mode: RepoMode;
  path: string;
  url: string;
  linked: boolean;
  cloned: boolean;
  syncing: boolean;
  fetchedAt: string | null;
  error: string;
};

export type RepositoryPatch = {
  mode?: Exclude<RepoMode, "">;
  path?: string;
  url?: string;
};

type LinkFields = Pick<Project, "RepoMode" | "RepoPath" | "RepoURL">;

/** Whether agents and previews have code to work from. */
export const hasLinkedRepository = (project: LinkFields | undefined | null) =>
  !!project &&
  ((project.RepoMode === "personal" && project.RepoPath.trim() !== "") ||
    (project.RepoMode === "official" && project.RepoURL !== ""));

/** A hint for a Personal path the server would refuse, or null. */
export const repoPathProblem = (path: string): string | null => {
  const trimmed = path.trim();
  if (trimmed === "") return null;
  if (trimmed.startsWith("~")) return "Avoid ~ — use an absolute path.";
  if (!trimmed.startsWith("/") && !/^[A-Za-z]:[\\/]/.test(trimmed)) {
    return "Use an absolute path, starting with /.";
  }
  return null;
};

const githubOwner = /^[a-z0-9][a-z0-9-]{0,38}$/;
const githubName = /^[a-z0-9._-]{1,100}$/;

/**
 * The form the server stores for https://github.com/<owner>/<repo>, or null
 * for anything else. It mirrors the server's check so the field can explain a
 * mistake before saving, and so an equivalent URL isn't treated as a change.
 */
export const normalizeGitHubURL = (raw: string): string | null => {
  const trimmed = raw.trim();
  const scheme = "https://";
  if (trimmed.slice(0, scheme.length).toLowerCase() !== scheme) return null;
  // Parsed by hand: the URL class drops a default :443 and accepts
  // credentials, both of which the server refuses.
  const [authority, ...rest] = trimmed.slice(scheme.length).split("/");
  const path = rest.join("/");
  if (authority.toLowerCase() !== "github.com" || /[?#%\s\\]/.test(path)) {
    return null;
  }
  const parts = path.replace(/\/$/, "").split("/");
  if (parts.length !== 2) return null;
  const owner = parts[0].toLowerCase();
  const name = parts[1].toLowerCase().replace(/\.git$/, "");
  if (
    !githubOwner.test(owner) ||
    !githubName.test(name) ||
    name === "." ||
    name === ".."
  ) {
    return null;
  }
  return `https://github.com/${owner}/${name}`;
};

export const githubURLProblem = (url: string): string | null =>
  url.trim() === "" || normalizeGitHubURL(url)
    ? null
    : "Use a GitHub repository URL like https://github.com/owner/repo.";

/** "owner/repo" for an Official URL. */
export const repositoryName = (url: string) =>
  url.replace(/^https:\/\/github\.com\//, "");

/**
 * Whether saving patch would delete Aycorn's clone: switching away from
 * Official, or pointing it at a different repository, while a clone exists.
 * Its agent branches go with it, so the change is confirmed first.
 */
export const removesClone = (
  status: RepositoryStatus,
  patch: RepositoryPatch,
): boolean => {
  if (status.mode !== "official" || status.url === "" || !status.cloned) {
    return false;
  }
  if (patch.mode !== undefined && patch.mode !== "official") return true;
  if (patch.url === undefined) return false;
  return (normalizeGitHubURL(patch.url) ?? patch.url.trim()) !== status.url;
};
