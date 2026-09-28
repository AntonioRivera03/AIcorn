import {
  ArrowRight,
  Eye,
  FileText,
  FolderKanban,
  Globe,
  Link2,
  Plus,
  Search,
  Send,
  SquarePen,
  Wrench,
} from "lucide-react";
import type { ToolIcon as ToolIconName } from "@/features/project-chat/activity";

const icons = {
  project: FolderKanban,
  read: Eye,
  search: Search,
  document: FileText,
  create: Plus,
  edit: SquarePen,
  move: ArrowRight,
  link: Link2,
  send: Send,
  web: Globe,
  tool: Wrench,
} satisfies Record<ToolIconName, unknown>;

export function ToolIcon({ name, className }: { name: ToolIconName; className?: string }) {
  const Icon = icons[name];
  return <Icon className={className} aria-hidden />;
}
