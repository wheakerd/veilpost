package httpserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
)

// WriteListenInfo reports the already-bound TCP address, independently of log
// level. The development launcher uses the PID to reject stale announcements.
func WriteListenInfo(path string, address net.Addr) (func(), error) {
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("listen-file must be an absolute path")
	}
	tcp, ok := address.(*net.TCPAddr)
	if !ok || tcp.Port == 0 {
		return nil, fmt.Errorf("listen-file requires a bound TCP listener")
	}
	data, err := json.Marshal(struct {
		PID  int    `json:"pid"`
		IP   string `json:"ip"`
		Port int    `json:"port"`
	}{os.Getpid(), tcp.IP.String(), tcp.Port})
	if err != nil {
		return nil, err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".listen-*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		file.Close()
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return nil, err
	}
	return func() {
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			return
		}
		current, err := os.ReadFile(path)
		if err == nil && bytes.Equal(current, data) {
			_ = os.Remove(path)
		}
	}, nil
}
