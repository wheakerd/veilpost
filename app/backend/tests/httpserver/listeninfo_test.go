package httpserver_test

import (
	"encoding/json"
	"github.com/wheakerd/veilpost/app/backend/internal/httpserver"
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestListenInfoReportsBoundAddressAndCleansUp(t *testing.T) {
	listener := listenerForTest(t)
	path := filepath.Join(t.TempDir(), "listen.json")
	cleanup, err := httpserver.WriteListenInfo(path, listener.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var info struct {
		PID  int    `json:"pid"`
		IP   string `json:"ip"`
		Port int    `json:"port"`
	}
	if err := json.Unmarshal(data, &info); err != nil {
		t.Fatal(err)
	}
	actual := listener.Addr().(*net.TCPAddr)
	if info.PID != os.Getpid() || info.IP != actual.IP.String() || info.Port != actual.Port || info.Port == 0 {
		t.Fatalf("announcement differs from bound listener: %+v", info)
	}
	mode, err := os.Stat(path)
	if err != nil || mode.Mode().Perm() != 0600 {
		t.Fatalf("announcement must be private: %v, %v", mode, err)
	}
	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("announcement was not removed: %v", err)
	}
}

func TestListenInfoDoesNotRemoveReplacedAnnouncement(t *testing.T) {
	listener := listenerForTest(t)
	path := filepath.Join(t.TempDir(), "listen.json")
	cleanup, err := httpserver.WriteListenInfo(path, listener.Addr())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"pid":123,"ip":"127.0.0.1","port":12345}`), 0600); err != nil {
		t.Fatal(err)
	}
	cleanup()
	if _, err := os.Stat(path); err != nil {
		t.Fatal("another announcement was removed")
	}
}

func TestListenInfoRejectsRelativePathOrUnboundPort(t *testing.T) {
	for _, test := range []struct {
		path string
		port int
	}{
		{"listen.json", 12345}, {filepath.Join(t.TempDir(), "listen.json"), 0},
	} {
		if _, err := httpserver.WriteListenInfo(test.path, &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: test.port}); err == nil {
			t.Fatal("invalid announcement accepted")
		}
	}
}
