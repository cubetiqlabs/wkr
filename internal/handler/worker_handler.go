package handler

import (
	"strconv"
	"strings"

	"github.com/cubetiqlabs/wkr/internal/middleware"
	"github.com/cubetiqlabs/wkr/internal/service"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

const (
	ERROR_WORKER_NOT_FOUND     = "worker not found"
	ERROR_INVALID_WORKER_ID    = "invalid worker ID"
	ERROR_INVALID_REQUEST_BODY = "invalid request body"
)

func workerWriteError(c fiber.Ctx, err error, action string) error {
	switch err {
	case service.ErrWorkerNotFound:
		return errResponse(c, fiber.StatusNotFound, err.Error())
	case service.ErrUnauthorized:
		return errResponse(c, fiber.StatusForbidden, err.Error())
	case service.ErrWorkerExists:
		return errResponse(c, fiber.StatusConflict, err.Error())
	case service.ErrRevisionNotFound:
		return errResponse(c, fiber.StatusNotFound, err.Error())
	default:
		if strings.Contains(err.Error(), "verification failed") {
			return errResponse(c, fiber.StatusBadRequest, err.Error())
		}
		return errResponse(c, fiber.StatusInternalServerError, "failed to "+action+" worker")
	}
}

type WorkerHandler struct {
	workerService *service.WorkerService
	quotaService  *service.QuotaService
}

func NewWorkerHandler(workerService *service.WorkerService, quotaService *service.QuotaService) *WorkerHandler {
	return &WorkerHandler{workerService: workerService, quotaService: quotaService}
}

func (h *WorkerHandler) Create(c fiber.Ctx) error {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		return errResponse(c, fiber.StatusUnauthorized, "unauthorized")
	}

	if err := h.quotaService.CheckWorkerLimit(c.Context(), userID); err != nil {
		if err == service.ErrQuotaWorkersExceeded {
			return errResponse(c, fiber.StatusForbidden, "maximum workers limit reached, upgrade your plan")
		}
	}

	var input service.CreateWorkerInput
	if err := c.Bind().Body(&input); err != nil {
		return errResponse(c, fiber.StatusBadRequest, ERROR_INVALID_REQUEST_BODY)
	}

	worker, err := h.workerService.Create(c.Context(), userID, input)
	if err != nil {
		return workerWriteError(c, err, "create")
	}

	return created(c, worker)
}

func (h *WorkerHandler) Get(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return errResponse(c, fiber.StatusBadRequest, ERROR_INVALID_WORKER_ID)
	}

	worker, err := h.workerService.Get(c.Context(), id)
	if err != nil {
		return errResponse(c, fiber.StatusNotFound, ERROR_WORKER_NOT_FOUND)
	}

	return ok(c, worker)
}

func (h *WorkerHandler) List(c fiber.Ctx) error {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		return errResponse(c, fiber.StatusUnauthorized, "unauthorized")
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	pageSize, _ := strconv.Atoi(c.Query("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	workers, total, err := h.workerService.List(c.Context(), userID, page, pageSize)
	if err != nil {
		return errResponse(c, fiber.StatusInternalServerError, "failed to list workers")
	}

	return okWithMeta(c, workers, &Meta{Page: page, PageSize: pageSize, Total: total})
}

func (h *WorkerHandler) Update(c fiber.Ctx) error {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		return errResponse(c, fiber.StatusUnauthorized, "unauthorized")
	}

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return errResponse(c, fiber.StatusBadRequest, ERROR_INVALID_WORKER_ID)
	}

	var input service.UpdateWorkerInput
	if err := c.Bind().Body(&input); err != nil {
		return errResponse(c, fiber.StatusBadRequest, ERROR_INVALID_REQUEST_BODY)
	}

	worker, err := h.workerService.Update(c.Context(), id, userID, input)
	if err != nil {
		return workerWriteError(c, err, "update")
	}

	return ok(c, worker)
}

func (h *WorkerHandler) Delete(c fiber.Ctx) error {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		return errResponse(c, fiber.StatusUnauthorized, "unauthorized")
	}

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return errResponse(c, fiber.StatusBadRequest, ERROR_INVALID_WORKER_ID)
	}

	if err := h.workerService.Delete(c.Context(), id, userID); err != nil {
		return workerWriteError(c, err, "delete")
	}

	return ok(c, fiber.Map{"deleted": true})
}

func (h *WorkerHandler) ListRevisions(c fiber.Ctx) error {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		return errResponse(c, fiber.StatusUnauthorized, "unauthorized")
	}

	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	if limit < 1 || limit > 100 {
		limit = 20
	}

	revisions, err := h.workerService.ListRevisions(c.Context(), c.Params("name"), userID, limit)
	if err != nil {
		return workerWriteError(c, err, "list revisions for")
	}

	return ok(c, revisions)
}

func (h *WorkerHandler) Rollback(c fiber.Ctx) error {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		return errResponse(c, fiber.StatusUnauthorized, "unauthorized")
	}

	var input struct {
		Version int `json:"version"`
	}
	if err := c.Bind().Body(&input); err != nil || input.Version < 1 {
		return errResponse(c, fiber.StatusBadRequest, "version is required and must be >= 1")
	}

	worker, err := h.workerService.Rollback(c.Context(), c.Params("name"), userID, input.Version)
	if err != nil {
		return workerWriteError(c, err, "rollback")
	}

	return ok(c, worker)
}

func (h *WorkerHandler) UpdateByName(c fiber.Ctx) error {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		return errResponse(c, fiber.StatusUnauthorized, "unauthorized")
	}

	var input service.UpdateWorkerInput
	if err := c.Bind().Body(&input); err != nil {
		return errResponse(c, fiber.StatusBadRequest, ERROR_INVALID_REQUEST_BODY)
	}

	worker, err := h.workerService.UpdateByName(c.Context(), c.Params("name"), userID, input)
	if err != nil {
		return workerWriteError(c, err, "update")
	}

	return ok(c, worker)
}

func (h *WorkerHandler) DeleteByName(c fiber.Ctx) error {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		return errResponse(c, fiber.StatusUnauthorized, "unauthorized")
	}

	if err := h.workerService.DeleteByName(c.Context(), c.Params("name"), userID); err != nil {
		return workerWriteError(c, err, "delete")
	}

	return ok(c, fiber.Map{"deleted": true})
}
