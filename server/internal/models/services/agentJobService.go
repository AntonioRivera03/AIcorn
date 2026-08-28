package services

import (
	"database/sql"
	"errors"
	"time"

	"github.com/waseem-polus/aycorn/server/internal/models"
	"github.com/waseem-polus/aycorn/server/internal/models/repos"
)

var (
	ErrInvalidPersona    = errors.New("persona not found")
	ErrInvalidTask       = errors.New("task not found")
	ErrJobNotFound       = errors.New("agent job not found")
	ErrInvalidJobStatus  = errors.New("agent job status does not allow this transition")
	ErrJobStatusConflict = errors.New("agent job status does not allow this transition")
)

type AgentJobService struct {
	JobRepo     *repos.AgentJobRepo
	RunRepo     *repos.AgentRunRepo
	PersonaRepo *repos.PersonaRepo
	TaskRepo    *repos.TaskRepo
}

func (s *AgentJobService) Enqueue(taskID int, personaID int, fromStage *int, toStage *int) (*models.AgentJob, error) {
	if s.PersonaRepo != nil {
		_, err := s.PersonaRepo.FindOne(personaID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, ErrInvalidPersona
			}
			return nil, err
		}
	}
	if s.TaskRepo != nil {
		_, err := s.TaskRepo.FindOne(int64(taskID))
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, ErrInvalidTask
			}
			return nil, err
		}
	}
	job := &models.AgentJob{
		Task:      taskID,
		Persona:   personaID,
		Status:    models.AgentJobStatusPending,
		FromStage: fromStage,
		ToStage:   toStage,
		Attempts:  0,
	}
	return s.JobRepo.Create(job)
}

func (s *AgentJobService) ClaimNext() (*models.AgentJob, error) {
	job, err := s.JobRepo.ClaimNext()
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return job, nil
}

func (s *AgentJobService) ResetStale(timeout time.Duration) (int, error) {
	return s.JobRepo.ResetStale(timeout)
}

func (s *AgentJobService) Complete(jobID int, output string, summary string, exitCode *int, usageJson string) error {
	job, err := s.JobRepo.FindOne(jobID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrJobNotFound
		}
		return err
	}
	if job.Status != models.AgentJobStatusClaimed && job.Status != models.AgentJobStatusRunning {
		return ErrInvalidJobStatus
	}
	finalStatus := models.AgentJobStatusCompleted
	if exitCode != nil && *exitCode != 0 {
		finalStatus = models.AgentJobStatusFailed
	}
	run := &models.AgentRun{
		Job:       jobID,
		Output:    output,
		Summary:   summary,
		ExitCode:  exitCode,
		UsageJson: usageJson,
	}
	if err := s.JobRepo.CreateRunAndCompleteTx(run, finalStatus); err != nil {
		if errors.Is(err, repos.ErrJobStatusConflict) {
			return ErrInvalidJobStatus
		}
		return err
	}
	return nil
}

func (s *AgentJobService) Get(jobID int) (*models.AgentJob, error) {
	return s.JobRepo.FindOne(jobID)
}

func (s *AgentJobService) ListByStatus(status string) ([]models.AgentJob, error) {
	return s.JobRepo.ListByStatus(status)
}

func (s *AgentJobService) FindByTask(taskID int) ([]models.AgentJob, error) {
	return s.JobRepo.FindByTask(taskID)
}

func (s *AgentJobService) ListByTask(taskID int) (*models.AgentJobsResponse, error) {
	if s.TaskRepo != nil {
		if _, err := s.TaskRepo.FindOne(int64(taskID)); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, ErrInvalidTask
			}
			return nil, err
		}
	}
	jobs, err := s.JobRepo.FindByTask(taskID)
	if err != nil {
		return nil, err
	}
	if jobs == nil {
		jobs = []models.AgentJob{}
	}
	runs := []models.AgentRun{}
	for _, j := range jobs {
		var rs []models.AgentRun
		if s.RunRepo != nil {
			rs, err = s.RunRepo.ListByJob(j.ID)
		} else {
			rs, err = s.JobRepo.ListByJob(j.ID)
		}
		if err != nil {
			return nil, err
		}
		if rs != nil {
			runs = append(runs, rs...)
		}
	}
	if runs == nil {
		runs = []models.AgentRun{}
	}
	return &models.AgentJobsResponse{Jobs: jobs, Runs: runs}, nil
}

func (s *AgentJobService) ListAll() ([]models.AgentJob, error) {
	return s.JobRepo.ListAll()
}

func (s *AgentJobService) MarkRunning(jobID int) (bool, error) {
	job, err := s.JobRepo.FindOne(jobID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, ErrJobNotFound
		}
		return false, err
	}
	if job.Status != models.AgentJobStatusClaimed {
		return false, ErrInvalidJobStatus
	}
	return s.JobRepo.MarkRunning(jobID)
}

func (s *AgentJobService) Fail(jobID int, errMsg string) (bool, error) {
	job, err := s.JobRepo.FindOne(jobID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, ErrJobNotFound
		}
		return false, err
	}
	if job.Status != models.AgentJobStatusClaimed && job.Status != models.AgentJobStatusRunning {
		return false, ErrInvalidJobStatus
	}
	return s.JobRepo.Fail(jobID, errMsg)
}

func (s *AgentJobService) UpdateStatus(jobID int, status string) (bool, error) {
	return s.JobRepo.UpdateStatus(jobID, status)
}

func (s *AgentJobService) LoadTaskForRun(taskID int) (*models.ChecklistTask, error) {
	if s.TaskRepo != nil {
		task, err := s.TaskRepo.FindOne(int64(taskID))
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, ErrInvalidTask
			}
			return nil, err
		}
		return task, nil
	}
	if s.JobRepo != nil && s.JobRepo.DB != nil {
		var name, body string
		if err := s.JobRepo.DB.QueryRow(`SELECT COALESCE(name,''), COALESCE(body,'[]') FROM task WHERE id = ?;`, taskID).Scan(&name, &body); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, ErrInvalidTask
			}
			return nil, err
		}
		return &models.ChecklistTask{Task: models.Task{ID: taskID, Name: name, Body: body}}, nil
	}
	return nil, ErrInvalidTask
}

// LoadPersonaForRun loads the persona for a worker run via the repo,
// mapping sql.ErrNoRows to ErrInvalidPersona.
func (s *AgentJobService) LoadPersonaForRun(personaID int) (*models.Persona, error) {
	if s.PersonaRepo == nil {
		return nil, ErrInvalidPersona
	}
	p, err := s.PersonaRepo.FindOne(personaID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrInvalidPersona
		}
		return nil, err
	}
	return p, nil
}
