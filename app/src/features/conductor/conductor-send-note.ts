import type { ConductorBoard } from "@/features/conductor/use-conductor";

// Sending a task to Conductor always queues it, even while Conductor can't
// run yet — so the toast needs to say why, not just that it happened.
export function conductorSendNote(
  board: Pick<ConductorBoard, "settings" | "configurationError"> | undefined,
): string {
  if (!board || board.settings.enabled) return "";
  if (board.configurationError)
    return ` Conductor isn't configured yet: ${board.configurationError}`;
  return " Conductor is paused — start it to work these tasks.";
}
