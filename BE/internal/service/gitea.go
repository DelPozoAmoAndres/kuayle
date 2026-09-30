package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kuayle/kuayle-backend/internal/domain"
	"github.com/kuayle/kuayle-backend/internal/dto"
	"github.com/kuayle/kuayle-backend/internal/realtime"
	"github.com/kuayle/kuayle-backend/internal/repository"
	"github.com/kuayle/kuayle-backend/pkg/crypto"
	gt "github.com/kuayle/kuayle-backend/pkg/gitea"
	log "github.com/sirupsen/logrus"
)

// contextKey is an unexported type for context keys defined in this package.
type contextKey string

// syncingFromGiteaKey is the context key used to flag that an update is being
// initiated from a Gitea webhook, preventing infinite sync loops back to Gitea.
const syncingFromGiteaKey contextKey = "syncing_from_gitea"

// SyncingFromGitea returns a new context flagged as originating from a Gitea
// webhook, so that downstream code can skip syncing back to Gitea.
func SyncingFromGitea(ctx context.Context) context.Context {
	return context.WithValue(ctx, syncingFromGiteaKey, true)
}

// IsSyncingFromGitea checks whether the context was flagged by SyncingFromGitea.
func IsSyncingFromGitea(ctx context.Context) bool {
	v, _ := ctx.Value(syncingFromGiteaKey).(bool)
	return v
}

// commentFromGiteaService is the subset of CommentService used to apply
// comments that arrive from a Gitea webhook. It is injected through
// SetCommentService to avoid an initialization cycle.
type commentFromGiteaService interface {
	CreateFromGitea(ctx context.Context, issue *domain.Issue, comment *domain.Comment) error
	UpdateFromGitea(ctx context.Context, comment *domain.Comment, body string) error
	DeleteFromGitea(ctx context.Context, comment *domain.Comment) error
}

type GiteaService struct {
	gtRepo        repository.GiteaRepo
	issueRepo     repository.IssueRepo
	statusRepo    repository.StatusRepo
	historyRepo   repository.IssueHistoryRepo
	projectRepo   *repository.ProjectRepository
	userRepo      repository.UserRepo
	commentRepo   repository.CommentRepo
	commentSvc    commentFromGiteaService
	encryptionKey []byte
	hub           *realtime.Hub
	frontendURL   string
}

func NewGiteaService(
	gtRepo repository.GiteaRepo,
	issueRepo repository.IssueRepo,
	statusRepo repository.StatusRepo,
	historyRepo repository.IssueHistoryRepo,
	projectRepo *repository.ProjectRepository,
	userRepo repository.UserRepo,
	commentRepo repository.CommentRepo,
	encryptionKey []byte,
	hub *realtime.Hub,
	frontendURL string,
) *GiteaService {
	return &GiteaService{
		gtRepo:        gtRepo,
		issueRepo:     issueRepo,
		statusRepo:    statusRepo,
		historyRepo:   historyRepo,
		projectRepo:   projectRepo,
		userRepo:      userRepo,
		commentRepo:   commentRepo,
		encryptionKey: encryptionKey,
		hub:           hub,
		frontendURL:   frontendURL,
	}
}

// SetCommentService wires the comment service used to persist comments that
// originate in Gitea (setter avoids an initialization cycle).
func (s *GiteaService) SetCommentService(commentSvc commentFromGiteaService) {
	s.commentSvc = commentSvc
}

// HasGiteaInstance reports whether the workspace has a connected Gitea
// instance. It is used to decide whether commenting requires a linked Gitea
// account.
func (s *GiteaService) HasGiteaInstance(ctx context.Context, workspaceID uuid.UUID) (bool, error) {
	inst, err := s.gtRepo.GetInstanceByWorkspace(ctx, workspaceID)
	if err != nil {
		return false, err
	}
	return inst != nil, nil
}

// Connect verifies the PAT and stores the instance.
func (s *GiteaService) Connect(ctx context.Context, workspaceID, userID uuid.UUID, req dto.GiteaConnectRequest) (*domain.GiteaInstance, error) {
	// Check if already connected
	existing, err := s.gtRepo.GetInstanceByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, fmt.Errorf("Gitea already connected to this workspace")
	}

	// Verify PAT
	client := gt.NewClient(req.InstanceURL, req.AccessToken)
	user, err := client.VerifyConnection()
	if err != nil {
		return nil, fmt.Errorf("failed to verify Gitea connection: %w", err)
	}

	// Encrypt the access token
	encToken, err := crypto.Encrypt(req.AccessToken, s.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("encrypting access token: %w", err)
	}

	inst := &domain.GiteaInstance{
		ID:           uuid.New(),
		WorkspaceID:  workspaceID,
		InstanceURL:  req.InstanceURL,
		AccountLogin: user.Login,
		AccessToken:  encToken,
		InstalledBy:  userID,
	}

	if err := s.gtRepo.CreateInstance(ctx, inst); err != nil {
		return nil, err
	}

	// Store webhook secret if provided
	if req.WebhookSecret != "" {
		encSecret, err := crypto.Encrypt(req.WebhookSecret, s.encryptionKey)
		if err != nil {
			log.WithError(err).Warn("failed to encrypt webhook secret")
		} else {
			oauthCfg := &domain.GiteaOAuthConfig{
				ID:            uuid.New(),
				WorkspaceID:   workspaceID,
				InstanceURL:   req.InstanceURL,
				WebhookSecret: encSecret,
			}
			if err := s.gtRepo.CreateOAuthConfig(ctx, oauthCfg); err != nil {
				log.WithError(err).Warn("failed to store webhook secret")
			}
		}
	}

	// Seed default auto-transition rules
	defaults := []struct {
		Event  string
		Status string
	}{
		{"branch_created", "in_progress"},
		{"pr_opened", "in_review"},
		{"pr_merged", "done"},
	}
	for _, d := range defaults {
		t := &domain.GiteaAutoTransition{
			ID:           uuid.New(),
			WorkspaceID:  workspaceID,
			Event:        d.Event,
			TargetStatus: d.Status,
			IsActive:     true,
		}
		_ = s.gtRepo.UpsertAutoTransition(ctx, t)
	}

	return inst, nil
}

// GetStatus returns the current Gitea integration status for a workspace.
func (s *GiteaService) GetStatus(ctx context.Context, workspaceID uuid.UUID) (*dto.GiteaStatusResponse, error) {
	inst, _ := s.gtRepo.GetInstanceByWorkspace(ctx, workspaceID)

	resp := &dto.GiteaStatusResponse{
		Connected: inst != nil,
		Repos:     []dto.GiteaRepoResponse{},
	}

	if inst != nil {
		resp.Instance = &dto.GiteaInstanceResponse{
			ID:           inst.ID.String(),
			InstanceURL:  inst.InstanceURL,
			AccountLogin: inst.AccountLogin,
			CreatedAt:    inst.CreatedAt.Format(time.RFC3339),
		}

		repos, err := s.gtRepo.ListReposByWorkspace(ctx, workspaceID)
		if err != nil {
			return nil, err
		}
		for _, r := range repos {
			resp.Repos = append(resp.Repos, dto.GiteaRepoResponse{
				ID:            r.ID.String(),
				GiteaRepoID:   r.GiteaRepoID,
				FullName:      r.FullName,
				DefaultBranch: r.DefaultBranch,
				IsActive:      r.IsActive,
			})
		}
	}

	return resp, nil
}

// ListAvailableRepos lists repos accessible to the authenticated Gitea user.
func (s *GiteaService) ListAvailableRepos(ctx context.Context, workspaceID uuid.UUID) ([]dto.GiteaAvailableRepoResponse, error) {
	inst, err := s.gtRepo.GetInstanceByWorkspace(ctx, workspaceID)
	if err != nil || inst == nil {
		return nil, fmt.Errorf("Gitea not connected")
	}

	token, err := s.decryptToken(inst.AccessToken)
	if err != nil {
		return nil, err
	}

	client := gt.NewClient(inst.InstanceURL, token)
	gtRepos, err := client.ListUserRepos()
	if err != nil {
		return nil, fmt.Errorf("listing repos: %w", err)
	}

	linkedRepos, err := s.gtRepo.ListReposByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	linkedMap := make(map[int64]bool)
	for _, lr := range linkedRepos {
		linkedMap[lr.GiteaRepoID] = true
	}

	var result []dto.GiteaAvailableRepoResponse
	for _, r := range gtRepos {
		result = append(result, dto.GiteaAvailableRepoResponse{
			GiteaRepoID:   r.ID,
			FullName:      r.FullName,
			DefaultBranch: r.DefaultBranch,
			Private:       r.Private,
			Linked:        linkedMap[r.ID],
		})
	}
	return result, nil
}

