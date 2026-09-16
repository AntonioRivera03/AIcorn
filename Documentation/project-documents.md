# Project documents

The Documents view keeps project knowledge separate from board tasks. Create an empty document with **New document**, edit the title and rich text directly, and search by title or body. Documents can be selected as the project's default view in General settings.

Edits autosave after 400 ms and are serialized per document. Edits made during a save remain in the queue. Switching documents flushes pending work; leaving the view also flushes. Failed edits remain available when returning to the view during the same browser session. The browser warns before closing or reloading a page with unsaved edits.

Each write uses the document revision. A conflicting save is rejected and the editor keeps the local draft. Retry a failed request, or copy any writing to keep and choose **Reload saved version** to explicitly discard the local draft. Deletion also checks the revision and requires confirmation.

Documents belong to one project and are removed when that project is deleted. Rich text uses the same Plate format and editor as task bodies.

## API

- `GET /api/documents/project/{projectId}` lists documents.
- `POST /api/documents/project/{projectId}` creates an empty document.
- `GET /api/documents/project/{projectId}/{id}` reads one document.
- `PUT /api/documents/project/{projectId}/{id}` accepts `revision` and changed `title`/`body` fields.
- `DELETE /api/documents/project/{projectId}/{id}` requires the current revision in `If-Match`.

Cross-project lookups return 404. Revision conflicts return 409. Titles are limited to 500 bytes and document bodies to 256 KB of valid rich text.

## Files and quick notes

**New note** immediately creates an empty editable item. Capture requests, decisions, or snippets such as “Fabiana wants us to do this”; titles and rich text save automatically.

**Upload file**, drag/drop, and screenshot paste accept one file at a time. Supported formats: PDF, Word DOC/DOCX, PNG/JPEG/GIF/WebP, and UTF-8 TXT/Markdown. The limit is 20 MiB per file; text imports are limited to 128 KB and are copied into the editable note. PDFs and images preview in the browser. Word files have an original-file download with an editable context note; Word content is not converted or executed. Search covers the title and written note, not OCR or binary file contents.

Files are stored as SQLite blobs in `project_document_file`, separate from the list metadata. This keeps uploads within the normal database backup/restore path. A project/document deletion cascades to its files; the existing document delete confirmation and revision check apply. Original file bytes never change when the title or note is edited. Download endpoints verify project membership, support ranges for PDF viewers, serve explicit content types, and force non-previewable formats to download.

Migration 00024 adds a separate attachment table and does not change existing task/stage CHECK values. Backend tests cover formats, limits, scope, revision conflicts, preserving originals, deletion, and HTTP ranges. Browser verification covers text import/rename, PDF rendering, DOCX upload, image upload, and clipboard screenshot paste.
