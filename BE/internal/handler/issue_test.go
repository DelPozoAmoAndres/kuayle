package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kuayle/kuayle-backend/internal/domain"
	"github.com/kuayle/kuayle-backend/internal/dto"
	"github.com/kuayle/kuayle-backend/internal/realtime"
	"github.com/kuayle/kuayle-backend/internal/service"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

// --- Test issue repos ---

type testIssueRepo struct {
	issues map[string]*domain.Issue
}

func newTestIssueRepo() *testIssueRepo {
	return &testIssueRepo{issues: make(map[string]*domain.Issue)}
}

func (r *testIssueRepo) Create(_ context.Context, _ *sqlx.Tx, issue *domain.Issue) error {
	r.issues[issue.Identifier] = issue
	return nil
}

func (r *testIssueRepo) NextNumber(_ context.Context, _ *sqlx.Tx, _ uuid.UUID, _ string) (int, error) {
	return len(r.issues) + 1, nil
}

func (r *testIssueRepo) GetByID(_ context.Context, id uuid.UUID) (*domain.Issue, error) {
	for _, issue := range r.issues {
		if issue.ID == id {
			return issue, nil
		}
	}
	return nil, nil
}

func (r *testIssueRepo) GetByIdentifier(_ context.Context, _ uuid.UUID, identifier string) (*domain.Issue, error) {
	if issue, ok := r.issues[identifier]; ok {
		return issue, nil
	}
	return nil, nil
}

func (r *testIssueRepo) List(_ context.Context, _ uuid.UUID, _ dto.IssueFilterParams) ([]domain.Issue, int, error) {
	issues := make([]domain.Issue, 0, len(r.issues))
	for _, issue := range r.issues {
		issues = append(issues, *issue)
	}
	return issues, len(issues), nil
}

func (r *testIssueRepo) Update(_ context.Context, issue *domain.Issue) error {
	r.issues[issue.Identifier] = issue
	return nil
}

func (r *testIssueRepo) UpdateProjectKey(_ context.Context, _ *sqlx.Tx, _ uuid.UUID, _ *uuid.UUID, _ int, _ string) error {
	return nil
}

func (r *testIssueRepo) Delete(_ context.Context, id uuid.UUID) error {
	for k, issue := range r.issues {
		if issue.ID == id {
			delete(r.issues, k)
			return nil
		}
	}
	return nil
}

func (r *testIssueRepo) SetLabels(_ context.Context, _ uuid.UUID, _ []uuid.UUID) error {
	return nil
}

func (r *testIssueRepo) GetLabels(_ context.Context, _ uuid.UUID) ([]domain.Label, error) {
	return nil, nil
}

func (r *testIssueRepo) GetLabelsForIssues(_ context.Context, _ []uuid.UUID) (map[uuid.UUID][]domain.Label, error) {
	return make(map[uuid.UUID][]domain.Label), nil
}

func (r *testIssueRepo) ListSubIssues(_ context.Context, parentID uuid.UUID) ([]domain.Issue, error) {
	var subs []domain.Issue
	for _, issue := range r.issues {
		if issue.ParentID != nil && *issue.ParentID == parentID {
			subs = append(subs, *issue)
		}
	}
	return subs, nil
}

func (r *testIssueRepo) CountSubIssues(_ context.Context, parentID uuid.UUID) (int, int, error) {
	total, done := 0, 0
	for _, issue := range r.issues {
		if issue.ParentID != nil && *issue.ParentID == parentID {
			total++
			if issue.Status == domain.IssueStatusDone || issue.Status == domain.IssueStatusCancelled {
				done++
			}
		}
	}
	return total, done, nil
}

func (r *testIssueRepo) CountSubIssuesForIssues(_ context.Context, issueIDs []uuid.UUID) (map[uuid.UUID]domain.SubIssueCount, error) {
	result := make(map[uuid.UUID]domain.SubIssueCount, len(issueIDs))
	for _, id := range issueIDs {
		total, done, _ := r.CountSubIssues(context.Background(), id)
		if total > 0 {
			result[id] = domain.SubIssueCount{IssueID: id, Total: total, Done: done}
		}
	}
	return result, nil
}

