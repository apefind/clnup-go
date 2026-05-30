//go:build windows

package main

import "io/fs"

// statExtra is a no-op on Windows: uid/gid don't exist,
// and atime/ctime are already set to mtime as fallbacks in statPath.
func statExtra(s *statInfo, fi fs.FileInfo) {}
