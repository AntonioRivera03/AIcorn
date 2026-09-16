package repos

import "github.com/waseem-polus/aycorn/server/internal/models"

// Fetch metadata only; task branch lists do not need multi-megabyte run output/diffs.
func (r *AgentJobRepo) TaskBranches(taskID int) ([]models.TaskBranch, error) {
	rows, err := r.DB.Query(`SELECT j.id,j.task,j.status,
 COALESCE(json_extract(j.requestJson,'$.intent'),''),
 COALESCE(json_extract(j.requestJson,'$.repoPath'),''),
 json_extract(a.artifactJson,'$.branch'),
 COALESCE(json_extract(a.artifactJson,'$.workspace'),''),
 COALESCE(json_extract(a.artifactJson,'$.baseCommit'),''),j.createdAt
 FROM agent_job j JOIN agent_run a ON a.job=j.id
 WHERE j.task=? AND COALESCE(json_extract(a.artifactJson,'$.branch'),'')<>''
 ORDER BY j.id DESC,a.id DESC`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	branches := []models.TaskBranch{}
	seen := map[string]int{}
	for rows.Next() {
		var b models.TaskBranch
		if err := rows.Scan(&b.JobID, &b.TaskID, &b.Status, &b.Intent, &b.RepoPath, &b.Branch, &b.Workspace, &b.BaseCommit, &b.CreatedAt); err != nil {
			return nil, err
		}
		key := b.RepoPath + "\x00" + b.Branch
		if index, exists := seen[key]; exists {
			// Keep one stable branch identity for existing preview URLs/history,
			// while showing the most recent turn's status and workspace metadata.
			branches[index].JobID = b.JobID
			branches[index].CreatedAt = b.CreatedAt
			continue
		}
		seen[key] = len(branches)
		b.MergedInto = []string{}
		branches = append(branches, b)
	}
	return branches, rows.Err()
}

func (r *AgentJobRepo) BranchHasActiveRun(root, branch string) (bool, error) {
	var active bool
	err := r.DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM agent_job j LEFT JOIN agent_run a ON a.job=j.id
 WHERE j.status IN ('pending','claimed','running','canceling')
 AND json_extract(j.requestJson,'$.repoPath')=? AND (json_extract(a.artifactJson,'$.branch')=? OR json_extract(j.requestJson,'$.chat.branch')=?))`, root, branch, branch).Scan(&active)
	return active, err
}
