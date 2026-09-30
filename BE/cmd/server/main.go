package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
	"github.com/labstack/echo/v4"
	log "github.com/sirupsen/logrus"

	"github.com/kuayle/kuayle-backend/internal/agent"
	"github.com/kuayle/kuayle-backend/internal/config"
	"github.com/kuayle/kuayle-backend/internal/domain"
	"github.com/kuayle/kuayle-backend/internal/handler"
	mw "github.com/kuayle/kuayle-backend/internal/middleware"
	"github.com/kuayle/kuayle-backend/internal/realtime"
	"github.com/kuayle/kuayle-backend/internal/repository"
	"github.com/kuayle/kuayle-backend/internal/service"
	"github.com/kuayle/kuayle-backend/pkg/crypto"
	"github.com/kuayle/kuayle-backend/pkg/storage"
)

func main() {
	log.SetFormatter(&log.TextFormatter{FullTimestamp: true})

	// Handle CLI subcommands
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		runMigrate(os.Args[2:])
		return
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Database
	db, err := sqlx.Connect("pgx", cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)

	// Realtime hub
	hub := realtime.NewHub()

	// Repositories
	userRepo := repository.NewUserRepository(db)
	refreshRepo := repository.NewRefreshTokenRepository(db)
	workspaceRepo := repository.NewWorkspaceRepository(db)
	issueRepo := repository.NewIssueRepository(db)
	labelRepo := repository.NewLabelRepository(db)
	commentRepo := repository.NewCommentRepository(db)
	projectRepo := repository.NewProjectRepository(db)
	notifRepo := repository.NewNotificationRepository(db)
	historyRepo := repository.NewIssueHistoryRepository(db)
	relationRepo := repository.NewIssueRelationRepository(db)
	templateRepo := repository.NewIssueTemplateRepository(db)
	viewRepo := repository.NewViewRepository(db)
	statusRepo := repository.NewStatusRepository(db)
	visibilityRepo := repository.NewProjectStatusVisibilityRepository(db)
	favRepo := repository.NewFavoriteRepository(db)
	prefsRepo := repository.NewUserPreferencesRepository(db)
	assetRepo := repository.NewAssetRepository(db)
	aiSettingsRepo := repository.NewAISettingsRepository(db)

	// Dev Machine control-plane store
	devMachineRepo := repository.NewDevMachineRepository(db)

	// Services
	authSvc := service.NewAuthService(userRepo, refreshRepo, cfg.JWTSecret)
	workspaceSvc := service.NewWorkspaceService(workspaceRepo, userRepo, statusRepo)
	notifSvc := service.NewNotificationService(notifRepo)
	issueSvc := service.NewIssueService(issueRepo, statusRepo, historyRepo, hub, notifSvc, projectRepo, workspaceRepo)
	labelSvc := service.NewLabelService(labelRepo)
	commentSvc := service.NewCommentService(commentRepo, issueRepo, userRepo, hub, notifSvc)
	projectSvc := service.NewProjectService(projectRepo)
	relationSvc := service.NewIssueRelationService(relationRepo, issueRepo)
	templateSvc := service.NewIssueTemplateService(templateRepo)
	viewSvc := service.NewViewService(viewRepo, hub)
	statusSvc := service.NewStatusService(statusRepo, visibilityRepo)
	favSvc := service.NewFavoriteService(favRepo)
	prefsSvc := service.NewPreferencesService(prefsRepo)
	aiSettingsSvc := service.NewAISettingsService(aiSettingsRepo, workspaceRepo, issueRepo, crypto.DeriveKey(cfg.JWTSecret+":ai"))

	// Dev Machine agent registry
	devMachineAgentReg := agent.NewRegistry(
		agent.NewClaudeCodeProvider(cfg.DevMachine.ClaudeCodeImage),
		agent.NewOpenCodeProvider(cfg.DevMachine.OpenCodeImage),
		agent.NewCodexProvider(cfg.DevMachine.CodexImage),
		agent.NewCustomCLIProvider(cfg.DevMachine.CustomImage),
	)

	var devMachineEncKey []byte
	if cfg.DevMachine.EncryptionKey != "" {
		devMachineEncKey = crypto.DeriveKey(cfg.DevMachine.EncryptionKey)
	}
	devMachineSvc := service.NewDevMachineService(
		devMachineRepo, devMachineAgentReg, cfg.DevMachine.Enabled, cfg.DevMachine.Domain,
		devMachineEncKey, time.Duration(cfg.DevMachine.TicketTTLSeconds)*time.Second,
		service.DevMachineImages{
			IDE: cfg.DevMachine.IDEImage, Browser: cfg.DevMachine.BrowserImage,
			Collector: cfg.DevMachine.CollectorImage,
			Egress:    cfg.DevMachine.EgressImage,
		}, cfg.FrontendURL,
	)

	// Handlers
	healthH := handler.NewHealthHandler(db)
	loginThrottle := mw.NewLoginThrottle(5, 15*time.Minute)
	authH := handler.NewAuthHandler(authSvc, cfg.Environment != "development", loginThrottle, cfg.IsSysAdmin)
	workspaceH := handler.NewWorkspaceHandler(workspaceSvc)
	issueH := handler.NewIssueHandler(issueSvc, commentSvc, userRepo, statusRepo, projectRepo, relationSvc)
	labelH := handler.NewLabelHandler(labelSvc)
	projectH := handler.NewProjectHandler(projectSvc)
	notifH := handler.NewNotificationHandler(notifSvc)
	wsH := handler.NewWSHandler(hub)
	relationH := handler.NewIssueRelationHandler(relationSvc)
	templateH := handler.NewIssueTemplateHandler(templateSvc)
	viewH := handler.NewViewHandler(viewSvc)
	statusH := handler.NewStatusHandler(statusSvc)
	favH := handler.NewFavoriteHandler(favSvc)
	prefsH := handler.NewPreferencesHandler(prefsSvc)
	aiSettingsH := handler.NewAISettingsHandler(aiSettingsSvc)
	devMachineH := handler.NewDevMachineHandler(devMachineSvc)
	analyticsRepo := repository.NewAnalyticsRepository(db)
	analyticsH := handler.NewAnalyticsHandler(analyticsRepo)
	systemH := handler.NewSystemHandler(cfg.SystemUpdaterURL, cfg.SystemUpdaterToken, cfg.IsSysAdmin)
	webhookRepo := repository.NewWebhookRepository(db)
	webhookSvc := service.NewWebhookService(webhookRepo, cfg.JWTSecret)
	webhookH := handler.NewWebhookHandler(webhookSvc)
	sharedLinkRepo := repository.NewSharedLinkRepository(db)
	sharedLinkSvc := service.NewSharedLinkService(sharedLinkRepo, workspaceRepo, projectRepo, viewRepo, issueRepo, userRepo, statusRepo, cfg.JWTSecret)
	sharedLinkH := handler.NewSharedLinkHandler(sharedLinkSvc, cfg.FrontendURL)
	store, err := storage.New(cfg.Storage)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}
	uploadH := handler.NewUploadHandler(store, assetRepo, issueRepo, cfg.JWTSecret)
	workspaceTransferRepo := repository.NewWorkspaceTransferRepository(db)
	workspaceTransferSvc := service.NewWorkspaceTransferService(workspaceTransferRepo, store)
	workspaceTransferH := handler.NewWorkspaceTransferHandler(workspaceTransferSvc)

	// GitHub integration
	var globalGitHubApp *service.GlobalGitHubAppConfig
	if cfg.GitHubApp.IsConfigured() {
		globalGitHubApp = &service.GlobalGitHubAppConfig{
			AppID:         cfg.GitHubApp.AppID,
			PrivateKey:    cfg.GitHubApp.PrivateKey,
			ClientID:      cfg.GitHubApp.ClientID,
			ClientSecret:  cfg.GitHubApp.ClientSecret,
			WebhookSecret: cfg.GitHubApp.WebhookSecret,
			Slug:          cfg.GitHubApp.Slug,
		}
		log.Info("Global GitHub App configured (SaaS mode)")
	}
	githubRepo := repository.NewGitHubRepository(db)
	githubSvc := service.NewGitHubService(
		githubRepo, issueRepo, statusRepo, historyRepo,
		crypto.DeriveKey(cfg.JWTSecret+":github"), hub, cfg.FrontendURL, cfg.GitHubWebhookURL,
		globalGitHubApp,
	)
	githubH := handler.NewGitHubHandler(githubSvc)

	// Gitea integration
	giteaRepo := repository.NewGiteaRepository(db)
	giteaSvc := service.NewGiteaService(
		giteaRepo, issueRepo, statusRepo, historyRepo, projectRepo,
		userRepo, commentRepo,
		crypto.DeriveKey(cfg.JWTSecret+":gitea"), hub, cfg.FrontendURL,
	)
	giteaH := handler.NewGiteaHandler(giteaSvc)

	// Wire Gitea sync into IssueService/CommentService and the comment service
	// into GiteaService (avoids circular init by using setters)
	issueSvc.SetGiteaService(giteaSvc)
	commentSvc.SetGiteaService(giteaSvc)
	giteaSvc.SetCommentService(commentSvc)

	// Background: clean up expired refresh tokens every hour
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			if err := refreshRepo.DeleteExpired(context.Background()); err != nil {
				log.WithError(err).Warn("failed to clean up expired refresh tokens")
			} else {
				log.Info("expired refresh tokens cleaned up")
			}
		}
	}()

	// Echo
	e := echo.New()
	e.HideBanner = true

	// Global middleware
	e.Use(mw.Recovery())
	e.Use(mw.Logging())
	e.Use(mw.CORS(cfg.FrontendURL))
	e.Use(mw.SecureHeaders())

	// Health
	e.GET("/health", healthH.Health)
	e.GET("/ready", healthH.Ready)

	// Auth (public) — rate limited: 5 requests/sec, burst of 10
	auth := e.Group("/api/auth", mw.RateLimit(5, 10))
	auth.POST("/register", authH.Register)
	auth.POST("/login", authH.Login)
	auth.POST("/refresh", authH.Refresh)
	auth.POST("/logout", authH.Logout)

	// Public share routes (no auth, rate limited)
	pub := e.Group("/api/public", mw.RateLimit(2, 5))
	pub.GET("/share/:token", sharedLinkH.GetPublicMeta)
	pub.GET("/share/:token/issues", sharedLinkH.ListPublicIssues)
	e.GET("/api/public/assets/:token", uploadH.PublicAsset, mw.RateLimit(10, 20))

	// Authenticated routes
	api := e.Group("/api", mw.Auth(cfg.JWTSecret))

	// User
	api.GET("/auth/me", authH.Me)
	api.PATCH("/auth/me", authH.UpdateProfile)
	api.GET("/preferences", prefsH.Get)
	api.PATCH("/preferences", prefsH.Update)
	api.GET("/system/update-status", systemH.UpdateStatus)
	api.POST("/system/update", systemH.StartUpdate)

	// Workspaces (no workspace context needed for list/create)
	api.GET("/workspaces", workspaceH.List)
	api.POST("/workspaces", workspaceH.Create)
	api.POST("/workspaces/import/preview", workspaceTransferH.Preview)
	api.POST("/workspaces/import", workspaceTransferH.Import)

	// Workspace-scoped routes
	ws := api.Group("/workspaces/:slug", mw.WorkspaceMembership(workspaceRepo))
	ws.GET("", workspaceH.Get)
	ws.PATCH("", workspaceH.Update, mw.RequireOwner())
	ws.DELETE("", workspaceH.Delete, mw.RequireOwner())
	ws.GET("/export", workspaceTransferH.Export, mw.RequirePermission(domain.PermWorkspaceTransfer))
	ws.POST("/invite", workspaceH.Invite, mw.RequirePermission("member:invite"))
	ws.GET("/members", workspaceH.ListMembers)
	ws.PATCH("/members/:userId", workspaceH.UpdateMemberRole, mw.RequirePermission("member:invite"))
	ws.DELETE("/members/:userId", workspaceH.RemoveMember, mw.RequirePermission("member:invite"))

	// Statuses (workspace-scoped)
	ws.GET("/statuses", statusH.List)
	ws.POST("/statuses", statusH.Create, mw.RequirePermission("workspace:manage"))
	ws.PATCH("/statuses/:statusId", statusH.Update, mw.RequirePermission("workspace:manage"))
	ws.DELETE("/statuses/:statusId", statusH.Delete, mw.RequirePermission("workspace:manage"))

	// Issues
	ws.GET("/issues", issueH.List)
	ws.POST("/issues", issueH.Create, mw.RequirePermission("issue:create"))
	ws.PATCH("/issues/bulk", issueH.BulkUpdate, mw.RequirePermission("issue:update"))
	ws.DELETE("/issues/bulk", issueH.BulkDelete, mw.RequirePermission("issue:delete_own"))
	ws.GET("/issues/:identifier", issueH.Get)
	ws.PATCH("/issues/:identifier", issueH.Update, mw.RequirePermission("issue:update"))
	ws.DELETE("/issues/:identifier", issueH.Delete, mw.RequirePermission("issue:delete_own"))
	ws.POST("/issues/:identifier/subscribe", issueH.Subscribe)
	ws.DELETE("/issues/:identifier/subscribe", issueH.Unsubscribe)
	ws.POST("/issues/:identifier/duplicate", issueH.Duplicate, mw.RequirePermission("issue:create"))
	ws.POST("/issues/:identifier/convert-to-project", issueH.ConvertToProject, mw.RequirePermission("project:manage"))
	ws.POST("/issues/:identifier/expand-description", aiSettingsH.ExpandIssueDescription, mw.RequirePermission("issue:update"))
	ws.GET("/issues/:identifier/comments", issueH.ListComments)
	ws.POST("/issues/:identifier/comments", issueH.CreateComment, mw.RequirePermission("issue:create"))
	ws.POST("/issues/:identifier/comments/:commentId/resolve", issueH.ResolveComment, mw.RequirePermission("issue:update"))
	ws.POST("/issues/:identifier/comments/:commentId/reopen", issueH.ReopenComment, mw.RequirePermission("issue:update"))
	ws.GET("/issues/:identifier/sub-issues", issueH.ListSubIssues)
	ws.POST("/issues/:identifier/sub-issues", issueH.CreateSubIssue, mw.RequirePermission("issue:create"))
	ws.POST("/issues/:identifier/sub-issues/bulk", issueH.BulkCreateSubIssues, mw.RequirePermission("issue:create"))
	ws.GET("/issues/:identifier/history", issueH.GetHistory)
	ws.POST("/issues/:identifier/triage/accept", issueH.TriageAccept, mw.RequirePermission("issue:update"))
	ws.POST("/issues/:identifier/triage/decline", issueH.TriageDecline, mw.RequirePermission("issue:update"))

	// Issue Relations
	ws.POST("/issues/:identifier/relations", relationH.Create, mw.RequirePermission("issue:update"))
	ws.GET("/issues/:identifier/relations", relationH.List)
	ws.DELETE("/issues/:identifier/relations/:relationId", relationH.Delete, mw.RequirePermission("issue:update"))

	// Issue Templates
	ws.GET("/issue-templates", templateH.List)
	ws.POST("/issue-templates", templateH.Create, mw.RequirePermission("issue:create"))
	ws.GET("/issue-templates/:id", templateH.Get)
	ws.PATCH("/issue-templates/:id", templateH.Update, mw.RequirePermission("issue:create"))
	ws.DELETE("/issue-templates/:id", templateH.Delete, mw.RequirePermission("issue:create"))

	// Labels
	ws.GET("/labels", labelH.List)
	ws.POST("/labels", labelH.Create, mw.RequirePermission("label:manage"))
	ws.PATCH("/labels/:id", labelH.Update, mw.RequirePermission("label:manage"))
	ws.DELETE("/labels/:id", labelH.Delete, mw.RequirePermission("label:manage"))

	// Projects
	ws.GET("/projects", projectH.List)
	ws.POST("/projects", projectH.Create, mw.RequirePermission("project:manage"))
	ws.GET("/projects/:id", projectH.Get)
	ws.PATCH("/projects/:id", projectH.Update, mw.RequirePermission("project:manage"))
	ws.DELETE("/projects/:id", projectH.Delete, mw.RequirePermission("project:manage"))

	// Views
	ws.GET("/views", viewH.List)
	ws.POST("/views", viewH.Create)
	ws.GET("/views/:id", viewH.Get)
	ws.PATCH("/views/:id", viewH.Update)
	ws.DELETE("/views/:id", viewH.Delete)

	// Analytics
	ws.GET("/analytics/overview", analyticsH.Overview)
	ws.GET("/analytics/distribution", analyticsH.IssueDistribution)
	ws.GET("/analytics/insights", analyticsH.Insights)
	ws.GET("/analytics/burnup", analyticsH.Burnup)

	// Webhooks
	ws.GET("/webhooks", webhookH.List, mw.RequirePermission("workspace:manage"))
	ws.POST("/webhooks", webhookH.Create, mw.RequirePermission("workspace:manage"))
	ws.PATCH("/webhooks/:id", webhookH.Update, mw.RequirePermission("workspace:manage"))
	ws.DELETE("/webhooks/:id", webhookH.Delete, mw.RequirePermission("workspace:manage"))

	// AI settings
	ws.GET("/ai-settings", aiSettingsH.Get, mw.RequireOwner())
	ws.GET("/ai-settings/issue-copy-prompt", aiSettingsH.GetIssueCopyPrompt, mw.RequirePermission("issue:read"))
	ws.PATCH("/ai-settings", aiSettingsH.Update, mw.RequireOwner())

	// GitHub integration (conditional)
	// Public webhook endpoint (no auth, signature-verified internally)
	e.POST("/api/github/webhook", githubH.HandleWebhook)
	e.POST("/api/dev-machine-ingest/events", devMachineH.IngestEvent, mw.MachineTokenRateLimit(20, 40))
	e.POST("/api/dev-machine-ingest/logs", devMachineH.IngestLog, mw.MachineTokenRateLimit(50, 100))

	// GitHub integration (workspace-scoped)
	ws.GET("/github/status", githubH.Status)
	ws.GET("/github/setup", githubH.Setup, mw.RequirePermission("workspace:manage"))
	ws.GET("/github/setup/callback", githubH.SetupCallback, mw.RequirePermission("workspace:manage"))
	ws.GET("/github/install", githubH.InstallURL, mw.RequirePermission("workspace:manage"))
	ws.GET("/github/callback", githubH.Callback, mw.RequirePermission("workspace:manage"))
	ws.DELETE("/github/disconnect", githubH.Disconnect, mw.RequirePermission("workspace:manage"))
	ws.DELETE("/github/app", githubH.DeleteApp, mw.RequirePermission("workspace:manage"))
	ws.GET("/github/repos", githubH.ListRepos, mw.RequirePermission("project:manage"))
	ws.POST("/github/repos", githubH.LinkRepos, mw.RequirePermission("project:manage"))
	ws.DELETE("/github/repos/:id", githubH.UnlinkRepo, mw.RequirePermission("project:manage"))
	ws.GET("/github/auto-transitions", githubH.ListAutoTransitions)
	ws.PATCH("/github/auto-transitions", githubH.UpdateAutoTransitions, mw.RequirePermission("workspace:manage"))
	ws.GET("/issues/:identifier/github", githubH.IssueGitHubActivity)
	ws.GET("/github/issue-links", githubH.AgentIssueLinks)

	// Gitea integration (public webhook endpoint)
	e.POST("/api/gitea/webhook", giteaH.HandleWebhook)

	// Gitea integration (workspace-scoped)
	ws.GET("/gitea/status", giteaH.Status)
	ws.POST("/gitea/connect", giteaH.Connect, mw.RequirePermission("workspace:manage"))
	ws.DELETE("/gitea/disconnect", giteaH.Disconnect, mw.RequirePermission("workspace:manage"))
	ws.GET("/gitea/repos", giteaH.ListRepos, mw.RequirePermission("project:manage"))
	ws.POST("/gitea/repos", giteaH.LinkRepos, mw.RequirePermission("project:manage"))
	ws.DELETE("/gitea/repos/:id", giteaH.UnlinkRepo, mw.RequirePermission("project:manage"))
	ws.GET("/gitea/auto-transitions", giteaH.ListAutoTransitions)
	ws.PATCH("/gitea/auto-transitions", giteaH.UpdateAutoTransitions, mw.RequirePermission("workspace:manage"))
	ws.GET("/issues/:identifier/gitea", giteaH.IssueGiteaActivity)
	ws.GET("/gitea/issue-links", giteaH.AgentIssueLinks)

	// Dev Machines — guarded by demo-mode restriction when active
	dm := ws.Group("", mw.DevMachineDemoGuard(cfg.DemoDevMachineAllowed))
	dm.GET("/dev-machines", devMachineH.List, mw.RequirePermission("dev_machine:read"))
	dm.POST("/dev-machines", devMachineH.Create, mw.RequirePermission("dev_machine:create"))
	dm.DELETE("/dev-machines/bulk", devMachineH.BulkDelete, mw.RequirePermission("dev_machine:admin"))
	dm.POST("/dev-machines/bulk/permanent-delete", devMachineH.BulkPermanentDelete, mw.RequirePermission("dev_machine:admin"))
	dm.GET("/dev-machine-names/suggestion", devMachineH.NameSuggestion, mw.RequirePermission("dev_machine:create"))
	dm.GET("/dev-machine-names/availability", devMachineH.NameAvailability, mw.RequirePermission("dev_machine:create"))
	dm.GET("/dev-machine-policy", devMachineH.GetPolicy, mw.RequirePermission("dev_machine:read"))
	dm.PATCH("/dev-machine-policy", devMachineH.UpdatePolicy, mw.RequirePermission("dev_machine:admin"))
	dm.GET("/dev-machine-scope-settings", devMachineH.ScopeSettings, mw.RequirePermission("dev_machine:read"))
	dm.GET("/dev-machine-scope-setting", devMachineH.ScopeSetting, mw.RequirePermission("dev_machine:read"))
	dm.PUT("/dev-machine-scope-setting", devMachineH.UpdateScopeSetting, mw.RequirePermission("dev_machine:manage"))
	dm.DELETE("/dev-machine-scope-setting", devMachineH.DeleteScopeSetting, mw.RequirePermission("dev_machine:manage"))
	dm.GET("/dev-machine-environments", devMachineH.Environments, mw.RequirePermission("dev_machine:read"))
	dm.POST("/dev-machine-environments", devMachineH.SnapshotEnvironment, mw.RequirePermission("dev_machine:admin"))
	dm.GET("/dev-machine-environments/:environmentId", devMachineH.GetEnvironment, mw.RequirePermission("dev_machine:read"))
	dm.DELETE("/dev-machine-environments/:environmentId", devMachineH.DeleteEnvironment, mw.RequirePermission("dev_machine:admin"))
	dm.GET("/dev-machine-providers", devMachineH.Providers, mw.RequirePermission("dev_machine:read"))
	dm.GET("/dev-machines/:machineId", devMachineH.Get, mw.RequirePermission("dev_machine:read"))
	dm.PATCH("/dev-machines/:machineId", devMachineH.Update, mw.RequirePermission("dev_machine:manage"))
	dm.DELETE("/dev-machines/:machineId", devMachineH.Delete, mw.RequirePermission("dev_machine:admin"))
	dm.POST("/dev-machines/:machineId/permanent-delete", devMachineH.PermanentDelete, mw.RequirePermission("dev_machine:admin"))
	dm.POST("/dev-machines/:machineId/start", devMachineH.Start, mw.RequirePermission("dev_machine:manage"))
	dm.POST("/dev-machines/:machineId/stop", devMachineH.Stop, mw.RequirePermission("dev_machine:manage"))
	dm.POST("/dev-machines/:machineId/pause", devMachineH.Pause, mw.RequirePermission("dev_machine:manage"))
	dm.POST("/dev-machines/:machineId/teardown", devMachineH.Teardown, mw.RequirePermission("dev_machine:manage"))
	dm.POST("/dev-machines/:machineId/activity", devMachineH.TouchActivity, mw.RequirePermission("dev_machine:read"))
	dm.GET("/dev-machines/:machineId/checkouts", devMachineH.Checkouts, mw.RequirePermission("dev_machine:read"))
	dm.POST("/dev-machines/:machineId/checkouts", devMachineH.CheckoutIssue, mw.RequirePermission("dev_machine:manage"))
	dm.GET("/dev-machines/:machineId/events", devMachineH.Events, mw.RequirePermission("dev_machine:read"))
	dm.GET("/dev-machines/:machineId/logs", devMachineH.Logs, mw.RequirePermission("dev_machine:read"))
	dm.GET("/dev-machines/:machineId/services", devMachineH.Services, mw.RequirePermission("dev_machine:read"))
	dm.GET("/dev-machines/:machineId/providers", devMachineH.MachineProviders, mw.RequirePermission("dev_machine:read"))
	dm.GET("/dev-machines/:machineId/resource-usage", devMachineH.ResourceUsage, mw.RequirePermission("dev_machine:read"))
	dm.POST("/dev-machines/:machineId/services/:service/launch", devMachineH.LaunchService, mw.RequirePermission("dev_machine:read"))
	dm.GET("/dev-machines/:machineId/terminal-sessions", devMachineH.ListTerminalSessions, mw.RequirePermission("dev_machine:read"))
	dm.POST("/dev-machines/:machineId/terminal-sessions", devMachineH.CreateTerminalSession, mw.RequirePermission("dev_machine:read"))
	dm.POST("/dev-machines/:machineId/terminal-sessions/:sessionId/close", devMachineH.CloseTerminalSession, mw.RequirePermission("dev_machine:read"))
	dm.GET("/dev-machines/:machineId/agent-runs", devMachineH.ListMachineAgentRuns, mw.RequirePermission("dev_machine:read"))
	dm.POST("/dev-machines/:machineId/agent-runs", devMachineH.CreateAgentRun, mw.RequirePermission("dev_machine:manage"))
	dm.GET("/agent-runs", devMachineH.ListAgentRuns, mw.RequirePermission("dev_machine:read"))
	dm.GET("/agent-runs/:agentRunId", devMachineH.GetAgentRun, mw.RequirePermission("dev_machine:read"))
	dm.POST("/agent-runs/:agentRunId/cancel", devMachineH.CancelAgentRun, mw.RequirePermission("dev_machine:manage"))
	dm.GET("/agent-runs/:agentRunId/trace", devMachineH.AgentRunTrace, mw.RequirePermission("dev_machine:read"))

	// Favorites
	ws.GET("/favorites", favH.List)
	ws.POST("/favorites", favH.Create)
	ws.DELETE("/favorites/:id", favH.Delete)

	// Shared Links
	ws.GET("/shared-links", sharedLinkH.List)
	ws.POST("/shared-links", sharedLinkH.Create)
	ws.PATCH("/shared-links/:id", sharedLinkH.Update)
	ws.DELETE("/shared-links/:id", sharedLinkH.Delete)

	// Uploads
	ws.POST("/upload", uploadH.Upload, mw.RequirePermission("issue:create"))
	ws.GET("/assets/:assetId", uploadH.GetAsset)
	ws.POST("/issues/:identifier/prompt-assets", uploadH.SignIssuePromptAssets)

	// WebSocket
	ws.GET("/ws", wsH.Handle)

	// Notifications (user-scoped, not workspace-scoped)
	api.GET("/notifications", notifH.List)
	api.PATCH("/notifications/:id", notifH.Update)
	api.POST("/notifications/:id/read", notifH.MarkRead)
	api.POST("/notifications/:id/unread", notifH.MarkUnread)
	api.POST("/notifications/:id/snooze", notifH.Snooze)
	api.POST("/notifications/:id/unsnooze", notifH.Unsnooze)
	api.POST("/notifications/:id/archive", notifH.Archive)
	api.POST("/notifications/:id/unarchive", notifH.Unarchive)
	api.POST("/notifications/mark-all-read", notifH.MarkAllRead)

	// Start
	addr := fmt.Sprintf(":%d", cfg.Port)
	log.Infof("Starting server on %s", addr)
	if err := e.Start(addr); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func runMigrate(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: server migrate [up|down|version]")
		os.Exit(1)
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL environment variable is required")
	}

	m, err := migrate.New("file://migrations", dbURL)
	if err != nil {
		log.Fatalf("Failed to create migrate instance: %v", err)
	}
	defer m.Close()

	switch args[0] {
	case "up":
		if err := m.Up(); err != nil && err != migrate.ErrNoChange {
			log.Fatalf("Migration up failed: %v", err)
		}
		log.Info("Migrations applied successfully")
	case "down":
		if err := m.Steps(-1); err != nil {
			log.Fatalf("Migration down failed: %v", err)
		}
		log.Info("Rolled back one migration")
	case "version":
		version, dirty, err := m.Version()
		if err != nil {
			log.Fatalf("Failed to get version: %v", err)
		}
		fmt.Printf("Version: %d, Dirty: %v\n", version, dirty)
	default:
		fmt.Printf("Unknown migrate command: %s\n", args[0])
		fmt.Println("Usage: server migrate [up|down|version]")
		os.Exit(1)
	}
}