// LinkRepos links Gitea repos to the workspace.
func (s *GiteaService) LinkRepos(ctx context.Context, workspaceID uuid.UUID, req dto.LinkGiteaReposRequest) error {
	inst, err := s.gtRepo.GetInstanceByWorkspace(ctx, workspaceID)
	if err != nil || inst == nil {
		return fmt.Errorf("Gitea not connected")
	}

	token, err := s.decryptToken(inst.AccessToken)
	if err != nil {
		return err
	}

	client := gt.NewClient(inst.InstanceURL, token)
	gtRepos, err := client.ListUserRepos()
	if err != nil {
		return fmt.Errorf("listing repos: %w", err)
	}

	repoMap := make(map[int64]gt.Repository)
	for _, r := range gtRepos {
		repoMap[r.ID] = r
	}

	for _, id := range req.GiteaRepoIDs {
		gtRepo, ok := repoMap[id]
		if !ok {
			continue
		}
		existing, _ := s.gtRepo.GetRepoByGiteaID(ctx, workspaceID, id)
		if existing != nil {
			continue
		}
		repo := &domain.GiteaRepoModel{
			ID:            uuid.New(),
			InstanceID:    inst.ID,
			WorkspaceID:   workspaceID,
			GiteaRepoID:   gtRepo.ID,
			FullName:      gtRepo.FullName,
			DefaultBranch: gtRepo.DefaultBranch,
			IsActive:      true,
		}
		if err := s.gtRepo.CreateRepo(ctx, repo); err != nil {
			log.WithError(err).WithField("repo", gtRepo.FullName).Warn("failed to link repo")
			continue
		}

		// Auto-create a Project in Kuayle for this repo
		if s.projectRepo != nil {
			desc := fmt.Sprintf("Auto-created from Gitea repo %s", gtRepo.FullName)
			project := &domain.Project{
				ID:          uuid.New(),
				WorkspaceID: workspaceID,
				Name:        gtRepo.FullName,
				Description: &desc,
				Status:      domain.ProjectStatusPlanned,
				SortOrder:   float64(time.Now().UnixMilli()),
			}
			if err := s.projectRepo.Create(ctx, project); err != nil {
				log.WithError(err).WithField("repo", gtRepo.FullName).Warn("failed to auto-create project")
			}
		}
	}
	return nil
}

// UnlinkRepo removes a linked repo.
func (s *GiteaService) UnlinkRepo(ctx context.Context, repoID uuid.UUID) error {
	return s.gtRepo.DeleteRepo(ctx, repoID)
}

// GetUserToken returns the caller's stored Gitea credentials (token included
// only in encrypted form on the user row; never exposed).
func (s *GiteaService) GetUserToken(ctx context.Context, workspaceID, userID uuid.UUID) (*domain.User, error) {
	_ = workspaceID
	return s.userRepo.GetByID(ctx, userID)
}

// SetUserToken stores the caller's own Gitea PAT so their comments are posted
// on their behalf instead of the workspace integration account. When the
// workspace has a connected instance the token is verified against it, which
// also derives the account login used to attribute incoming Gitea comments.
// An empty token removes the stored credential.
func (s *GiteaService) SetUserToken(ctx context.Context, workspaceID, userID uuid.UUID, token string) (*domain.User, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, fmt.Errorf("user not found")
	}

	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		user.GiteaToken = nil
		user.GiteaLogin = nil
		if err := s.userRepo.Update(ctx, user); err != nil {
			return nil, err
		}
		return user, nil
	}

	inst, err := s.gtRepo.GetInstanceByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}

	login := user.GiteaLogin
	if inst != nil {
		verified, err := gt.NewClient(inst.InstanceURL, trimmed).VerifyConnection()
		if err != nil {
			return nil, ErrInvalidGiteaToken
		}
		login = &verified.Login
	}

	encrypted, err := crypto.Encrypt(trimmed, s.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("encrypting Gitea token: %w", err)
	}
	user.GiteaToken = &encrypted
	user.GiteaLogin = login
	if err := s.userRepo.Update(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

// Disconnect removes the Gitea instance.
func (s *GiteaService) Disconnect(ctx context.Context, workspaceID uuid.UUID) error {
	_ = s.gtRepo.DeleteOAuthConfig(ctx, workspaceID)
	return s.gtRepo.DeleteInstance(ctx, workspaceID)
}

// --- Webhook Event Processing ---

// VerifyWebhookSignature verifies the Gitea webhook HMAC-SHA256 signature.
// If no webhook secret is configured, skips verification (allows unsigned webhooks).
func (s *GiteaService) VerifyWebhookSignature(ctx context.Context, workspaceID uuid.UUID, payload []byte, signature string) bool {
	oauthCfg, err := s.gtRepo.GetOAuthConfigByWorkspace(ctx, workspaceID)
	// No secret configured on our side = skip verification.
	if err != nil || oauthCfg == nil || oauthCfg.WebhookSecret == "" {
		return true
	}
	// A secret is configured: the webhook must be signed.
	if signature == "" {
		return false
	}

	secret, err := crypto.Decrypt(oauthCfg.WebhookSecret, s.encryptionKey)
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(signature), []byte(expected))
}

// ResolveAndVerifyWebhook tries to match an incoming webhook to a workspace by
// checking the signature against all configured instances. Returns the matched workspaceID.
func (s *GiteaService) ResolveAndVerifyWebhook(ctx context.Context, payload []byte, signature string) (uuid.UUID, bool) {
	// Extract repository owner from payload to narrow down
	var partial struct {
		Repository struct {
			Owner struct {
				Login string `json:"login"`
			} `json:"owner"`
			FullName string `json:"full_name"`
		} `json:"repository"`
	}
	if err := json.Unmarshal(payload, &partial); err != nil {
		return uuid.Nil, false
	}

	instances, err := s.gtRepo.ListInstances(ctx)
	if err != nil || len(instances) == 0 {
		return uuid.Nil, false
	}

	for _, inst := range instances {
		oauthCfg, err := s.gtRepo.GetOAuthConfigByWorkspace(ctx, inst.WorkspaceID)
		if err != nil || oauthCfg == nil {
			continue
		}

		secret, err := crypto.Decrypt(oauthCfg.WebhookSecret, s.encryptionKey)
		if err != nil {
			continue
		}

		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(payload)
		expected := hex.EncodeToString(mac.Sum(nil))
		if hmac.Equal([]byte(signature), []byte(expected)) {
			return inst.WorkspaceID, true
		}
	}

	return uuid.Nil, false
}

// HandleWebhookEvent processes an incoming Gitea webhook event.
func (s *GiteaService) HandleWebhookEvent(ctx context.Context, workspaceID uuid.UUID, eventType string, payload []byte) error {
	switch eventType {
	case "pull_request":
		return s.processPullRequestEvent(ctx, workspaceID, payload)
	case "push":
		return s.processPushEvent(ctx, workspaceID, payload)
	case "issues":
		return s.processIssuesEvent(ctx, workspaceID, payload)
	case "issue_label":
		// Label changes are the incoming half of the status/label equivalence.
		return s.processIssuesEvent(ctx, workspaceID, payload)
	case "issue_comment", "comment":
		return s.processIssueCommentEvent(ctx, workspaceID, payload)
	default:
		return nil
	}
}

func (s *GiteaService) broadcastAppRefresh(workspaceID uuid.UUID, resources ...string) {
	if s.hub == nil {
		return
	}
	s.hub.Broadcast(workspaceID, realtime.Event{
		Type: "app.refresh",
		Payload: map[string]any{
			"source":    "gitea",
			"resources": resources,
		},
	})
}

func (s *GiteaService) broadcastRealtimeEvent(workspaceID uuid.UUID, event realtime.Event) {
	if s.hub == nil {
		return
	}
	s.hub.Broadcast(workspaceID, event)
}

