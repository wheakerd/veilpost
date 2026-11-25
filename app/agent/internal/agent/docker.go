package agent

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

const (
	aptGet               = "/usr/bin/apt-get"
	dpkgQuery            = "/usr/bin/dpkg-query"
	dockerInstallTimeout = 15 * time.Minute
	dockerSourceLimit    = 16
)

var dockerSourcePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

// DockerInstaller reports locally approved sources and installs Docker Engine
// using the selected source. Results contain only a stable status and reason.
type DockerInstaller interface {
	Sources() []string
	Install(ctx context.Context, source string) (status, reason string)
}

type aptDockerInstaller struct{ directory string }

// NewDockerInstaller opts a Linux agent into Docker Engine installation.
func NewDockerInstaller(directory string) (DockerInstaller, error) {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		return nil, errors.New("a Linux agent running as root is required for Docker installation")
	}
	for _, binary := range []string{aptGet, dpkgQuery} {
		if info, err := os.Stat(binary); err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
			return nil, errors.New("APT and dpkg are required for Docker installation")
		}
	}
	if !filepath.IsAbs(directory) || !trustedSourceDirectory(directory) {
		return nil, errors.New("use an absolute, root-owned Docker sources directory with no writable or symbolic-link ancestors")
	}
	return &aptDockerInstaller{directory: filepath.Clean(directory)}, nil
}

func trustedSourceDirectory(directory string) bool {
	for path := filepath.Clean(directory); ; path = filepath.Dir(path) {
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() || info.Mode().Perm()&0022 != 0 || !rootOwned(info) {
			return false
		}
		if filepath.Dir(path) == path {
			return true
		}
	}
}

// Every deb822 stanza names its signing key. A profile can include signed
// distribution sources alongside the Docker repository for dependencies.
func signedDockerSources(contents string, trustedKeyring func(string) bool) bool {
	hasSource, hasKey, hasEntry := false, false, false
	for _, raw := range strings.Split(contents+"\n\n", "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case strings.HasPrefix(line, "#"):
			continue
		case line == "":
			if hasEntry {
				if !hasKey {
					return false
				}
				hasSource = true
			}
			hasKey, hasEntry = false, false
		default:
			hasEntry = true
			if raw[0] == ' ' || raw[0] == '\t' {
				continue
			}
			key, value, _ := strings.Cut(line, ":")
			switch strings.ToLower(strings.TrimSpace(key)) {
			case "signed-by":
				rings := strings.Fields(value)
				if hasKey || len(rings) != 1 || !filepath.IsAbs(rings[0]) || !trustedKeyring(rings[0]) {
					return false
				}
				hasKey = true
			case "trusted", "allow-insecure", "allow-weak", "allow-downgrade-to-insecure":
				return false
			}
		}
	}
	return hasSource
}

func trustedDockerKeyring(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0022 == 0 &&
		rootOwned(info) && trustedSourceDirectory(filepath.Dir(path))
}

func (d *aptDockerInstaller) profile(source string) string {
	if !dockerSourcePattern.MatchString(source) || !trustedSourceDirectory(d.directory) {
		return ""
	}
	path := filepath.Join(d.directory, source+".sources")
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 || !rootOwned(info) || info.Size() > 64<<10 {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil || !signedDockerSources(string(data), trustedDockerKeyring) {
		return ""
	}
	return path
}

func (d *aptDockerInstaller) Sources() []string {
	sources := []string{}
	entries, err := os.ReadDir(d.directory)
	if err != nil {
		return sources
	}
	for _, entry := range entries {
		name, ok := strings.CutSuffix(entry.Name(), ".sources")
		if ok && d.profile(name) != "" {
			sources = append(sources, name)
			if len(sources) == dockerSourceLimit {
				break
			}
		}
	}
	return sources
}

func aptSourceOptions(profile string) []string {
	return []string{
		"-o", "Dir::Etc::sourcelist=" + profile,
		"-o", "Dir::Etc::sourceparts=-",
		"-o", "APT::Get::List-Cleanup=false",
		"-o", "APT::Get::AllowUnauthenticated=false",
		"-o", "Acquire::AllowInsecureRepositories=false",
		"-o", "Acquire::AllowDowngradeToInsecureRepositories=false",
		"-o", "APT::Update::Error-Mode=any",
		"-o", "DPkg::Lock::Timeout=30",
	}
}

func packageInstalled(ctx context.Context, name string) bool {
	value, err := exec.CommandContext(ctx, dpkgQuery, "-W", "-f=${db:Status-Status}", name).Output()
	return err == nil && string(value) == "installed"
}

func (d *aptDockerInstaller) Install(ctx context.Context, source string) (string, string) {
	profile := d.profile(source)
	if profile == "" {
		return "failed", "source_unavailable"
	}
	ctx, cancel := context.WithTimeout(ctx, dockerInstallTimeout)
	defer cancel()
	if packageInstalled(ctx, "docker-ce") || packageInstalled(ctx, "docker.io") {
		return "already_installed", ""
	}
	run := func(args ...string) error {
		command := exec.CommandContext(ctx, aptGet, append(aptSourceOptions(profile), args...)...)
		command.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
		return command.Run()
	}
	err := run("update")
	if err == nil {
		err = run("install", "-y", "--no-remove", "--no-install-recommends", "docker-ce", "docker-ce-cli", "containerd.io")
	}
	if err == nil && packageInstalled(ctx, "docker-ce") {
		return "succeeded", ""
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "failed", "timed_out"
	}
	return "failed", "install_failed"
}
