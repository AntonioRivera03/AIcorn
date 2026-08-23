package services

import (
	"errors"

	"github.com/waseem-polus/aycorn/server/internal/models"
	"github.com/waseem-polus/aycorn/server/internal/models/repos"
)

var ErrInvalidPersonaHarness = errors.New("persona harness is not supported")
var ErrInvalidPersonaModel = errors.New("persona model is not supported")

type PersonaService struct {
	PersonaRepo *repos.PersonaRepo
}

func validatePersona(persona *models.Persona) error {
	if persona.Harness == "" {
		persona.Harness = models.PersonaHarnessClaudeCode
	}
	if persona.Model == "" {
		persona.Model = models.PersonaModelSonnet
	}
	if !models.IsValidPersonaHarness(persona.Harness) {
		return ErrInvalidPersonaHarness
	}
	if !models.IsValidPersonaModel(persona.Model) {
		return ErrInvalidPersonaModel
	}
	if persona.AllowedTools == nil {
		persona.AllowedTools = []string{}
	}
	return nil
}

func (service *PersonaService) GetAll() ([]models.Persona, error) {
	return service.PersonaRepo.All()
}

func (service *PersonaService) Get(id int) (*models.Persona, error) {
	return service.PersonaRepo.FindOne(id)
}

func (service *PersonaService) Create(persona *models.Persona) (*models.Persona, error) {
	if err := validatePersona(persona); err != nil {
		return nil, err
	}
	return service.PersonaRepo.Create(persona)
}

func (service *PersonaService) Update(persona *models.Persona) (bool, error) {
	if err := validatePersona(persona); err != nil {
		return false, err
	}
	return service.PersonaRepo.Update(persona)
}

func (service *PersonaService) Delete(id int) (bool, error) {
	return service.PersonaRepo.Delete(id)
}

func (service *PersonaService) BulkCreate(personas []models.Persona) (models.BulkResult, error) {
	for index := range personas {
		if err := validatePersona(&personas[index]); err != nil {
			return models.BulkResult{}, err
		}
	}
	created, err := service.PersonaRepo.CreateMany(personas)
	if err != nil {
		return models.BulkResult{}, err
	}
	return models.BulkResult{Success: created, Failed: len(personas) - created}, nil
}

func (service *PersonaService) BulkUpdate(personas []models.Persona) (models.BulkResult, error) {
	unique := make([]models.Persona, 0, len(personas))
	indices := make(map[int]int, len(personas))
	for index := range personas {
		if err := validatePersona(&personas[index]); err != nil {
			return models.BulkResult{}, err
		}
		if existingIndex, exists := indices[personas[index].ID]; exists {
			unique[existingIndex] = personas[index]
			continue
		}
		indices[personas[index].ID] = len(unique)
		unique = append(unique, personas[index])
	}
	updated, err := service.PersonaRepo.UpdateMany(unique)
	if err != nil {
		return models.BulkResult{}, err
	}
	return models.BulkResult{Success: updated, Skipped: len(unique) - updated}, nil
}

func (service *PersonaService) BulkDelete(ids []int) (models.BulkResult, error) {
	ids = dedupeInts(ids)
	deleted, err := service.PersonaRepo.DeleteMany(ids)
	if err != nil {
		return models.BulkResult{}, err
	}
	return models.BulkResult{Success: deleted, Skipped: len(ids) - deleted}, nil
}
