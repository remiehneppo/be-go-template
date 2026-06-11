package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
	domainuser "github.com/remihneppo/be-go-template/internal/domain/user"
	"github.com/remihneppo/be-go-template/internal/platform/ctxkeys"
	apperrors "github.com/remihneppo/be-go-template/internal/platform/errors"
)

type UserHandler struct {
	service domainuser.Service
}

func NewUserHandler(service domainuser.Service) *UserHandler {
	return &UserHandler{service: service}
}

func (h *UserHandler) RegisterRoutes(group *gin.RouterGroup, middleware ...gin.HandlerFunc) {
	users := group.Group("/users", middleware...)
	users.GET("/me", h.GetMe)
}

func (h *UserHandler) GetMe(c *gin.Context) {
	userID := contextString(c, ctxkeys.UserID)
	if userID == "" {
		Error(c, apperrors.New(apperrors.CodeUnauthorized, "Unauthorized", http.StatusUnauthorized))
		return
	}
	usr, err := h.service.GetMe(c.Request.Context(), userID)
	if err != nil {
		reportContextError(c, err)
		return
	}
	response := userResponseFromDomain(*usr)
	if WriteETagOrNotModified(c, response) {
		return
	}
	OK(c, response)
}
