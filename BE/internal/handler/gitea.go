package handler

import (
	"errors"
	"io"
	"net/http"

	"github.com/google/uuid"
	"github.com/kuayle/kuayle-backend/internal/domain"
	"github.com/kuayle/kuayle-backend/internal/dto"
	"github.com/kuayle/kuayle-backend/internal/service"
	"github.com/kuayle/kuayle-backend/pkg/response"
	"github.com/kuayle/kuayle-backend/pkg/validate"
	"github.com/labstack/echo/v4"
	log "github.com/sirupsen/logrus"
)

type GiteaHandler struct {
	gtSvc *service.GiteaService
}

func NewGiteaHandler(gtSvc *service.GiteaService) *GiteaHandler {
	return &GiteaHandler{gtSvc: gtSvc}
}

// Status returns the Gitea integration status for the workspace.
func (h *GiteaHandler) Status(c echo.Context) error {
	ws := c.Get("workspace").(*domain.Workspace)
	status, err := h.gtSvc.GetStatus(c.Request().Context(), ws.ID)
	if err != nil {
		return response.InternalError(c)
	}
	return response.Success(c, http.StatusOK, status)
}

// Connect verifies PAT and connects a Gitea instance to the workspace.
func (h *GiteaHandler) Connect(c echo.Context) error {
	ws := c.Get("workspace").(*domain.Workspace)
	userID := c.Get("user_id").(uuid.UUID)
	var req dto.GiteaConnectRequest
	if err := c.Bind(&req); err != nil {
		return response.Error(c, http.StatusBadRequest, "BAD_REQUEST", "Invalid request body")
	}
	inst, err := h.gtSvc.Connect(c.Request().Context(), ws.ID, userID, req)
	if err != nil {
		return response.Error(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
	}
	return response.Success(c, http.StatusOK, map[string]any{
		"id":            inst.ID,
		"account_login": inst.AccountLogin,
	})
}

// Disconnect removes the Gitea instance from the workspace.
func (h *GiteaHandler) Disconnect(c echo.Context) error {
	ws := c.Get("workspace").(*domain.Workspace)
	if err := h.gtSvc.Disconnect(c.Request().Context(), ws.ID); err != nil {
		return response.Error(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
	}
	return response.Success(c, http.StatusOK, map[string]string{"status": "disconnected"})
}

// ListRepos lists available Gitea repos for the connected instance.
func (h *GiteaHandler) ListRepos(c echo.Context) error {
	ws := c.Get("workspace").(*domain.Workspace)
	repos, err := h.gtSvc.ListAvailableRepos(c.Request().Context(), ws.ID)
	if err != nil {
		return response.Error(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
	}
	if repos == nil {
		repos = []dto.GiteaAvailableRepoResponse{}
	}
	return response.Success(c, http.StatusOK, repos)
}

// LinkRepos links selected Gitea repos to the workspace.
func (h *GiteaHandler) LinkRepos(c echo.Context) error {
	ws := c.Get("workspace").(*domain.Workspace)
	var req dto.LinkGiteaReposRequest
	if err := c.Bind(&req); err != nil {
		return response.Error(c, http.StatusBadRequest, "BAD_REQUEST", "Invalid request body")
	}
	if err := h.gtSvc.LinkRepos(c.Request().Context(), ws.ID, req); err != nil {
		return response.Error(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
	}
	return response.Success(c, http.StatusOK, map[string]string{"status": "linked"})
}

// UnlinkRepo removes a linked repo.
func (h *GiteaHandler) UnlinkRepo(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return response.Error(c, http.StatusBadRequest, "BAD_REQUEST", "Invalid repo ID")
	}
	if err := h.gtSvc.UnlinkRepo(c.Request().Context(), id); err != nil {
		return response.Error(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
	}
	return response.Success(c, http.StatusOK, map[string]string{"status": "unlinked"})
}

// GetUserToken reports whether the caller has a Gitea token stored (the token
// itself is never returned).
func (h *GiteaHandler) GetUserToken(c echo.Context) error {
	ws := c.Get("workspace").(*domain.Workspace)
	userID := c.Get("user_id").(uuid.UUID)
	user, err := h.gtSvc.GetUserToken(c.Request().Context(), ws.ID, userID)
	if err != nil || user == nil {
		return response.InternalError(c)
	}
	return response.Success(c, http.StatusOK, map[string]any{
		"has_gitea_token": user.GiteaToken != nil && *user.GiteaToken != "",
		"gitea_login":     user.GiteaLogin,
	})
}

// SetUserToken stores (or clears, with an empty token) the caller's own Gitea
// PAT so comments are posted on their behalf.
func (h *GiteaHandler) SetUserToken(c echo.Context) error {
	ws := c.Get("workspace").(*domain.Workspace)
	userID := c.Get("user_id").(uuid.UUID)

	var req dto.GiteaUserTokenRequest
	if err := c.Bind(&req); err != nil {
		return response.Error(c, http.StatusBadRequest, "BAD_REQUEST", "Invalid request body")
	}
	if err := validate.Struct(&req); err != nil {
		return response.Error(c, http.StatusBadRequest, "BAD_REQUEST", "Token is too long")
	}

	user, err := h.gtSvc.SetUserToken(c.Request().Context(), ws.ID, userID, req.Token)
	if errors.Is(err, service.ErrInvalidGiteaToken) {
		return response.Error(c, http.StatusBadRequest, "INVALID_GITEA_TOKEN", err.Error())
	}
	if err != nil || user == nil {
		return response.InternalError(c)
	}
	return response.Success(c, http.StatusOK, map[string]any{
		"has_gitea_token": user.GiteaToken != nil && *user.GiteaToken != "",
		"gitea_login":     user.GiteaLogin,
	})
}

// ListAutoTransitions returns the auto-transition rules.
func (h *GiteaHandler) ListAutoTransitions(c echo.Context) error {
	ws := c.Get("workspace").(*domain.Workspace)
	transitions, err := h.gtSvc.ListAutoTransitions(c.Request().Context(), ws.ID)
	if err != nil {
		return response.InternalError(c)
	}
	if transitions == nil {
		transitions = []dto.GiteaAutoTransitionResponse{}
	}
	return response.Success(c, http.StatusOK, transitions)
}

// UpdateAutoTransitions updates the auto-transition rules.
func (h *GiteaHandler) UpdateAutoTransitions(c echo.Context) error {
	ws := c.Get("workspace").(*domain.Workspace)
	var req dto.UpdateAutoTransitionsRequest
	if err := c.Bind(&req); err != nil {
		return response.Error(c, http.StatusBadRequest, "BAD_REQUEST", "Invalid request body")
	}
	if err := h.gtSvc.UpdateAutoTransitions(c.Request().Context(), ws.ID, req); err != nil {
		return response.Error(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
	}
	return response.Success(c, http.StatusOK, map[string]string{"status": "updated"})
}

// IssueGiteaActivity returns Gitea PRs, branches, commits linked to an issue.
func (h *GiteaHandler) IssueGiteaActivity(c echo.Context) error {
	ws := c.Get("workspace").(*domain.Workspace)
	identifier := c.Param("identifier")
	activity, err := h.gtSvc.GetIssueActivity(c.Request().Context(), ws.ID, identifier)
	if err != nil {
		return response.Error(c, http.StatusNotFound, "NOT_FOUND", err.Error())
	}
	return response.Success(c, http.StatusOK, activity)
}

// AgentIssueLinks returns issue-PR/branch/commit links for agent consumption.
func (h *GiteaHandler) AgentIssueLinks(c echo.Context) error {
	ws := c.Get("workspace").(*domain.Workspace)
	identifier := c.QueryParam("identifier")
	if identifier == "" {
		return response.Error(c, http.StatusBadRequest, "BAD_REQUEST", "identifier query param required")
	}
	activity, err := h.gtSvc.GetIssueActivity(c.Request().Context(), ws.ID, identifier)
	if err != nil {
		return response.Error(c, http.StatusNotFound, "NOT_FOUND", err.Error())
	}
	return response.Success(c, http.StatusOK, activity)
}

// HandleWebhook receives incoming Gitea webhook events (public endpoint).
func (h *GiteaHandler) HandleWebhook(c echo.Context) error {
	signature := c.Request().Header.Get("X-Gitea-Signature")
	eventType := c.Request().Header.Get("X-Gitea-Event")

	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return c.NoContent(http.StatusBadRequest)
	}

	// Resolve workspace from the payload
	workspaceID, err := h.gtSvc.ResolveWorkspaceFromPayload(c.Request().Context(), body)
	if err != nil || workspaceID == uuid.Nil {
		return c.NoContent(http.StatusNotFound)
	}

	// Verify the signature against this workspace's webhook secret
	// (service skips verification when no secret is configured)
	if !h.gtSvc.VerifyWebhookSignature(c.Request().Context(), workspaceID, body, signature) {
		return c.NoContent(http.StatusUnauthorized)
	}

	if err := h.gtSvc.HandleWebhookEvent(c.Request().Context(), workspaceID, eventType, body); err != nil {
		log.WithError(err).WithField("workspace_id", workspaceID).WithField("event", eventType).Warn("gitea webhook processing failed")
		return c.NoContent(http.StatusInternalServerError)
	}

	return c.NoContent(http.StatusOK)
}
