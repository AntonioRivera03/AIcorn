import { useState } from "react";
import { RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  useAISettings,
  useUpdateAISettings,
} from "@/features/ai/queries/use-ai";
import type { AISettings } from "@/features/ai/types";

export function AIEngineSettings() {
  const query = useAISettings();
  const update = useUpdateAISettings();
  const [changes, setDraft] = useState<AISettings | null>(null);
  const draft = changes ?? query.data?.settings;
  const save = () => {
    if (draft && JSON.stringify(draft) !== JSON.stringify(query.data?.settings))
      update.mutate(draft, {
        onSuccess: () => setDraft(null),
        onError: () => setDraft(null),
      });
  };
  return (
    <section
      className="rounded-xl border border-border p-5 space-y-5"
      aria-label="AI engine settings"
    >
      <div className="flex items-center justify-between gap-3">
        <div>
          <h2 className="font-medium">OpenCode</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            Runs start only when you ask. Task owners and stages stay
            independent.
          </p>
        </div>
        <Button
          variant="ghost"
          size="sm"
          disabled={query.isFetching}
          onClick={() => void query.refetch()}
        >
          <RefreshCw className="size-3" /> Check
        </Button>
      </div>
      {query.isError && (
        <p className="text-sm text-destructive" role="alert">
          {query.error.message}
        </p>
      )}
      {!draft ? (
        <p className="text-sm text-muted-foreground">Loading settings…</p>
      ) : (
        <>
          <div className="space-y-2">
            <Label htmlFor="ai-model">Model</Label>
            <Input
              id="ai-model"
              value={draft.model}
              placeholder="provider/model"
              disabled={update.isPending}
              onChange={(e) => setDraft({ ...draft, model: e.target.value })}
              onBlur={save}
            />
            <p className="text-xs text-muted-foreground">
              Use a model available through your OpenCode account.
              Authentication stays with OpenCode.
            </p>
          </div>
          <details>
            <summary className="cursor-pointer text-sm text-muted-foreground">
              Execution settings
            </summary>
            <div className="mt-4 space-y-4">
              <div className="space-y-2">
                <Label htmlFor="ai-executable">OpenCode executable</Label>
                <Input
                  id="ai-executable"
                  value={draft.executable}
                  placeholder="Detect automatically"
                  disabled={update.isPending}
                  onChange={(e) =>
                    setDraft({ ...draft, executable: e.target.value })
                  }
                  onBlur={save}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="ai-timeout">Time limit (seconds)</Label>
                <Input
                  id="ai-timeout"
                  type="number"
                  min={10}
                  max={1800}
                  value={draft.timeoutSeconds}
                  disabled={update.isPending}
                  onChange={(e) =>
                    setDraft({
                      ...draft,
                      timeoutSeconds: Number(e.target.value),
                    })
                  }
                  onBlur={save}
                />
                <p className="text-xs text-muted-foreground">
                  The run stops at this limit. A dollar spending cap is not
                  supported by this adapter.
                </p>
              </div>
            </div>
          </details>
        </>
      )}
      <p className="text-sm text-muted-foreground" role="status">
        {query.isFetching
          ? "Checking engine…"
          : query.data?.engine.ready
            ? `Executable ready · OpenCode ${query.data.engine.version}. Model access is checked when a run starts.`
            : query.data?.engine.error}
      </p>
    </section>
  );
}
