// Keep raw streaming/invalid results inspectable; render a finished structured
// result as the planning notes or human handoff the user actually needs.
export function conductorOutput(output: string): string {
  try {
    const result = JSON.parse(output.trim().replace(/^```json\n/, "").replace(/```$/, ""));
    if (!result || typeof result !== "object") return output;
    if (typeof result.summary === "string") {
      return result.summary + (typeof result.blocker === "string" && result.blocker ? `\n\n### Blocker\n${result.blocker}` : "");
    }
    if (typeof result.context === "string") {
      return result.context + (typeof result.missingContext === "string" && result.missingContext ? `\n\n### Missing context\n${result.missingContext}` : "");
    }
  } catch { /* Partial output is expected while streaming. */ }
  return output;
}
