import { AIProvider } from "@/features/ai/ai-provider";
import { PreviewBanner } from "@/features/environments/preview-banner";
import * as React from "react";
import {
  Outlet,
  createFileRoute,
  redirect,
  useRouterState,
} from "@tanstack/react-router";
import { AppSidebar } from "@/components/sidebar/AppSidebar";
import {
  SidebarInset,
  SidebarProvider,
  useSidebar,
} from "@/components/ui/sidebar";
import { meQueryOptions } from "@/features/auth/queries/me-query";
import { WorkspaceProvider } from "@/features/workspaces/workspace-provider";
import {
  resolveWorkspace,
  selectWorkspace,
} from "@/features/workspaces/workspace-selection";

// Everything under /app needs a signed-in, onboarded user, and every API call
// below it is scoped to the workspace picked here.
export const Route = createFileRoute("/app")({
  beforeLoad: async ({ context, location }) => {
    const me = await context.queryClient.fetchQuery(meQueryOptions);
    if (!me) {
      throw redirect({ to: "/login", search: { redirect: location.href } });
    }
    if (!me.account.emailVerified) {
      throw redirect({ to: "/verify-email" });
    }
    if (!me.account.usage) {
      throw redirect({ to: "/onboarding" });
    }
    selectWorkspace(resolveWorkspace(me.workspaces).id);
  },
  component: AppLayout,
});

function MobileSidebarClose() {
  const { isMobile, setOpenMobile } = useSidebar();
  const pathname = useRouterState({ select: (s) => s.location.pathname });

  React.useEffect(() => {
    if (isMobile) setOpenMobile(false);
  }, [pathname]);

  React.useEffect(() => {
    if (!isMobile) return;
    const handleClick = (e: MouseEvent) => {
      const target = e.target as HTMLElement;
      if (target.closest("[data-sidebar]") && target.closest("a")) {
        setOpenMobile(false);
      }
    };
    document.addEventListener("click", handleClick);
    return () => document.removeEventListener("click", handleClick);
  }, [isMobile, setOpenMobile]);

  return null;
}

function AppLayout() {
  const sidebarCookie = document.cookie
    .split("; ")
    .find((c) => c.startsWith("sidebar_state="))
    ?.split("=")[1];
  const defaultSidebarOpen = sidebarCookie !== "false";

  return (
    <WorkspaceProvider>
      <SidebarProvider
        defaultOpen={defaultSidebarOpen}
        style={
          {
            "--sidebar-width": "calc(var(--spacing) * 72)",
            "--header-height": "calc(var(--spacing) * 12)",
          } as React.CSSProperties
        }
      >
        <AIProvider>
          <MobileSidebarClose />
          <AppSidebar variant="inset" />
          <SidebarInset>
            <PreviewBanner />
            <Outlet />
          </SidebarInset>
        </AIProvider>
      </SidebarProvider>
    </WorkspaceProvider>
  );
}
