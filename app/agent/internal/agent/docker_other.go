//go:build !linux

package agent

import "os"

func rootOwned(os.FileInfo) bool { return false }
