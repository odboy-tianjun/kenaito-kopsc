//go:build windows

package executor

import "os"

// Windows has no stable per-file device ID in os.FileInfo. Directory walks do
// not cross volumes on their own, so -x degrades to "do not follow reparse
// points", which filepath.WalkDir already guarantees.
func deviceOf(os.FileInfo) (uint64, bool) { return 0, false }

func deviceOfPath(string) (uint64, bool) { return 0, false }
