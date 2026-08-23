package repos

import (
	"database/sql"
)

type StagePersonaRepo struct {
	DB *sql.DB
}

func (repo *StagePersonaRepo) Bind(stageID int, personaID int) error {
	query := `
		INSERT INTO stage_persona (stage_id, persona_id)
		SELECT s.id, p.id
		FROM stage s
		JOIN persona p ON p.id = ?
		WHERE s.id = ?
		ON CONFLICT(stage_id) DO UPDATE SET persona_id = excluded.persona_id
		RETURNING stage_id;
	`
	var boundStageID int
	return repo.DB.QueryRow(query, personaID, stageID).Scan(&boundStageID)
}

func (repo *StagePersonaRepo) Unbind(stageID int) (bool, error) {
	result, err := repo.DB.Exec("DELETE FROM stage_persona WHERE stage_id = ?;", stageID)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}