func (r *testIssueRepo) WouldCreateCycle(_ context.Context, issueID, parentID uuid.UUID) (bool, error) {
	if issueID == parentID {
		return true, nil
	}
	for _, issue := range r.issues {
		if issue.ID == parentID && issue.ParentID != nil {
			return r.WouldCreateCycle(context.Background(), issueID, *issue.ParentID)
		}
	}
	return false, nil
}

func (r *testIssueRepo) CycleIsActive(_ context.Context, _ uuid.UUID) (bool, error) {
	return false, nil
}

func (r *testIssueRepo) BulkUpdate(_ context.Context, _ uuid.UUID, _ []uuid.UUID, _ *string, _ *int, _ *uuid.UUID, _ *uuid.UUID) (int, error) {
	return 0, nil
}

func (r *testIssueRepo) BulkDelete(_ context.Context, _ uuid.UUID, _ []uuid.UUID) (int, error) {
	return 0, nil
}

func (r *testIssueRepo) SetAssignees(_ context.Context, _ uuid.UUID, _ []uuid.UUID) error {
	return nil
}

func (r *testIssueRepo) GetAssignees(_ context.Context, _ uuid.UUID) ([]uuid.UUID, error) {
	return nil, nil
}

func (r *testIssueRepo) GetAssigneesForIssues(_ context.Context, _ []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error) {
	return make(map[uuid.UUID][]uuid.UUID), nil
}

func (r *testIssueRepo) BeginTx(_ context.Context) (*sqlx.Tx, error) {
	return nil, nil
}

func (r *testIssueRepo) GetByGiteaIssueIndex(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ int64) (*domain.Issue, error) {
	return nil, nil
}

type testHistoryRepo struct{}

func (r *testHistoryRepo) Create(_ context.Context, _, _ uuid.UUID, _ string, _, _ *string) error {
	return nil
}

func (r *testHistoryRepo) ListByIssue(_ context.Context, _ uuid.UUID) ([]domain.IssueHistory, error) {
	return nil, nil
}

type testStatusRepo struct{}

func (r *testStatusRepo) Create(_ context.Context, _ *domain.WorkspaceStatus) error {
	return nil
}

func (r *testStatusRepo) GetByID(_ context.Context, _ uuid.UUID) (*domain.WorkspaceStatus, error) {
	return nil, nil
}

func (r *testStatusRepo) GetByIDs(_ context.Context, _ []uuid.UUID) ([]domain.WorkspaceStatus, error) {
	return nil, nil
}

func (r *testStatusRepo) GetByWorkspaceAndSlug(_ context.Context, _ uuid.UUID, _ string) (*domain.WorkspaceStatus, error) {
	return nil, nil
}

func (r *testStatusRepo) ListByWorkspace(_ context.Context, _ uuid.UUID) ([]domain.WorkspaceStatus, error) {
	return nil, nil
}

func (r *testStatusRepo) Update(_ context.Context, _ *domain.WorkspaceStatus) error {
	return nil
}

func (r *testStatusRepo) Delete(_ context.Context, _ uuid.UUID) error {
	return nil
}

func (r *testStatusRepo) NextPosition(_ context.Context, _ uuid.UUID) (int, error) {
	return 0, nil
}

type testCommentRepo struct {
	comments []domain.Comment
}

func (r *testCommentRepo) Create(_ context.Context, comment *domain.Comment) error {
	r.comments = append(r.comments, *comment)
	return nil
}

func (r *testCommentRepo) ListByIssue(_ context.Context, _ uuid.UUID) ([]domain.Comment, error) {
	return r.comments, nil
}

func (r *testCommentRepo) ListReplies(_ context.Context, _ uuid.UUID) ([]domain.Comment, error) {
	return nil, nil
}

func (r *testCommentRepo) GetByID(_ context.Context, id uuid.UUID) (*domain.Comment, error) {
	for _, c := range r.comments {
		if c.ID == id {
			return &c, nil
		}
	}
	return nil, nil
}

