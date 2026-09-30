package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/kuayle/kuayle-backend/internal/domain"
	"github.com/kuayle/kuayle-backend/internal/repository"
	"github.com/kuayle/kuayle-backend/pkg/crypto"
	gt "github.com/kuayle/kuayle-backend/pkg/gitea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubGiteaRepo implements only the method under test; every other method
// panics if called, which keeps these tests honest.
type stubGiteaRepo struct {
	repository.GiteaRepo
	cfg *domain.GiteaOAuthConfig
}

func (s *stubGiteaRepo) GetOAuthConfigByWorkspace(_ context.Context, _ uuid.UUID) (*domain.GiteaOAuthConfig, error) {
	return s.cfg, nil
}

func signPayload(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func newWebhookService(t *testing.T, webhookSecret string) *GiteaService {
	t.Helper()
	key := crypto.DeriveKey("gitea-webhook-test")
	svc := &GiteaService{
		gtRepo:        &stubGiteaRepo{},
		encryptionKey: key,
	}
	if webhookSecret != "" {
		encrypted, err := crypto.Encrypt(webhookSecret, key)
		require.NoError(t, err)
		svc.gtRepo = &stubGiteaRepo{cfg: &domain.GiteaOAuthConfig{
			ID:            uuid.New(),
			WorkspaceID:   uuid.New(),
			InstanceURL:   "https://gitea.example.lan",
			WebhookSecret: encrypted,
		}}
	}
	return svc
}

// The workspace must be resolved *before* verification, otherwise the lookup
// always misses and every signed webhook gets rejected.
func TestVerifyWebhookSignature_UsesWorkspaceSecret(t *testing.T) {
	ctx := context.Background()
	payload := []byte(`{"action":"created","issue":{"number":1}}`)
	secret := "s3cr3t"
	svc := newWebhookService(t, secret)

	require.True(t, svc.VerifyWebhookSignature(ctx, uuid.New(), payload, signPayload(secret, payload)),
		"a valid signature must be accepted")
	require.False(t, svc.VerifyWebhookSignature(ctx, uuid.New(), payload, "deadbeef"),
		"a wrong signature must be rejected")
	require.False(t, svc.VerifyWebhookSignature(ctx, uuid.New(), payload, ""),
		"an unsigned webhook must be rejected when a secret is configured")
}

// When no webhook secret is configured on our side we cannot verify anything,
// so the webhook is accepted (both signed and unsigned).
func TestVerifyWebhookSignature_SkipsWhenNoSecretConfigured(t *testing.T) {
	ctx := context.Background()
	payload := []byte(`{"action":"created","issue":{"number":1}}`)
	svc := newWebhookService(t, "")

	require.True(t, svc.VerifyWebhookSignature(ctx, uuid.New(), payload, ""))
	require.True(t, svc.VerifyWebhookSignature(ctx, uuid.New(), payload, signPayload("whatever", payload)))
}

// A signature produced with a different secret must not be accepted.
func TestVerifyWebhookSignature_RejectsForeignSecret(t *testing.T) {
	ctx := context.Background()
	payload := []byte(`{"action":"opened"}`)
	svc := newWebhookService(t, "my-secret")

	require.False(t, svc.VerifyWebhookSignature(ctx, uuid.New(), payload, signPayload("other-secret", payload)))
}

// --- Per-user Gitea token ---

type stubTokenGiteaRepo struct {
	repository.GiteaRepo
	inst  *domain.GiteaInstance
	repos []domain.GiteaRepoModel
}

func (s *stubTokenGiteaRepo) GetInstanceByWorkspace(_ context.Context, _ uuid.UUID) (*domain.GiteaInstance, error) {
	return s.inst, nil
}

func (s *stubTokenGiteaRepo) ListReposByWorkspace(_ context.Context, _ uuid.UUID) ([]domain.GiteaRepoModel, error) {
	return s.repos, nil
}

type stubTokenUserRepo struct {
	repository.UserRepo
	user *domain.User
}

func (r *stubTokenUserRepo) GetByID(_ context.Context, _ uuid.UUID) (*domain.User, error) {
	return r.user, nil
}

func (r *stubTokenUserRepo) Update(_ context.Context, user *domain.User) error {
	r.user = user
	return nil
}

func newTokenService(t *testing.T, instanceURL string) (*GiteaService, *stubTokenUserRepo) {
	t.Helper()
	userRepo := &stubTokenUserRepo{user: &domain.User{ID: uuid.New(), Email: "u@example.com", Name: "U"}}
	var gtRepo repository.GiteaRepo = &stubTokenGiteaRepo{}
	if instanceURL != "" {
		gtRepo = &stubTokenGiteaRepo{inst: &domain.GiteaInstance{
			ID:          uuid.New(),
			WorkspaceID: uuid.New(),
			InstanceURL: instanceURL,
		}}
	}
	key := crypto.DeriveKey("gitea-user-token-test")
	return &GiteaService{gtRepo: gtRepo, userRepo: userRepo, encryptionKey: key}, userRepo
}

// A token accepted by the instance is stored encrypted and the login is
// derived from the verified account.
func TestSetUserToken_VerifiesAndStores(t *testing.T) {
	gitea := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/user", r.URL.Path)
		require.Equal(t, "token my-token", r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{"id":1,"login":"octo.cat","full_name":"Octo"}`))
	}))
	defer gitea.Close()

	svc, userRepo := newTokenService(t, gitea.URL)
	user, err := svc.SetUserToken(context.Background(), uuid.New(), userRepo.user.ID, "my-token")

	require.NoError(t, err)
	require.NotNil(t, user.GiteaToken)
	require.NotEqual(t, "my-token", *user.GiteaToken, "the token must be stored encrypted")
	decrypted, err := crypto.Decrypt(*user.GiteaToken, svc.encryptionKey)
	require.NoError(t, err)
	require.Equal(t, "my-token", decrypted)
	if assert.NotNil(t, user.GiteaLogin) {
		assert.Equal(t, "octo.cat", *user.GiteaLogin)
	}
}

// A token the instance rejects must surface as ErrInvalidGiteaToken and must
// not be persisted.
func TestSetUserToken_RejectsInvalidToken(t *testing.T) {
	gitea := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer gitea.Close()

	svc, userRepo := newTokenService(t, gitea.URL)
	user, err := svc.SetUserToken(context.Background(), uuid.New(), userRepo.user.ID, "bad-token")

	assert.ErrorIs(t, err, ErrInvalidGiteaToken)
	assert.Nil(t, user)
	assert.Nil(t, userRepo.user.GiteaToken)
}

// An empty token clears both stored credentials.
func TestSetUserToken_EmptyTokenClears(t *testing.T) {
	svc, userRepo := newTokenService(t, "")
	enc, err := crypto.Encrypt("old", svc.encryptionKey)
	require.NoError(t, err)
	login := "octo.cat"
	userRepo.user.GiteaToken = &enc
	userRepo.user.GiteaLogin = &login

	user, err := svc.SetUserToken(context.Background(), uuid.New(), userRepo.user.ID, "   ")

	require.NoError(t, err)
	assert.Nil(t, user.GiteaToken)
	assert.Nil(t, user.GiteaLogin)
}

// Without a connected instance the token cannot be verified, so it is stored
// as-is and the existing login is preserved.
func TestSetUserToken_StoresUnverifiedWithoutInstance(t *testing.T) {
	svc, userRepo := newTokenService(t, "")
	login := "octo.cat"
	userRepo.user.GiteaLogin = &login

	user, err := svc.SetUserToken(context.Background(), uuid.New(), userRepo.user.ID, "pending-token")

	require.NoError(t, err)
	require.NotNil(t, user.GiteaToken)
	decrypted, err := crypto.Decrypt(*user.GiteaToken, svc.encryptionKey)
	require.NoError(t, err)
	require.Equal(t, "pending-token", decrypted)
	if assert.NotNil(t, user.GiteaLogin) {
		assert.Equal(t, "octo.cat", *user.GiteaLogin)
	}
}

// Outgoing comments must be posted with the author's own token, never with
// the workspace integration token.
func TestSyncCommentToGitea_UsesAuthorToken(t *testing.T) {
	var gotAuth string
	gitea := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"id":77,"body":"hi","html_url":"u"}`))
	}))
	defer gitea.Close()

	svc, userRepo := newTokenService(t, gitea.URL)
	enc, err := crypto.Encrypt("author-token", svc.encryptionKey)
	require.NoError(t, err)
	userRepo.user.GiteaToken = &enc
	authorID := userRepo.user.ID

	svc.gtRepo = &stubTokenGiteaRepo{
		inst: &domain.GiteaInstance{ID: uuid.New(), WorkspaceID: uuid.New(), InstanceURL: gitea.URL},
		repos: []domain.GiteaRepoModel{{
			ID: uuid.New(), WorkspaceID: uuid.New(), GiteaRepoID: 1,
			FullName: "octo/rocket", IsActive: true,
		}},
	}
	issue := &domain.Issue{
		ID: uuid.New(), WorkspaceID: uuid.New(),
		GiteaIssueIndex: int64Ptr(7), GiteaInstanceID: &uuid.Nil,
	}
	issue.GiteaInstanceID = &[]uuid.UUID{uuid.New()}[0]
	comment := &domain.Comment{ID: uuid.New(), IssueID: issue.ID, UserID: &authorID, Body: "hello"}

	require.NoError(t, svc.SyncCommentToGitea(context.Background(), issue, comment))
	require.Equal(t, "token author-token", gotAuth)
	require.NotNil(t, comment.GiteaCommentID)
	require.Equal(t, int64(77), *comment.GiteaCommentID)
}

