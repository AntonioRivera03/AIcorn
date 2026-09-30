import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { apiJson } from "@/lib/api";
import type { MentionTask } from "@/features/project-chat/mentions";
import {
  isWorking,
  type ChatConversation,
  type ChatSummary,
  type ChatTurn,
} from "@/features/project-chat/types";

const base = (projectId: number) => `/api/project-chats/project/${projectId}`;
const chatsKey = (projectId: number) => ["project-chats", projectId];
const chatKey = (projectId: number, chatId: number) => ["project-chat", projectId, chatId];

export function useProjectChatsQuery(projectId: number) {
  return useQuery({
    queryKey: chatsKey(projectId),
    queryFn: () => apiJson<ChatSummary[]>(base(projectId)),
    // Quick while a chat is working, so its status and order stay current.
    refetchInterval: (query) =>
      query.state.data?.some((chat) => isWorking(chat.status)) ? 1500 : 10000,
  });
}

export function useProjectChatQuery(projectId: number, chatId: number | undefined) {
  return useQuery({
    queryKey: chatKey(projectId, chatId ?? 0),
    queryFn: () => apiJson<ChatConversation>(`${base(projectId)}/chats/${chatId}`),
    enabled: !!chatId,
    refetchInterval: (query) =>
      query.state.data?.turns.some((turn) => isWorking(turn.status)) ? 700 : false,
  });
}

// Task numbers and titles, for #mentions and for naming tasks Chatter changed.
export function useChatTasksQuery(projectId: number) {
  return useQuery({
    queryKey: ["project-chat-tasks", projectId],
    queryFn: () => apiJson<MentionTask[]>(`${base(projectId)}/tasks`),
    refetchInterval: 5000,
  });
}

export type SendInput = { message: string; key: string; taskIds: number[] };

// Without a chat, sending starts one; the turn names the chat it landed in.
export function useSendChatMessageMutation(projectId: number, chatId: number | undefined) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (input: SendInput) =>
      apiJson<ChatTurn>(
        chatId ? `${base(projectId)}/chats/${chatId}/messages` : `${base(projectId)}/messages`,
        { method: "POST", body: JSON.stringify(input) },
      ),
    onSuccess: (turn) => {
      client.setQueryData<ChatConversation>(
        chatKey(projectId, turn.conversationId),
        (old) =>
          old
            ? { ...old, turns: [...old.turns.filter((t) => t.id !== turn.id), turn] }
            : old,
      );
      void client.invalidateQueries({ queryKey: chatKey(projectId, turn.conversationId) });
      void client.invalidateQueries({ queryKey: chatsKey(projectId) });
    },
  });
}

export function useCancelTurnMutation(projectId: number, chatId: number | undefined) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (turnId: number) =>
      apiJson<void>(`${base(projectId)}/${turnId}/cancel`, { method: "POST" }),
    onSettled: () => void client.invalidateQueries({ queryKey: chatKey(projectId, chatId ?? 0) }),
    onError: (error: Error) => toast.error(error.message),
  });
}

export function useRenameChatMutation(projectId: number) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (input: { chatId: number; title: string }) =>
      apiJson<void>(`${base(projectId)}/chats/${input.chatId}`, {
        method: "PUT",
        body: JSON.stringify({ title: input.title }),
      }),
    onSuccess: () => void client.invalidateQueries({ queryKey: chatsKey(projectId) }),
    onError: (error: Error) => toast.error(error.message),
  });
}

export function useDeleteChatMutation(projectId: number) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (chatId: number) =>
      apiJson<void>(`${base(projectId)}/chats/${chatId}`, { method: "DELETE" }),
    onSuccess: (_, chatId) => {
      client.setQueryData<ChatSummary[]>(chatsKey(projectId), (chats) =>
        chats?.filter((chat) => chat.id !== chatId),
      );
      client.removeQueries({ queryKey: chatKey(projectId, chatId) });
    },
    onError: (error: Error) => toast.error(error.message),
  });
}
