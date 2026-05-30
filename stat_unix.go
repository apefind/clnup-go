//go:build !windows && !darwin

package main

import (
	"io/fs"
	"syscall"
)

// statExtra fills Unix-specific fields on Linux (and other non-Darwin Unix).
// Atim/Ctim are of type syscall.Timespec with a Nano() method.
func statExtra(s *statInfo, fi fs.FileInfo) {
	sys, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return
	}
	s.uid   = int64(sys.Uid)
	s.gid   = int64(sys.Gid)
	s.atime = sys.Atim.Nano()
	s.ctime = sys.Ctim.Nano()
}
