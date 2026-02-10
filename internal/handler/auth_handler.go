package handler

import (
	"github.com/cubetiqlabs/wkr/internal/service"
	"github.com/gofiber/fiber/v3"
)

type AuthHandler struct {
	authService *service.AuthService
}

func NewAuthHandler(authService *service.AuthService) *AuthHandler {
	return &AuthHandler{authService: authService}
}

func (h *AuthHandler) Register(c fiber.Ctx) error {
	var input service.RegisterInput
	if err := c.Bind().Body(&input); err != nil {
		return errResponse(c, fiber.StatusBadRequest, "invalid request body")
	}

	resp, err := h.authService.Register(c.Context(), input)
	if err != nil {
		switch err {
		case service.ErrEmailExists:
			return errResponse(c, fiber.StatusConflict, err.Error())
		default:
			return errResponse(c, fiber.StatusInternalServerError, "registration failed")
		}
	}

	return created(c, resp)
}

func (h *AuthHandler) Login(c fiber.Ctx) error {
	var input service.LoginInput
	if err := c.Bind().Body(&input); err != nil {
		return errResponse(c, fiber.StatusBadRequest, "invalid request body")
	}

	resp, err := h.authService.Login(c.Context(), input)
	if err != nil {
		switch err {
		case service.ErrInvalidCredentials:
			return errResponse(c, fiber.StatusUnauthorized, err.Error())
		default:
			return errResponse(c, fiber.StatusInternalServerError, "login failed")
		}
	}

	return ok(c, resp)
}