// An author without a token must not silently fall back to the workspace
// token: the sync is skipped instead.
func TestSyncCommentToGitea_SkipsAuthorWithoutToken(t *testing.T) {
	called := false
	gitea := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		_, _ = w.Write([]byte(`{"id":1}`))
	}))
	defer gitea.Close()

	svc, userRepo := newTokenService(t, gitea.URL)
	svc.gtRepo = &stubTokenGiteaRepo{
		inst: &domain.GiteaInstance{ID: uuid.New(), WorkspaceID: uuid.New(), InstanceURL: gitea.URL},
		repos: []domain.GiteaRepoModel{{
			ID: uuid.New(), WorkspaceID: uuid.New(), GiteaRepoID: 1,
			FullName: "octo/rocket", IsActive: true,
		}},
	}
	authorID := userRepo.user.ID
	issue := &domain.Issue{
		ID: uuid.New(), WorkspaceID: uuid.New(),
		GiteaIssueIndex: int64Ptr(7),
	}
	issue.GiteaInstanceID = &[]uuid.UUID{uuid.New()}[0]
	comment := &domain.Comment{ID: uuid.New(), IssueID: issue.ID, UserID: &authorID, Body: "hello"}

	require.NoError(t, svc.SyncCommentToGitea(context.Background(), issue, comment))
	assert.False(t, called, "no Gitea call must be made without an author token")
	assert.Nil(t, comment.GiteaCommentID)
}

