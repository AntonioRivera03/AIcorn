import { useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ExternalLink,
  GitBranch,
  GitPullRequest,
  Link2,
  Trash2,
} from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";

type TaskLink = {
  id: number;
  taskId: number;
  url: string;
  kind: "branch" | "pull_request";
  label: string;
  reference: string;
  revision: number;
};
async function request<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await fetch(url, init);
  if (!response.ok)
    throw new Error(
      (await response.text()).trim() || "Could not load GitHub links",
    );
  return response.status === 204 ? (undefined as T) : response.json();
}

export function TaskGitHubLinks({ taskId }: { taskId: number }) {
  const client = useQueryClient();
  const key = ["task-github-links", taskId];
  const url = `/api/task-links/task/${taskId}`;
  const query = useQuery({
    queryKey: key,
    queryFn: () => request<TaskLink[]>(url),
    enabled: taskId > 0,
    refetchInterval: 5000,
  });
  const [newURL, setNewURL] = useState("");
  const pending = useRef<string | null>(null);
  const create = useMutation({
    mutationFn: (value: string) =>
      request<TaskLink>(url, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ url: value, label: "" }),
      }),
    onSuccess: (saved, sent) => {
      setNewURL((current) => (current.trim() === sent ? "" : current));
      client.setQueryData<TaskLink[]>(key, (items) => [
        ...(items ?? []).filter((item) => item.id !== saved.id),
        saved,
      ]);
    },
    onSettled: () => {
      pending.current = null;
    },
  });
  const add = () => {
    const value = newURL.trim();
    if (!value || pending.current) return;
    pending.current = value;
    create.mutate(value);
  };
  if (!taskId) return null;
  return (
    <section
      aria-label="GitHub links"
      className="my-3 rounded-lg border bg-card"
    >
      <div className="flex items-center gap-2 border-b px-3 py-2">
        <Link2 className="size-4 text-muted-foreground" />
        <h2 className="text-sm font-medium">GitHub links</h2>
      </div>
      {query.isPending && (
        <p className="p-3 text-sm text-muted-foreground">Loading links…</p>
      )}
      {query.error && (
        <p role="alert" className="p-3 text-sm text-destructive">
          {query.error.message}
          <Button variant="link" onClick={() => query.refetch()}>
            Retry
          </Button>
        </p>
      )}
      <div className="divide-y">
        {query.data?.map((link) => (
          <LinkRow key={link.id} link={link} />
        ))}
      </div>
      <div className="p-3">
        <Input
          aria-label="Add GitHub PR or branch URL"
          placeholder="Paste a GitHub PR or branch URL…"
          value={newURL}
          onChange={(event) => {
            setNewURL(event.target.value);
            create.reset();
          }}
          onBlur={add}
          onKeyDown={(event) => {
            if (event.key === "Enter") {
              event.preventDefault();
              add();
            }
            if (event.key === "Escape") {
              setNewURL("");
              create.reset();
            }
          }}
        />
        <p className="mt-1 text-xs text-muted-foreground">
          Press Enter or leave the field to attach a link.
        </p>
        {create.error && (
          <p role="alert" className="mt-1 text-xs text-destructive">
            {create.error.message}
          </p>
        )}
      </div>
    </section>
  );
}

