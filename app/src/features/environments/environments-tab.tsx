import { useId, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Box, CheckCircle2, RefreshCw, Square } from "lucide-react";
import { toast } from "sonner";
import { BulkActionsToolbarBase } from "@/components/bulk-actions-toolbar-base";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { selectedItemClasses, useSharedSelection } from "@/hooks/useSelection";
import { EnvironmentCard } from "./environment-card";
import {
  environmentRequest,
  useEnvironmentMutations,
  useEnvironmentSettings,
  useEnvironments,
  type ConnectionStatus,
  type EnvironmentBranch,
  type EnvironmentSettings,
} from "./use-environments";

function AutoField({
  label,
  value,
  save,
  multiline = false,
  hint,
  numeric = false,
}: {
  label: string;
  value: string | number;
  save: (value: string) => void;
  multiline?: boolean;
  hint?: string;
  numeric?: boolean;
}) {
  const id = useId();
  const [draft, setDraft] = useState(String(value));
  const props = {
    id,
    value: draft,
    onChange: (
      event: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>,
    ) => setDraft(event.target.value),
    onBlur: () => {
      if (draft !== String(value)) save(draft);
    },
  };
  return (
    <div className="min-w-0 space-y-2">
      <Label htmlFor={id}>{label}</Label>
      {multiline ? (
        <Textarea
          {...props}
          spellCheck={false}
          className="min-h-28 font-mono text-xs"
        />
      ) : (
        <Input {...props} type={numeric ? "number" : "text"} />
      )}
      {hint && <p className="text-xs text-muted-foreground">{hint}</p>}
    </div>
  );
}

function RecipeSettings({
  settings,
  update,
}: {
  settings: EnvironmentSettings;
  update: (patch: Partial<EnvironmentSettings>) => void;
}) {
  const stringField = (
    key: keyof EnvironmentSettings,
    label: string,
    hint?: string,
    multiline = false,
  ) => (
    <AutoField
      key={`${key}-${String(settings[key])}`}
      label={label}
      value={String(settings[key])}
      hint={hint}
      multiline={multiline}
      save={(value) => update({ [key]: value })}
    />
  );
  const numberField = (key: keyof EnvironmentSettings, label: string) => (
    <AutoField
      key={`${key}-${String(settings[key])}`}
      label={label}
      value={Number(settings[key])}
      numeric
      save={(value) => {
        if (!value || !Number.isInteger(Number(value))) {
          toast.error("Enter a whole number");
          return;
        }
        update({ [key]: Number(value) });
      }}
    />
  );
  const commandField = (key: "command" | "testCommand", label: string) => (
    <AutoField
      key={`${key}-${JSON.stringify(settings[key])}`}
      label={label}
      value={JSON.stringify(settings[key])}
      multiline
      hint="JSON argument array. Use [] to keep the image’s default command or disable tests together with an empty test target."
      save={(value) => {
        try {
          const command: unknown = JSON.parse(value);
          if (
            !Array.isArray(command) ||
            !command.every((arg) => typeof arg === "string")
          )
            throw new Error();
          update({ [key]: command });
        } catch {
          toast.error("Enter a JSON array of command arguments");
        }
      }}
    />
  );
  return (
    <details className="rounded-xl border border-border p-4">
      <summary className="cursor-pointer font-medium">
        Build recipe and resource limits
      </summary>
      <div className="mt-5 space-y-5">
        <p className="text-sm text-muted-foreground">
          This project recipe is copied into each new environment. Editing it
          affects future previews. Builds run in Docker; application previews
          and test jobs run in Kubernetes.
        </p>
        <div className="grid gap-4 md:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="environment-profile">Application profile</Label>
            <Select
              value={settings.profile}
              onValueChange={(value) =>
                update({ profile: value as EnvironmentSettings["profile"] })
              }
            >
              <SelectTrigger id="environment-profile" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="aycorn">
                  AICorn · enforced preview mode
                </SelectItem>
                <SelectItem value="custom">Custom application</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-2">
            <Label htmlFor="environment-platform">Image platform</Label>
            <Select
              value={settings.platform}
              onValueChange={(platform) => update({ platform })}
            >
              <SelectTrigger id="environment-platform" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="linux/amd64">Linux · AMD64</SelectItem>
                <SelectItem value="linux/arm64">Linux · ARM64</SelectItem>
              </SelectContent>
            </Select>
          </div>
          {stringField("previewTarget", "Preview build target")}
          {stringField(
            "testTarget",
            "Test build target",
            "Leave empty, together with an empty test command, to skip container tests.",
          )}
          {numberField("port", "Container port")}
          {stringField("healthPath", "Readiness path")}
        </div>
        {stringField(
          "dockerfile",
          "Dockerfile recipe",
          "Use named preview and test stages. Source files are filtered before building; host databases, credentials, Git metadata, and dependency folders are excluded.",
          true,
        )}
        <div className="grid gap-4 md:grid-cols-2">
          {commandField("command", "Application command")}
          {commandField("testCommand", "Test command")}
        </div>
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {numberField("cpuMillis", "Preview CPU (millicores)")}
          {numberField("memoryMiB", "Preview memory (MiB)")}
          {numberField("storageGiB", "Preview data (GiB)")}
          {numberField("testCpuMillis", "Test CPU (millicores)")}
          {numberField("testMemoryMiB", "Test memory (MiB)")}
          {numberField("timeoutSeconds", "Build and startup timeout (seconds)")}
          {stringField(
            "storageClass",
            "Storage class",
            "Empty uses the cluster’s default provisioner.",
          )}
        </div>
        <div className="flex items-start gap-3">
          <Checkbox
            id="environment-test-network"
            checked={settings.allowTestNetwork}
            onCheckedChange={(checked) =>
              update({ allowTestNetwork: checked === true })
            }
          />
          <div>
            <Label htmlFor="environment-test-network">
              Allow internet access during tests
            </Label>
            <p className="mt-1 text-xs text-muted-foreground">
              Allows public HTTP/HTTPS and cluster DNS for test jobs. Cluster
              network policy support is required. Preview applications keep
              outbound traffic blocked.
            </p>
          </div>
        </div>
      </div>
    </details>
  );
}

