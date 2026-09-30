import { useState } from "react";
import { Ellipsis, LoaderCircle, Pencil, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { DeleteChatDialog } from "@/features/project-chat/chat-list/delete-chat-dialog";
import { compactTime } from "@/features/project-chat/compact-time";
import { isWorking, serverTime, type ChatSummary } from "@/features/project-chat/types";
import { cn } from "@/lib/utils";

type ChatListRowProps = {
  chat: ChatSummary;
  active: boolean;
  onOpen: () => void;
  onRename: (title: string) => void;
  onDelete: () => void;
};

export function ChatListRow({ chat, active, onOpen, onRename, onDelete }: ChatListRowProps) {
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(chat.title);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const working = isWorking(chat.status);
  const title = chat.title || "Untitled chat";

  const commit = () => {
    setEditing(false);
    if (draft.trim() !== chat.title) onRename(draft);
  };

  if (editing)
    return (
      <input
        autoFocus
        aria-label="Chat title"
        value={draft}
        maxLength={120}
        onChange={(event) => setDraft(event.target.value)}
        onBlur={commit}
        onKeyDown={(event) => {
          event.stopPropagation();
          if (event.key === "Enter") event.currentTarget.blur();
          if (event.key === "Escape") {
            setDraft(chat.title);
            setEditing(false);
          }
        }}
        className="h-9 w-full rounded-lg bg-accent px-2.5 text-sm outline-none ring-1 ring-ring/50"
      />
    );

  return (
    <>
      <div
        className={cn(
          "group/row relative flex h-9 items-center rounded-lg hover:bg-accent/60",
          active && "bg-accent",
        )}
      >
        <button
          type="button"
          aria-current={active || undefined}
          onClick={onOpen}
          onDoubleClick={() => {
            setDraft(chat.title);
            setEditing(true);
          }}
          className="flex h-full min-w-0 flex-1 items-center gap-2 rounded-lg px-2.5 text-left text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
        >
          <Tooltip>
            <TooltipTrigger asChild>
              <span className={cn("min-w-0 flex-1 truncate", !chat.title && "text-muted-foreground")}>
                {title}
              </span>
            </TooltipTrigger>
            <TooltipContent side="right" className="max-w-xs">
              {title}
            </TooltipContent>
          </Tooltip>
          <span className="shrink-0 text-xs tabular-nums text-muted-foreground group-hover/row:invisible">
            {working ? (
              <LoaderCircle className="size-3.5 animate-spin text-primary" aria-label="Working" />
            ) : (
              compactTime(serverTime(chat.updatedAt))
            )}
          </span>
        </button>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={`Actions for ${title}`}
              className="absolute right-1 size-7 text-muted-foreground opacity-0 group-hover/row:opacity-100 focus-visible:opacity-100 data-[state=open]:opacity-100"
            >
              <Ellipsis />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start" side="right" className="min-w-36">
            <DropdownMenuItem
              onClick={() => {
                setDraft(chat.title);
                setEditing(true);
              }}
            >
              <Pencil />
              Rename
            </DropdownMenuItem>
            <DropdownMenuItem variant="destructive" onClick={() => setDeleteOpen(true)}>
              <Trash2 />
              Delete
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
      <DeleteChatDialog
        title={chat.title}
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        onConfirm={onDelete}
      />
    </>
  );
}
