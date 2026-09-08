import { ListView } from "@/components/project/views/listView/list-view";
import { ProjectContentHeader } from "@/components/project/project-details-content-header";
import { useContext, useEffect, useMemo, useState } from "react";
import { ProjectContext } from "@/contexts/project/ProjectContext";
import { useProjectDetailsQuery } from "@/queries/useProjectDetailsQuery";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "../ui/tabs";
import { KanbanView } from "./views/kanbanView/kanban-view";
import { MonthView } from "./views/calendarViews/month-view";
import { CalendarProvider } from "@/features/calendar/contexts/calendar-context";
import { DndProvider } from "@/features/calendar/contexts/dnd-context";
import { WeekView } from "./views/calendarViews/week-view";
import { useSharedSelection } from "@/hooks/useSelection";
import { BulkActionsToolbar } from "@/features/task/bulk-actions-toolbar";
import { useConductor } from "@/features/conductor/use-conductor";
import { ConductorContext } from "@/features/conductor/conductor-context";
import { ConductorControls, ConductorFrame } from "@/features/conductor/conductor-board";
import {
  CalendarDaysIcon,
  CalendarIcon,
  LayoutDashboardIcon,
  Rows3Icon,
} from "lucide-react";

export function ProjectDetails({
  view,
  setView,
  projectId,
}: {
  view: string;
  setView: (view: string) => void;
  projectId: number;
}) {
  const [newTaskOpen, setNewTaskOpen] = useState(false);
  const [conductorOnly, setConductorOnly] = useState(false);
  const conductor = useConductor(projectId);
  const projectContext = useContext(ProjectContext);
  const {
    SetProject,
    SetWorkflow,
    SetStages,
    SetChecklists,
    SetTasks,
    Filter,
    Tasks,
  } = projectContext;
  const selection = useSharedSelection();
  const selectedTasks = useMemo(
    () => Tasks.filter((t) => selection.selectedIds.has(t.ID.toString())),
    [Tasks, selection.selectedIds],
  );
  const { isPending, data, isFetching, refetch } = useProjectDetailsQuery(
    projectId,
    Filter,
    !newTaskOpen,
  );

  useEffect(() => {
    if (data && !isPending && !isFetching) {
      SetTasks(data.Tasks);
      SetChecklists(data.Checklists);
      SetProject(data.Project);
      SetWorkflow(data.Workflow);
      SetStages(data.Stages);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data, isFetching, isPending, projectId]);

  useEffect(() => {
    refetch();
  }, [refetch, Filter]);

  const visibleContext = useMemo(() => {
    if (!conductorOnly) return projectContext;
    const ids = new Set(conductor.data?.tasks.map((task) => task.taskId));
    return { ...projectContext, Tasks: Tasks.filter((task) => ids.has(task.ID)) };
  }, [conductorOnly, conductor.data?.tasks, projectContext, Tasks]);

  return (
    <ConductorContext.Provider value={conductor}>
    <ProjectContext.Provider value={visibleContext}>
    <div className="flex flex-col gap-4 grow min-h-0 overflow-visible">
      <ProjectContentHeader />

      <div className="flex grow flex-col gap-4 overflow-visible min-h-0">
        <CalendarProvider events={[]} users={[]} view="month">
          <DndProvider>
            <Tabs value={view} onValueChange={setView} className="h-full">
              <div className="flex flex-wrap items-center justify-between gap-3">
              <TabsList>
                <TabsTrigger value="list">
                  <Rows3Icon />
                  List
                </TabsTrigger>
                <TabsTrigger value="kanban">
                  <LayoutDashboardIcon />
                  Kanban
                </TabsTrigger>
                <TabsTrigger value="month">
                  <CalendarDaysIcon />
                  Month
                </TabsTrigger>
                <TabsTrigger value="week">
                  <CalendarIcon />
                  Week
                </TabsTrigger>
              </TabsList>
              <ConductorControls projectId={projectId} only={conductorOnly} onOnlyChange={(value) => { setConductorOnly(value); selection.clearSelection(); }} />
              </div>

              <TabsContent
                value="list"
                className="h-full overflow-visible min-h-0"
              >
                <ConductorFrame only={conductorOnly}><ListView setTaskDrawerOpen={setNewTaskOpen} /></ConductorFrame>
              </TabsContent>
              <TabsContent
                value="kanban"
                className="h-full overflow-visible min-h-0"
              >
                <ConductorFrame only={conductorOnly}><KanbanView setTaskDrawerOpen={setNewTaskOpen} /></ConductorFrame>
              </TabsContent>
              <TabsContent
                value="month"
                className="h-full overflow-visible min-h-0"
              >
                <MonthView setTaskDrawerOpen={setNewTaskOpen} />
              </TabsContent>
              <TabsContent
                value="week"
                className="h-full overflow-visible min-h-0"
              >
                <WeekView setTaskDrawerOpen={setNewTaskOpen} />
              </TabsContent>
            </Tabs>
          </DndProvider>
        </CalendarProvider>
      </div>

      <BulkActionsToolbar
        selectedTasks={selectedTasks}
        onClear={selection.clearSelection}
      />
    </div>
    </ProjectContext.Provider>
    </ConductorContext.Provider>
  );
}
