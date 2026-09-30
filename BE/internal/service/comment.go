package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/kuayle/kuayle-backend/internal/domain"
	"github.com/kuayle/kuayle-backend/internal/dto"
	"github.com/kuayle/kuayle-backend/internal/realtime"
	"github.com/kuayle/kuayle-backend/internal/repository"
	"github.com/kuayle/kuayle-backend/pkg/sanitize"
	log "github.com/sirupsen/logrus"
)

var (
	// ErrGiteaTokenRequired is returned when a user tries to comment in a
	// workspace connected to Gitea without a linked Gitea token.
	ErrGiteaTokenRequired = errors.New("add your Gitea token in your profile to comment in this workspace")
	// ErrInvalidGiteaToken is returned when a user token fails verification.
	ErrInvalidGiteaToken = errors.New("the Gitea token was rejected by the instance")
)

// giteaCommentSync is the subset of GiteaService the comment flow needs. Using
// a narrow interface keeps the initialization order flexible (it is injected
// through SetGiteaService) and lets tests fake the integration.
type giteaCommentSync interface {
	HasGiteaInstance(ctx context.Context, workspaceID uuid.UUID) (bool, error)
	SyncCommentToGitea(ctx context.Context, issue *domain.Issue, comment *domain.Comment) error
}

// mentionSpanRegex matches <span ...> tags that contain data-type="mention" and captures data-id UUID.
// Uses two patterns to handle either attribute order.
var mentionSpanRegex1 = regexp.MustCompile(`<span[^>]*data-type="mention"[^>]*data-id="([0-9a-f-]{36})"[^>]*>`)
var mentionSpanRegex2 = regexp.MustCompile(`<span[^>]*data-id="([0-9a-f-]{36})"[^>]*data-type="mention"[^>]*>`)

// extractMentionedUserIDs parses mention spans from sanitized HTML and returns unique user UUIDs.
func extractMentionedUserIDs(html string) []uuid.UUID {
	seen := make(map[uuid.UUID]bool)
	var result []uuid.UUID
	for _, re := range []*regexp.Regexp{mentionSpanRegex1, mentionSpanRegex2} {
		matches := re.FindAllStringSubmatch(html, -1)
		for _, m := range matches {
			if uid, err := uuid.Parse(m[1]); err == nil && !seen[uid] {
				seen[uid] = true
				result = append(result, uid)
			}
		}
	}
	return result
}

type CommentService struct {
	commentRepo repository.CommentRepo
	issueRepo   repository.IssueRepo
	userRepo    repository.UserRepo
	hub         *realtime.Hub
	notifSvc    *NotificationService
	gitea       giteaCommentSync
}

func NewCommentService(commentRepo repository.CommentRepo, issueRepo repository.IssueRepo, userRepo repository.UserRepo, hub *realtime.Hub, notifSvc *NotificationService) *CommentService {
	return &CommentService{commentRepo: commentRepo, issueRepo: issueRepo, userRepo: userRepo, hub: hub, notifSvc: notifSvc}
}

// SetGiteaService injects the Gitea integration. It uses a setter to avoid an
// initialization cycle between CommentService and GiteaService.
func (s *CommentService) SetGiteaService(gitea giteaCommentSync) {
	s.gitea = gitea
}

// Create persists a comment written in Kuayle and mirrors it to Gitea when the
// issue is linked to a Gitea issue.
func (s *CommentService) Create(ctx context.Context, workspaceID, issueID, userID uuid.UUID, req dto.CreateCommentRequest) (*domain.Comment, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	// In workspaces connected to Gitea the commenter must have linked their own
	// Gitea token, otherwise the comment could not be posted on their behalf.
	if s.gitea != nil {
		connected, err := s.gitea.HasGiteaInstance(ctx, workspaceID)
		if err != nil {
			log.WithError(err).WithField("workspace_id", workspaceID).Warn("failed to check Gitea instance before commenting")
		} else if connected && (user == nil || user.GiteaToken == nil || strings.TrimSpace(*user.GiteaToken) == "") {
			return nil, ErrGiteaTokenRequired
		}
	}

	comment := &domain.Comment{
		ID:      uuid.New(),
		IssueID: issueID,
		UserID:  &userID,
		Body:    sanitize.SanitizeHTML(req.Body),
	}
	// Keep the author around as a fallback so the UI can render something even
	// when the user record is not available.
	if user != nil {
		comment.AuthorLogin = user.GiteaLogin
		comment.AuthorAvatarURL = user.AvatarURL
	}
	if req.ParentID != nil {
		pid, err := uuid.Parse(*req.ParentID)
		if err != nil {
			return nil, err
		}
		comment.ParentID = &pid
	}
	if err := s.commentRepo.Create(ctx, comment); err != nil {
		return nil, err
	}

	// Broadcast and notify
	issue, _ := s.issueRepo.GetByID(ctx, issueID)
	s.notifyCreated(ctx, issue, comment)

	// Mirror the comment to Gitea when the issue lives there too.
	if s.gitea != nil && issue != nil && issue.GiteaIssueIndex != nil {
		if err := s.gitea.SyncCommentToGitea(ctx, issue, comment); err != nil {
			log.WithError(err).WithField("comment_id", comment.ID).Warn("failed to sync comment to Gitea")
		}
	}

	return comment, nil
}

