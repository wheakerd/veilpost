package httpserver

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wheakerd/veilpost/app/backend/internal/apps"
	"github.com/wheakerd/veilpost/app/backend/internal/auth"
	"github.com/wheakerd/veilpost/app/backend/internal/certs"
	"github.com/wheakerd/veilpost/app/backend/internal/nodes"
)

// Handlers are the API groups backed by real storage. Nil groups are not
// registered, so their routes answer 404.
type Handlers struct {
	Auth  *auth.Handler
	Nodes *nodes.Handler
	Apps  *apps.Handler
	Certs *certs.Handler
}

// NewRouter registers only implemented public endpoints. Authentication and node
// operations must be added with their real authorization rules, never mock users.
// trustedProxies must already be validated IP addresses or CIDR ranges.
func NewRouter(logger *slog.Logger, trustedProxies []string, handlers Handlers) *gin.Engine {
	router := gin.New()
	// Forwarded client addresses are honored only from explicitly configured proxies.
	if err := router.SetTrustedProxies(trustedProxies); err != nil {
		panic("invalid trusted proxy configuration")
	}
	router.HandleMethodNotAllowed = true
	router.RedirectTrailingSlash = false
	router.RedirectFixedPath = false
	router.Use(requestLog(logger), recoverPanic(logger), responseHeaders())

	router.GET("/api/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "veilpost"})
	})
	if handlers.Auth != nil {
		handler := handlers.Auth
		handler.Register(router.Group("/api/auth"))
		router.GET("/api/ready", func(c *gin.Context) {
			if handler.Service.Ready(c.Request.Context()) != nil {
				c.JSON(503, gin.H{"status": "unavailable"})
				return
			}
			c.JSON(200, gin.H{"status": "ready"})
		})
	}
	if handlers.Nodes != nil {
		handlers.Nodes.RegisterConsole(router.Group("/api/nodes"))
		handlers.Nodes.RegisterAgent(router.Group("/api/agent"))
	}
	if handlers.Apps != nil {
		handlers.Apps.Register(router.Group("/api/apps"))
	}
	if handlers.Certs != nil {
		handlers.Certs.RegisterCredentials(router.Group("/api/dns-credentials"))
		handlers.Certs.RegisterCertificates(router.Group("/api/certificates"))
	}
	router.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{"code": "NOT_FOUND"})
	})
	router.NoMethod(func(c *gin.Context) {
		c.JSON(http.StatusMethodNotAllowed, gin.H{"code": "METHOD_NOT_ALLOWED"})
	})
	return router
}

func responseHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Next()
	}
}

func requestLog(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		// Only log the route template, never the URL, params, body, headers or cookies.
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		logger.Info("http request", "method", c.Request.Method, "route", route,
			"status", c.Writer.Status(), "duration", time.Since(start))
	}
}

func recoverPanic(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if recover() != nil {
				// Panic values can contain credentials. Do not print them or the request.
				logger.Error("http handler panic", "route", c.FullPath())
				if !c.Writer.Written() {
					c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"code": "INTERNAL_ERROR"})
				} else {
					c.Abort()
				}
			}
		}()
		c.Next()
	}
}
