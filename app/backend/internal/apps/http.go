package apps

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/wheakerd/veilpost/app/backend/internal/auth"
)

// Handler serves /api/apps with the session cookie and the browser write
// rules of the authentication API. It never logs settings, passwords or app
// identifiers; the router logs only the route template.
type Handler struct {
	Service *Service
	Auth    *auth.Handler
	Logger  *slog.Logger
}

const bodyLimit = 16 << 10

func (h *Handler) Register(group *gin.RouterGroup) {
	group.GET("", func(c *gin.Context) {
		apps, err := h.Service.List(c.Request.Context(), h.Auth.Token(c))
		if !h.failed(c, err) {
			c.JSON(200, gin.H{"apps": apps})
		}
	})
	group.POST("/:id/install", func(c *gin.Context) {
		var input struct{}
		if !h.Auth.RequireWrite(c) || !decode(c, &input) {
			return
		}
		app, err := h.Service.Install(c.Request.Context(), h.Auth.Token(c), c.Param("id"))
		if !h.failed(c, err) {
			c.JSON(200, gin.H{"app": app})
		}
	})
	group.POST("/:id/uninstall", func(c *gin.Context) {
		var input struct {
			Password string `json:"password"`
		}
		if !h.Auth.RequireWrite(c) || !decode(c, &input) {
			return
		}
		if err := h.Service.Uninstall(c.Request.Context(), h.Auth.Token(c), c.Param("id"), input.Password); !h.failed(c, err) {
			c.Status(204)
		}
	})
	group.GET("/:id/settings", func(c *gin.Context) {
		settings, err := h.Service.Settings(c.Request.Context(), h.Auth.Token(c), c.Param("id"))
		if !h.failed(c, err) {
			c.JSON(200, settings)
		}
	})
	group.POST("/:id/settings", func(c *gin.Context) {
		var input Update
		if !h.Auth.RequireWrite(c) || !decode(c, &input) {
			return
		}
		settings, err := h.Service.UpdateSettings(c.Request.Context(), h.Auth.Token(c), c.Param("id"), input)
		if !h.failed(c, err) {
			c.JSON(200, settings)
		}
	})
}

// decode accepts exactly one bounded JSON object without unknown fields.
func decode(c *gin.Context, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, bodyLimit))
	decoder.DisallowUnknownFields()
	if decoder.Decode(target) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		c.AbortWithStatusJSON(400, gin.H{"code": "REQUEST_FAILED"})
		return false
	}
	return true
}

func (h *Handler) failed(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	var setting *SettingError
	if errors.As(err, &setting) {
		body := gin.H{"code": setting.Code}
		if setting.Field != "" {
			body["field"] = setting.Field
		}
		c.AbortWithStatusJSON(400, body)
		return true
	}
	var api *auth.APIError
	if errors.As(err, &api) {
		c.AbortWithStatusJSON(api.Status, gin.H{"code": api.Code})
		return true
	}
	h.Logger.Error("app operation failed")
	c.AbortWithStatusJSON(503, gin.H{"code": "SERVICE_UNAVAILABLE"})
	return true
}