// CreateFromGitea persists a comment created in Gitea. The comment is expected
// to carry its gitea_comment_id and a resolved author; it is sanitized,
// broadcast and used to notify participants, but never synced back to Gitea.
func (s *CommentService) CreateFromGitea(ctx context.Context, issue *domain.Issue, comment *domain.Comment) error {
	comment.Body = sanitize.SanitizeHTML(comment.Body)
	if err := s.commentRepo.Create(ctx, comment); err != nil {
		return err
	}
	s.notifyCreated(ctx, issue, comment)
	return nil
}

// UpdateFromGitea applies a body edited in Gitea and broadcasts the change.
func (s *CommentService) UpdateFromGitea(ctx context.Context, comment *domain.Comment, body string) error {
	comment.Body = sanitize.SanitizeHTML(body)
	if err := s.commentRepo.Update(ctx, comment.ID, comment.Body); err != nil {
		return err
	}
	s.broadcastCommentEvent(ctx, comment, "comment.updated")
	return nil
}

// DeleteFromGitea removes a comment deleted in Gitea and broadcasts the change.
func (s *CommentService) DeleteFromGitea(ctx context.Context, comment *domain.Comment) error {
	if err := s.commentRepo.Delete(ctx, comment.ID); err != nil {
		return err
	}
	s.broadcastCommentEvent(ctx, comment, "comment.deleted")
	return nil
}

// notifyCreated broadcasts comment.created and notifies everyone interested in
// the issue. Notification targeting is nil-safe: comments authored by Gitea
// users have no local user ID.
func (s *CommentService) notifyCreated(ctx context.Context, issue *domain.Issue, comment *domain.Comment) {
	if issue == nil {
		return
	}

	s.hub.Broadcast(issue.WorkspaceID, realtime.Event{
		Type: "comment.created",
		Payload: map[string]string{
			"issue_id":   comment.IssueID.String(),
			"comment_id": comment.ID.String(),
			"identifier": issue.Identifier,
		},
	})

	// Notify issue creator + assignees (except the commenter)
	recipients := make(map[uuid.UUID]bool)
	if comment.UserID == nil || issue.CreatorID != *comment.UserID {
		recipients[issue.CreatorID] = true
	}
	if issue.AssigneeID != nil && (comment.UserID == nil || *issue.AssigneeID != *comment.UserID) {
		recipients[*issue.AssigneeID] = true
	}
	assignees, _ := s.issueRepo.GetAssignees(ctx, issue.ID)
	for _, uid := range assignees {
		if comment.UserID == nil || uid != *comment.UserID {
			recipients[uid] = true
		}
	}
	if repo, ok := s.issueRepo.(issueSubscriberRepo); ok {
		subscribers, _ := repo.GetSubscribers(ctx, issue.ID)
		for _, uid := range subscribers {
			recipients[uid] = true
		}
	}

	title := fmt.Sprintf("New comment on %s: %s", issue.Identifier, issue.Title)
	for uid := range recipients {
		if err := s.notifSvc.Create(ctx, uid, issue.WorkspaceID, &issue.ID, "commented", title); err != nil {
			log.WithError(err).Warn("failed to create comment notification")
			continue
		}
		s.hub.BroadcastToUser(issue.WorkspaceID, uid, realtime.Event{
			Type:    "notification.created",
			Payload: map[string]string{"type": "commented"},
		})
	}

	// Notify @mentioned users (skip commenter and already-notified recipients)
	mentionedIDs := extractMentionedUserIDs(comment.Body)
	mentionTitle := fmt.Sprintf("You were mentioned in %s: %s", issue.Identifier, issue.Title)
	for _, uid := range mentionedIDs {
		if (comment.UserID != nil && uid == *comment.UserID) || recipients[uid] {
			continue
		}
		if err := s.notifSvc.Create(ctx, uid, issue.WorkspaceID, &issue.ID, "mentioned", mentionTitle); err != nil {
			log.WithError(err).Warn("failed to create mention notification")
			continue
		}
		s.hub.BroadcastToUser(issue.WorkspaceID, uid, realtime.Event{
			Type:    "notification.created",
			Payload: map[string]string{"type": "mentioned"},
		})
	}
}

// broadcastCommentEvent notifies the frontend that a comment changed or was
// removed outside of Kuayle (Gitea webhook).
func (s *CommentService) broadcastCommentEvent(ctx context.Context, comment *domain.Comment, eventType string) {
	issue, _ := s.issueRepo.GetByID(ctx, comment.IssueID)
	if issue == nil {
		return
	}
	s.hub.Broadcast(issue.WorkspaceID, realtime.Event{
		Type: eventType,
		Payload: map[string]string{
			"issue_id":   comment.IssueID.String(),
			"comment_id": comment.ID.String(),
		},
	})
}

func (s *CommentService) ListByIssue(ctx context.Context, issueID uuid.UUID) ([]domain.Comment, error) {
	return s.commentRepo.ListByIssue(ctx, issueID)
}

func (s *CommentService) ListReplies(ctx context.Context, parentID uuid.UUID) ([]domain.Comment, error) {
	return s.commentRepo.ListReplies(ctx, parentID)
}

func (s *CommentService) GetByID(ctx context.Context, id uuid.UUID) (*domain.Comment, error) {
	return s.commentRepo.GetByID(ctx, id)
}

func (s *CommentService) Resolve(ctx context.Context, id uuid.UUID) error {
	return s.commentRepo.Resolve(ctx, id)
}

func (s *CommentService) Reopen(ctx context.Context, id uuid.UUID) error {
	return s.commentRepo.Reopen(ctx, id)
}
