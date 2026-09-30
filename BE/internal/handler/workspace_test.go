package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kuayle/kuayle-backend/internal/domain"
	"github.com/kuayle/kuayle-backend/internal/repository"
	"github.com/kuayle/kuayle-backend/internal/service"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

type workspaceCreateRepoStub struct {
	repository.WorkspaceRepo
	existing  *domain.Workspace
	lookupErr error
	createErr error
}

func (r *workspaceCreateRepoStub) GetBySlug(context.Context, string) (*domain.Workspace, error) {
	return r.existing, r.lookupErr
}

func (r *workspaceCreateRepoStub) CreateWithMemberAndLabels(context.Context, *domain.Workspace, *domain.WorkspaceMember, []domain.Label) error {
	return r.createErr
}

func performCreateWorkspaceRequest(t *testing.T, repo repository.WorkspaceRepo) *httptest.ResponseRecorder {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/workspaces", strings.NewReader(`{"name":"Test","slug":"test"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user_id", uuid.New())
	h := NewWorkspaceHandler(service.NewWorkspaceService(repo, nil, nil))

	require.NoError(t, h.Create(c))
	return rec
}

func TestWorkspaceHandler_CreateReturnsTypedSlugConflict(t *testing.T) {
	rec := performCreateWorkspaceRequest(t, &workspaceCreateRepoStub{
		existing: &domain.Workspace{ID: uuid.New(), Slug: "test"},
	})

	require.Equal(t, http.StatusConflict, rec.Code)
	require.Contains(t, rec.Body.String(), `"code":"WORKSPACE_SLUG_TAKEN"`)
}

func TestWorkspaceHandler_CreateReturnsInternalErrorForRepositoryFailure(t *testing.T) {
	rec := performCreateWorkspaceRequest(t, &workspaceCreateRepoStub{
		lookupErr: errors.New("database unavailable"),
	})

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Contains(t, rec.Body.String(), `"code":"INTERNAL_ERROR"`)
}
