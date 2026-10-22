package timing

import "golang.org/x/sys/windows"

var (
	winmm           = windows.NewLazySystemDLL("winmm.dll")
	procTimeBegin   = winmm.NewProc("timeBeginPeriod")
	procTimeEnd     = winmm.NewProc("timeEndPeriod")
	periodRequested bool
)

func beginPeriod() {
	if procTimeBegin.Find() == nil {
		r, _, _ := procTimeBegin.Call(1)
		periodRequested = r == 0 // TIMERR_NOERROR
	}
}

func endPeriod() {
	if periodRequested && procTimeEnd.Find() == nil {
		procTimeEnd.Call(1)
		periodRequested = false
	}
}
