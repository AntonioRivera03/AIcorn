import type { Value } from "platejs";

export const emptyDocument: Value = [{ type: "p", children: [{ text: "" }] }];

export const toValidBody = (value: unknown): Value => {
  if (!Array.isArray(value) || value.length === 0) return emptyDocument;
  return value as Value;
};

export const normalizeBodyValue = (value: unknown): Value => toValidBody(value);
