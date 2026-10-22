//go:build !windows

package timing

// Unix kernels already schedule with sub-millisecond resolution.
func beginPeriod() {}
func endPeriod()   {}
