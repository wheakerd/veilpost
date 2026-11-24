package certs

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/wheakerd/veilpost/app/backend/internal/auth"
)

// Handler serves /api/dns-credentials and /api/certificates with the session
// cookie and the browser write-request rules. It never logs request bodies,
// credential values, domains or identifiers; the router logs only the route
// template.
type Handler struct {
	Service *Service
	Auth    *auth.Handler
	Logger  *slog.Logger
}

func (h *Handler) RegisterCredentials(group *gin.RouterGroup) {
	group.GET("", func(c *gin.Context) {
		credentials, err := h.Service.Credentials(c.Request.Context(), h.Auth.Token(c))
		if !h.failed(c, err) {
			c.JSON(200, gin.H{"credentials": credentials})
		}
	})
	group.POST("", func(c *gin.Context) {
		var input struct {
			Name     string            `json:"name"`
			Provider string            `json:"provider"`
			Secrets  map[string]string `json:"secrets"`
			Password string            `json:"password"`
		}
		if !h.Auth.RequireWrite(c) || !decode(c, &input, 16<<10) {
			return
		}
		credential, err := h.Service.CreateCredential(c.Request.Context(), h.Auth.Token(c), input.Name, input.Provider, input.Secrets, input.Password)
		if !h.failed(c, err) {
			c.JSON(201, gin.H{"credential": credential})
		}
	})
	group.POST("/:id/update", func(c *gin.Context) {
		var input struct {
			Name     string            `json:"name"`
			Secrets  map[string]string `json:"secrets"`
			Password string            `json:"password"`
		}
		if !h.Auth.RequireWrite(c) || !decode(c, &input, 16<<10) {
			return
		}
		credential, err := h.Service.UpdateCredential(c.Request.Context(), h.Auth.Token(c), c.Param("id"), input.Name, input.Secrets, input.Password)
		if !h.failed(c, err) {
			c.JSON(200, gin.H{"credential": credential})
		}
	})
	group.POST("/:id/delete", func(c *gin.Context) {
		var input struct {
			Password string `json:"password"`
		}
		if !h.Auth.RequireWrite(c) || !decode(c, &input, 8<<10) {
			return
		}
		if err := h.Service.DeleteCredential(c.Request.Context(), h.Auth.Token(c), c.Param("id"), input.Password); !h.failed(c, err) {
			c.Status(204)
		}
	})
	group.POST("/:id/verify", func(c *gin.Context) {
		var input struct{}
		if !h.Auth.RequireWrite(c) || !decode(c, &input, 1<<10) {
			return
		}
		if err := h.Service.VerifyCredential(c.Request.Context(), h.Auth.Token(c), c.Param("id")); !h.failed(c, err) {
			c.JSON(200, gin.H{"status": "ok"})
		}
	})
}

func (h *Handler) RegisterCertificates(group *gin.RouterGroup) {
	group.GET("", func(c *gin.Context) {
		certificates, err := h.Service.Certificates(c.Request.Context(), h.Auth.Token(c))
		if !h.failed(c, err) {
			c.JSON(200, gin.H{"certificates": certificates})
		}
	})
	group.POST("", func(c *gin.Context) {
		var input RequestInput
		if !h.Auth.RequireWrite(c) || !decode(c, &input, 64<<10) {
			return
		}
		certificate, err := h.Service.Request(c.Request.Context(), h.Auth.Token(c), input)
		if !h.failed(c, err) {
			c.JSON(202, gin.H{"certificate": certificate})
		}
	})
	group.POST("/:id/reissue", func(c *gin.Context) {
		var input struct{}
		if !h.Auth.RequireWrite(c) || !decode(c, &input, 1<<10) {
			return
		}
		certificate, err := h.Service.Reissue(c.Request.Context(), h.Auth.Token(c), c.Param("id"))
		if !h.failed(c, err) {
			c.JSON(202, gin.H{"certificate": certificate})
		}
	})
	group.POST("/:id/delete", func(c *gin.Context) {
		var input struct{}
		if !h.Auth.RequireWrite(c) || !decode(c, &input, 1<<10) {
			return
		}
		if err := h.Service.DeleteCertificate(c.Request.Context(), h.Auth.Token(c), c.Param("id")); !h.failed(c, err) {
			c.Status(204)
		}
	})
	group.GET("/:id/chain", func(c *gin.Context) {
		chain, name, err := h.Service.Chain(c.Request.Context(), h.Auth.Token(c), c.Param("id"))
		if h.failed(c, err) {
			return
		}
		c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": chainFileName(name)}))
		c.Data(200, "application/pem-certificate-chain", []byte(chain))
	})
}

// chainFileName derives an ASCII file name from the first certificate name.
func chainFileName(name string) string {
	name = strings.Replace(name, "*.", "wildcard.", 1)
	return name + ".pem"
}

// decode accepts exactly one JSON object without unknown fields.
func decode(c *gin.Context, target any, limit int64) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, limit))
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
	var api *auth.APIError
	if errors.As(err, &api) {
		c.AbortWithStatusJSON(api.Status, gin.H{"code": api.Code})
		return true
	}
	h.Logger.Error("certificate operation failed")
	c.AbortWithStatusJSON(503, gin.H{"code": "SERVICE_UNAVAILABLE"})
	return true
}
