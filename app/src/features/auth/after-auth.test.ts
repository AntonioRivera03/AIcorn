import { describe, expect, it } from "vitest";
import { destinationAfterAuth, safeRedirect } from "@/features/auth/after-auth";
import type { Account, Me } from "@/features/auth/types";

const me = (account: Partial<Account> = {}): Me => ({
  account: {
    id: 1,
    email: "ada@example.com",
    name: "Ada",
    usage: "solo",
    emailVerified: true,
    ...account,
  },
  workspaces: [],
});

describe("destinationAfterAuth", () => {
  it("takes an invite first, since accepting it confirms the email", () => {
    expect(destinationAfterAuth(me({ emailVerified: false, usage: "" }), { invite: "AB CD" })).toBe(
      "/onboarding?step=organization&code=AB%20CD",
    );
  });

  it("asks an unconfirmed user to confirm before onboarding", () => {
    expect(destinationAfterAuth(me({ emailVerified: false, usage: "" }))).toBe("/verify-email");
  });

  it("onboards a confirmed user who hasn't chosen how to use Aycorn", () => {
    expect(destinationAfterAuth(me({ usage: "" }))).toBe("/onboarding");
  });

  it("returns an onboarded user to where they were headed", () => {
    expect(destinationAfterAuth(me(), { redirect: "/app/settings" })).toBe("/app/settings");
    expect(destinationAfterAuth(me())).toBe("/app");
  });
});

describe("safeRedirect", () => {
  it("only follows same-site paths", () => {
    expect(safeRedirect("//evil.example")).toBe("/app");
    expect(safeRedirect("https://evil.example")).toBe("/app");
    expect(safeRedirect("/app/usage")).toBe("/app/usage");
  });
});