func (r *testCommentRepo) GetByGiteaCommentID(_ context.Context, giteaCommentID int64) (*domain.Comment, error) {
	for _, c := range r.comments {
		if c.GiteaCommentID != nil && *c.GiteaCommentID == giteaCommentID {
			return &c, nil
		}
	}
	return nil, nil
}

func (r *testCommentRepo) Update(_ context.Context, id uuid.UUID, body string) error {
	for i, c := range r.comments {
		if c.ID == id {
			r.comments[i].Body = body
		}
	}
	return nil
}

func (r *testCommentRepo) Delete(_ context.Context, id uuid.UUID) error {
	remaining := make([]domain.Comment, 0, len(r.comments))
	for _, c := range r.comments {
		if c.ID != id && (c.ParentID == nil || *c.ParentID != id) {
			remaining = append(remaining, c)
		}
	}
	r.comments = remaining
	return nil
}

func (r *testCommentRepo) SetGiteaCommentID(_ context.Context, id uuid.UUID, giteaCommentID int64) error {
	for i, c := range r.comments {
		if c.ID == id {
			r.comments[i].GiteaCommentID = &giteaCommentID
		}
	}
	return nil
}

func (r *testCommentRepo) Resolve(_ context.Context, id uuid.UUID) error {
	return nil
}

func (r *testCommentRepo) Reopen(_ context.Context, id uuid.UUID) error {
	return nil
}

type testNotifRepo struct{}

func (r *testNotifRepo) Create(_ context.Context, _ *domain.Notification) error { return nil }
func (r *testNotifRepo) CreateOrRefresh(_ context.Context, _ *domain.Notification, _ time.Duration) error {
	return nil
}
func (r *testNotifRepo) GetByID(_ context.Context, _ uuid.UUID) (*domain.Notification, error) {
	return nil, nil
}
func (r *testNotifRepo) ListByUser(_ context.Context, _ uuid.UUID, _, _ int) ([]domain.Notification, error) {
	return nil, nil
}
func (r *testNotifRepo) ListSnoozed(_ context.Context, _ uuid.UUID) ([]domain.Notification, error) {
	return nil, nil
}
func (r *testNotifRepo) ListArchived(_ context.Context, _ uuid.UUID, _ int) ([]domain.Notification, error) {
	return nil, nil
}
func (r *testNotifRepo) Update(_ context.Context, _ *domain.Notification) error  { return nil }
func (r *testNotifRepo) MarkAllRead(_ context.Context, _ uuid.UUID) error        { return nil }
func (r *testNotifRepo) UnreadCount(_ context.Context, _ uuid.UUID) (int, error) { return 0, nil }

// --- Context helpers ---

func setupIssueContext(e *echo.Echo, method, path, body string) (echo.Context, *httptest.ResponseRecorder) {
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	return c, rec
}

func setWorkspaceContext(c echo.Context) uuid.UUID {
	wsID := uuid.New()
	ws := &domain.Workspace{ID: wsID, Name: "Test", Slug: "test"}
	c.Set("workspace", ws)
	c.Set("workspace_id", wsID)
	c.Set("workspace_role", "owner")
	c.Set("user_id", uuid.New())
	return wsID
}

// --- Tests ---

func TestIssueHandler_List(t *testing.T) {
	e := echo.New()
	issueRepo := newTestIssueRepo()
	historyRepo := &testHistoryRepo{}
	hub := realtime.NewHub()

	// Pre-populate
	wsID := uuid.New()
	issueRepo.issues["ENG-1"] = &domain.Issue{
		ID:          uuid.New(),
		WorkspaceID: wsID,
		Identifier:  "ENG-1",
		Title:       "Test Issue",
		Status:      domain.IssueStatusTodo,
	}

	notifSvc := service.NewNotificationService(&testNotifRepo{})
	issueSvc := service.NewIssueService(issueRepo, &testStatusRepo{}, historyRepo, hub, notifSvc, nil, nil)
	commentSvc := service.NewCommentService(&testCommentRepo{}, issueRepo, &testUserRepo{}, hub, notifSvc)
	h := NewIssueHandler(issueSvc, commentSvc, &testUserRepo{}, &testStatusRepo{}, nil, nil)

	c, rec := setupIssueContext(e, http.MethodGet, "/api/workspaces/test/issues", "")
	ws := &domain.Workspace{ID: wsID, Name: "Test", Slug: "test"}
	c.Set("workspace", ws)

	err := h.List(c)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "ENG-1")
}

