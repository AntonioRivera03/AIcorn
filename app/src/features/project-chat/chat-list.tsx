import { SquarePen } from "lucide-react";
import { Button } from "@/components/ui/button";
import { ChatListRow } from "@/features/project-chat/chat-list/chat-list-row";
import {
  useDeleteChatMutation,
  useProjectChatsQuery,
  useRenameChatMutation,
} from "@/features/project-chat/queries/use-project-chats";
import { cn } from "@/lib/utils";

type ChatListProps = {
  projectId: number;
  activeId: number | undefined;
  onOpen: (chatId: number | undefined) => void;
  className?: string;
};

export function ChatList({ projectId, activeId, onOpen, className }: ChatListProps) {
  const chats = useProjectChatsQuery(projectId);
  const rename = useRenameChatMutation(projectId);
  const remove = useDeleteChatMutation(projectId);

  return (
    <nav aria-label="Chats" className={cn("flex min-h-0 flex-col gap-2", className)}>
      <Button
        variant="ghost"
        className="justify-start gap-2 px-2.5 text-muted-foreground"
        onClick={() => onOpen(undefined)}
      >
        <SquarePen />
        New chat
      </Button>
      <div className="flex min-h-0 flex-col gap-0.5 overflow-y-auto">
        {chats.isError && (
          <p role="alert" className="px-2.5 text-sm text-destructive">
            Couldn't load chats.{" "}
            <button type="button" className="underline" onClick={() => void chats.refetch()}>
              Retry
            </button>
          </p>
        )}
        {chats.data?.map((chat) => (
          <ChatListRow
            key={chat.id}
            chat={chat}
            active={chat.id === activeId}
            onOpen={() => onOpen(chat.id)}
            onRename={(title) => rename.mutate({ chatId: chat.id, title })}
            onDelete={() =>
              remove.mutate(chat.id, {
                onSuccess: () => {
                  if (chat.id === activeId) onOpen(undefined);
                },
              })
            }
          />
        ))}
      </div>
    </nav>
  );
}
