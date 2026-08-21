import { useState } from "react";
import { ChevronDownIcon, ChevronRightIcon } from "lucide-react";
import {
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
} from "@/components/ui/sidebar";

const STORAGE_KEY = (section: string) => `sidebar_collapsed_${section}`;

function readCollapsed(section: string): boolean {
  try {
    return localStorage.getItem(STORAGE_KEY(section)) === "true";
  } catch {
    return false;
  }
}

function writeCollapsed(section: string, collapsed: boolean) {
  try {
    if (collapsed) {
      localStorage.setItem(STORAGE_KEY(section), "true");
    } else {
      localStorage.removeItem(STORAGE_KEY(section));
    }
  } catch {
    return;
  }
}

type Props = {
  section: string;
  label: string;
} & React.ComponentProps<typeof SidebarGroup>;

export function CollapsibleNavGroup({ section, label, children, ...props }: Props) {
  const [collapsed, setCollapsed] = useState(() => readCollapsed(section));

  const toggleCollapsed = () => {
    const next = !collapsed;
    setCollapsed(next);
    writeCollapsed(section, next);
  };

  return (
    <SidebarGroup {...props}>
      <SidebarGroupLabel asChild>
        <button
          type="button"
          onClick={toggleCollapsed}
          aria-expanded={!collapsed}
          aria-label={`${collapsed ? "Expand" : "Collapse"} ${label} section`}
          className="w-full cursor-pointer"
        >
          <span>{label}</span>
          {collapsed ? (
            <ChevronRightIcon className="ml-auto" />
          ) : (
            <ChevronDownIcon className="ml-auto" />
          )}
        </button>
      </SidebarGroupLabel>
      {!collapsed && (
        <SidebarGroupContent className="flex flex-col gap-2">
          {children}
        </SidebarGroupContent>
      )}
    </SidebarGroup>
  );
}
