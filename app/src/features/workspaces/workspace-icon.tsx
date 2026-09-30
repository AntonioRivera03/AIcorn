import { Building2, User } from "lucide-react";
import type { WorkspaceKind } from "@/features/workspaces/types";

export function WorkspaceIcon({ kind, className }: { kind: WorkspaceKind; className?: string }) {
  const Icon = kind === "personal" ? User : Building2;
  return <Icon className={className} />;
}
