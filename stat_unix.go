//go:build !windows

package main

import (
	"io/fs"
	"syscall"
)

// statExtra fills Unix-specific fields: uid, gid, atime, ctime.
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
