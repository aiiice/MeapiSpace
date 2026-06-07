package main

import (
	"github.com/lxn/win"
	"golang.org/x/sys/windows"
)

func applyGlassWindow(hwnd win.HWND, width, height int) {
	style := win.GetWindowLongPtr(hwnd, win.GWL_STYLE)
	style &^= uintptr(win.WS_CAPTION | win.WS_THICKFRAME | win.WS_SYSMENU | win.WS_MINIMIZEBOX | win.WS_MAXIMIZEBOX)
	style |= uintptr(win.WS_POPUP)
	win.SetWindowLongPtr(hwnd, int(win.GWL_STYLE), style)

	exStyle := win.GetWindowLongPtr(hwnd, win.GWL_EXSTYLE)
	exStyle &^= uintptr(win.WS_EX_APPWINDOW | win.WS_EX_LAYERED)
	exStyle |= uintptr(win.WS_EX_TOOLWINDOW | win.WS_EX_TOPMOST)
	win.SetWindowLongPtr(hwnd, int(win.GWL_EXSTYLE), exStyle)

	applyRoundedRegion(hwnd, width, height)
	win.SetWindowPos(hwnd, 0, 0, 0, int32(width), int32(height), win.SWP_NOMOVE|win.SWP_NOZORDER|win.SWP_FRAMECHANGED)
}

func keepPopupTopmost(hwnd win.HWND) {
	win.SetWindowPos(hwnd, win.HWND_TOPMOST, 0, 0, 0, 0, win.SWP_NOMOVE|win.SWP_NOSIZE|win.SWP_SHOWWINDOW)
	win.SetForegroundWindow(hwnd)
}

func applyRoundedRegion(hwnd win.HWND, width, height int) {
	gdi32 := windows.NewLazySystemDLL("gdi32.dll")
	user32 := windows.NewLazySystemDLL("user32.dll")
	createRgn := gdi32.NewProc("CreateRoundRectRgn")
	setRgn := user32.NewProc("SetWindowRgn")
	rgn, _, _ := createRgn.Call(0, 0, uintptr(width+1), uintptr(height+1), 18, 18)
	if rgn == 0 {
		return
	}
	ret, _, _ := setRgn.Call(uintptr(hwnd), rgn, 1)
	if ret == 0 {
		win.DeleteObject(win.HGDIOBJ(rgn))
	}
}
