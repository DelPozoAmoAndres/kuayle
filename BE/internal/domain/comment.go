package domain

import (
	"time"

	"github.com/google/uuid"
)

// Comment is a comment on an issue. UserID is nullable because a comment can
// come from a Gitea user that has no Kuayle account; in that case the author is
// described by AuthorLogin/AuthorAvatarURL.
type Comment struct {
	ID              uuid.UUID  `json:"id" db:"id"`
	IssueID         uuid.UUID  `json:"issue_id" db:"issue_id"`
	UserID          *uuid.UUID `json:"user_id" db:"user_id"`
	AuthorLogin     *string    `json:"author_login" db:"author_login"`
	AuthorAvatarURL *string    `json:"author_avatar_url" db:"author_avatar_url"`
	GiteaCommentID  *int64     `json:"gitea_comment_id" db:"gitea_comment_id"`
	Body            string     `json:"body" db:"body"`
	ParentID        *uuid.UUID `json:"parent_id" db:"parent_id"`
	ResolvedAt      *time.Time `json:"resolved_at" db:"resolved_at"`
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at" db:"updated_at"`
}