// giteaWebhookPR mirrors the Gitea pull_request webhook payload.
type giteaWebhookPR struct {
	Action      string `json:"action"`
	Number      int    `json:"number"`
	PullRequest struct {
		ID      int64  `json:"id"`
		Number  int    `json:"number"`
		Title   string `json:"title"`
		State   string `json:"state"`
		Draft   bool   `json:"draft"`
		Merged  bool   `json:"merged"`
		HTMLURL string `json:"html_url"`
		Head    struct {
			Ref string `json:"ref"`
		} `json:"head"`
		Base struct {
			Ref string `json:"ref"`
		} `json:"base"`
		User struct {
			Login     string `json:"login"`
			AvatarURL string `json:"avatar_url"`
		} `json:"user"`
		Additions int        `json:"additions"`
		Deletions int        `json:"deletions"`
		MergedAt  *time.Time `json:"merged_at"`
		ClosedAt  *time.Time `json:"closed_at"`
		Body      string     `json:"body"`
	} `json:"pull_request"`
	Repository struct {
		ID int64 `json:"id"`
	} `json:"repository"`
	Sender struct {
		Login string `json:"login"`
	} `json:"sender"`
}

func (s *GiteaService) processPullRequestEvent(ctx context.Context, workspaceID uuid.UUID, payload []byte) error {
	var event giteaWebhookPR
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("unmarshaling PR event: %w", err)
	}

	repo, err := s.gtRepo.GetRepoByGiteaID(ctx, workspaceID, event.Repository.ID)
	if err != nil || repo == nil {
		return nil
	}

	state := event.PullRequest.State
	if event.PullRequest.Draft {
		state = "draft"
	}
	if event.PullRequest.Merged {
		state = "merged"
	}

	issue := s.resolveIssueFromRef(ctx, workspaceID,
		event.PullRequest.Head.Ref,
		event.PullRequest.Title,
		event.PullRequest.Body,
	)

	var issueID *uuid.UUID
	if issue != nil {
		issueID = &issue.ID
	}

	avatarURL := event.PullRequest.User.AvatarURL
	headRef := event.PullRequest.Head.Ref
	baseRef := event.PullRequest.Base.Ref

	pr := &domain.GiteaPullRequest{
		ID:              uuid.New(),
		WorkspaceID:     workspaceID,
		IssueID:         issueID,
		GiteaRepoID:     repo.ID,
		GiteaPRID:       event.PullRequest.ID,
		Number:          event.PullRequest.Number,
		Title:           event.PullRequest.Title,
		State:           state,
		AuthorLogin:     event.PullRequest.User.Login,
		AuthorAvatarURL: &avatarURL,
		HTMLURL:         event.PullRequest.HTMLURL,
		HeadBranch:      &headRef,
		BaseBranch:      &baseRef,
		Additions:       event.PullRequest.Additions,
		Deletions:       event.PullRequest.Deletions,
		MergedAt:        event.PullRequest.MergedAt,
		ClosedAt:        event.PullRequest.ClosedAt,
	}

	if err := s.gtRepo.UpsertPullRequest(ctx, pr); err != nil {
		return fmt.Errorf("upserting PR: %w", err)
	}

	if issue != nil {
		switch {
		case event.Action == "opened" || event.Action == "ready_for_review":
			s.applyAutoTransition(ctx, workspaceID, "pr_opened", issue)
		case event.PullRequest.Merged:
			s.applyAutoTransition(ctx, workspaceID, "pr_merged", issue)
		}

		s.broadcastRealtimeEvent(workspaceID, realtime.Event{
			Type:    "gitea:pr_updated",
			Payload: map[string]any{"issue_id": issue.ID, "pr_number": pr.Number, "state": state},
		})
		s.broadcastAppRefresh(workspaceID, "issues")
	}

	return nil
}

// giteaWebhookPush mirrors the Gitea push webhook payload.
type giteaWebhookPush struct {
	Ref     string `json:"ref"`
	Commits []struct {
		ID        string `json:"id"`
		Message   string `json:"message"`
		URL       string `json:"url"`
		Timestamp string `json:"timestamp"`
		Author    struct {
			Name     string `json:"name"`
			Username string `json:"username"`
		} `json:"author"`
	} `json:"commits"`
	Repository struct {
		ID      int64  `json:"id"`
		HTMLURL string `json:"html_url"`
	} `json:"repository"`
	CreateBranch bool `json:"create_branch"`
	Sender       struct {
		Login string `json:"login"`
	} `json:"sender"`
}

func (s *GiteaService) processPushEvent(ctx context.Context, workspaceID uuid.UUID, payload []byte) error {
	var event giteaWebhookPush
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("unmarshaling push event: %w", err)
	}

	repo, err := s.gtRepo.GetRepoByGiteaID(ctx, workspaceID, event.Repository.ID)
	if err != nil || repo == nil {
		return nil
	}

	// Detect branch creation via create_branch flag (Gitea doesn't have a separate create event)
	if event.CreateBranch {
		branchName := strings.TrimPrefix(event.Ref, "refs/heads/")
		issue := s.resolveIssueFromRef(ctx, workspaceID, branchName)
		var issueID *uuid.UUID
		if issue != nil {
			issueID = &issue.ID
		}

		branchURL := fmt.Sprintf("%s/src/branch/%s", strings.TrimRight(event.Repository.HTMLURL, "/"), branchName)
		branch := &domain.GiteaBranch{
			ID:           uuid.New(),
			WorkspaceID:  workspaceID,
			IssueID:      issueID,
			GiteaRepoID:  repo.ID,
			Name:         branchName,
			HTMLURL:      &branchURL,
		}

		if err := s.gtRepo.UpsertBranch(ctx, branch); err != nil {
			log.WithError(err).Warn("failed to upsert branch")
		}

		if issue != nil {
			s.applyAutoTransition(ctx, workspaceID, "branch_created", issue)
			s.broadcastRealtimeEvent(workspaceID, realtime.Event{
				Type:    "gitea:branch_created",
				Payload: map[string]any{"issue_id": issue.ID, "branch": branchName},
			})
			s.broadcastAppRefresh(workspaceID, "issues")
		}
	}

	updatedIssues := make(map[uuid.UUID]bool)
	for _, c := range event.Commits {
		issue := s.resolveIssueFromRef(ctx, workspaceID, c.Message)
		var issueID *uuid.UUID
		if issue != nil {
			issueID = &issue.ID
		}

		committedAt, _ := time.Parse(time.RFC3339, c.Timestamp)
		if committedAt.IsZero() {
			committedAt = time.Now()
		}

		username := c.Author.Username
		commit := &domain.GiteaCommit{
			ID:           uuid.New(),
			WorkspaceID:  workspaceID,
			IssueID:      issueID,
			GiteaRepoID:  repo.ID,
			SHA:          c.ID,
			Message:      c.Message,
			AuthorLogin:  &username,
			HTMLURL:      c.URL,
			CommittedAt:  committedAt,
		}

		if err := s.gtRepo.UpsertCommit(ctx, commit); err != nil {
			log.WithError(err).WithField("sha", c.ID).Warn("failed to upsert commit")
			continue
		}

		if issue != nil {
			updatedIssues[issue.ID] = true
			s.broadcastRealtimeEvent(workspaceID, realtime.Event{
				Type:    "gitea:commit_pushed",
				Payload: map[string]any{"issue_id": issue.ID, "sha": commit.SHA},
			})
		}
	}
	if len(updatedIssues) > 0 {
		s.broadcastAppRefresh(workspaceID, "issues")
	}

	return nil
}

// --- Issue Webhook Processing ---

// giteaWebhookIssue mirrors the Gitea issues webhook payload.
type giteaWebhookIssue struct {
	Action  string `json:"action"`
	Issue   struct {
		ID     int64  `json:"id"`
		Number int64  `json:"number"`
		Title  string `json:"title"`
		Body   string `json:"body"`
		State  string `json:"state"`
		Labels []giteaWebhookLabel `json:"labels"`
	} `json:"issue"`
	Repository struct {
		ID       int64  `json:"id"`
		FullName string `json:"full_name"`
	} `json:"repository"`
	Sender struct {
		Login string `json:"login"`
	} `json:"sender"`
}