func int64Ptr(v int64) *int64 { return &v }

// --- Status ↔ Gitea label equivalence ---

// Label matching picks the workspace status a Gitea label refers to,
// preferring a label that differs from the status the issue is on.
func TestStatusForLabels_MatchesAndPrefersFreshLabel(t *testing.T) {
	statuses := []domain.WorkspaceStatus{
		{ID: uuid.New(), Name: "In Progress", Slug: "in_progress"},
		{ID: uuid.New(), Name: "Done", Slug: "done"},
	}
	labels := func(names ...string) []giteaWebhookLabel {
		out := make([]giteaWebhookLabel, 0, len(names))
		for _, n := range names {
			out = append(out, giteaWebhookLabel{Name: n})
		}
		return out
	}

	// No match: unrelated labels leave the status alone.
	assert.Nil(t, statusForLabels(statuses, labels("bug", "help wanted"), nil))

	// Match by name and by slug, case-insensitively.
	assert.Equal(t, "In Progress", statusForLabels(statuses, labels("IN PROGRESS"), nil).Name)
	assert.Equal(t, "In Progress", statusForLabels(statuses, labels("in_progress"), nil).Name)

	// With two status labels the one the user just added wins over the
	// current status.
	assert.Equal(t, "Done", statusForLabels(statuses, labels("In Progress", "Done"), &statuses[0].ID).Name)

	// When every status label is the current one, it is returned unchanged.
	assert.Equal(t, "In Progress", statusForLabels(statuses, labels("In Progress"), &statuses[0].ID).Name)
}

