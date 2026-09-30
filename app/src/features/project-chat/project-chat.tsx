import { useContext, useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useNavigate, useSearch } from "@tanstack/react-router";
import { PanelLeft, SquarePen } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Sheet, SheetContent, SheetTitle } from "@/components/ui/sheet";
import { ProjectContext } from "@/contexts/project/ProjectContext";
import { ChatComposer } from "@/features/project-chat/chat-composer";
import { ChatList } from "@/features/project-chat/chat-list";
import { ChatTimeline } from "@/features/project-chat/chat-timeline";
import {
  useChatTasksQuery,
  useProjectChatQuery,
} from "@/features/project-chat/queries/use-project-chats";
import { isWorking } from "@/features/project-chat/types";
import { useChatNames } from "@/features/project-chat/use-chat-names";
import { cn } from "@/lib/utils";

// Chats with Chatter, the project's knowledge bank and task manager. The
// list sits on the left; the conversation floats beside it. A new chat
// centers its composer, which moves to the bottom once the chat has begun.
export function ProjectChat({ projectId }: { projectId: number }) {
  const { chat: chatId } = useSearch({ from: "/app/project/$projectId" });
  const navigate = useNavigate({ from: "/app/project/$projectId" });
  const client = useQueryClient();
  const { Project } = useContext(ProjectContext);
  const [listOpen, setListOpen] = useState(false);
  const conversation = useProjectChatQuery(projectId, chatId);
  const tasks = useChatTasksQuery(projectId);
  const names = useChatNames(tasks.data);
  const turns = conversation.data?.turns ?? [];
  const activeTurn = turns.find((turn) => isWorking(turn.status));
  const started = !!chatId && (turns.length > 0 || conversation.isPending);
  const lastStatus = turns.at(-1)?.status;

  const openChat = (id: number | undefined) => {
    setListOpen(false);
    void navigate({ search: (previous) => ({ ...previous, chat: id }) });
  };

  // A chat that was deleted, or belongs elsewhere, falls back to a new one.
  useEffect(() => {
    if (conversation.isError)
      void navigate({ search: (previous) => ({ ...previous, chat: undefined }), replace: true });
  }, [conversation.isError, navigate]);

  // Chatter may have changed tasks: refresh what other views show.
  useEffect(() => {
    void client.invalidateQueries({ queryKey: ["projectDetails", projectId] });
    void client.invalidateQueries({ queryKey: ["task-ownership", projectId] });
    void client.invalidateQueries({ queryKey: ["project-chat-tasks", projectId] });
  }, [client, projectId, lastStatus]);

  const list = (
    <ChatList projectId={projectId} activeId={chatId} onOpen={openChat} className="h-full" />
  );

  return (
    <div className="flex h-full min-h-[28rem] flex-1 gap-4">
      <aside className="hidden w-60 shrink-0 md:block">{list}</aside>
      <Sheet open={listOpen} onOpenChange={setListOpen}>
        <SheetContent side="left" className="w-72 p-3 pt-10">
          <SheetTitle className="sr-only">Chats</SheetTitle>
          {list}
        </SheetContent>
      </Sheet>

      <section aria-label="Chat" className="relative flex min-w-0 flex-1 flex-col">
        <div className="flex items-center gap-1 md:hidden">
          <Button variant="ghost" size="icon-sm" aria-label="Show chats" onClick={() => setListOpen(true)}>
            <PanelLeft />
          </Button>
          <Button variant="ghost" size="icon-sm" aria-label="New chat" onClick={() => openChat(undefined)}>
            <SquarePen />
          </Button>
        </div>

        {started && <ChatTimeline turns={turns} names={names} />}

        {/* One wrapper for both positions, so the move from the middle to the
            bottom animates. */}
        <div
          className={cn(
            "absolute inset-x-0 px-2 transition-[top,translate] duration-500 ease-[cubic-bezier(0.4,0,0.2,1)] motion-reduce:transition-none",
            started ? "top-full -translate-y-full pb-2" : "top-[42%] -translate-y-1/2",
          )}
        >
          {started && (
            <div className="pointer-events-none absolute inset-x-0 -top-10 h-10 bg-gradient-to-t from-background to-transparent" />
          )}
          {!started && (
            <h1 className="mb-6 text-center text-2xl font-normal tracking-tight sm:text-3xl">
              What’s next for {Project.Name || "this project"}?
            </h1>
          )}
          <div className={cn(started && "bg-background")}>
            <ChatComposer
              key={chatId ?? "new"}
              projectId={projectId}
              chatId={chatId}
              tasks={tasks.data}
              tasksLoading={tasks.isPending}
              activeTurn={activeTurn}
              onSent={(turn) => {
                if (turn.conversationId !== chatId) openChat(turn.conversationId);
              }}
            />
          </div>
        </div>
      </section>
    </div>
  );
}
