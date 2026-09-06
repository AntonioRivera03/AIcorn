import { SELF_ASSIGNEE } from "@/features/task/stage-move-assignee";

type TaskWithAssignee = {
  readonly Assignee: string;
};

export type AssigneeOptionState = {
  readonly filteredOptions: readonly string[];
  readonly hasExactMatch: boolean;
  readonly showCreate: boolean;
  readonly trimmedSearch: string;
};

export const createAssigneeOptions = (
  tasks: readonly TaskWithAssignee[],
): readonly string[] => {
  const names = new Set<string>([SELF_ASSIGNEE]);
  for (const task of tasks) {
    if (task.Assignee) names.add(task.Assignee);
  }
  return [...names];
};

export const getAssigneeOptionState = (
  options: readonly string[],
  searchValue: string,
): AssigneeOptionState => {
  const trimmedSearch = searchValue.trim();
  const normalizedSearch = trimmedSearch.toLowerCase();
  const filteredOptions =
    normalizedSearch === ""
      ? options
      : options.filter((option) =>
          option.toLowerCase().includes(normalizedSearch),
        );
  const hasExactMatch = options.some(
    (option) => option.toLowerCase() === normalizedSearch,
  );

  return {
    filteredOptions,
    hasExactMatch,
    showCreate: trimmedSearch !== "" && !hasExactMatch,
    trimmedSearch,
  };
};
