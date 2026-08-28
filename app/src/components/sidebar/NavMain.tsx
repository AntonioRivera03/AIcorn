import type React from "react";
import { Link } from "@tanstack/react-router";
import {
  SidebarGroup,
  SidebarGroupContent,
  SidebarMenu,
  SidebarMenuBadge,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar";
import { CollapsibleNavGroup } from "@/components/sidebar/CollapsibleNavGroup";
import { useIsMobile } from "@/hooks/useMobile";

export function NavMain({
  items,
  label,
}: {
  items: {
    title: string;
    url: string;
    icon?: React.ComponentType;
    badge?: React.ReactElement;
  }[];
  label?: string;
}) {
  const isMobile = useIsMobile();

  const menu = (
    <SidebarMenu>
      {items.map((item) => (
        <SidebarMenuItem key={item.title}>
          <SidebarMenuButton
            size={isMobile ? "lg" : "default"}
            tooltip={item.title}
            asChild
          >
            <Link
              to={item.url}
              className="flex"
              activeOptions={{ exact: item.url === "/" }}
              activeProps={{ "data-active": true }}
            >
              {item.icon && <item.icon />}
              <span>{item.title}</span>
            </Link>
          </SidebarMenuButton>
          {item?.badge && <SidebarMenuBadge>{item.badge}</SidebarMenuBadge>}
        </SidebarMenuItem>
      ))}
    </SidebarMenu>
  );

  if (!label) {
    return (
      <SidebarGroup>
        <SidebarGroupContent className="flex flex-col gap-2">
          {menu}
        </SidebarGroupContent>
      </SidebarGroup>
    );
  }

  return (
    <CollapsibleNavGroup section={label} label={label}>
      {menu}
    </CollapsibleNavGroup>
  );
}
