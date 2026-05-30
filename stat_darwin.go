//go:build darwin

package main

import (
	"io/fs"
	"syscall"
)

// statExtra fills Unix-specific fields on macOS.
// Darwin uses Atimespec/Ctimespec instead of Atim/Ctim.
func statExtra(s *statInfo, fi fs.FileInfo) {
	sys, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return
	}
	s.uid   = int64(sys.Uid)
	s.gid   = int64(sys.Gid)
	s.atime = sys.Atimespec.Nano()
	s.ctime = sys.Ctimespec.Nano()
}