func TestIssueHandler_Get_Found(t *testing.T) {
	e := echo.New()
	issueRepo := newTestIssueRepo()
	historyRepo := &testHistoryRepo{}
	hub := realtime.NewHub()

	wsID := uuid.New()
	issueRepo.issues["ENG-1"] = &domain.Issue{
		ID:          uuid.New(),
		WorkspaceID: wsID,
		Identifier:  "ENG-1",
		Title:       "Test Issue",
		Status:      domain.IssueStatusTodo,
		CreatorID:   uuid.New(),
	}

	notifSvc := service.NewNotificationService(&testNotifRepo{})
	issueSvc := service.NewIssueService(issueRepo, &testStatusRepo{}, historyRepo, hub, notifSvc, nil, nil)
	commentSvc := service.NewCommentService(&testCommentRepo{}, issueRepo, &testUserRepo{}, hub, notifSvc)
	h := NewIssueHandler(issueSvc, commentSvc, &testUserRepo{}, &testStatusRepo{}, nil, nil)

	c, rec := setupIssueContext(e, http.MethodGet, "/api/workspaces/test/issues/ENG-1", "")
	ws := &domain.Workspace{ID: wsID, Name: "Test", Slug: "test"}
	c.Set("workspace", ws)
	c.SetParamNames("identifier")
	c.SetParamValues("ENG-1")

	err := h.Get(c)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "ENG-1")
}

