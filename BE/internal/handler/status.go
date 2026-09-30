package handler

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/kuayle/kuayle-backend/internal/domain"
	"github.com/kuayle/kuayle-backend/internal/dto"
	"github.com/kuayle/kuayle-backend/internal/service"
	"github.com/kuayle/kuayle-backend/pkg/response"
	"github.com/kuayle/kuayle-backend/pkg/validate"
	"github.com/labstack/echo/v4"
)

type StatusHandler struct {
	statusSvc *service.StatusService
}

func NewStatusHandler(statusSvc *service.StatusService) *StatusHandler {
	return &StatusHandler{statusSvc: statusSvc}
}

func (h *StatusHandler) List(c echo.Context) error {
	ws := c.Get("workspace").(*domain.Workspace)

	ctx := c.Request().Context()
	statuses, err := h.statusSvc.List(ctx, ws.ID)
	if err != nil {
		return response.InternalError(c)
	}

	// Batch load project IDs for all statuses
	statusIDs := make([]uuid.UUID, len(statuses))
	for i, s := range statuses {
		statusIDs[i] = s.ID
	}
	projectIDsMap, _ := h.statusSvc.ListProjectIDsForStatuses(ctx, statusIDs)

	resp := make([]dto.StatusResponse, len(statuses))
	for i, s := range statuses {
		resp[i] = toStatusResponse(s)
		if pids, ok := projectIDsMap[s.ID]; ok && len(pids) > 0 {
			pidStrs := make([]string, len(pids))
			for j, pid := range pids {
				pidStrs[j] = pid.String()
			}
			resp[i].ProjectIDs = pidStrs
		}
	}
	return response.Success(c, http.StatusOK, resp)
}

func (h *StatusHandler) Create(c echo.Context) error {
	var req dto.CreateStatusRequest
	if err := c.Bind(&req); err != nil {
		return response.Error(c, http.StatusBadRequest, "BAD_REQUEST", "Invalid request body")
	}
	if err := validate.Struct(&req); err != nil {
		details := make([]dto.ErrorDetail, 0)
		for _, e := range validate.FormatErrors(err) {
			details = append(details, dto.ErrorDetail{Field: e["field"], Message: e["message"]})
		}
		return response.ValidationError(c, details)
	}

	ws := c.Get("workspace").(*domain.Workspace)

	ctx := c.Request().Context()
	status, err := h.statusSvc.Create(ctx, ws.ID, req)
	if err != nil {
		return response.Error(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
	}
	resp := toStatusResponse(*status)
	if pids, _ := h.statusSvc.ListProjectsForStatus(ctx, status.ID); len(pids) > 0 {
		pidStrs := make([]string, len(pids))
		for i, pid := range pids {
			pidStrs[i] = pid.String()
		}
		resp.ProjectIDs = pidStrs
	}
	return response.Success(c, http.StatusCreated, resp)
}

func (h *StatusHandler) Update(c echo.Context) error {
	var req dto.UpdateStatusRequest
	if err := c.Bind(&req); err != nil {
		return response.Error(c, http.StatusBadRequest, "BAD_REQUEST", "Invalid request body")
	}

	idStr := c.Param("statusId")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return response.Error(c, http.StatusBadRequest, "BAD_REQUEST", "Invalid status ID")
	}

	ws := c.Get("workspace").(*domain.Workspace)
	ctx := c.Request().Context()
	status, err := h.statusSvc.Update(ctx, ws.ID, id, req)
	if err != nil {
		return response.Error(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
	}
	resp := toStatusResponse(*status)
	if pids, _ := h.statusSvc.ListProjectsForStatus(ctx, status.ID); len(pids) > 0 {
		pidStrs := make([]string, len(pids))
		for i, pid := range pids {
			pidStrs[i] = pid.String()
		}
		resp.ProjectIDs = pidStrs
	}
	return response.Success(c, http.StatusOK, resp)
}

func (h *StatusHandler) Delete(c echo.Context) error {
	idStr := c.Param("statusId")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return response.Error(c, http.StatusBadRequest, "BAD_REQUEST", "Invalid status ID")
	}

	ws := c.Get("workspace").(*domain.Workspace)
	if err := h.statusSvc.Delete(c.Request().Context(), ws.ID, id); err != nil {
		return response.Error(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
	}
	return response.Success(c, http.StatusOK, map[string]string{"status": "deleted"})
}

func toStatusResponse(s domain.WorkspaceStatus) dto.StatusResponse {
	return dto.StatusResponse{
		ID:          s.ID.String(),
		WorkspaceID: s.WorkspaceID.String(),
		Name:        s.Name,
		Slug:        s.Slug,
		Category:    string(s.Category),
		Color:       s.Color,
		Position:    s.Position,
		IsDefault:   s.IsDefault,
		CreatedAt:   s.CreatedAt,
		UpdatedAt:   s.UpdatedAt,
	}
}
