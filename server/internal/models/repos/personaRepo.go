package repos

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/waseem-polus/aycorn/server/internal/models"
)

type PersonaRepo struct {
	DB *sql.DB
}

const personaColumns = "id, name, system_prompt, harness, model, agent, allowed_tools, timeCreated, timeModified"

func scanPersona(scanner interface{ Scan(...any) error }, persona *models.Persona) error {
	var allowedTools string
	var timeCreated string
	var timeModified string
	if err := scanner.Scan(
		&persona.ID,
		&persona.Name,
		&persona.SystemPrompt,
		&persona.Harness,
		&persona.Model,
		&persona.Agent,
		&allowedTools,
		&timeCreated,
		&timeModified,
	); err != nil {
		return err
	}
	created, err := time.Parse(time.RFC3339, timeCreated)
	if err != nil {
		return fmt.Errorf("parse persona %d creation time: %w", persona.ID, err)
	}
	modified, err := time.Parse(time.RFC3339, timeModified)
	if err != nil {
		return fmt.Errorf("parse persona %d modification time: %w", persona.ID, err)
	}
	persona.TimeCreated = &created
	persona.TimeModified = &modified
	tools, err := models.ParseAllowedTools(allowedTools)
	if err != nil {
		return fmt.Errorf("parse persona %d allowed tools: %w", persona.ID, err)
	}
	persona.AllowedTools = tools
	persona.SystemPrompt = models.NormalizeBody(persona.SystemPrompt)
	return nil
}

func personaWriteArgs(persona *models.Persona) ([]any, error) {
	allowedTools, err := models.EncodeAllowedTools(persona.AllowedTools)
	if err != nil {
		return nil, err
	}
	return []any{persona.Name, persona.SystemPrompt, persona.Harness, persona.Model, persona.Agent, allowedTools}, nil
}

func (repo *PersonaRepo) All() ([]models.Persona, error) {
	rows, err := repo.DB.Query("SELECT " + personaColumns + " FROM persona ORDER BY id ASC;")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	personas := []models.Persona{}
	for rows.Next() {
		persona := models.Persona{}
		if err := scanPersona(rows, &persona); err != nil {
			return nil, err
		}
		personas = append(personas, persona)
	}
	return personas, rows.Err()
}

func (repo *PersonaRepo) FindOne(id int) (*models.Persona, error) {
	persona := models.Persona{}
	if err := scanPersona(repo.DB.QueryRow("SELECT "+personaColumns+" FROM persona WHERE id = ?;", id), &persona); err != nil {
		return nil, err
	}
	return &persona, nil
}

func (repo *PersonaRepo) Create(persona *models.Persona) (*models.Persona, error) {
	args, err := personaWriteArgs(persona)
	if err != nil {
		return nil, err
	}
	query := `
		INSERT INTO persona (name, system_prompt, harness, model, agent, allowed_tools)
		VALUES (?, ?, ?, ?, ?, ?)
		RETURNING ` + personaColumns + `;
	`
	created := models.Persona{}
	if err := scanPersona(repo.DB.QueryRow(query, args...), &created); err != nil {
		return nil, err
	}
	return &created, nil
}

func (repo *PersonaRepo) Update(persona *models.Persona) (bool, error) {
	args, err := personaWriteArgs(persona)
	if err != nil {
		return false, err
	}
	args = append(args, persona.ID)
	result, err := repo.DB.Exec(`
		UPDATE persona
		SET name = ?, system_prompt = ?, harness = ?, model = ?, agent = ?, allowed_tools = ?
		WHERE id = ?;
	`, args...)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected > 0, err
}

func (repo *PersonaRepo) Delete(id int) (bool, error) {
	result, err := repo.DB.Exec("DELETE FROM persona WHERE id = ?;", id)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected > 0, err
}

func (repo *PersonaRepo) CreateMany(personas []models.Persona) (int, error) {
	if len(personas) == 0 {
		return 0, nil
	}
	tx, err := repo.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	statement, err := tx.Prepare(`
		INSERT INTO persona (name, system_prompt, harness, model, agent, allowed_tools)
		VALUES (?, ?, ?, ?, ?, ?);
	`)
	if err != nil {
		return 0, err
	}
	defer statement.Close()

	for index := range personas {
		args, err := personaWriteArgs(&personas[index])
		if err != nil {
			return 0, err
		}
		if _, err := statement.Exec(args...); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(personas), nil
}

func (repo *PersonaRepo) UpdateMany(personas []models.Persona) (int, error) {
	if len(personas) == 0 {
		return 0, nil
	}
	tx, err := repo.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	statement, err := tx.Prepare(`
		UPDATE persona
		SET name = ?, system_prompt = ?, harness = ?, model = ?, agent = ?, allowed_tools = ?
		WHERE id = ?;
	`)
	if err != nil {
		return 0, err
	}
	defer statement.Close()

	affected := int64(0)
	for index := range personas {
		args, err := personaWriteArgs(&personas[index])
		if err != nil {
			return 0, err
		}
		result, err := statement.Exec(append(args, personas[index].ID)...)
		if err != nil {
			return 0, err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return 0, err
		}
		affected += rows
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return int(affected), nil
}

func (repo *PersonaRepo) DeleteMany(ids []int) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	placeholders, args := intIdPlaceholders(ids)
	result, err := repo.DB.Exec("DELETE FROM persona WHERE id IN ("+placeholders+");", args...)
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	return int(affected), err
}
