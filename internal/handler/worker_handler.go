package handler

import (
	"strconv"

	"github.com/cubetiqlabs/wkr/internal/middleware"
	"github.com/cubetiqlabs/wkr/internal/service"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

const (
	ERROR_WORKER_NOT_FOUND = "worker not found"
	ERROR_INVALID_WORKER_ID  = "invalid worker ID"
)

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

	// Enforce worker count limit
	if err := h.quotaService.CheckWorkerLimit(c.Context(), userID); err != nil {
		if err == service.ErrQuotaWorkersExceeded {
			return errResponse(c, fiber.StatusForbidden, "maximum workers limit reached, upgrade your plan")
		}
	}

	var input service.CreateWorkerInput
	if err := c.Bind().Body(&input); err != nil {
		return errResponse(c, fiber.StatusBadRequest, "invalid request body")
	}

	worker, err := h.workerService.Create(c.Context(), userID, input)
	if err != nil {
		switch err {
		case service.ErrWorkerExists:
			return errResponse(c, fiber.StatusConflict, err.Error())
		default:
			return errResponse(c, fiber.StatusInternalServerError, "failed to create worker")
		}
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
		return errResponse(c, fiber.StatusBadRequest, "invalid request body")
	}

	worker, err := h.workerService.Update(c.Context(), id, userID, input)
	if err != nil {
		switch err {
		case service.ErrWorkerNotFound:
			return errResponse(c, fiber.StatusNotFound, err.Error())
		case service.ErrUnauthorized:
			return errResponse(c, fiber.StatusForbidden, err.Error())
		default:
			return errResponse(c, fiber.StatusInternalServerError, "failed to update worker")
		}
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
		switch err {
		case service.ErrWorkerNotFound:
			return errResponse(c, fiber.StatusNotFound, err.Error())
		case service.ErrUnauthorized:
			return errResponse(c, fiber.StatusForbidden, err.Error())
		default:
			return errResponse(c, fiber.StatusInternalServerError, "failed to delete worker")
		}
	}

	return ok(c, fiber.Map{"deleted": true})
}

// UpdateByName handles PUT /api/v1/workers/by-name/:name
func (h *WorkerHandler) UpdateByName(c fiber.Ctx) error {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		return errResponse(c, fiber.StatusUnauthorized, "unauthorized")
	}

	var input service.UpdateWorkerInput
	if err := c.Bind().Body(&input); err != nil {
		return errResponse(c, fiber.StatusBadRequest, "invalid request body")
	}

	worker, err := h.workerService.UpdateByName(c.Context(), c.Params("name"), userID, input)
	if err != nil {
		switch err {
		case service.ErrWorkerNotFound:
			return errResponse(c, fiber.StatusNotFound, err.Error())
		case service.ErrUnauthorized:
			return errResponse(c, fiber.StatusForbidden, err.Error())
		default:
			return errResponse(c, fiber.StatusInternalServerError, "failed to update worker")
		}
	}

	return ok(c, worker)
}

// DeleteByName handles DELETE /api/v1/workers/by-name/:name
func (h *WorkerHandler) DeleteByName(c fiber.Ctx) error {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		return errResponse(c, fiber.StatusUnauthorized, "unauthorized")
	}

	if err := h.workerService.DeleteByName(c.Context(), c.Params("name"), userID); err != nil {
		switch err {
		case service.ErrWorkerNotFound:
			return errResponse(c, fiber.StatusNotFound, err.Error())
		case service.ErrUnauthorized:
			return errResponse(c, fiber.StatusForbidden, err.Error())
		default:
			return errResponse(c, fiber.StatusInternalServerError, "failed to delete worker")
		}
	}

	return ok(c, fiber.Map{"deleted": true})
}