// giteaWebhookLabel is the label shape embedded in issue webhook payloads.
type giteaWebhookLabel struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

func (s *GiteaService) processIssuesEvent(ctx context.Context, workspaceID uuid.UUID, payload []byte) error {
	var event giteaWebhookIssue
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("unmarshaling issues event: %w", err)
	}

	repo, err := s.gtRepo.GetRepoByGiteaID(ctx, workspaceID, event.Repository.ID)
	if err != nil || repo == nil {
		return nil
	}

	// Find the Gitea instance to get the instance ID for storing on the issue
	inst, err := s.gtRepo.GetInstanceByWorkspace(ctx, workspaceID)
	if err != nil || inst == nil {
		return nil
	}

	// Flag context to prevent infinite sync loops
	ctx = SyncingFromGitea(ctx)

	switch event.Action {
	case "opened", "created":
		return s.syncIssueFromGiteaOpen(ctx, workspaceID, inst, repo, event)
	case "edited":
		return s.syncIssueFromGiteaEdit(ctx, workspaceID, inst, event)
	case "closed":
		return s.syncIssueFromGiteaClose(ctx, workspaceID, inst, event)
	case "reopened":
		return s.syncIssueFromGiteaReopen(ctx, workspaceID, inst, event)
	// Label changes make Gitea labels and Kuayle statuses equivalent in both
	// directions: whichever status label is present becomes the issue status.
	case "labeled", "unlabeled", "label_updated", "label_cleared", "label_changed":
		return s.syncStatusFromGiteaLabels(ctx, workspaceID, inst, event)
	default:
		return nil
	}
}

// syncStatusFromGiteaLabels applies label changes made in Gitea to the issue
// status in Kuayle. The first label matching a workspace status (by name or
// slug, case-insensitively) wins; when no status label is present the current
// status is left untouched. The outgoing sync is then run to normalize the
// label set (e.g. drop a stale status label the user left in place).
func (s *GiteaService) syncStatusFromGiteaLabels(ctx context.Context, workspaceID uuid.UUID, inst *domain.GiteaInstance, event giteaWebhookIssue) error {
	issue, err := s.issueRepo.GetByGiteaIssueIndex(ctx, workspaceID, inst.ID, event.Issue.Number)
	if err != nil || issue == nil {
		return nil
	}

	statuses, err := s.statusRepo.ListByWorkspace(ctx, workspaceID)
	if err != nil || len(statuses) == 0 {
		return nil
	}

	target := statusForLabels(statuses, event.Issue.Labels, issue.StatusID)
	if target == nil {
		return nil // no status label on the issue: keep the current status
	}
	if issue.StatusID != nil && *issue.StatusID == target.ID && issue.Status == domain.IssueStatus(target.Slug) {
		return s.SyncIssueStatusLabelsToGitea(ctx, issue) // still normalize duplicate labels
	}

	old := string(issue.Status)
	issue.StatusID = &target.ID
	issue.Status = domain.IssueStatus(target.Slug)
	if err := s.issueRepo.Update(ctx, issue); err != nil {
		log.WithError(err).WithField("issue_id", issue.ID).Warn("failed to sync status from Gitea labels")
		return nil
	}
	_ = s.historyRepo.Create(ctx, issue.ID, uuid.Nil, "status", &old, &target.Slug)

	s.broadcastRealtimeEvent(workspaceID, realtime.Event{
		Type:    "issue.updated",
		Payload: issue,
	})
	s.broadcastAppRefresh(workspaceID, "issues")
	s.applyStatusAutomation(ctx, workspaceID, issue, map[uuid.UUID]bool{})
	return s.SyncIssueStatusLabelsToGitea(ctx, issue)
}

// statusForLabels returns the workspace status named by one of the Gitea
// labels (matching status name or slug, case-insensitively), or nil when no
// label refers to a status. When several status labels are present it prefers
// one that differs from the current status, so the label the user just added
// wins over the stale one.
func statusForLabels(statuses []domain.WorkspaceStatus, labels []giteaWebhookLabel, currentID *uuid.UUID) *domain.WorkspaceStatus {
	var firstMatch *domain.WorkspaceStatus
	for _, label := range labels {
		for i := range statuses {
			if !strings.EqualFold(statuses[i].Name, label.Name) && !strings.EqualFold(statuses[i].Slug, label.Name) {
				continue
			}
			if currentID == nil || statuses[i].ID != *currentID {
				return &statuses[i]
			}
			if firstMatch == nil {
				firstMatch = &statuses[i]
			}
		}
	}
	return firstMatch
}

func (s *GiteaService) syncIssueFromGiteaOpen(ctx context.Context, workspaceID uuid.UUID, inst *domain.GiteaInstance, repo *domain.GiteaRepoModel, event giteaWebhookIssue) error {
	// Check if already synced
	existing, err := s.issueRepo.GetByGiteaIssueIndex(ctx, workspaceID, inst.ID, event.Issue.Number)
	if err != nil || existing != nil {
		return nil
	}

	// Find the project linked to this Gitea repo (by name match), creating it
	// when it does not exist yet.
	project, err := s.findOrCreateProjectForRepo(ctx, workspaceID, repo.FullName)
	if err != nil {
		return fmt.Errorf("resolving project for gitea repo: %w", err)
	}

	prefix := projectPrefix(project.Name)

	tx, err := s.issueRepo.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	number, err := s.issueRepo.NextNumber(ctx, tx, workspaceID, prefix)
	if err != nil {
		return err
	}

	identifier := fmt.Sprintf("%s-%d", prefix, number)

	// Use the user who connected Gitea as the creator
	creatorID := inst.InstalledBy

	issue := &domain.Issue{
		ID:              uuid.New(),
		WorkspaceID:     workspaceID,
		ProjectID:       &project.ID,
		Number:          number,
		Identifier:      identifier,
		Title:           event.Issue.Title,
		Description:     &event.Issue.Body,
		Status:          domain.IssueStatusTodo,
		CreatorID:       creatorID,
		SortOrder:       -float64(number) * 1000,
		GiteaIssueIndex: &event.Issue.Number,
		GiteaInstanceID: &inst.ID,
		Triaged:         true,
	}

	// Resolve status_id against the workspace statuses
	if ts, err := s.statusRepo.GetByWorkspaceAndSlug(ctx, workspaceID, string(domain.IssueStatusTodo)); err == nil && ts != nil {
		issue.StatusID = &ts.ID
	}

	if err := s.issueRepo.Create(ctx, tx, issue); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	s.broadcastRealtimeEvent(workspaceID, realtime.Event{
		Type:    "issue.created",
		Payload: issue,
	})
	s.broadcastAppRefresh(workspaceID, "issues")
	// The issue may already carry status labels when it is opened in Gitea.
	return s.syncStatusFromGiteaLabels(ctx, workspaceID, inst, event)
}

func (s *GiteaService) syncIssueFromGiteaEdit(ctx context.Context, workspaceID uuid.UUID, inst *domain.GiteaInstance, event giteaWebhookIssue) error {
	issue, err := s.issueRepo.GetByGiteaIssueIndex(ctx, workspaceID, inst.ID, event.Issue.Number)
	if err != nil || issue == nil {
		return nil
	}

	issue.Title = event.Issue.Title
	issue.Description = &event.Issue.Body

	if err := s.issueRepo.Update(ctx, issue); err != nil {
		log.WithError(err).WithField("issue_id", issue.ID).Warn("failed to sync issue edit from Gitea")
		return nil
	}

	s.broadcastRealtimeEvent(workspaceID, realtime.Event{
		Type:    "issue.updated",
		Payload: issue,
	})
	s.broadcastAppRefresh(workspaceID, "issues")
	return nil
}