// Kuayle status colors are "#rrggbb"; Gitea wants plain hex.
func TestGiteaLabelColor(t *testing.T) {
	hex := func(s string) *string { return &s }
	assert.Equal(t, "ef4444", giteaLabelColor(hex("#ef4444")))
	assert.Equal(t, "ef4444", giteaLabelColor(hex("ef4444")))
	assert.Equal(t, "ededed", giteaLabelColor(hex("#nope")))
	assert.Equal(t, "ededed", giteaLabelColor(nil))
}

// outgoing sync: the status label is created when missing, attached to the
// issue, stale status labels are dropped and unrelated labels survive.
func TestSyncIssueStatusLabelsToGitea(t *testing.T) {
	type fakeGitea struct {
		mu          sync.Mutex
		repoLabels  []gt.Label
		issueLabels []gt.Label
	}
	f := &fakeGitea{
		repoLabels:  []gt.Label{{ID: 1, Name: "bug"}},
		issueLabels: []gt.Label{{ID: 1, Name: "bug"}, {ID: 9, Name: "Backlog"}},
	}
	gitea := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/issues/") && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(f.issueLabels)
		case strings.Contains(r.URL.Path, "/issues/") && r.Method == http.MethodPost:
			var body struct {
				Labels []int64 `json:"labels"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			for _, id := range body.Labels {
				for _, l := range f.repoLabels {
					if l.ID == id {
						f.issueLabels = append(f.issueLabels, l)
					}
				}
			}
			_ = json.NewEncoder(w).Encode(f.issueLabels)
		case strings.Contains(r.URL.Path, "/issues/") && r.Method == http.MethodDelete:
			id := path.Base(r.URL.Path)
			kept := f.issueLabels[:0]
			for _, l := range f.issueLabels {
				if strconv.FormatInt(l.ID, 10) != id {
					kept = append(kept, l)
				}
			}
			f.issueLabels = kept
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/labels"):
			_ = json.NewEncoder(w).Encode(f.repoLabels)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/labels"):
			var body struct {
				Name  string `json:"name"`
				Color string `json:"color"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			label := gt.Label{ID: 42, Name: body.Name, Color: body.Color}
			f.repoLabels = append(f.repoLabels, label)
			_ = json.NewEncoder(w).Encode(label)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer gitea.Close()

	svc, _ := newTokenService(t, gitea.URL)
	workspaceToken, err := crypto.Encrypt("workspace-token", svc.encryptionKey)
	require.NoError(t, err)
	svc.gtRepo = &stubTokenGiteaRepo{
		inst: &domain.GiteaInstance{
			ID: uuid.New(), WorkspaceID: uuid.New(),
			InstanceURL: gitea.URL, AccessToken: workspaceToken,
		},
		repos: []domain.GiteaRepoModel{{
			ID: uuid.New(), WorkspaceID: uuid.New(), GiteaRepoID: 1,
			FullName: "octo/rocket", IsActive: true,
		}},
	}
	statusID := uuid.New()
	color := "#3b82f6"
	svc.statusRepo = &stubStatusLabelRepo{statuses: []domain.WorkspaceStatus{
		{ID: uuid.New(), Name: "Backlog", Slug: "backlog"},
		{ID: statusID, Name: "In Progress", Slug: "in_progress", Color: &color},
	}}
	issue := &domain.Issue{
		ID: uuid.New(), WorkspaceID: uuid.New(),
		GiteaIssueIndex: int64Ptr(7), StatusID: &statusID, Status: "in_progress",
	}
	issue.GiteaInstanceID = &[]uuid.UUID{uuid.New()}[0]

	require.NoError(t, svc.SyncIssueStatusLabelsToGitea(context.Background(), issue))

	f.mu.Lock()
	defer f.mu.Unlock()
	names := make([]string, 0, len(f.issueLabels))
	for _, l := range f.issueLabels {
		names = append(names, l.Name)
	}
	assert.ElementsMatch(t, []string{"bug", "In Progress"}, names, "stale status label dropped, unrelated kept")
	createdIdx := -1
	for i, l := range f.repoLabels {
		if l.Name == "In Progress" {
			createdIdx = i
		}
	}
	require.NotEqual(t, -1, createdIdx, "status label must be created on the repo")
	assert.Equal(t, "3b82f6", f.repoLabels[createdIdx].Color, "label color comes from the status color")
}

// Incoming label change: the matching label becomes the issue status, a
// history entry is recorded, and the label set is normalized.
func TestSyncStatusFromGiteaLabels(t *testing.T) {
	doneID := uuid.New()
	statuses := []domain.WorkspaceStatus{
		{ID: uuid.New(), Name: "Todo", Slug: "todo", Category: domain.StatusCategoryUnstarted},
		{ID: doneID, Name: "Done", Slug: "done", Category: domain.StatusCategoryCompleted},
	}
	svc, _ := newTokenService(t, "")
	svc.statusRepo = &stubStatusLabelRepo{statuses: statuses, byID: map[uuid.UUID]*domain.WorkspaceStatus{
		statuses[0].ID: &statuses[0],
	}}
	svc.issueRepo = &stubLabelIssueRepo{issue: &domain.Issue{
		ID: uuid.New(), WorkspaceID: uuid.New(), Status: "todo",
	}}
	svc.historyRepo = &stubLabelHistoryRepo{}
	svc.gtRepo = &stubTokenGiteaRepo{
		inst: &domain.GiteaInstance{ID: uuid.New(), WorkspaceID: uuid.New(), InstanceURL: "http://127.0.0.1:1"},
		repos: []domain.GiteaRepoModel{{
			ID: uuid.New(), WorkspaceID: uuid.New(), GiteaRepoID: 1,
			FullName: "octo/rocket", IsActive: true,
		}},
	}

	var event giteaWebhookIssue
	event.Issue.Number = 7
	event.Issue.Labels = []giteaWebhookLabel{{Name: "Done"}}

	inst := &domain.GiteaInstance{ID: uuid.New(), WorkspaceID: uuid.New()}
	require.NoError(t, svc.syncStatusFromGiteaLabels(context.Background(), uuid.New(), inst, event))

	issue := svc.issueRepo.(*stubLabelIssueRepo).issue
	if assert.NotNil(t, issue.StatusID) {
		assert.Equal(t, doneID, *issue.StatusID)
	}
	assert.Equal(t, domain.IssueStatus("done"), issue.Status)
	require.Len(t, svc.historyRepo.(*stubLabelHistoryRepo).entries, 1, "status change is recorded in history")
}

type stubStatusLabelRepo struct {
	repository.StatusRepo
	statuses []domain.WorkspaceStatus
	byID     map[uuid.UUID]*domain.WorkspaceStatus
}

func (r *stubStatusLabelRepo) ListByWorkspace(_ context.Context, _ uuid.UUID) ([]domain.WorkspaceStatus, error) {
	return r.statuses, nil
}

func (r *stubStatusLabelRepo) GetByID(_ context.Context, id uuid.UUID) (*domain.WorkspaceStatus, error) {
	return r.byID[id], nil
}

type stubLabelIssueRepo struct {
	repository.IssueRepo
	issue *domain.Issue
}

func (r *stubLabelIssueRepo) GetByGiteaIssueIndex(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ int64) (*domain.Issue, error) {
	return r.issue, nil
}

func (r *stubLabelIssueRepo) Update(_ context.Context, issue *domain.Issue) error {
	r.issue = issue
	return nil
}

func (r *stubLabelIssueRepo) ListSubIssues(_ context.Context, _ uuid.UUID) ([]domain.Issue, error) {
	return nil, nil
}

func (r *stubLabelIssueRepo) CountSubIssues(_ context.Context, _ uuid.UUID) (int, int, error) {
	return 0, 0, nil
}

type stubLabelHistoryRepo struct {
	repository.IssueHistoryRepo
	entries []string
}

func (r *stubLabelHistoryRepo) Create(_ context.Context, _, _ uuid.UUID, field string, _, _ *string) error {
	r.entries = append(r.entries, field)
	return nil
}
