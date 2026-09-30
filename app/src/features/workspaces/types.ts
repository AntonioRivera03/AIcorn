export type WorkspaceKind = "personal" | "organization";

export type Role = "owner" | "admin" | "member";

// A workspace as seen by the signed-in user: their role is part of it.
export type Workspace = {
  id: number;
  kind: WorkspaceKind;
  name: string;
  role: Role;
};

export type Member = {
  accountId: number;
  name: string;
  email: string;
  role: Role;
  timeJoined: string | null;
};

export type Invite = {
  id: number;
  email: string;
  role: Role;
  invitedBy: string;
  timeCreated: string | null;
  timeExpires: string;
};

// Returned once, to the inviter, right after creating an invite.
export type CreatedInvite = Invite & {
  code: string;
  link: string;
  emailSent: boolean;
  emailError?: string;
};

export type InvitePreview = {
  workspaceName: string;
  email: string;
  invitedBy: string;
};

export const canManageMembers = (workspace: Workspace) =>
  workspace.kind === "organization" &&
  (workspace.role === "owner" || workspace.role === "admin");