func TestIssueHandler_Get_NotFound(t *testing.T) {
	e := echo.New()
	issueRepo := newTestIssueRepo()
	historyRepo := &testHistoryRepo{}
	hub := realtime.NewHub()

	notifSvc := service.NewNotificationService(&testNotifRepo{})
	issueSvc := service.NewIssueService(issueRepo, &testStatusRepo{}, historyRepo, hub, notifSvc, nil, nil)
	commentSvc := service.NewCommentService(&testCommentRepo{}, issueRepo, &testUserRepo{}, hub, notifSvc)
	h := NewIssueHandler(issueSvc, commentSvc, &testUserRepo{}, &testStatusRepo{}, nil, nil)

	c, rec := setupIssueContext(e, http.MethodGet, "/api/workspaces/test/issues/ENG-999", "")
	ws := &domain.Workspace{ID: uuid.New(), Name: "Test", Slug: "test"}
	c.Set("workspace", ws)
	c.SetParamNames("identifier")
	c.SetParamValues("ENG-999")

	err := h.Get(c)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestIssueHandler_Create_ValidationError(t *testing.T) {
	e := echo.New()
	issueRepo := newTestIssueRepo()
	historyRepo := &testHistoryRepo{}
	hub := realtime.NewHub()

	notifSvc := service.NewNotificationService(&testNotifRepo{})
	issueSvc := service.NewIssueService(issueRepo, &testStatusRepo{}, historyRepo, hub, notifSvc, nil, nil)
	commentSvc := service.NewCommentService(&testCommentRepo{}, issueRepo, &testUserRepo{}, hub, notifSvc)
	h := NewIssueHandler(issueSvc, commentSvc, &testUserRepo{}, &testStatusRepo{}, nil, nil)

	// Missing required fields
	c, rec := setupIssueContext(e, http.MethodPost, "/api/workspaces/test/issues", `{"title": ""}`)
	setWorkspaceContext(c)

	err := h.Create(c)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestIssueHandler_Delete_Success(t *testing.T) {
	e := echo.New()
	issueRepo := newTestIssueRepo()
	historyRepo := &testHistoryRepo{}
	hub := realtime.NewHub()

	wsID := uuid.New()
	issueRepo.issues["ENG-1"] = &domain.Issue{
		ID:          uuid.New(),
		WorkspaceID: wsID,
		Identifier:  "ENG-1",
		Title:       "Delete me",
	}

	notifSvc := service.NewNotificationService(&testNotifRepo{})
	issueSvc := service.NewIssueService(issueRepo, &testStatusRepo{}, historyRepo, hub, notifSvc, nil, nil)
	commentSvc := service.NewCommentService(&testCommentRepo{}, issueRepo, &testUserRepo{}, hub, notifSvc)
	h := NewIssueHandler(issueSvc, commentSvc, &testUserRepo{}, &testStatusRepo{}, nil, nil)

	c, rec := setupIssueContext(e, http.MethodDelete, "/api/workspaces/test/issues/ENG-1", "")
	ws := &domain.Workspace{ID: wsID, Name: "Test", Slug: "test"}
	c.Set("workspace", ws)
	c.SetParamNames("identifier")
	c.SetParamValues("ENG-1")

	err := h.Delete(c)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "deleted")
}

func TestIssueHandler_CreateComment_ValidationError(t *testing.T) {
	e := echo.New()
	issueRepo := newTestIssueRepo()
	historyRepo := &testHistoryRepo{}
	hub := realtime.NewHub()

	notifSvc := service.NewNotificationService(&testNotifRepo{})
	issueSvc := service.NewIssueService(issueRepo, &testStatusRepo{}, historyRepo, hub, notifSvc, nil, nil)
	commentSvc := service.NewCommentService(&testCommentRepo{}, issueRepo, &testUserRepo{}, hub, notifSvc)
	h := NewIssueHandler(issueSvc, commentSvc, &testUserRepo{}, &testStatusRepo{}, nil, nil)

	c, rec := setupIssueContext(e, http.MethodPost, "/api/workspaces/test/issues/ENG-1/comments", `{"body": ""}`)
	setWorkspaceContext(c)
	c.SetParamNames("identifier")
	c.SetParamValues("ENG-1")

	err := h.CreateComment(c)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// fakeGiteaCommentSync fakes the Gitea integration for comment tests.
type fakeGiteaCommentSync struct {
	connected bool
}

func (f *fakeGiteaCommentSync) HasGiteaInstance(context.Context, uuid.UUID) (bool, error) {
	return f.connected, nil
}

func (f *fakeGiteaCommentSync) SyncCommentToGitea(context.Context, *domain.Issue, *domain.Comment) error {
	return nil
}

func TestIssueHandler_CreateComment_GiteaLoginRequired(t *testing.T) {
	e := echo.New()
	issueRepo := newTestIssueRepo()
	historyRepo := &testHistoryRepo{}
	hub := realtime.NewHub()

	wsID := uuid.New()
	issueRepo.issues["ENG-1"] = &domain.Issue{
		ID:          uuid.New(),
		WorkspaceID: wsID,
		Identifier:  "ENG-1",
		Title:       "Test Issue",
		Status:      domain.IssueStatusTodo,
		CreatorID:   uuid.New(),
	}

	notifSvc := service.NewNotificationService(&testNotifRepo{})
	issueSvc := service.NewIssueService(issueRepo, &testStatusRepo{}, historyRepo, hub, notifSvc, nil, nil)
	userID := uuid.New()
	commentSvc := service.NewCommentService(&testCommentRepo{}, issueRepo, &testUserRepo{specificUserID: userID}, hub, notifSvc)
	commentSvc.SetGiteaService(&fakeGiteaCommentSync{connected: true})
	h := NewIssueHandler(issueSvc, commentSvc, &testUserRepo{}, &testStatusRepo{}, nil, nil)

	c, rec := setupIssueContext(e, http.MethodPost, "/api/workspaces/test/issues/ENG-1/comments", `{"body": "hello"}`)
	ws := &domain.Workspace{ID: wsID, Name: "Test", Slug: "test"}
	c.Set("workspace", ws)
	c.Set("user_id", userID)
	c.SetParamNames("identifier")
	c.SetParamValues("ENG-1")

	err := h.CreateComment(c)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Contains(t, rec.Body.String(), "GITEA_LOGIN_REQUIRED")
}
