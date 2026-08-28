import type { Stage } from "@/types/types";

type StageWithPersona = Pick<Stage, "Persona"> & {
  readonly Persona: NonNullable<Stage["Persona"]>;
};

export const shouldShowPersonaIndicator = (
  stage: Pick<Stage, "Persona">,
): stage is StageWithPersona => stage.Persona != null;
