// The editor keeps its read revision until a save succeeds or it is reopened.
// Background Conductor updates must not advance this baseline.
const revisions = new Map<number, string>();
export const taskBodyRevision = (taskId: number) => revisions.get(taskId);
export function rememberTaskBodyRevision(taskId: number, response: Response) {
  const revision = response.headers.get("ETag");
  if (revision) revisions.set(taskId, revision);
}
