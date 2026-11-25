package nodes

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"time"

	"github.com/wheakerd/veilpost/app/backend/internal/auth"
)

const (
	dockerQueueLifetime = 5 * time.Minute
	dockerReportWindow  = 20 * time.Minute
	dockerSourceLimit   = 16
)

var dockerSourcePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

// DockerNode exposes the node's registered sources and latest installation.
type DockerNode struct {
	Sources []string           `json:"sources"`
	Job     *DockerInstallView `json:"job,omitempty"`
}

// DockerInstall identifies a single job delivered to the agent.
type DockerInstall struct {
	ID     string `json:"id"`
	Source string `json:"source"`
}

type DockerInstallView struct {
	DockerInstall
	Status    string `json:"status"`
	UpdatedAt string `json:"updatedAt"`
	Error     string `json:"error,omitempty"`
}

type DockerReport struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type DockerPoll struct {
	Sources []string      `json:"sources"`
	Result  *DockerReport `json:"result,omitempty"`
	Busy    bool          `json:"busy,omitempty"`
}

type dockerRow struct {
	sources, id, source, status, failure sql.NullString
	updated                              sql.NullInt64
}

func dockerStatus(status string, updated int64, now time.Time) string {
	elapsed := now.Sub(time.UnixMilli(updated))
	switch {
	case status == "queued" && elapsed > dockerQueueLifetime:
		return "expired"
	case status == "running" && elapsed > dockerReportWindow:
		return "unknown"
	default:
		return status
	}
}

func (row dockerRow) view(now time.Time) (*DockerNode, error) {
	result := &DockerNode{Sources: []string{}}
	if row.sources.Valid {
		if err := json.Unmarshal([]byte(row.sources.String), &result.Sources); err != nil {
			return nil, err
		}
	}
	if row.id.Valid {
		result.Job = &DockerInstallView{
			DockerInstall: DockerInstall{ID: row.id.String, Source: row.source.String},
			Status:        dockerStatus(row.status.String, row.updated.Int64, now),
			UpdatedAt:     timestamp(row.updated.Int64),
			Error:         row.failure.String,
		}
	}
	if len(result.Sources) == 0 && result.Job == nil {
		return nil, nil
	}
	return result, nil
}

func validDockerReport(result *DockerReport) bool {
	if !nodeIDPattern.MatchString(result.ID) {
		return false
	}
	switch result.Status {
	case "succeeded", "already_installed":
		return result.Error == ""
	case "failed":
		return slices.Contains([]string{"source_unavailable", "install_failed", "timed_out"}, result.Error)
	default:
		return false
	}
}

func dockerSources(values []string) (string, bool) {
	if len(values) > dockerSourceLimit {
		return "", false
	}
	for i, value := range values {
		if !dockerSourcePattern.MatchString(value) || slices.Contains(values[:i], value) {
			return "", false
		}
	}
	if len(values) == 0 {
		return "[]", true
	}
	encoded, err := json.Marshal(values)
	return string(encoded), err == nil
}

// recordDockerPoll updates source capabilities and accepts a signed result for
// this node's current job. An identical report can be submitted again safely.
func (s *Service) recordDockerPoll(ctx context.Context, nodeID string, input *DockerPoll) (string, error) {
	sources := "[]"
	if input != nil {
		var valid bool
		sources, valid = dockerSources(input.Sources)
		if !valid || input.Result != nil && !validDockerReport(input.Result) {
			return "", auth.Failure("REQUEST_FAILED", 400)
		}
	}
	if sources == "[]" {
		if _, err := s.db.ExecContext(ctx, "UPDATE node_docker SET sources='[]' WHERE node_id=? AND sources<>'[]'", nodeID); err != nil {
			return "", err
		}
	} else {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO node_docker(node_id,sources)
 SELECT id,? FROM nodes WHERE id=? AND status='active'
 ON CONFLICT(node_id) DO UPDATE SET sources=excluded.sources WHERE node_docker.sources<>excluded.sources`,
			sources, nodeID); err != nil {
			return "", err
		}
	}
	if input != nil && input.Result != nil {
		result := input.Result
		if _, err := s.db.ExecContext(ctx, `UPDATE node_docker SET job_status=?,job_at=?,job_error=?
 WHERE node_id=? AND job_id=? AND job_status='running'
 AND EXISTS (SELECT 1 FROM nodes WHERE id=node_id AND status='active')`,
			result.Status, s.now().UnixMilli(), result.Error, nodeID, result.ID); err != nil {
			return "", err
		}
	}
	if input != nil && input.Busy {
		return "[]", nil
	}
	return sources, nil
}

// claimDockerInstall delivers each job at most once. An unreported running job
// becomes unknown after its report window rather than being sent again.
func (s *Service) claimDockerInstall(ctx context.Context, nodeID, sources string) (*DockerInstall, error) {
	if sources == "[]" {
		return nil, nil
	}
	now := s.now()
	var job DockerInstall
	err := s.db.QueryRowContext(ctx, `UPDATE node_docker SET job_status='running',job_at=?
 WHERE node_id=? AND job_status='queued' AND job_at>=?
 AND EXISTS (SELECT 1 FROM json_each(?) WHERE value=job_source)
 AND EXISTS (SELECT 1 FROM nodes WHERE id=node_id AND status='active')
 RETURNING job_id,job_source`, now.UnixMilli(), nodeID, now.Add(-dockerQueueLifetime).UnixMilli(), sources).
		Scan(&job.ID, &job.Source)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &job, nil
}

// QueueDockerInstall confirms the administrator password and queues an install
// from a source last advertised by the active online node.
func (s *Service) QueueDockerInstall(ctx context.Context, session, nodeID, source, password string) (Node, error) {
	if !dockerSourcePattern.MatchString(source) {
		return Node{}, auth.Failure("REQUEST_FAILED", 400)
	}
	tx, status, err := s.confirmedStatus(ctx, session, nodeID, password)
	if err != nil {
		return Node{}, err
	}
	defer tx.Rollback()
	if status != "active" {
		return Node{}, auth.Failure("DOCKER_SOURCE_UNAVAILABLE", 409)
	}

	now := s.now()
	var lastSeen sql.NullInt64
	var row dockerRow
	err = tx.QueryRowContext(ctx, `SELECT n.last_seen_at,d.sources,d.job_status,d.job_at
 FROM nodes n LEFT JOIN node_docker d ON d.node_id=n.id WHERE n.id=?`, nodeID).
		Scan(&lastSeen, &row.sources, &row.status, &row.updated)
	if err != nil {
		return Node{}, err
	}
	view, err := row.view(now)
	if err != nil {
		return Node{}, err
	}
	if !lastSeen.Valid || now.Sub(time.UnixMilli(lastSeen.Int64)) > OnlineWindow ||
		view == nil || !slices.Contains(view.Sources, source) {
		return Node{}, auth.Failure("DOCKER_SOURCE_UNAVAILABLE", 409)
	}
	switch dockerStatus(row.status.String, row.updated.Int64, now) {
	case "queued", "running":
		return Node{}, auth.Failure("DOCKER_INSTALL_BUSY", 409)
	}
	if _, err = tx.ExecContext(ctx, `UPDATE node_docker SET job_id=?,job_source=?,job_status='queued',job_at=?,job_error=NULL
 WHERE node_id=?`, randomToken(16), source, now.UnixMilli(), nodeID); err != nil {
		return Node{}, err
	}
	if err = tx.Commit(); err != nil {
		return Node{}, err
	}
	s.notify(nodeID)
	return s.node(ctx, nodeID)
}
