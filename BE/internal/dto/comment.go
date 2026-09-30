package dto

import "time"

type CreateCommentRequest struct {
	Body string `json:"body" validate:"required,min=1"`
}

type UpdateCommentRequest struct {
	Body string `json:"body" validate:"required,min=1"`
}

type CommentResponse struct {
	ID              string        `json:"id"`
	IssueID         string        `json:"issue_id"`
	UserID          *string       `json:"user_id"`
	AuthorLogin     *string       `json:"author_login,omitempty"`
	AuthorAvatarURL *string       `json:"author_avatar_url,omitempty"`
	GiteaCommentID  *int64        `json:"gitea_comment_id,omitempty"`
	Body            string        `json:"body"`
	ResolvedAt      *time.Time    `json:"resolved_at,omitempty"`
	User            *UserResponse `json:"user,omitempty"`
	CreatedAt       time.Time     `json:"created_at"`
	UpdatedAt       time.Time     `json:"updated_at"`
}
