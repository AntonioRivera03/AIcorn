import type { ScheduledJob, TaskTemplate } from "./use-jobs";

export type JobGridItem =
  | { kind: "templates"; template: TaskTemplate }
  | { kind: "jobs"; job: ScheduledJob; template?: TaskTemplate };

export function jobGridItems(
  templates: TaskTemplate[],
  jobs: ScheduledJob[],
  search: string,
): JobGridItem[] {
  const used = new Set(jobs.map((job) => job.templateId));
  const items: JobGridItem[] = [
    ...jobs.map((job) => ({
      kind: "jobs" as const,
      job,
      template: templates.find((template) => template.id === job.templateId),
    })),
    ...templates
      .filter((template) => !used.has(template.id))
      .map((template) => ({ kind: "templates" as const, template })),
  ];
  const terms = search.toLocaleLowerCase().trim().split(/\s+/).filter(Boolean);
  return items.filter((item) => {
    const text = [
      item.kind === "jobs" ? item.job.name : item.template.name,
      item.kind === "jobs" ? "job" : "template",
      item.template?.title,
      item.template?.body,
      item.kind === "jobs" ? item.job.schedule : "",
    ]
      .join(" ")
      .toLocaleLowerCase();
    return terms.every((term) => text.includes(term));
  });
}
