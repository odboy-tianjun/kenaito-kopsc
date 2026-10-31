//go:build unix

package executor

import (
	"os"
	"syscall"
)

// deviceOf returns the device ID backing a file, used to implement du's -x
// (do not cross filesystems).
func deviceOf(info os.FileInfo) (uint64, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return uint64(stat.Dev), true
}

func deviceOfPath(path string) (uint64, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, false
	}
	return deviceOf(info)
}