func (s *GiteaService) syncIssueFromGiteaClose(ctx context.Context, workspaceID uuid.UUID, inst *domain.GiteaInstance, event giteaWebhookIssue) error {
	issue, err := s.issueRepo.GetByGiteaIssueIndex(ctx, workspaceID, inst.ID, event.Issue.Number)
	if err != nil || issue == nil {
		return nil
	}

	issue.Status = domain.IssueStatusDone
	if ts, err := s.statusRepo.GetByWorkspaceAndSlug(ctx, workspaceID, string(domain.IssueStatusDone)); err == nil && ts != nil {
		issue.StatusID = &ts.ID
	}

	if err := s.issueRepo.Update(ctx, issue); err != nil {
		log.WithError(err).WithField("issue_id", issue.ID).Warn("failed to sync issue close from Gitea")
		return nil
	}

	s.broadcastRealtimeEvent(workspaceID, realtime.Event{
		Type:    "issue.updated",
		Payload: issue,
	})
	s.broadcastAppRefresh(workspaceID, "issues")
	// Keep the Gitea label equivalent to the status the close produced.
	s.syncStatusLabel(ctx, issue)
	return nil
}

func (s *GiteaService) syncIssueFromGiteaReopen(ctx context.Context, workspaceID uuid.UUID, inst *domain.GiteaInstance, event giteaWebhookIssue) error {
	issue, err := s.issueRepo.GetByGiteaIssueIndex(ctx, workspaceID, inst.ID, event.Issue.Number)
	if err != nil || issue == nil {
		return nil
	}

	issue.Status = domain.IssueStatusTodo
	if ts, err := s.statusRepo.GetByWorkspaceAndSlug(ctx, workspaceID, string(domain.IssueStatusTodo)); err == nil && ts != nil {
		issue.StatusID = &ts.ID
	}

	if err := s.issueRepo.Update(ctx, issue); err != nil {
		log.WithError(err).WithField("issue_id", issue.ID).Warn("failed to sync issue reopen from Gitea")
		return nil
	}

	s.broadcastRealtimeEvent(workspaceID, realtime.Event{
		Type:    "issue.updated",
		Payload: issue,
	})
	s.broadcastAppRefresh(workspaceID, "issues")
	// Keep the Gitea label equivalent to the status the reopen produced.
	s.syncStatusLabel(ctx, issue)
	return nil
}

// --- Comment Webhook Processing ---

// giteaWebhookIssueComment mirrors the Gitea issue_comment webhook payload.
type giteaWebhookIssueComment struct {
	Action  string `json:"action"`
	Comment struct {
		ID     int64  `json:"id"`
		Body   string `json:"body"`
		User   struct {
			Login     string `json:"login"`
			AvatarURL string `json:"avatar_url"`
		} `json:"user"`
		CreatedAt *time.Time `json:"created_at"`
		HTMLURL   string     `json:"html_url"`
	} `json:"comment"`
	Issue struct {
		Number int64 `json:"number"`
	} `json:"issue"`
	Repository struct {
		ID       int64  `json:"id"`
		FullName string `json:"full_name"`
	} `json:"repository"`
	Sender struct {
		Login string `json:"login"`
	} `json:"sender"`
}

// processIssueCommentEvent keeps comments created, edited or deleted in Gitea
// in sync with the local copy.
func (s *GiteaService) processIssueCommentEvent(ctx context.Context, workspaceID uuid.UUID, payload []byte) error {
	var event giteaWebhookIssueComment
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("unmarshaling issue_comment event: %w", err)
	}
	if s.commentSvc == nil || s.commentRepo == nil {
		return nil
	}

	inst, err := s.gtRepo.GetInstanceByWorkspace(ctx, workspaceID)
	if err != nil || inst == nil {
		return nil
	}

	issue, err := s.issueRepo.GetByGiteaIssueIndex(ctx, workspaceID, inst.ID, event.Issue.Number)
	if err != nil || issue == nil {
		return nil
	}

	// Flag context to prevent infinite sync loops
	ctx = SyncingFromGitea(ctx)

	switch event.Action {
	case "created":
		// Idempotency: ignore comments we already imported.
		existing, err := s.commentRepo.GetByGiteaCommentID(ctx, event.Comment.ID)
		if err != nil || existing != nil {
			return nil
		}

		var userID *uuid.UUID
		if s.userRepo != nil && event.Comment.User.Login != "" {
			if user, err := s.userRepo.GetWorkspaceMemberByGiteaLogin(ctx, workspaceID, event.Comment.User.Login); err == nil && user != nil {
				id := user.ID
				userID = &id
			}
		}

		// The Gitea author may have no Kuayle account: keep the login and
		// avatar so the UI can still render it.
		login := event.Comment.User.Login
		avatarURL := event.Comment.User.AvatarURL
		giteaCommentID := event.Comment.ID

		comment := &domain.Comment{
			ID:              uuid.New(),
			IssueID:         issue.ID,
			UserID:          userID,
			AuthorLogin:     &login,
			GiteaCommentID:  &giteaCommentID,
			Body:            event.Comment.Body,
		}
		if avatarURL != "" {
			comment.AuthorAvatarURL = &avatarURL
		}
		return s.commentSvc.CreateFromGitea(ctx, issue, comment)

	case "edited":
		comment, err := s.commentRepo.GetByGiteaCommentID(ctx, event.Comment.ID)
		if err != nil || comment == nil {
			return nil
		}
		return s.commentSvc.UpdateFromGitea(ctx, comment, event.Comment.Body)

	case "deleted":
		comment, err := s.commentRepo.GetByGiteaCommentID(ctx, event.Comment.ID)
		if err != nil || comment == nil {
			return nil
		}
		return s.commentSvc.DeleteFromGitea(ctx, comment)

	default:
		return nil
	}
}

// --- Gitea Sync Helpers ---

// getClientForWorkspace returns a Gitea API client for the given workspace.
func (s *GiteaService) getClientForWorkspace(ctx context.Context, workspaceID uuid.UUID) (*gt.Client, *domain.GiteaInstance, error) {
	inst, err := s.gtRepo.GetInstanceByWorkspace(ctx, workspaceID)
	if err != nil || inst == nil {
		return nil, nil, fmt.Errorf("Gitea not connected")
	}

	token, err := s.decryptToken(inst.AccessToken)
	if err != nil {
		return nil, nil, err
	}

	client := gt.NewClient(inst.InstanceURL, token)
	return client, inst, nil
}

// getRepoForIssue finds the linked Gitea repo associated with an issue's workspace.
func (s *GiteaService) getRepoForIssue(ctx context.Context, issue *domain.Issue) (*domain.GiteaRepoModel, *domain.GiteaInstance, error) {
	if issue.GiteaInstanceID == nil {
		return nil, nil, fmt.Errorf("issue not linked to a Gitea instance")
	}

	repos, err := s.gtRepo.ListReposByWorkspace(ctx, issue.WorkspaceID)
	if err != nil || len(repos) == 0 {
		return nil, nil, fmt.Errorf("no linked repos found")
	}

	inst, err := s.gtRepo.GetInstanceByWorkspace(ctx, issue.WorkspaceID)
	if err != nil || inst == nil {
		return nil, nil, fmt.Errorf("Gitea not connected")
	}

	// Return the first active linked repo
	for _, repo := range repos {
		if repo.IsActive {
			return &repo, inst, nil
		}
	}
	return &repos[0], inst, nil
}

// SyncIssueToGitea creates or updates an issue in Gitea when it is created or
// updated in Kuayle. This should only be called when NOT processing a Gitea
// webhook (i.e., when the context is NOT flagged with SyncingFromGitea).
func (s *GiteaService) SyncIssueToGitea(ctx context.Context, issue *domain.Issue) error {
	if IsSyncingFromGitea(ctx) {
		return nil
	}

	client, _, err := s.getClientForWorkspace(ctx, issue.WorkspaceID)
	if err != nil {
		return nil // silently skip if Gitea is not connected
	}

	// Find the linked repo to use
	repos, err := s.gtRepo.ListReposByWorkspace(ctx, issue.WorkspaceID)
	if err != nil || len(repos) == 0 {
		return nil
	}
	repo := repos[0]
	owner := strings.SplitN(repo.FullName, "/", 2)
	if len(owner) != 2 {
		return nil
	}

	body := ""
	if issue.Description != nil {
		body = *issue.Description
	}

	if issue.GiteaIssueIndex != nil {
		// Update existing Gitea issue
		state := ""
		switch issue.Status {
		case domain.IssueStatusDone:
			state = "closed"
		}
		giteaIssue, err := client.EditIssue(owner[0], owner[1], int(*issue.GiteaIssueIndex), issue.Title, body, state)
		if err != nil {
			log.WithError(err).WithField("issue_id", issue.ID).Warn("failed to update issue in Gitea")
			return nil
		}
		log.WithField("gitea_issue_number", giteaIssue.Index).Info("updated issue in Gitea")
		return nil
	}

	// Create new Gitea issue
	giteaIssue, err := client.CreateIssue(owner[0], owner[1], issue.Title, body)
	if err != nil {
		log.WithError(err).WithField("issue_id", issue.ID).Warn("failed to create issue in Gitea")
		return nil
	}

	// Store the Gitea issue index and instance ID on the Kuayle issue
	issue.GiteaIssueIndex = &giteaIssue.Index
	instID := repos[0].InstanceID
	issue.GiteaInstanceID = &instID
	if err := s.issueRepo.Update(ctx, issue); err != nil {
		log.WithError(err).WithField("issue_id", issue.ID).Warn("failed to store Gitea issue index")
	}

	log.WithField("gitea_issue_number", giteaIssue.Index).Info("created issue in Gitea")
	return nil
}

