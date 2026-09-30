package domain

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID          uuid.UUID `json:"id" db:"id"`
	Email       string    `json:"email" db:"email"`
	Name        string    `json:"name" db:"name"`
	DisplayName string    `json:"display_name" db:"display_name"`
	AvatarURL   *string   `json:"avatar_url" db:"avatar_url"`
	// GiteaLogin is the Gitea account derived from the linked token. It is not
	// globally unique (two Gitea instances can share a login), so it is always
	// resolved per workspace membership.
	GiteaLogin *string `json:"gitea_login" db:"gitea_login"`
	// GiteaToken is the user's encrypted Gitea PAT, used to post comments on
	// their behalf instead of the workspace integration token. Never serialized.
	GiteaToken   *string   `json:"-" db:"gitea_token"`
	PasswordHash string    `json:"-" db:"password_hash"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}
