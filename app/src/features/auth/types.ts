import type { Workspace } from "@/features/workspaces/types";

// Empty until the user finishes onboarding.
export type Usage = "solo" | "organization" | "";

export type Account = {
  id: number;
  email: string;
  name: string;
  usage: Usage;
};

export type Me = {
  account: Account;
  workspaces: Workspace[];
};
