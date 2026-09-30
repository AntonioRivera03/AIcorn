import { describe, expect, it } from "vitest";
import { conductorSendNote } from "@/features/conductor/conductor-send-note";

describe("conductorSendNote", () => {
  it("is silent while Conductor is running", () => {
    expect(
      conductorSendNote({ settings: { enabled: true } as never, configurationError: "" }),
    ).toBe("");
  });

  it("explains a missing configuration", () => {
    expect(
      conductorSendNote({
        settings: { enabled: false } as never,
        configurationError: "This workflow has no stage for finished work.",
      }),
    ).toBe(
      " Conductor isn't configured yet: This workflow has no stage for finished work.",
    );
  });

  it("explains a plain pause when configured", () => {
    expect(
      conductorSendNote({ settings: { enabled: false } as never, configurationError: "" }),
    ).toBe(" Conductor is paused — start it to work these tasks.");
  });

  it("is silent without board data", () => {
    expect(conductorSendNote(undefined)).toBe("");
  });
});
