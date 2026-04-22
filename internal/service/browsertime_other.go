//go:build !windows

package service

import "time"

var browserTimeStart = time.Now()

func browserUptimeMillis() float64 {
	return float64(time.Since(browserTimeStart).Nanoseconds()) / 1_000_000
}
