import React from "react";
import { NavPinnedProjects } from "@/components/sidebar/NavPinnedProjects";
import { NavMain } from "@/components/sidebar/NavMain";
import { NavSecondary } from "@/components/sidebar/NavSecondary";
import {
  Sidebar,
  SidebarContent,
  SidebarHeader,
} from "@/components/ui/sidebar";
import { WorkspaceSwitcher } from "@/features/workspaces/workspace-switcher";
import {
  CalendarClockIcon,
  ChartAreaIcon,
  FolderIcon,
  LinkIcon,
  SettingsIcon,
  TagsIcon,
  WorkflowIcon,
} from "lucide-react";
import { Badge } from "../ui/badge";

const data = {
  navMain: [
    {
      title: "Projects",
      url: "/app",
      icon: FolderIcon,
    },
    {
      title: "Upcoming",
      url: "/app/upcoming",
      icon: CalendarClockIcon,
    },
  ],
  navConfigure: [
    {
      title: "Workflows",
      url: "/app/workflows",
      icon: WorkflowIcon,
    },
    {
      title: "Task Types",
      url: "/app/task-types",
      icon: TagsIcon,
    },
    {
      title: "Task Links",
      url: "/app/task-links",
      icon: LinkIcon,
      badge: <Badge className="text-xs py-0.5 -rotate-2">New!</Badge>,
    },
  ],
  navSecondary: [
    {
      title: "Usage",
      url: "/app/usage",
      icon: ChartAreaIcon,
    },
    {
      title: "Settings",
      url: "/app/settings",
      icon: SettingsIcon,
    },
  ],
};

export function AppSidebar({ ...props }: React.ComponentProps<typeof Sidebar>) {
  return (
    <Sidebar collapsible="offcanvas" {...props}>
      <SidebarHeader>
        <WorkspaceSwitcher />
      </SidebarHeader>
      <SidebarContent>
        <NavMain items={data.navMain} />
        <NavMain items={data.navConfigure} label="Configuration" />
        <NavPinnedProjects />
        <NavSecondary items={data.navSecondary} className="mt-auto" />
      </SidebarContent>
    </Sidebar>
  );
}
