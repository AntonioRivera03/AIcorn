import {
  Page,
  PageContent,
  PageHeader,
  PageTitle,
} from "@/components/page/Page";
import { Separator } from "@/components/ui/separator";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { AISettingsPanel } from "@/features/ai/settings/ai-settings-panel";
import { AccountPanel } from "@/features/settings/account-panel";
import { EmptyPanel } from "@/features/settings/empty-panel";
import { PreferencesPanel } from "@/features/settings/preferences-panel";
import { MembersPanel } from "@/features/workspaces/members/members-panel";
import { createFileRoute } from "@tanstack/react-router";

const TABS = ["account", "members", "preferences", "ai", "notifications"] as const;
type SettingsTab = (typeof TABS)[number];

export const Route = createFileRoute("/app/settings")({
  validateSearch: (search: Record<string, unknown>): { tab?: SettingsTab } => ({
    tab: TABS.find((tab) => tab === search.tab),
  }),
  component: RouteComponent,
});

function RouteComponent() {
  const { tab = "account" } = Route.useSearch();
  const navigate = Route.useNavigate();

  return (
    <Page>
      <PageHeader breadcrumb={["Settings"]} />
      <PageContent>
        <PageTitle
          title="Settings"
          description="Manage your account, who's in your organization, your preferences, and Aycorn AI."
        />

        <Tabs
          value={tab}
          onValueChange={(value) =>
            navigate({ search: { tab: value as SettingsTab }, replace: true })
          }
          className="flex-1 min-h-0"
        >
          <TabsList className="max-w-full justify-start overflow-x-auto">
            <TabsTrigger value="account">Account</TabsTrigger>
            <TabsTrigger value="members">Members</TabsTrigger>
            <TabsTrigger value="preferences">Preferences</TabsTrigger>
            <TabsTrigger value="ai">Aycorn AI</TabsTrigger>
            <TabsTrigger value="notifications">Notifications</TabsTrigger>
          </TabsList>

          <Separator />

          <TabsContent value="account" className="pt-4">
            <AccountPanel />
          </TabsContent>

          <TabsContent value="members" className="pt-4">
            <MembersPanel />
          </TabsContent>

          <TabsContent value="preferences" className="pt-4">
            <PreferencesPanel />
          </TabsContent>

          <TabsContent value="ai" className="pt-4">
            <AISettingsPanel />
          </TabsContent>

          <TabsContent value="notifications" className="pt-4">
            <EmptyPanel
              title="Notifications"
              description="Decide which events Aycorn should alert you about."
            />
          </TabsContent>
        </Tabs>
      </PageContent>
    </Page>
  );
}