// SyncIssueCloseToGitea closes an issue in Gitea when it is closed in Kuayle.
func (s *GiteaService) SyncIssueCloseToGitea(ctx context.Context, issue *domain.Issue) error {
	if IsSyncingFromGitea(ctx) {
		return nil
	}
	if issue.GiteaIssueIndex == nil {
		return nil
	}

	client, _, err := s.getClientForWorkspace(ctx, issue.WorkspaceID)
	if err != nil {
		return nil
	}

	repos, err := s.gtRepo.ListReposByWorkspace(ctx, issue.WorkspaceID)
	if err != nil || len(repos) == 0 {
		return nil
	}
	repo := repos[0]
	owner := strings.SplitN(repo.FullName, "/", 2)
	if len(owner) != 2 {
		return nil
	}

	if _, err := client.CloseIssue(owner[0], owner[1], int(*issue.GiteaIssueIndex)); err != nil {
		log.WithError(err).WithField("issue_id", issue.ID).Warn("failed to close issue in Gitea")
	}
	return nil
}

// SyncIssueReopenToGitea reopens an issue in Gitea when it is reopened in Kuayle.
func (s *GiteaService) SyncIssueReopenToGitea(ctx context.Context, issue *domain.Issue) error {
	if IsSyncingFromGitea(ctx) {
		return nil
	}
	if issue.GiteaIssueIndex == nil {
		return nil
	}

	client, _, err := s.getClientForWorkspace(ctx, issue.WorkspaceID)
	if err != nil {
		return nil
	}

	repos, err := s.gtRepo.ListReposByWorkspace(ctx, issue.WorkspaceID)
	if err != nil || len(repos) == 0 {
		return nil
	}
	repo := repos[0]
	owner := strings.SplitN(repo.FullName, "/", 2)
	if len(owner) != 2 {
		return nil
	}

	if _, err := client.ReopenIssue(owner[0], owner[1], int(*issue.GiteaIssueIndex)); err != nil {
		log.WithError(err).WithField("issue_id", issue.ID).Warn("failed to reopen issue in Gitea")
	}
	return nil
}

// SyncIssueStatusLabelsToGitea mirrors the issue status in Kuayle as an
// exclusive label on the Gitea issue, keeping statuses and Gitea labels
// equivalent in both directions. It is idempotent and only writes when the
// remote label set differs, which is what keeps webhook round-trips from
// looping (our own label writes come back as issue_label events that find
// nothing left to change).
func (s *GiteaService) SyncIssueStatusLabelsToGitea(ctx context.Context, issue *domain.Issue) error {
	if issue == nil || issue.GiteaIssueIndex == nil {
		return nil
	}
	statuses, err := s.statusRepo.ListByWorkspace(ctx, issue.WorkspaceID)
	if err != nil || len(statuses) == 0 {
		return nil
	}
	current := currentStatusForIssue(issue, statuses)
	if current == nil {
		return nil
	}
	repo, inst, err := s.getRepoForIssue(ctx, issue)
	if err != nil {
		return nil // not linked to Gitea
	}
	owner := strings.SplitN(repo.FullName, "/", 2)
	if len(owner) != 2 {
		return nil
	}
	token, err := s.decryptToken(inst.AccessToken)
	if err != nil {
		return nil
	}
	client := gt.NewClient(inst.InstanceURL, token)
	issueIndex := int(*issue.GiteaIssueIndex)

	// Every workspace status name/slug counts as a "status label"; anything
	// else on the issue (e.g. "bug") is left untouched.
	statusLabels := make(map[string]bool, len(statuses)*2)
	for i := range statuses {
		statusLabels[strings.ToLower(statuses[i].Name)] = true
		statusLabels[strings.ToLower(statuses[i].Slug)] = true
	}

	target, err := ensureStatusLabel(client, owner[0], owner[1], current)
	if err != nil {
		return err
	}

	remote, err := client.GetIssueLabels(owner[0], owner[1], issueIndex)
	if err != nil {
		return fmt.Errorf("listing Gitea issue labels: %w", err)
	}
	hasCurrent := false
	for _, label := range remote {
		if label.ID == target.ID {
			hasCurrent = true
			continue
		}
		if statusLabels[strings.ToLower(label.Name)] {
			if err := client.RemoveIssueLabel(owner[0], owner[1], int64(issueIndex), label.ID); err != nil {
				log.WithError(err).WithField("issue_id", issue.ID).WithField("label", label.Name).
					Warn("failed to remove stale status label from Gitea issue")
			}
		}
	}
	if !hasCurrent {
		if err := client.AddIssueLabels(owner[0], owner[1], issueIndex, []int64{target.ID}); err != nil {
			return fmt.Errorf("adding status label to Gitea issue: %w", err)
		}
	}
	return nil
}

// syncStatusLabel runs the status-label sync and only logs failures, so it can
// be called from paths where a label hiccup must not break the status update.
func (s *GiteaService) syncStatusLabel(ctx context.Context, issue *domain.Issue) {
	if err := s.SyncIssueStatusLabelsToGitea(ctx, issue); err != nil {
		log.WithError(err).WithField("issue_id", issue.ID).Warn("failed to sync status label to Gitea")
	}
}

// currentStatusForIssue resolves the workspace status the issue is on,
// preferring the status_id column and falling back to the legacy slug field.
func currentStatusForIssue(issue *domain.Issue, statuses []domain.WorkspaceStatus) *domain.WorkspaceStatus {
	if issue.StatusID != nil {
		for i := range statuses {
			if statuses[i].ID == *issue.StatusID {
				return &statuses[i]
			}
		}
	}
	for i := range statuses {
		if statuses[i].Slug == string(issue.Status) {
			return &statuses[i]
		}
	}
	return nil
}

// ensureStatusLabel returns the repo label equivalent to the given status,
// creating it when it does not exist yet.
func ensureStatusLabel(client *gt.Client, owner, repo string, status *domain.WorkspaceStatus) (*gt.Label, error) {
	labels, err := client.ListLabels(owner, repo)
	if err != nil {
		return nil, fmt.Errorf("listing Gitea repo labels: %w", err)
	}
	for i := range labels {
		if strings.EqualFold(labels[i].Name, status.Name) {
			return &labels[i], nil
		}
	}
	created, err := client.CreateLabel(owner, repo, status.Name, giteaLabelColor(status.Color), "Kuayle status")
	if err != nil {
		return nil, fmt.Errorf("creating Gitea status label: %w", err)
	}
	return created, nil
}

// giteaLabelColor converts a Kuayle status color ("#ef4444") to the plain
// six-hex-digit form Gitea expects, falling back to its default gray.
func giteaLabelColor(color *string) string {
	if color != nil {
		c := strings.TrimPrefix(strings.TrimSpace(*color), "#")
		if _, err := strconv.ParseUint(c, 16, 32); err == nil && len(c) == 6 {
			return c
		}
	}
	return "ededed"
}

