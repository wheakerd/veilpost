package auth

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const CookieName = "vpn_session"

type Handler struct {
	Service      *Service
	Development  bool
	PublicOrigin string
	Logger       *slog.Logger
}

func (h *Handler) Register(group *gin.RouterGroup) {
	group.GET("/session", func(c *gin.Context) {
		session, err := h.Service.Session(c.Request.Context(), h.token(c))
		if h.failed(c, err) {
			return
		}
		if session.Status == "anonymous" && h.token(c) != "" {
			h.cookie(c, "", -1)
		}
		c.JSON(200, session)
	})
	group.GET("/security", func(c *gin.Context) {
		status, err := h.Service.SecurityStatus(c.Request.Context(), h.token(c))
		if !h.failed(c, err) {
			c.JSON(200, status)
		}
	})
	for _, path := range []string{"/login", "/verify", "/logout", "/password", "/sessions/revoke-others",
		"/two-factor/setup", "/two-factor/enable", "/two-factor/disable", "/two-factor/recovery-codes"} {
		group.POST(path, h.write)
	}
}

func (h *Handler) token(c *gin.Context) string { token, _ := c.Cookie(CookieName); return token }
func (h *Handler) cookie(c *gin.Context, value string, maxAge int) {
	cookie := &http.Cookie{Name: CookieName, Value: value, Path: "/", HttpOnly: true, Secure: !h.Development, SameSite: http.SameSiteStrictMode, MaxAge: maxAge}
	if maxAge < 0 {
		cookie.Expires = time.Unix(1, 0)
	}
	http.SetCookie(c.Writer, cookie)
}
func (h *Handler) failed(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	var api *APIError
	if errors.As(err, &api) {
		c.AbortWithStatusJSON(api.Status, gin.H{"code": api.Code})
		return true
	}
	h.Logger.Error("authentication operation failed")
	c.AbortWithStatusJSON(503, gin.H{"code": "SERVICE_UNAVAILABLE"})
	return true
}
func (h *Handler) accept(c *gin.Context, result Result) {
	h.cookie(c, result.Token, int(result.Lifetime.Seconds()))
	c.JSON(200, result.Session)
}

func (h *Handler) write(c *gin.Context) {
	if !h.RequireWrite(c) {
		return
	}
	var input struct {
		Username    string `json:"username"`
		Password    string `json:"password"`
		NewPassword string `json:"newPassword"`
		Method      string `json:"method"`
		Code        string `json:"code"`
		SetupID     string `json:"setupId"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 8192))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		c.AbortWithStatusJSON(400, gin.H{"code": "REQUEST_FAILED"})
		return
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		c.AbortWithStatusJSON(400, gin.H{"code": "REQUEST_FAILED"})
		return
	}
	if len(input.Password) > 1024 || len(input.NewPassword) > 1024 || len(input.Username) > 128 || len(input.Code) > 128 || len(input.SetupID) > 128 {
		c.AbortWithStatusJSON(400, gin.H{"code": "REQUEST_FAILED"})
		return
	}
	token := h.token(c)
	ctx := c.Request.Context()
	switch strings.TrimPrefix(c.FullPath(), "/api/auth") {
	case "/login":
		result, err := h.Service.Login(ctx, input.Username, input.Password, token, c.ClientIP())
		if !h.failed(c, err) {
			h.accept(c, result)
		}
	case "/verify":
		result, err := h.Service.Verify(ctx, token, input.Method, input.Code)
		if !h.failed(c, err) {
			h.accept(c, result)
		}
	case "/logout":
		if !h.failed(c, h.Service.Logout(ctx, token)) {
			h.cookie(c, "", -1)
			c.Status(204)
		}
	case "/password":
		result, err := h.Service.ChangePassword(ctx, token, input.Password, input.NewPassword)
		if !h.failed(c, err) {
			h.accept(c, result)
		}
	case "/sessions/revoke-others":
		if !h.failed(c, h.Service.RevokeOtherSessions(ctx, token)) {
			c.Status(204)
		}
	case "/two-factor/recovery-codes":
		codes, err := h.Service.RegenerateRecoveryCodes(ctx, token, input.Password, input.Code)
		if !h.failed(c, err) {
			c.JSON(200, gin.H{"recoveryCodes": codes})
		}
	case "/two-factor/setup":
		result, err := h.Service.BeginSetup(ctx, token, input.Password)
		if !h.failed(c, err) {
			c.JSON(200, result)
		}
	case "/two-factor/enable":
		result, codes, err := h.Service.Enable(ctx, token, input.SetupID, input.Code)
		if !h.failed(c, err) {
			h.cookie(c, result.Token, int(result.Lifetime.Seconds()))
			c.JSON(200, gin.H{"status": result.Session.Status, "user": result.Session.User, "recoveryCodes": codes})
		}
	case "/two-factor/disable":
		result, err := h.Service.Disable(ctx, token, input.Password, input.Code)
		if !h.failed(c, err) {
			h.accept(c, result)
		}
	}
}
