package nodes_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wheakerd/veilpost/app/backend/internal/auth"
	"github.com/wheakerd/veilpost/app/backend/internal/nodes"
)

func TestDockerInstallRequiresAnOnlineNodeAndIsDeliveredOnce(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	f.nodes.SetPollHold(time.Millisecond)
	target := f.enrolled(t, "docker-node")
	other := f.enrolled(t, "other-node")
	router := newRouter(f, true)
	sources := []string{"official", "mirror"}

	poll := func(a agent, docker *nodes.DockerPoll) nodes.PollResult {
		t.Helper()
		body, err := json.Marshal(map[string]any{"agentVersion": "1.0.0", "configVersion": 0, "docker": docker})
		if err != nil {
			t.Fatal(err)
		}
		signed := a.signed(f.clock(), string(body))
		req := httptest.NewRequest("POST", "http://console.test"+signed.Route, strings.NewReader(string(body)))
		for header, value := range map[string]string{
			"Content-Type": "application/json", nodes.HeaderNode: signed.NodeID,
			nodes.HeaderTimestamp: signed.Timestamp, nodes.HeaderNonce: signed.Nonce, nodes.HeaderSignature: signed.Signature,
		} {
			req.Header.Set(header, value)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		var result nodes.PollResult
		if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &result) != nil {
			t.Fatalf("poll status %d, body %s", response.Code, response.Body)
		}
		return result
	}
	install := func(password, source string) *httptest.ResponseRecorder {
		t.Helper()
		body, _ := json.Marshal(map[string]string{"password": password, "source": source})
		req := httptest.NewRequest("POST", "http://console.test/api/nodes/"+target.id+"/docker/install", strings.NewReader(string(body)))
		req.Header.Set("Origin", "http://console.test")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Requested-With", "Veilpost")
		req.Header.Set("Cookie", auth.CookieName+"="+f.session)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}
	getJob := func() *nodes.DockerInstallView {
		t.Helper()
		list, err := f.nodes.List(ctx, f.session)
		if err != nil {
			t.Fatal(err)
		}
		for _, node := range list {
			if node.ID == target.id && node.Docker != nil {
				return node.Docker.Job
			}
		}
		return nil
	}

	if _, err := f.nodes.QueueDockerInstall(ctx, f.session, target.id, "mirror", testPassword); err == nil {
		t.Fatal("install was accepted before the node advertised sources")
	}
	poll(target, &nodes.DockerPoll{Sources: sources})
	if res := install("incorrect", "mirror"); res.Code != 403 {
		t.Fatalf("wrong password status %d", res.Code)
	}
	if res := install(testPassword, "unregistered"); res.Code != 409 {
		t.Fatalf("unregistered source status %d", res.Code)
	}
	if res := install(testPassword, "mirror"); res.Code != 202 {
		t.Fatalf("install status %d, body %s", res.Code, res.Body)
	}
	if res := install(testPassword, "mirror"); res.Code != 409 {
		t.Fatalf("duplicate install status %d", res.Code)
	}
	if job := getJob(); job == nil || job.Source != "mirror" || job.Status != "queued" {
		t.Fatalf("queued job: %+v", job)
	}
	if poll(other, &nodes.DockerPoll{Sources: sources}).DockerInstall != nil {
		t.Fatal("a different node received the installation")
	}
	if poll(target, &nodes.DockerPoll{Sources: sources, Busy: true}).DockerInstall != nil ||
		poll(target, nil).DockerInstall != nil {
		t.Fatal("a busy or legacy poll received the installation")
	}
	job := poll(target, &nodes.DockerPoll{Sources: sources}).DockerInstall
	if job == nil || job.Source != "mirror" || getJob().Status != "running" {
		t.Fatalf("claimed job: %+v", job)
	}
	if poll(target, &nodes.DockerPoll{Sources: sources}).DockerInstall != nil {
		t.Fatal("an already delivered job was sent again")
	}
	report := &nodes.DockerReport{ID: job.ID, Status: "succeeded"}
	poll(other, &nodes.DockerPoll{Sources: sources, Result: report})
	if getJob().Status != "running" {
		t.Fatal("a different node could complete the job")
	}
	poll(target, &nodes.DockerPoll{Sources: sources, Result: report})
	poll(target, &nodes.DockerPoll{Sources: sources, Result: report})
	if getJob().Status != "succeeded" {
		t.Fatalf("final job: %+v", getJob())
	}

	if res := install(testPassword, "official"); res.Code != 202 {
		t.Fatalf("next install status %d, body %s", res.Code, res.Body)
	}
	f.advance(6 * time.Minute)
	if poll(target, &nodes.DockerPoll{Sources: sources}).DockerInstall != nil || getJob().Status != "expired" {
		t.Fatal("an expired job was delivered")
	}
}
