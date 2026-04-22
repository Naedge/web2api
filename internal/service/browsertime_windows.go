//go:build windows

package service

import "syscall"

var (
	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	getTickCount64Proc = kernel32.NewProc("GetTickCount64")
)

func browserUptimeMillis() float64 {
	value, _, _ := getTickCount64Proc.Call()
	return float64(value)
}
