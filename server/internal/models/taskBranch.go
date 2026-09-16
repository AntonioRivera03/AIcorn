package models

// TaskBranch reads the durable run association; live status comes from Git.
// Existing artifact JSON already holds this data, so no schema migration is needed.
type TaskBranch struct {
	JobID      int      `json:"jobId"`
	TaskID     int      `json:"taskId"`
	Status     string   `json:"status"`
	Intent     string   `json:"intent"`
	RepoPath   string   `json:"repoPath"`
	Branch     string   `json:"branch"`
	Workspace  string   `json:"workspace"`
	BaseCommit string   `json:"baseCommit"`
	CreatedAt  string   `json:"createdAt"`
	Dirty      bool     `json:"dirty"`
	MergedInto []string `json:"mergedInto"`
	Problem    string   `json:"problem,omitempty"`
}