// SyncCommentToGitea creates a comment in Gitea when a comment is created in
// Kuayle, storing the returned Gitea comment ID so it can be edited or deleted
// later. The comment is posted with the author's own Gitea token so it appears
// on their behalf instead of the workspace integration account. Like the issue
// sync, it is a no-op while processing a Gitea webhook.
func (s *GiteaService) SyncCommentToGitea(ctx context.Context, issue *domain.Issue, comment *domain.Comment) error {
	if IsSyncingFromGitea(ctx) {
		return nil
	}
	if issue == nil || issue.GiteaIssueIndex == nil {
		return nil
	}

	repo, inst, err := s.getRepoForIssue(ctx, issue)
	if err != nil {
		return nil // silently skip when the workspace is not linked
	}
	owner := strings.SplitN(repo.FullName, "/", 2)
	if len(owner) != 2 {
		return nil
	}
	token, err := s.authorToken(ctx, comment)
	if err != nil {
		log.WithError(err).WithField("comment_id", comment.ID).Warn("comment author has no usable Gitea token; skipping Gitea sync")
		return nil
	}

	client := gt.NewClient(inst.InstanceURL, token)
	giteaComment, err := client.CreateIssueComment(owner[0], owner[1], int(*issue.GiteaIssueIndex), comment.Body)
	if err != nil {
		log.WithError(err).WithField("comment_id", comment.ID).Warn("failed to create comment in Gitea")
		return nil
	}

	giteaCommentID := giteaComment.ID
	comment.GiteaCommentID = &giteaCommentID
	if s.commentRepo != nil {
		if err := s.commentRepo.SetGiteaCommentID(ctx, comment.ID, giteaCommentID); err != nil {
			log.WithError(err).WithField("comment_id", comment.ID).Warn("failed to store Gitea comment ID")
		}
	}
	log.WithField("gitea_comment_id", giteaCommentID).Info("created comment in Gitea")
	return nil
}

// authorToken decrypts the Gitea token of the comment author so outgoing
// comments are posted as them.
func (s *GiteaService) authorToken(ctx context.Context, comment *domain.Comment) (string, error) {
	if comment == nil || comment.UserID == nil {
		return "", fmt.Errorf("comment has no local author")
	}
	user, err := s.userRepo.GetByID(ctx, *comment.UserID)
	if err != nil {
		return "", err
	}
	if user == nil || user.GiteaToken == nil || strings.TrimSpace(*user.GiteaToken) == "" {
		return "", fmt.Errorf("user %s has no Gitea token", comment.UserID)
	}
	return s.decryptToken(*user.GiteaToken)
}

// SyncCommentDeleteToGitea deletes a comment in Gitea when it is deleted in
// Kuayle. It only acts on comments that were mirrored to Gitea.
func (s *GiteaService) SyncCommentDeleteToGitea(ctx context.Context, issue *domain.Issue, comment *domain.Comment) error {
	if IsSyncingFromGitea(ctx) {
		return nil
	}
	if issue == nil || issue.GiteaIssueIndex == nil {
		return nil
	}
	if comment == nil || comment.GiteaCommentID == nil {
		return nil
	}

	repo, inst, err := s.getRepoForIssue(ctx, issue)
	if err != nil {
		return nil
	}
	owner := strings.SplitN(repo.FullName, "/", 2)
	if len(owner) != 2 {
		return nil
	}
	token, err := s.authorToken(ctx, comment)
	if err != nil {
		log.WithError(err).WithField("comment_id", comment.ID).Warn("comment author has no usable Gitea token; skipping Gitea delete")
		return nil
	}

	client := gt.NewClient(inst.InstanceURL, token)
	if err := client.DeleteIssueComment(owner[0], owner[1], *comment.GiteaCommentID); err != nil {
		log.WithError(err).WithField("comment_id", comment.ID).Warn("failed to delete comment in Gitea")
	}
	return nil
}

// --- Issue Linking ---

func (s *GiteaService) resolveIssueFromRef(ctx context.Context, workspaceID uuid.UUID, texts ...string) *domain.Issue {
	seen := make(map[string]bool)
	for _, text := range texts {
		matches := issueIdentifierRegex.FindAllString(text, -1)
		for _, m := range matches {
			if seen[m] {
				continue
			}
			seen[m] = true
			issue, err := s.issueRepo.GetByIdentifier(ctx, workspaceID, strings.ToUpper(m))
			if err == nil && issue != nil {
				return issue
			}
		}
	}
	return nil
}

// --- Auto-Transitions ---

func (s *GiteaService) applyAutoTransition(ctx context.Context, workspaceID uuid.UUID, event string, issue *domain.Issue) {
	rule, err := s.gtRepo.GetAutoTransitionByEvent(ctx, workspaceID, event)
	if err != nil || rule == nil {
		return
	}

	oldStatus := string(issue.Status)
	newStatus := rule.TargetStatus
	if oldStatus == newStatus {
		return
	}

	issue.Status = domain.IssueStatus(newStatus)
	if rule.TargetStatusID != nil {
		issue.StatusID = rule.TargetStatusID
	} else {
		if ts, err := s.statusRepo.GetByWorkspaceAndSlug(ctx, workspaceID, newStatus); err == nil && ts != nil {
			issue.StatusID = &ts.ID
		}
	}

	if err := s.issueRepo.Update(ctx, issue); err != nil {
		log.WithError(err).WithField("issue_id", issue.ID).Warn("failed to auto-transition issue")
		return
	}

	_ = s.historyRepo.Create(ctx, issue.ID, uuid.Nil, "status", &oldStatus, &newStatus)

	s.broadcastRealtimeEvent(workspaceID, realtime.Event{
		Type:    "issue.updated",
		Payload: issue,
	})
	s.syncStatusLabel(ctx, issue)
	s.applyStatusAutomation(ctx, workspaceID, issue, map[uuid.UUID]bool{})
}

func (s *GiteaService) applyStatusAutomation(ctx context.Context, workspaceID uuid.UUID, issue *domain.Issue, visited map[uuid.UUID]bool) {
	if issue == nil || visited[issue.ID] {
		return
	}
	visited[issue.ID] = true

	// Sub-issue auto-close used to be gated by a per-team setting; with teams
	// removed it now applies to every completed issue.
	category := s.issueStatusCategory(ctx, issue)
	if category == domain.StatusCategoryCompleted {
		s.autoCloseSubIssues(ctx, workspaceID, issue.ID, visited)
	}
	if issue.ParentID != nil {
		s.maybeAutoCloseParent(ctx, workspaceID, *issue.ParentID, visited)
	}
}

func (s *GiteaService) maybeAutoCloseParent(ctx context.Context, workspaceID, parentID uuid.UUID, visited map[uuid.UUID]bool) {
	parent, err := s.issueRepo.GetByID(ctx, parentID)
	if err != nil || parent == nil || parent.WorkspaceID != workspaceID || visited[parent.ID] {
		return
	}
	total, done, err := s.issueRepo.CountSubIssues(ctx, parent.ID)
	if err != nil || total == 0 || total != done {
		return
	}
	s.moveIssueToCompleted(ctx, workspaceID, parent, visited)
}

func (s *GiteaService) autoCloseSubIssues(ctx context.Context, workspaceID, parentID uuid.UUID, visited map[uuid.UUID]bool) {
	subIssues, err := s.issueRepo.ListSubIssues(ctx, parentID)
	if err != nil {
		return
	}
	for i := range subIssues {
		sub := subIssues[i]
		if sub.WorkspaceID != workspaceID || s.isTerminalStatus(ctx, &sub) {
			continue
		}
		s.moveIssueToCompleted(ctx, workspaceID, &sub, visited)
	}
}

func (s *GiteaService) moveIssueToCompleted(ctx context.Context, workspaceID uuid.UUID, issue *domain.Issue, visited map[uuid.UUID]bool) {
	if issue == nil || s.isTerminalStatus(ctx, issue) {
		return
	}
	completedStatus, err := s.completedStatus(ctx, workspaceID)
	if err != nil || completedStatus == nil {
		return
	}

	old := string(issue.Status)
	issue.Status = domain.IssueStatus(completedStatus.Slug)
	issue.StatusID = &completedStatus.ID
	if err := s.issueRepo.Update(ctx, issue); err != nil {
		log.WithError(err).WithField("issue_id", issue.ID).Warn("failed to auto-close issue")
		return
	}
	newVal := completedStatus.Slug
	_ = s.historyRepo.Create(ctx, issue.ID, uuid.Nil, "status", &old, &newVal)
	s.broadcastRealtimeEvent(workspaceID, realtime.Event{Type: "issue.updated", Payload: issue})
	s.syncStatusLabel(ctx, issue)
	s.applyStatusAutomation(ctx, workspaceID, issue, visited)
}

