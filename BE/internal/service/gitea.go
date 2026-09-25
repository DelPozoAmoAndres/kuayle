package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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

type GiteaService struct {
	gtRepo         *repository.GiteaRepository
	issueRepo      repository.IssueRepo
	teamRepo       repository.TeamRepo
	teamStatusRepo repository.TeamStatusRepo
	historyRepo    repository.IssueHistoryRepo
	encryptionKey  []byte
	hub            *realtime.Hub
	frontendURL    string
}

func NewGiteaService(
	gtRepo *repository.GiteaRepository,
	issueRepo repository.IssueRepo,
	teamRepo repository.TeamRepo,
	teamStatusRepo repository.TeamStatusRepo,
	historyRepo repository.IssueHistoryRepo,
	encryptionKey []byte,
	hub *realtime.Hub,
	frontendURL string,
) *GiteaService {
	return &GiteaService{
		gtRepo:         gtRepo,
		issueRepo:      issueRepo,
		teamRepo:       teamRepo,
		teamStatusRepo: teamStatusRepo,
		historyRepo:    historyRepo,
		encryptionKey:  encryptionKey,
		hub:            hub,
		frontendURL:    frontendURL,
	}
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
		}
	}
	return nil
}

// UnlinkRepo removes a linked repo.
func (s *GiteaService) UnlinkRepo(ctx context.Context, repoID uuid.UUID) error {
	return s.gtRepo.DeleteRepo(ctx, repoID)
}

// Disconnect removes the Gitea instance.
func (s *GiteaService) Disconnect(ctx context.Context, workspaceID uuid.UUID) error {
	_ = s.gtRepo.DeleteOAuthConfig(ctx, workspaceID)
	return s.gtRepo.DeleteInstance(ctx, workspaceID)
}

// --- Webhook Event Processing ---

// VerifyWebhookSignature verifies the Gitea webhook HMAC-SHA256 signature.
// Gitea uses the X-Gitea-Signature header with the hex-encoded HMAC digest.
func (s *GiteaService) VerifyWebhookSignature(ctx context.Context, workspaceID uuid.UUID, payload []byte, signature string) bool {
	oauthCfg, err := s.gtRepo.GetOAuthConfigByWorkspace(ctx, workspaceID)
	if err != nil || oauthCfg == nil {
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
		if issue.TeamID != nil {
			ts, err := s.teamStatusRepo.GetByTeamAndSlug(ctx, *issue.TeamID, newStatus)
			if err == nil && ts != nil {
				issue.StatusID = &ts.ID
			}
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
	s.applyStatusAutomation(ctx, workspaceID, issue, map[uuid.UUID]bool{})
}

func (s *GiteaService) applyStatusAutomation(ctx context.Context, workspaceID uuid.UUID, issue *domain.Issue, visited map[uuid.UUID]bool) {
	if issue == nil || visited[issue.ID] || s.teamRepo == nil {
		return
	}
	visited[issue.ID] = true

	category := s.issueStatusCategory(ctx, issue)
	if category == domain.StatusCategoryCompleted && issue.TeamID != nil {
		team, err := s.teamRepo.GetByID(ctx, *issue.TeamID)
		if err == nil && team != nil && team.SubIssueAutoCloseEnabled {
			s.autoCloseSubIssues(ctx, workspaceID, issue.ID, visited)
		}
	}
	if issue.ParentID != nil {
		s.maybeAutoCloseParent(ctx, workspaceID, *issue.ParentID, visited)
	}
}

func (s *GiteaService) maybeAutoCloseParent(ctx context.Context, workspaceID, parentID uuid.UUID, visited map[uuid.UUID]bool) {
	parent, err := s.issueRepo.GetByID(ctx, parentID)
	if err != nil || parent == nil || parent.WorkspaceID != workspaceID || visited[parent.ID] || s.teamRepo == nil {
		return
	}
	if parent.TeamID == nil {
		return
	}
	team, err := s.teamRepo.GetByID(ctx, *parent.TeamID)
	if err != nil || team == nil || !team.ParentAutoCloseEnabled {
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
	if issue == nil || issue.TeamID == nil || s.isTerminalStatus(ctx, issue) {
		return
	}
	completedStatus, err := s.completedStatusForTeam(ctx, *issue.TeamID)
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
	s.applyStatusAutomation(ctx, workspaceID, issue, visited)
}

func (s *GiteaService) completedStatusForTeam(ctx context.Context, teamID uuid.UUID) (*domain.TeamStatus, error) {
	status, err := s.teamStatusRepo.GetByTeamAndSlug(ctx, teamID, string(domain.IssueStatusDone))
	if err == nil && status != nil {
		return status, nil
	}
	statuses, err := s.teamStatusRepo.ListByTeam(ctx, teamID)
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
		status, err := s.teamStatusRepo.GetByID(ctx, *issue.StatusID)
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