function LinkRow({ link }: { link: TaskLink }) {
  const client = useQueryClient();
  const key = ["task-github-links", link.taskId];
  const url = `/api/task-links/task/${link.taskId}/${link.id}`;
  const [confirm, setConfirm] = useState(false);
  const edit = useMutation({
    scope: { id: `task-github-link-${link.id}` },
    mutationFn: async (patch: Partial<Pick<TaskLink, "url" | "label">>) => {
      await client.cancelQueries({ queryKey: key });
      const current = client
        .getQueryData<TaskLink[]>(key)
        ?.find((item) => item.id === link.id);
      if (!current) throw new Error("This link is no longer available.");
      return request<TaskLink>(url, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          url: current.url,
          label: current.label,
          revision: current.revision,
          ...patch,
        }),
      });
    },
    onSuccess: (saved) =>
      client.setQueryData<TaskLink[]>(key, (items) =>
        items?.map((item) => (item.id === saved.id ? saved : item)),
      ),
    onError: () => {
      void client.invalidateQueries({ queryKey: key });
    },
  });
  const remove = useMutation({
    scope: { id: `task-github-link-${link.id}` },
    mutationFn: () => {
      const current =
        client
          .getQueryData<TaskLink[]>(key)
          ?.find((item) => item.id === link.id) ?? link;
      return request<void>(url, {
        method: "DELETE",
        headers: { "If-Match": String(current.revision) },
      });
    },
    onSuccess: () =>
      client.setQueryData<TaskLink[]>(key, (items) =>
        items?.filter((item) => item.id !== link.id),
      ),
    onError: (error: Error) => {
      toast.error(error.message);
      void client.invalidateQueries({ queryKey: key });
    },
  });
  const Icon = link.kind === "pull_request" ? GitPullRequest : GitBranch;
  return (
    <div className="flex items-start gap-2 p-3" aria-label={link.reference}>
      <Icon className="mt-2 size-4 shrink-0 text-muted-foreground" />
      <div className="min-w-0 flex-1 space-y-1">
        <LinkField
          label="GitHub link label"
          value={link.label}
          placeholder={link.reference}
          save={(value) => edit.mutateAsync({ label: value })}
        />
        <LinkField
          label="GitHub link URL"
          value={link.url}
          save={(value) => edit.mutateAsync({ url: value })}
        />
      </div>
      <Button variant="ghost" size="icon" asChild>
        <a
          href={link.url}
          target="_blank"
          rel="noopener noreferrer"
          aria-label={`Open ${link.reference} on GitHub`}
        >
          <ExternalLink className="size-4" />
        </a>
      </Button>
      <Button
        variant="ghost"
        size="icon"
        aria-label={`Remove ${link.reference}`}
        onClick={() => setConfirm(true)}
      >
        <Trash2 className="size-4" />
      </Button>
      <AlertDialog open={confirm} onOpenChange={setConfirm}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Remove GitHub link?</AlertDialogTitle>
            <AlertDialogDescription>
              This removes the reference from this task. The branch or pull
              request on GitHub is unchanged.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={remove.isPending}
              onClick={() => remove.mutate()}
            >
              Remove link
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

function LinkField({
  label,
  value,
  placeholder,
  save,
}: {
  label: string;
  value: string;
  placeholder?: string;
  save: (value: string) => Promise<unknown>;
}) {
  const [draft, setDraft] = useState<string | null>(null);
  const [error, setError] = useState("");
  const pending = useRef<string | null>(null);
  const commit = async () => {
    if (
      draft === null ||
      (draft === value && !error) ||
      draft === pending.current
    )
      return;
    const sent = draft;
    pending.current = sent;
    try {
      await save(sent);
      setDraft((current) => (current === sent ? null : current));
      setError("");
    } catch (error) {
      setError(
        error instanceof Error ? error.message : "Could not save this link",
      );
    } finally {
      if (pending.current === sent) pending.current = null;
    }
  };
  return (
    <div>
      <Input
        aria-label={label}
        placeholder={placeholder}
        value={draft ?? value}
        onChange={(event) => setDraft(event.target.value)}
        onBlur={() => void commit()}
        onKeyDown={(event) => {
          if (event.key === "Enter") event.currentTarget.blur();
          if (event.key === "Escape") {
            setDraft(null);
            setError("");
          }
        }}
        className="h-8 border-transparent px-1 shadow-none hover:border-input focus-visible:border-input"
      />
      {error && (
        <p role="alert" className="text-xs text-destructive">
          {error}
        </p>
      )}
    </div>
  );
}