// completedStatus returns the status used to close issues for a workspace: the
// "done" status, or the first status whose category is "completed".
func (s *GiteaService) completedStatus(ctx context.Context, workspaceID uuid.UUID) (*domain.WorkspaceStatus, error) {
	status, err := s.statusRepo.GetByWorkspaceAndSlug(ctx, workspaceID, string(domain.IssueStatusDone))
	if err == nil && status != nil {
		return status, nil
	}
	statuses, err := s.statusRepo.ListByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	for i := range statuses {
		if statuses[i].Category == domain.StatusCategoryCompleted {
			return &statuses[i], nil
		}
	}
	return nil, nil
}

func (s *GiteaService) isTerminalStatus(ctx context.Context, issue *domain.Issue) bool {
	category := s.issueStatusCategory(ctx, issue)
	return category == domain.StatusCategoryCompleted || category == domain.StatusCategoryCancelled
}

func (s *GiteaService) issueStatusCategory(ctx context.Context, issue *domain.Issue) domain.StatusCategory {
	if issue.StatusID != nil {
		status, err := s.statusRepo.GetByID(ctx, *issue.StatusID)
		if err == nil && status != nil {
			return status.Category
		}
	}
	switch issue.Status {
	case domain.IssueStatusDone:
		return domain.StatusCategoryCompleted
	case domain.IssueStatusCancelled:
		return domain.StatusCategoryCancelled
	case domain.IssueStatusInProgress, domain.IssueStatusInReview:
		return domain.StatusCategoryStarted
	case domain.IssueStatusTodo:
		return domain.StatusCategoryUnstarted
	default:
		return domain.StatusCategoryBacklog
	}
}

// --- Data Access ---

func (s *GiteaService) GetIssueActivity(ctx context.Context, workspaceID uuid.UUID, identifier string) (*dto.GiteaIssueActivityResponse, error) {
	issue, err := s.issueRepo.GetByIdentifier(ctx, workspaceID, identifier)
	if err != nil || issue == nil {
		return nil, fmt.Errorf("issue not found")
	}

	prs, _ := s.gtRepo.ListPRsWithRepoByIssue(ctx, issue.ID)
	branches, _ := s.gtRepo.ListBranchesWithRepoByIssue(ctx, issue.ID)
	commits, _ := s.gtRepo.ListCommitsWithRepoByIssue(ctx, issue.ID)

	resp := &dto.GiteaIssueActivityResponse{
		PullRequests: make([]dto.GiteaPullRequestResponse, 0, len(prs)),
		Branches:     make([]dto.GiteaBranchResponse, 0, len(branches)),
		Commits:      make([]dto.GiteaCommitResponse, 0, len(commits)),
	}

	for _, pr := range prs {
		avatarURL, headBranch, baseBranch := "", "", ""
		if pr.AuthorAvatarURL != nil {
			avatarURL = *pr.AuthorAvatarURL
		}
		if pr.HeadBranch != nil {
			headBranch = *pr.HeadBranch
		}
		if pr.BaseBranch != nil {
			baseBranch = *pr.BaseBranch
		}
		resp.PullRequests = append(resp.PullRequests, dto.GiteaPullRequestResponse{
			ID: pr.ID.String(), Number: pr.Number, Title: pr.Title, State: pr.State,
			AuthorLogin: pr.AuthorLogin, AuthorAvatarURL: avatarURL, HTMLURL: pr.HTMLURL,
			HeadBranch: headBranch, BaseBranch: baseBranch,
			Additions: pr.Additions, Deletions: pr.Deletions,
			RepoFullName: pr.RepoFullName, MergedAt: pr.MergedAt,
			CreatedAt: pr.CreatedAt, UpdatedAt: pr.UpdatedAt,
		})
	}

	for _, b := range branches {
		htmlURL := ""
		if b.HTMLURL != nil {
			htmlURL = *b.HTMLURL
		}
		resp.Branches = append(resp.Branches, dto.GiteaBranchResponse{
			ID: b.ID.String(), Name: b.Name, HTMLURL: htmlURL, RepoFullName: b.RepoFullName,
		})
	}

	for _, c := range commits {
		authorLogin, authorAvatar := "", ""
		if c.AuthorLogin != nil {
			authorLogin = *c.AuthorLogin
		}
		if c.AuthorAvatarURL != nil {
			authorAvatar = *c.AuthorAvatarURL
		}
		shortSHA := c.SHA
		if len(shortSHA) > 7 {
			shortSHA = shortSHA[:7]
		}
		resp.Commits = append(resp.Commits, dto.GiteaCommitResponse{
			ID: c.ID.String(), SHA: c.SHA, ShortSHA: shortSHA, Message: c.Message,
			AuthorLogin: authorLogin, AuthorAvatarURL: authorAvatar,
			HTMLURL: c.HTMLURL, RepoFullName: c.RepoFullName, CommittedAt: c.CommittedAt,
		})
	}

	return resp, nil
}

// --- Auto Transition Management ---

func (s *GiteaService) ListAutoTransitions(ctx context.Context, workspaceID uuid.UUID) ([]dto.GiteaAutoTransitionResponse, error) {
	transitions, err := s.gtRepo.ListAutoTransitions(ctx, workspaceID)
	if err != nil {
		return nil, err
	}

	var result []dto.GiteaAutoTransitionResponse
	for _, t := range transitions {
		var statusID *string
		if t.TargetStatusID != nil {
			sid := t.TargetStatusID.String()
			statusID = &sid
		}
		result = append(result, dto.GiteaAutoTransitionResponse{
			Event: t.Event, TargetStatus: t.TargetStatus, TargetStatusID: statusID, IsActive: t.IsActive,
		})
	}
	return result, nil
}

func (s *GiteaService) UpdateAutoTransitions(ctx context.Context, workspaceID uuid.UUID, req dto.UpdateAutoTransitionsRequest) error {
	for _, rule := range req.Transitions {
		var statusID *uuid.UUID
		if rule.TargetStatusID != nil {
			parsed, err := uuid.Parse(*rule.TargetStatusID)
			if err == nil {
				statusID = &parsed
			}
		}
		t := &domain.GiteaAutoTransition{
			ID: uuid.New(), WorkspaceID: workspaceID,
			Event: rule.Event, TargetStatus: rule.TargetStatus, TargetStatusID: statusID, IsActive: rule.IsActive,
		}
		if err := s.gtRepo.UpsertAutoTransition(ctx, t); err != nil {
			return err
		}
	}
	return nil
}

// --- Helpers ---

func (s *GiteaService) decryptToken(encToken string) (string, error) {
	return crypto.Decrypt(encToken, s.encryptionKey)
}

// findOrCreateProjectForRepo returns the workspace project whose name matches
// the Gitea repository full name, creating it when it does not exist yet.
func (s *GiteaService) findOrCreateProjectForRepo(ctx context.Context, workspaceID uuid.UUID, fullName string) (*domain.Project, error) {
	projects, err := s.projectRepo.ListByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	for i := range projects {
		if projects[i].Name == fullName {
			return &projects[i], nil
		}
	}

	project := &domain.Project{
		ID:          uuid.New(),
		WorkspaceID: workspaceID,
		Name:        fullName,
		Status:      domain.ProjectStatusPlanned,
	}
	if err := s.projectRepo.Create(ctx, project); err != nil {
		return nil, err
	}
	return project, nil
}

// ResolveWorkspaceFromPayload extracts the repository ID from a webhook payload
// and finds the workspace it belongs to.
func (s *GiteaService) ResolveWorkspaceFromPayload(ctx context.Context, payload []byte) (uuid.UUID, error) {
	var partial struct {
		Repository struct {
			ID int64 `json:"id"`
		} `json:"repository"`
	}
	if err := json.Unmarshal(payload, &partial); err != nil || partial.Repository.ID == 0 {
		return uuid.Nil, fmt.Errorf("could not extract repository ID from payload")
	}

	repo, err := s.gtRepo.GetRepoByGiteaIDGlobal(ctx, partial.Repository.ID)
	if err != nil || repo == nil {
		return uuid.Nil, fmt.Errorf("repo not linked")
	}

	return repo.WorkspaceID, nil
}
