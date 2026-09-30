package domain

import (
	"time"

	"github.com/google/uuid"
)

type StatusCategory string

const (
	StatusCategoryBacklog   StatusCategory = "backlog"
	StatusCategoryUnstarted StatusCategory = "unstarted"
	StatusCategoryStarted   StatusCategory = "started"
	StatusCategoryCompleted StatusCategory = "completed"
	StatusCategoryCancelled StatusCategory = "cancelled"
)

// WorkspaceStatus is a status owned by a workspace. The `team_statuses` table is
// scanned with SELECT *, so the legacy nullable `team_id` column must stay
// mapped even though the application never writes it anymore.
type WorkspaceStatus struct {
	ID          uuid.UUID      `json:"id" db:"id"`
	TeamID      *uuid.UUID     `json:"-" db:"team_id"`
	WorkspaceID uuid.UUID      `json:"workspace_id" db:"workspace_id"`
	Name        string         `json:"name" db:"name"`
	Slug        string         `json:"slug" db:"slug"`
	Category    StatusCategory `json:"category" db:"category"`
	Color       *string        `json:"color" db:"color"`
	Position    int            `json:"position" db:"position"`
	IsDefault   bool           `json:"is_default" db:"is_default"`
	CreatedAt   time.Time      `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at" db:"updated_at"`
}
