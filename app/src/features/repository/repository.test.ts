import { describe, expect, it } from "vitest";
import {
  githubURLProblem,
  hasLinkedRepository,
  normalizeGitHubURL,
  removesClone,
  repoPathProblem,
  type RepositoryStatus,
} from "@/features/repository/repository";

const status = (extra: Partial<RepositoryStatus> = {}): RepositoryStatus => ({
  mode: "official",
  path: "/home/me/app",
  url: "https://github.com/acme/app",
  linked: true,
  cloned: true,
  syncing: false,
  fetchedAt: null,
  error: "",
  ...extra,
});

describe("hasLinkedRepository", () => {
  it("follows the chosen mode, not whichever field is filled", () => {
    const project = {
      RepoMode: "personal" as const,
      RepoPath: "/code/app",
      RepoURL: "https://github.com/acme/app",
    };
    expect(hasLinkedRepository(project)).toBe(true);
    expect(hasLinkedRepository({ ...project, RepoPath: "  " })).toBe(false);
    expect(hasLinkedRepository({ ...project, RepoMode: "official" })).toBe(
      true,
    );
    expect(
      hasLinkedRepository({ ...project, RepoMode: "official", RepoURL: "" }),
    ).toBe(false);
    expect(hasLinkedRepository({ ...project, RepoMode: "" })).toBe(false);
    expect(hasLinkedRepository(undefined)).toBe(false);
  });
});

describe("normalizeGitHubURL", () => {
  it("normalizes the forms the server accepts", () => {
    expect(normalizeGitHubURL("https://github.com/acme/app")).toBe(
      "https://github.com/acme/app",
    );
    expect(normalizeGitHubURL(" https://GitHub.com/Acme/App.git ")).toBe(
      "https://github.com/acme/app",
    );
    expect(normalizeGitHubURL("https://github.com/acme/my.app_2/")).toBe(
      "https://github.com/acme/my.app_2",
    );
  });

  it("rejects every other scheme and shape", () => {
    for (const url of [
      "http://github.com/acme/app",
      "git@github.com:acme/app.git",
      "ssh://git@github.com/acme/app.git",
      "file:///srv/app.git",
      "/srv/app.git",
      "ext::sh -c touch% /tmp/x",
      "https://github.com/acme",
      "https://github.com/acme/app/tree/main",
      "https://token@github.com/acme/app",
      "https://github.com:443/acme/app",
      "https://github.com.evil.example/acme/app",
      "https://github.com/acme/app?tab=readme",
      "https://github.com/acme/app%2F..",
      "https://github.com/-acme/app",
    ]) {
      expect(normalizeGitHubURL(url), url).toBeNull();
    }
  });
});

describe("field hints", () => {
  it("explains paths and URLs the server would refuse", () => {
    expect(repoPathProblem("")).toBeNull();
    expect(repoPathProblem("/home/me/app")).toBeNull();
    expect(repoPathProblem("~/app")).toMatch(/absolute/);
    expect(repoPathProblem("code/app")).toMatch(/absolute/);
    expect(githubURLProblem("")).toBeNull();
    expect(githubURLProblem("https://github.com/acme/app")).toBeNull();
    expect(githubURLProblem("git@github.com:acme/app.git")).toMatch(
      /github\.com\/owner\/repo/,
    );
  });
});

describe("removesClone", () => {
  it("asks before a change deletes Aycorn's clone", () => {
    expect(removesClone(status(), { mode: "personal" })).toBe(true);
    expect(
      removesClone(status(), { url: "https://github.com/acme/other" }),
    ).toBe(true);
    expect(removesClone(status(), { url: "" })).toBe(true);
  });

  it("doesn't ask when nothing would be deleted", () => {
    expect(
      removesClone(status(), { url: "https://github.com/Acme/App.git" }),
    ).toBe(false);
    expect(removesClone(status(), { mode: "official" })).toBe(false);
    expect(removesClone(status(), { path: "/elsewhere" })).toBe(false);
    expect(removesClone(status({ cloned: false }), { mode: "personal" })).toBe(
      false,
    );
    expect(
      removesClone(status({ mode: "personal" }), {
        url: "https://github.com/acme/other",
      }),
    ).toBe(false);
  });
});
