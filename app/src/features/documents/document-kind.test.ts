import { describe, expect, it } from "vitest";
import { documentKind, documentTime, formatFileSize } from "@/features/documents/document-kind";

const file = (name: string, mediaType: string) => ({ file: { name, mediaType, size: 10 } });

describe("documentKind", () => {
  it("labels each kind and picks its viewer", () => {
    expect(documentKind({})).toEqual({ label: "Note", viewer: "text" });
    expect(documentKind(file("spec.pdf", "application/pdf"))).toEqual({ label: "PDF", viewer: "pdf" });
    expect(documentKind(file("shot.png", "image/png"))).toEqual({ label: "PNG", viewer: "image" });
    expect(documentKind(file("mail.eml", "message/rfc822"))).toEqual({ label: "Email", viewer: "text" });
    expect(documentKind(file("plan.MD", "text/plain; charset=utf-8"))).toEqual({ label: ".md", viewer: "text" });
    expect(documentKind(file("notes.txt", "text/plain; charset=utf-8"))).toEqual({ label: ".txt", viewer: "text" });
    expect(documentKind(file("brief.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document"))).toEqual({ label: "Word", viewer: "file" });
  });
});

describe("formatFileSize", () => {
  it("rounds small files up to a kilobyte", () => {
    expect(formatFileSize(10)).toBe("1 KB");
    expect(formatFileSize(3 * 1024 * 1024)).toBe("3.0 MB");
  });
});

describe("documentTime", () => {
  it("reads the server's timestamps as UTC", () => {
    expect(documentTime("2026-09-27 21:05:00")).toBe("2026-09-27T21:05:00Z");
    expect(documentTime("2026-09-27T21:05:00Z")).toBe("2026-09-27T21:05:00Z");
  });
});