export function EnvironmentsTab({ projectId }: { projectId: number }) {
  const settingsQuery = useEnvironmentSettings(projectId);
  const environments = useEnvironments(projectId);
  const mutations = useEnvironmentMutations(projectId);
  const selection = useSharedSelection();
  const [chosenBranch, setChosenBranch] = useState("");
  const [chosenSource, setChosenSource] = useState<{
    branch: string;
    value: "working-tree" | "commit";
  } | null>(null);
  const branches = useQuery({
    queryKey: ["environment-branches", projectId, "details"],
    queryFn: () =>
      environmentRequest<EnvironmentBranch[]>(
        `/api/environments/project/${projectId}/branches?details=true`,
      ),
  });
  const connection = useQuery({
    queryKey: ["environment-connection", projectId],
    queryFn: ({ signal }) =>
      environmentRequest<ConnectionStatus>(
        `/api/environments/project/${projectId}/check`,
        "GET",
        undefined,
        signal,
      ),
    retry: false,
    staleTime: 30000,
  });
  const selectedBranch =
    branches.data?.find((item) => item.name === chosenBranch) ||
    branches.data?.find((item) => item.current) ||
    branches.data?.find((item) => item.name === "main") ||
    branches.data?.[0];
  const branch = selectedBranch?.name || "";
  const source =
    selectedBranch?.hasWorktree &&
    (chosenSource?.branch !== branch || chosenSource.value !== "commit")
      ? "working-tree"
      : "commit";
  const selected = (environments.data?.environments ?? [])
    .filter((e) => selection.selectedIds.has(`environment-${e.id}`))
    .map((e) => e.id);
  if (settingsQuery.isPending)
    return (
      <p className="p-6 text-sm text-muted-foreground">Loading environments…</p>
    );
  if (!settingsQuery.data)
    return (
      <p role="alert" className="p-6 text-destructive">
        Could not load environment settings. {settingsQuery.error?.message}
      </p>
    );
  const settings = settingsQuery.data;
  const update = (patch: Partial<EnvironmentSettings>) =>
    settingsQuery.update.mutate(patch);
  return (
    <section className="space-y-6 pb-24">
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Box className="size-5" />
            Branch environments
          </CardTitle>
          <CardDescription>
            Run a version of your application for review, with its own data and
            test results.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-5">
          <div className="grid gap-4 md:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="environment-context">Kubernetes context</Label>
              <Select
                value={settings.context || "none"}
                onValueChange={(value) =>
                  update({ context: value === "none" ? "" : value })
                }
              >
                <SelectTrigger id="environment-context" className="w-full">
                  <SelectValue placeholder="Choose a context" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="none">Choose a context</SelectItem>
                  {Array.from(
                    new Set([
                      ...(connection.data?.contexts ?? []),
                      ...(settings.context ? [settings.context] : []),
                    ]),
                  ).map((context) => (
                    <SelectItem key={context} value={context}>
                      {context}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label htmlFor="environment-transport">Image delivery</Label>
              <Select
                value={settings.transport}
                onValueChange={(transport) =>
                  update({
                    transport: transport as EnvironmentSettings["transport"],
                  })
                }
              >
                <SelectTrigger id="environment-transport" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="kind">Local kind cluster</SelectItem>
                  <SelectItem value="registry">Container registry</SelectItem>
                </SelectContent>
              </Select>
            </div>
            {settings.transport === "kind" ? (
              <AutoField
                key={settings.kindCluster}
                label="kind cluster name"
                value={settings.kindCluster}
                save={(kindCluster) => update({ kindCluster })}
              />
            ) : (
              <>
                <AutoField
                  key={settings.imageRepository}
                  label="Image repository"
                  value={settings.imageRepository}
                  hint="For example ghcr.io/you/aycorn-preview. Docker must already be signed in."
                  save={(imageRepository) => update({ imageRepository })}
                />
                <AutoField
                  key={`secret-ns-${settings.pullSecretNamespace}`}
                  label="Pull secret namespace (optional)"
                  value={settings.pullSecretNamespace}
                  save={(pullSecretNamespace) =>
                    update({ pullSecretNamespace })
                  }
                />
                <AutoField
                  key={`secret-${settings.pullSecretName}`}
                  label="Pull secret name (optional)"
                  value={settings.pullSecretName}
                  save={(pullSecretName) => update({ pullSecretName })}
                />
              </>
            )}
          </div>
          <div className="flex flex-wrap items-center gap-3">
            <Button
              size="sm"
              variant="outline"
              disabled={connection.isFetching}
              onClick={() => void connection.refetch()}
            >
              <RefreshCw
                className={`size-4 ${connection.isFetching ? "animate-spin motion-reduce:animate-none" : ""}`}
              />
              Check connection
            </Button>
            <p
              role="status"
              className={`text-sm ${connection.data?.connected ? "text-primary" : "text-muted-foreground"}`}
            >
              {connection.isFetching ? (
                "Checking cluster and tools…"
              ) : connection.data?.connected ? (
                <span className="inline-flex items-center gap-1">
                  <CheckCircle2 className="size-4" />
                  Connected
                </span>
              ) : (
                connection.error?.message ||
                connection.data?.error ||
                "Check the cluster connection to continue."
              )}
            </p>
          </div>
          <p className="text-xs text-muted-foreground">
            {settingsQuery.update.isPending
              ? "Saving…"
              : settingsQuery.update.isError
                ? "The last change could not be saved. Correct it and try again."
                : "Changes save automatically. Each preview uses the settings captured when it was created."}
          </p>
          <div className="grid gap-4 md:grid-cols-2">
            <AutoField
              key={`max-${settings.maxRunning}`}
              numeric
              label="Maximum running previews"
              value={settings.maxRunning}
              save={(value) => update({ maxRunning: Number(value) })}
            />
            <AutoField
              key={`retention-${settings.retentionHours}`}
              numeric
              label="Auto-stop after (hours)"
              value={settings.retentionHours}
              save={(value) => update({ retentionHours: Number(value) })}
            />
          </div>
          <div className="flex items-start gap-3">
            <Checkbox
              id="environment-auto-preview"
              checked={settings.autoPreview}
              onCheckedChange={(checked) =>
                update({ autoPreview: checked === true })
              }
            />
            <div>
              <Label htmlFor="environment-auto-preview">
                Prepare previews when Conductor finishes code tasks
              </Label>
              <p className="mt-1 text-xs text-muted-foreground">
                Applies to future completions. Builds run independently of human
                review. Failed builds never restart the coding agent.
              </p>
            </div>
          </div>
        </CardContent>
      </Card>
      <div className="space-y-4">
        <div className="flex flex-wrap items-end gap-3">
          <div className="min-w-48 flex-1 space-y-2">
            <Label htmlFor="environment-branch">Local branch</Label>
            <Select
              value={branch || undefined}
              disabled={!branches.data?.length}
              onValueChange={(value) => {
                setChosenBranch(value);
                setChosenSource(null);
              }}
            >
              <SelectTrigger id="environment-branch" className="w-full">
                <SelectValue
                  placeholder={
                    branches.isPending
                      ? "Loading branches…"
                      : "Link a repository in General settings"
                  }
                />
              </SelectTrigger>
              <SelectContent>
                {branches.data?.map((item) => (
                  <SelectItem key={item.name} value={item.name}>
                    {item.name}
                    {item.current ? " (current)" : ""}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="min-w-44 space-y-2">
            <Label htmlFor="environment-source">Source</Label>
            <Select
              value={source}
              disabled={!selectedBranch}
              onValueChange={(value) =>
                setChosenSource({
                  branch,
                  value: value === "working-tree" ? "working-tree" : "commit",
                })
              }
            >
              <SelectTrigger
                id="environment-source"
                aria-describedby="environment-source-hint"
                className="w-full"
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem
                  value="working-tree"
                  disabled={!selectedBranch?.hasWorktree}
                >
                  Working tree
                </SelectItem>
                <SelectItem value="commit">Latest commit</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <Button
            disabled={
              !selectedBranch || mutations.create.isPending || !settings.context
            }
            onClick={() =>
              mutations.create.mutate({
                branch,
                includeChanges: source === "working-tree",
                requestKey: crypto.randomUUID(),
              })
            }
          >
            <Box className="size-4" />
            New preview
          </Button>
          <Button
            variant="outline"
            size="icon"
            aria-label="Refresh branches"
            disabled={branches.isFetching}
            onClick={() => void branches.refetch()}
          >
            <RefreshCw className="size-4" />
          </Button>
        </div>
        <p
          id="environment-source-hint"
          className="text-xs text-muted-foreground"
        >
          {source === "working-tree"
            ? "Captures this branch’s local files, including uncommitted edits, as a fixed snapshot. Later edits require a new preview."
            : "Uses the branch’s latest commit. Local uncommitted edits are excluded."}
          {selectedBranch &&
            !selectedBranch.hasWorktree &&
            " This branch has no local working tree."}
        </p>
        {(environments.error || branches.error) && (
          <p role="alert" className="text-sm text-destructive">
            {environments.error?.message || branches.error?.message}
          </p>
        )}
        {!environments.isPending && !environments.data?.environments.length && (
          <div className="rounded-xl border border-dashed border-border p-8 text-center text-sm text-muted-foreground">
            No previews yet. Choose a branch or preview an agent’s completed
            work from its task.
          </div>
        )}
        <div className="grid items-start gap-4 xl:grid-cols-2">
          {environments.data?.environments.map((e) => (
            <article
              key={e.id}
              {...selection.getItemProps(`environment-${e.id}`)}
              data-task-card=""
              className={`selectable relative min-w-0 rounded-xl border border-border bg-card ${selectedItemClasses()}`}
            >
              <div className="flex items-center gap-2 border-b border-border px-4 py-2">
                <Checkbox
                  aria-label={`Select ${e.name}`}
                  onClick={(event) => event.stopPropagation()}
                  checked={selection.selectedIds.has(`environment-${e.id}`)}
                  onCheckedChange={(checked) =>
                    selection.setSelectedIds((old) => {
                      const next = new Set(old);
                      if (checked) next.add(`environment-${e.id}`);
                      else next.delete(`environment-${e.id}`);
                      return next;
                    })
                  }
                />
                <span className="text-xs text-muted-foreground">
                  Preview #{e.id}
                  {e.jobId ? ` · Run #${e.jobId}` : ""}
                </span>
              </div>
              <EnvironmentCard environment={e} />
            </article>
          ))}
        </div>
      </div>
      <RecipeSettings settings={settings} update={update} />
      <BulkActionsToolbarBase
        count={selected.length}
        onClear={selection.clearSelection}
        delete={{
          title: `Delete ${selected.length} environments and their data?`,
          description:
            "Permanently removes the selected previews and their databases. Source branches and task worktrees are kept.",
          busy: mutations.bulk.isPending,
          onConfirm: () =>
            mutations.bulk.mutate(
              { ids: selected, action: "delete" },
              { onSuccess: selection.clearSelection },
            ),
        }}
      >
        <Button
          variant="ghost"
          size="sm"
          disabled={mutations.bulk.isPending}
          onClick={() =>
            mutations.bulk.mutate(
              { ids: selected, action: "stop" },
              { onSuccess: selection.clearSelection },
            )
          }
        >
          <Square className="size-4" />
          Stop
        </Button>
      </BulkActionsToolbarBase>
    </section>
  );
}
