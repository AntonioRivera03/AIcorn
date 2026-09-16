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
