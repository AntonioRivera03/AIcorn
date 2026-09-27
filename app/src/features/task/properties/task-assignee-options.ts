type TaskWithAssignee = {
  readonly Assignee: string;
};

// Assignees are the workspace's members. Anything else already assigned on a
// task (an AI agent's name, someone who has since left) stays selectable under
// "Other" so it can be reassigned consistently.
export type AssigneeOptions = {
  readonly members: readonly string[];
  readonly others: readonly string[];
};

export const createAssigneeOptions = (
  memberNames: readonly string[],
  tasks: readonly TaskWithAssignee[],
): AssigneeOptions => {
  const members = [...new Set(memberNames)];
  const memberSet = new Set(members);
  const others = new Set<string>();
  for (const task of tasks) {
    if (task.Assignee && !memberSet.has(task.Assignee)) others.add(task.Assignee);
  }
  return { members, others: [...others] };
};

export const filterAssigneeOptions = (
  options: AssigneeOptions,
  searchValue: string,
): AssigneeOptions => {
  const search = searchValue.trim().toLowerCase();
  if (search === "") return options;
  const matches = (option: string) => option.toLowerCase().includes(search);
  return {
    members: options.members.filter(matches),
    others: options.others.filter(matches),
  };
};
