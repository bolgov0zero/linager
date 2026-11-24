//go:build windows

package main

import "syscall"

const synchronize = 0x00100000

// waitExit waits (at most 20 seconds) until the process pid is gone.
func waitExit(pid int) {
	h, err := syscall.OpenProcess(synchronize, false, uint32(pid))
	if err != nil {
		return // already gone
	}
	defer func() { _ = syscall.CloseHandle(h) }()
	_, _ = syscall.WaitForSingleObject(h, 20000)
}
