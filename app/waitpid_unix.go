//go:build !windows

package main

import (
	"syscall"
	"time"
)

// waitExit waits (at most 20 seconds) until the process pid is gone: a program started by an update must not meet the
// copy that started it (the single instance lock).
func waitExit(pid int) {
	for i := 0; i < 200; i++ {
		if syscall.Kill(pid, 0) != nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}
