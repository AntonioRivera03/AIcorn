import { useEffect } from "react";
import { Link, useNavigate } from "@tanstack/react-router";
import { Check, ChevronsUpDown, LogOut, Plus, Settings, UserPlus } from "lucide-react";
import { toast } from "sonner";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuShortcut,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  useSidebar,
} from "@/components/ui/sidebar";
import { TruncatedText } from "@/components/truncated-text";
import { useLogoutMutation } from "@/features/auth/queries/auth-mutations";
import type { Workspace } from "@/features/workspaces/types";
import { useWorkspace } from "@/features/workspaces/workspace-context";
import { WorkspaceIcon } from "@/features/workspaces/workspace-icon";

const ROLE_LABELS = { owner: "Owner", admin: "Admin", member: "Member" } as const;

const describe = (workspace: Workspace) =>
  workspace.kind === "personal"
    ? "Personal"
    : `Organization · ${ROLE_LABELS[workspace.role]}`;

// Alt+1…9 jumps to the nth workspace, matching the order in the menu.
const useWorkspaceShortcuts = (
  workspaces: Workspace[],
  switchWorkspace: (id: number) => Promise<void>,
) => {
  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (!event.altKey || event.ctrlKey || event.metaKey || event.shiftKey) return;
      const digit = Number(event.code.replace("Digit", ""));
      const target = workspaces[digit - 1];
      if (!event.code.startsWith("Digit") || !target) return;
      event.preventDefault();
      void switchWorkspace(target.id);
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [workspaces, switchWorkspace]);
};

export function WorkspaceSwitcher() {
  const { account, workspace, workspaces, switchWorkspace } = useWorkspace();
  const { isMobile } = useSidebar();
  const logout = useLogoutMutation();
  const navigate = useNavigate();
  useWorkspaceShortcuts(workspaces, switchWorkspace);

  const handleLogout = () =>
    logout.mutate(undefined, {
      onSuccess: () => navigate({ to: "/login" }),
      onError: (err) => toast.error(err.message || "Failed to log out."),
    });

  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <SidebarMenuButton
              size="lg"
              className="data-[state=open]:bg-sidebar-accent data-[state=open]:text-sidebar-accent-foreground"
            >
              <span className="flex aspect-square size-8 items-center justify-center rounded-lg bg-sidebar-primary text-sidebar-primary-foreground">
                <WorkspaceIcon kind={workspace.kind} className="size-4" />
              </span>
              <span className="grid min-w-0 flex-1 text-left leading-tight">
                <TruncatedText text={workspace.name} side="right" className="text-sm font-semibold" />
                <span className="truncate text-xs text-muted-foreground">
                  {describe(workspace)}
                </span>
              </span>
              <ChevronsUpDown className="ml-auto size-4" />
            </SidebarMenuButton>
          </DropdownMenuTrigger>
          <DropdownMenuContent
            className="w-(--radix-dropdown-menu-trigger-width) min-w-60"
            align="start"
            side={isMobile ? "bottom" : "right"}
            sideOffset={4}
          >
            <DropdownMenuLabel className="text-xs font-normal text-muted-foreground">
              Workspaces
            </DropdownMenuLabel>
            {workspaces.map((w, index) => (
              <DropdownMenuItem key={w.id} onSelect={() => void switchWorkspace(w.id)}>
                <WorkspaceIcon kind={w.kind} className="size-4" />
                <TruncatedText text={w.name} side="right" className="min-w-0 flex-1" />
                {w.id === workspace.id && <Check className="size-4" />}
                {index < 9 && <DropdownMenuShortcut>⌥{index + 1}</DropdownMenuShortcut>}
              </DropdownMenuItem>
            ))}
            {workspace.kind === "organization" && (
              <DropdownMenuItem asChild>
                <Link to="/app/settings" search={{ tab: "members" }}>
                  <UserPlus className="size-4" />
                  Members & invites
                </Link>
              </DropdownMenuItem>
            )}
            <DropdownMenuItem asChild>
              <Link to="/onboarding" search={{ step: "organization" }}>
                <Plus className="size-4" />
                Create or join an organization
              </Link>
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuLabel className="flex flex-col font-normal">
              <TruncatedText text={account.name} className="text-sm" />
              <TruncatedText text={account.email} className="text-xs text-muted-foreground" />
            </DropdownMenuLabel>
            <DropdownMenuItem asChild>
              <Link to="/app/settings" search={{ tab: "account" }}>
                <Settings className="size-4" />
                Settings
              </Link>
            </DropdownMenuItem>
            <DropdownMenuItem onSelect={handleLogout} disabled={logout.isPending}>
              <LogOut className="size-4" />
              Log out
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </SidebarMenuItem>
    </SidebarMenu>
  );
}
