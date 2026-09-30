package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/kuayle/kuayle-backend/internal/domain"
	"github.com/kuayle/kuayle-backend/internal/dto"
	"github.com/kuayle/kuayle-backend/internal/repository"
)

// StatusService manages the statuses of a workspace (owned by the legacy
// `team_statuses` table).
type StatusService struct {
	statusRepo     repository.StatusRepo
	visibilityRepo repository.ProjectStatusVisibilityRepo
}

func NewStatusService(statusRepo repository.StatusRepo, visibilityRepo repository.ProjectStatusVisibilityRepo) *StatusService {
	return &StatusService{statusRepo: statusRepo, visibilityRepo: visibilityRepo}
}

func (s *StatusService) List(ctx context.Context, workspaceID uuid.UUID) ([]domain.WorkspaceStatus, error) {
	return s.statusRepo.ListByWorkspace(ctx, workspaceID)
}

func (s *StatusService) GetByWorkspaceAndSlug(ctx context.Context, workspaceID uuid.UUID, slug string) (*domain.WorkspaceStatus, error) {
	return s.statusRepo.GetByWorkspaceAndSlug(ctx, workspaceID, slug)
}

func (s *StatusService) Create(ctx context.Context, workspaceID uuid.UUID, req dto.CreateStatusRequest) (*domain.WorkspaceStatus, error) {
	pos, err := s.statusRepo.NextPosition(ctx, workspaceID)
	if err != nil {
		return nil, err
	}

	slug := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(req.Name), " ", "_"))

	status := &domain.WorkspaceStatus{
		ID:          uuid.New(),
		WorkspaceID: workspaceID,
		Name:        req.Name,
		Slug:        slug,
		Category:    domain.StatusCategory(req.Category),
		Color:       req.Color,
		Position:    pos,
		IsDefault:   false,
	}

	if err := s.statusRepo.Create(ctx, status); err != nil {
		return nil, err
	}

	// Set project visibility if project IDs are provided
	if len(req.ProjectIDs) > 0 {
		for _, pidStr := range req.ProjectIDs {
			pid, err := uuid.Parse(pidStr)
			if err != nil {
				continue
			}
			existingIDs, _ := s.visibilityRepo.ListVisibleStatuses(ctx, pid)
			updatedIDs := append(existingIDs, status.ID)
			_ = s.visibilityRepo.SetVisibleStatuses(ctx, pid, updatedIDs)
		}
	}

	return status, nil
}

func (s *StatusService) Update(ctx context.Context, workspaceID, id uuid.UUID, req dto.UpdateStatusRequest) (*domain.WorkspaceStatus, error) {
	status, err := s.statusRepo.GetByID(ctx, id)
	if err != nil || status == nil || status.WorkspaceID != workspaceID {
		return nil, fmt.Errorf("status not found")
	}

	if req.Name != nil {
		status.Name = *req.Name
	}
	if req.Color != nil {
		status.Color = req.Color
	}
	if req.Position != nil {
		status.Position = *req.Position
	}

	if err := s.statusRepo.Update(ctx, status); err != nil {
		return nil, err
	}

	// Update project visibility if ProjectIDs is provided
	if req.ProjectIDs != nil {
		// First, remove this status from all projects
		existingProjects, _ := s.visibilityRepo.ListProjectsForStatus(ctx, id)
		for _, pid := range existingProjects {
			visibleIDs, _ := s.visibilityRepo.ListVisibleStatuses(ctx, pid)
			filtered := make([]uuid.UUID, 0, len(visibleIDs))
			for _, sid := range visibleIDs {
				if sid != id {
					filtered = append(filtered, sid)
				}
			}
			_ = s.visibilityRepo.SetVisibleStatuses(ctx, pid, filtered)
		}
		// Then, add this status to the specified projects
		for _, pidStr := range *req.ProjectIDs {
			pid, err := uuid.Parse(pidStr)
			if err != nil {
				continue
			}
			visibleIDs, _ := s.visibilityRepo.ListVisibleStatuses(ctx, pid)
			updatedIDs := append(visibleIDs, id)
			_ = s.visibilityRepo.SetVisibleStatuses(ctx, pid, updatedIDs)
		}
	}

	return status, nil
}

func (s *StatusService) ListProjectIDsForStatuses(ctx context.Context, statusIDs []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error) {
	return s.visibilityRepo.ListProjectIDsByStatuses(ctx, statusIDs)
}

func (s *StatusService) ListProjectsForStatus(ctx context.Context, statusID uuid.UUID) ([]uuid.UUID, error) {
	return s.visibilityRepo.ListProjectsForStatus(ctx, statusID)
}

func (s *StatusService) Delete(ctx context.Context, workspaceID, id uuid.UUID) error {
	status, err := s.statusRepo.GetByID(ctx, id)
	if err != nil || status == nil || status.WorkspaceID != workspaceID {
		return fmt.Errorf("status not found")
	}
	if status.IsDefault {
		return fmt.Errorf("cannot delete the default status")
	}
	return s.statusRepo.Delete(ctx, id)
}

// defaultStatusSpecs is the default status set seeded for every new workspace.
type defaultStatusSpec struct {
	Name     string
	Slug     string
	Category domain.StatusCategory
	Position int
}

func defaultStatusSpecs() []defaultStatusSpec {
	return []defaultStatusSpec{
		{"Backlog", "backlog", domain.StatusCategoryBacklog, 0},
		{"Todo", "todo", domain.StatusCategoryUnstarted, 1},
		{"In Progress", "in_progress", domain.StatusCategoryStarted, 2},
		{"In Review", "in_review", domain.StatusCategoryStarted, 3},
		{"Done", "done", domain.StatusCategoryCompleted, 4},
		{"Cancelled", "cancelled", domain.StatusCategoryCancelled, 5},
	}
}
