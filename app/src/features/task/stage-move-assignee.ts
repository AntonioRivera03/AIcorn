export const SELF_ASSIGNEE = "Me" as const;

type NamedPersona = {
  readonly Name: string;
};

type StageWithPersona = {
  readonly Persona?: NamedPersona | null;
};

type StageMoveAssigneeInput = {
  readonly currentAssignee: string;
  readonly targetPersonaName?: string | null;
  readonly personaNames: ReadonlySet<string>;
};

export const createPersonaNames = (
  personas: readonly NamedPersona[],
  stages: readonly StageWithPersona[],
): ReadonlySet<string> => {
  const names = new Set<string>();
  for (const persona of personas) {
    if (persona.Name !== "") names.add(persona.Name);
  }
  for (const stage of stages) {
    if (stage.Persona?.Name) names.add(stage.Persona.Name);
  }
  return names;
};

export const isPersona = (
  name: string,
  personaNames: ReadonlySet<string>,
): boolean => personaNames.has(name);

export const getStageMoveAssignee = ({
  currentAssignee,
  targetPersonaName,
  personaNames,
}: StageMoveAssigneeInput): string => {
  if (!targetPersonaName || currentAssignee === SELF_ASSIGNEE) {
    return currentAssignee;
  }
  return currentAssignee === "" || isPersona(currentAssignee, personaNames)
    ? targetPersonaName
    : currentAssignee;
};
