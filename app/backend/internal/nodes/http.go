package nodes

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wheakerd/veilpost/app/backend/internal/auth"
)

// Handler serves /api/nodes (browser, session cookie, same-origin writes)
// and /api/agent (nodes, Ed25519 signatures). It never logs request bodies,
// tokens, keys or node identifiers; the router logs only the route template.
type Handler struct {
	Service *Service
	Auth    *auth.Handler
	Logger  *slog.Logger
}

const agentBodyLimit = 64 << 10

func (h *Handler) RegisterConsole(group *gin.RouterGroup) {
	group.GET("", func(c *gin.Context) {
		nodes, err := h.Service.List(c.Request.Context(), h.Auth.Token(c))
		if !h.failed(c, err) {
			c.JSON(200, gin.H{"nodes": nodes})
		}
	})
	group.POST("", func(c *gin.Context) {
		var input struct {
			Name     string `json:"name"`
			Password string `json:"password"`
		}
		if !h.Auth.RequireWrite(c) || !decode(c, &input) {
			return
		}
		node, enrollment, err := h.Service.Create(c.Request.Context(), h.Auth.Token(c), input.Name, input.Password)
		if !h.failed(c, err) {
			c.JSON(201, gin.H{"node": node, "enrollment": enrollment})
		}
	})
	group.POST("/:id/revoke", func(c *gin.Context) {
		var input struct {
			Password string `json:"password"`
		}
		if !h.Auth.RequireWrite(c) || !decode(c, &input) {
			return
		}
		node, err := h.Service.Revoke(c.Request.Context(), h.Auth.Token(c), c.Param("id"), input.Password)
		if !h.failed(c, err) {
			c.JSON(200, gin.H{"node": node})
		}
	})
	group.POST("/:id/enrollment", func(c *gin.Context) {
		var input struct {
			Password string `json:"password"`
		}
		if !h.Auth.RequireWrite(c) || !decode(c, &input) {
			return
		}
		node, enrollment, err := h.Service.Reissue(c.Request.Context(), h.Auth.Token(c), c.Param("id"), input.Password)
		if !h.failed(c, err) {
			c.JSON(200, gin.H{"node": node, "enrollment": enrollment})
		}
	})
	group.POST("/:id/delete", func(c *gin.Context) {
		var input struct {
			Password string `json:"password"`
		}
		if !h.Auth.RequireWrite(c) || !decode(c, &input) {
			return
		}
		if err := h.Service.Delete(c.Request.Context(), h.Auth.Token(c), c.Param("id"), input.Password); !h.failed(c, err) {
			c.Status(204)
		}
	})
	group.POST("/:id/docker/install", func(c *gin.Context) {
		var input struct {
			Source   string `json:"source"`
			Password string `json:"password"`
		}
		if !h.Auth.RequireWrite(c) || !decode(c, &input) {
			return
		}
		node, err := h.Service.QueueDockerInstall(c.Request.Context(), h.Auth.Token(c), c.Param("id"), input.Source, input.Password)
		if !h.failed(c, err) {
			c.JSON(http.StatusAccepted, gin.H{"node": node})
		}
	})
}

func (h *Handler) RegisterAgent(group *gin.RouterGroup) {
	group.POST("/enroll", func(c *gin.Context) {
		body, ok := agentBody(c)
		if !ok {
			return
		}
		var input EnrollRequest
		if json.Unmarshal(body, &input) != nil {
			c.AbortWithStatusJSON(400, gin.H{"code": "REQUEST_FAILED"})
			return
		}
		result, err := h.Service.Enroll(c.Request.Context(), c.ClientIP(), input)
		if !h.failed(c, err) {
			c.JSON(200, result)
		}
	})
	group.POST("/poll", func(c *gin.Context) {
		body, ok := agentBody(c)
		if !ok {
			return
		}
		nodeID, err := h.Service.Authenticate(c.Request.Context(), SignedRequest{
			Method: c.Request.Method, Route: c.FullPath(),
			NodeID: c.GetHeader(HeaderNode), Timestamp: c.GetHeader(HeaderTimestamp),
			Nonce: c.GetHeader(HeaderNonce), Signature: c.GetHeader(HeaderSignature), Body: body,
		})
		if h.failed(c, err) {
			return
		}
		var input struct {
			AgentVersion  string      `json:"agentVersion"`
			ConfigVersion int64       `json:"configVersion"`
			Docker        *DockerPoll `json:"docker,omitempty"`
		}
		// Unknown fields are ignored so newer agents can report more status.
		if json.Unmarshal(body, &input) != nil {
			c.AbortWithStatusJSON(400, gin.H{"code": "REQUEST_FAILED"})
			return
		}
		// The server-wide write timeout is shorter than a long poll.
		_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(h.Service.hold + 15*time.Second))
		result, err := h.Service.poll(c.Request.Context(), nodeID, input.AgentVersion, input.ConfigVersion, input.Docker)
		if c.Request.Context().Err() != nil {
			c.Abort()
			return
		}
		if !h.failed(c, err) {
			c.JSON(200, result)
		}
	})
}

// agentBody reads a bounded JSON body; the signature covers its exact bytes.
func agentBody(c *gin.Context) ([]byte, bool) {
	contentType, _, _ := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if contentType != "application/json" {
		c.AbortWithStatusJSON(415, gin.H{"code": "REQUEST_FAILED"})
		return nil, false
	}
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, agentBodyLimit))
	if err != nil {
		c.AbortWithStatusJSON(413, gin.H{"code": "REQUEST_FAILED"})
		return nil, false
	}
	return body, true
}

// decode accepts exactly one JSON object without unknown fields.
func decode(c *gin.Context, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 8192))
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
	h.Logger.Error("node operation failed")
	c.AbortWithStatusJSON(503, gin.H{"code": "SERVICE_UNAVAILABLE"})
	return true
}
