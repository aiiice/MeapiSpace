package main

import (
	"fmt"
	"image"
	"sync"
	"syscall"
	"unsafe"

	"github.com/lxn/win"
	"golang.org/x/sys/windows"
)

const (
	ulwAlpha  = 0x00000002
	acSrcOver = 0x00
)

var (
	layeredClassOnce sync.Once
	layeredClassErr  error
	layeredClassName = syscall.StringToUTF16Ptr("MeapiSpaceLayeredWindow")
	layeredWndProcCB = syscall.NewCallback(layeredWndProc)

	layeredMu      sync.Mutex
	layeredWindows = map[win.HWND]*LayeredWindow{}

	procUpdateLayeredWindow = windows.NewLazySystemDLL("user32.dll").NewProc("UpdateLayeredWindow")
)

type LayeredWindow struct {
	hwnd   win.HWND
	width  int
	height int
	x      int
	y      int
	title  string

	render       func() *image.RGBA
	mouseDown    func(x, y int) bool
	mouseDouble  func(x, y int) bool
	mouseMove    func(x, y int) bool
	mouseLeave   func() bool
	mouseRightUp func(x, y int) bool
	keyDown      func(vk uintptr) bool
	charInput    func(r rune) bool
	focusChange  func(focused bool)
}

func NewLayeredWindow(title string, width, height int) (*LayeredWindow, error) {
	if err := registerLayeredClass(); err != nil {
		return nil, err
	}

	hinst := win.GetModuleHandle(nil)
	titlePtr := syscall.StringToUTF16Ptr(title)
	hwnd := win.CreateWindowEx(
		uint32(win.WS_EX_LAYERED|win.WS_EX_TOOLWINDOW|win.WS_EX_TOPMOST),
		layeredClassName,
		titlePtr,
		uint32(win.WS_POPUP),
		0,
		0,
		int32(width),
		int32(height),
		0,
		0,
		hinst,
		nil,
	)
	if hwnd == 0 {
		return nil, fmt.Errorf("创建窗口失败")
	}

	lw := &LayeredWindow{
		hwnd:   hwnd,
		width:  width,
		height: height,
		title:  title,
	}
	layeredMu.Lock()
	layeredWindows[hwnd] = lw
	layeredMu.Unlock()
	return lw, nil
}

func (lw *LayeredWindow) Handle() win.HWND {
	if lw == nil {
		return 0
	}
	return lw.hwnd
}

func (lw *LayeredWindow) SetPosition(x, y int) {
	lw.x = x
	lw.y = y
}

func (lw *LayeredWindow) SetSize(width, height int) {
	if lw == nil {
		return
	}
	lw.width = width
	lw.height = height
}

func (lw *LayeredWindow) SetBounds(x, y, width, height int) {
	if lw == nil || lw.hwnd == 0 {
		return
	}
	lw.x = x
	lw.y = y
	lw.width = width
	lw.height = height
	_ = lw.updateAt(false)
	win.SetWindowPos(lw.hwnd, win.HWND_TOPMOST, int32(x), int32(y), int32(width), int32(height), win.SWP_SHOWWINDOW)
	win.ShowWindow(lw.hwnd, win.SW_SHOW)
}

func (lw *LayeredWindow) Position() (int, int) {
	if lw == nil {
		return 0, 0
	}
	lw.syncPosition()
	return lw.x, lw.y
}

func (lw *LayeredWindow) Bounds() (int, int, int, int) {
	if lw == nil {
		return 0, 0, 0, 0
	}
	lw.syncPosition()
	return lw.x, lw.y, lw.width, lw.height
}

func (lw *LayeredWindow) Visible() bool {
	if lw == nil || lw.hwnd == 0 {
		return false
	}
	return win.IsWindowVisible(lw.hwnd)
}

func (lw *LayeredWindow) Show() {
	if lw == nil || lw.hwnd == 0 {
		return
	}
	_ = lw.Update()
	win.SetWindowPos(lw.hwnd, win.HWND_TOPMOST, int32(lw.x), int32(lw.y), int32(lw.width), int32(lw.height), win.SWP_SHOWWINDOW)
	win.ShowWindow(lw.hwnd, win.SW_SHOW)
}

func (lw *LayeredWindow) Hide() {
	if lw == nil || lw.hwnd == 0 {
		return
	}
	win.ShowWindow(lw.hwnd, win.SW_HIDE)
}

func (lw *LayeredWindow) BringToFront(focus bool) {
	if lw == nil || lw.hwnd == 0 {
		return
	}
	win.SetWindowPos(lw.hwnd, win.HWND_TOPMOST, int32(lw.x), int32(lw.y), int32(lw.width), int32(lw.height), win.SWP_SHOWWINDOW)
	if focus {
		win.SetForegroundWindow(lw.hwnd)
		win.SetFocus(lw.hwnd)
	}
}

func (lw *LayeredWindow) Destroy() {
	if lw == nil || lw.hwnd == 0 {
		return
	}
	hwnd := lw.hwnd
	lw.hwnd = 0
	layeredMu.Lock()
	delete(layeredWindows, hwnd)
	layeredMu.Unlock()
	win.DestroyWindow(hwnd)
}

func (lw *LayeredWindow) Update() error {
	return lw.updateAt(true)
}

func (lw *LayeredWindow) updateAt(sync bool) error {
	if lw == nil || lw.hwnd == 0 || lw.render == nil {
		return nil
	}
	if sync {
		lw.syncPosition()
	}
	img := lw.render()
	if img == nil {
		return nil
	}
	return updateLayeredWindow(lw.hwnd, lw.x, lw.y, img)
}

func (lw *LayeredWindow) syncPosition() {
	if lw == nil || lw.hwnd == 0 || !win.IsWindowVisible(lw.hwnd) {
		return
	}
	var rect win.RECT
	if win.GetWindowRect(lw.hwnd, &rect) {
		lw.x = int(rect.Left)
		lw.y = int(rect.Top)
		lw.width = int(rect.Right - rect.Left)
		lw.height = int(rect.Bottom - rect.Top)
	}
}

func registerLayeredClass() error {
	layeredClassOnce.Do(func() {
		hinst := win.GetModuleHandle(nil)
		wc := win.WNDCLASSEX{
			CbSize:        uint32(unsafe.Sizeof(win.WNDCLASSEX{})),
			Style:         win.CS_HREDRAW | win.CS_VREDRAW | win.CS_DBLCLKS,
			LpfnWndProc:   layeredWndProcCB,
			HInstance:     hinst,
			HCursor:       win.LoadCursor(0, win.MAKEINTRESOURCE(win.IDC_ARROW)),
			LpszClassName: layeredClassName,
		}
		if atom := win.RegisterClassEx(&wc); atom == 0 {
			layeredClassErr = fmt.Errorf("注册窗口类失败")
		}
	})
	return layeredClassErr
}

func layeredWndProc(hwnd win.HWND, msg uint32, wParam, lParam uintptr) uintptr {
	layeredMu.Lock()
	lw := layeredWindows[hwnd]
	layeredMu.Unlock()

	switch msg {
	case win.WM_LBUTTONDOWN:
		if lw != nil {
			x, y := pointFromLParam(lParam)
			if lw.mouseDown != nil && lw.mouseDown(x, y) {
				return 0
			}
		}
		win.ReleaseCapture()
		win.SendMessage(hwnd, win.WM_NCLBUTTONDOWN, uintptr(win.HTCAPTION), 0)
		return 0
	case win.WM_LBUTTONDBLCLK:
		if lw != nil {
			x, y := pointFromLParam(lParam)
			if lw.mouseDouble != nil && lw.mouseDouble(x, y) {
				return 0
			}
		}
	case win.WM_MOUSEMOVE:
		if lw != nil {
			trackMouseLeave(hwnd)
			x, y := pointFromLParam(lParam)
			if lw.mouseMove != nil && lw.mouseMove(x, y) {
				return 0
			}
		}
	case win.WM_MOUSELEAVE:
		if lw != nil && lw.mouseLeave != nil && lw.mouseLeave() {
			return 0
		}
	case win.WM_MOVE:
		if lw != nil {
			lw.syncPosition()
		}
	case win.WM_RBUTTONUP:
		if lw != nil {
			x, y := pointFromLParam(lParam)
			if lw.mouseRightUp != nil && lw.mouseRightUp(x, y) {
				return 0
			}
		}
	case win.WM_KEYDOWN:
		if lw != nil && lw.keyDown != nil && lw.keyDown(wParam) {
			return 0
		}
	case win.WM_CHAR:
		if lw != nil && lw.charInput != nil && lw.charInput(rune(wParam)) {
			return 0
		}
	case win.WM_SETFOCUS:
		if lw != nil && lw.focusChange != nil {
			lw.focusChange(true)
		}
	case win.WM_KILLFOCUS:
		if lw != nil && lw.focusChange != nil {
			lw.focusChange(false)
		}
	case win.WM_CLOSE:
		win.ShowWindow(hwnd, win.SW_HIDE)
		return 0
	case win.WM_DESTROY:
		layeredMu.Lock()
		delete(layeredWindows, hwnd)
		layeredMu.Unlock()
	}

	return win.DefWindowProc(hwnd, msg, wParam, lParam)
}

func trackMouseLeave(hwnd win.HWND) {
	tme := win.TRACKMOUSEEVENT{
		CbSize:      uint32(unsafe.Sizeof(win.TRACKMOUSEEVENT{})),
		DwFlags:     win.TME_LEAVE,
		HwndTrack:   hwnd,
		DwHoverTime: 0,
	}
	win.TrackMouseEvent(&tme)
}

func pointFromLParam(lp uintptr) (int, int) {
	x := int(int16(uint16(lp & 0xffff)))
	y := int(int16(uint16((lp >> 16) & 0xffff)))
	return x, y
}

func updateLayeredWindow(hwnd win.HWND, x, y int, img *image.RGBA) error {
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 {
		return nil
	}

	screenDC := win.GetDC(0)
	if screenDC == 0 {
		return fmt.Errorf("获取屏幕 DC 失败")
	}
	defer win.ReleaseDC(0, screenDC)

	memDC := win.CreateCompatibleDC(screenDC)
	if memDC == 0 {
		return fmt.Errorf("创建内存 DC 失败")
	}
	defer win.DeleteDC(memDC)

	header := win.BITMAPINFOHEADER{
		BiSize:        uint32(unsafe.Sizeof(win.BITMAPINFOHEADER{})),
		BiWidth:       int32(width),
		BiHeight:      -int32(height),
		BiPlanes:      1,
		BiBitCount:    32,
		BiCompression: win.BI_RGB,
	}
	var bits unsafe.Pointer
	hbmp := win.CreateDIBSection(screenDC, &header, win.DIB_RGB_COLORS, &bits, 0, 0)
	if hbmp == 0 || bits == nil {
		return fmt.Errorf("创建 DIB 失败")
	}
	defer win.DeleteObject(win.HGDIOBJ(hbmp))

	old := win.SelectObject(memDC, win.HGDIOBJ(hbmp))
	defer win.SelectObject(memDC, old)

	copyPremultipliedBGRA(bits, img)

	dst := win.POINT{X: int32(x), Y: int32(y)}
	size := win.SIZE{CX: int32(width), CY: int32(height)}
	src := win.POINT{}
	blend := win.BLENDFUNCTION{
		BlendOp:             acSrcOver,
		BlendFlags:          0,
		SourceConstantAlpha: 255,
		AlphaFormat:         win.AC_SRC_ALPHA,
	}
	ret, _, err := procUpdateLayeredWindow.Call(
		uintptr(hwnd),
		uintptr(screenDC),
		uintptr(unsafe.Pointer(&dst)),
		uintptr(unsafe.Pointer(&size)),
		uintptr(memDC),
		uintptr(unsafe.Pointer(&src)),
		0,
		uintptr(unsafe.Pointer(&blend)),
		ulwAlpha,
	)
	if ret == 0 {
		return fmt.Errorf("更新窗口失败：%v", err)
	}
	return nil
}

func copyPremultipliedBGRA(bits unsafe.Pointer, img *image.RGBA) {
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	dst := unsafe.Slice((*byte)(bits), width*height*4)
	i := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		row := img.PixOffset(bounds.Min.X, y)
		for x := 0; x < width; x++ {
			r := img.Pix[row+x*4+0]
			g := img.Pix[row+x*4+1]
			b := img.Pix[row+x*4+2]
			a := img.Pix[row+x*4+3]
			dst[i+0] = byte((uint16(b) * uint16(a)) / 255)
			dst[i+1] = byte((uint16(g) * uint16(a)) / 255)
			dst[i+2] = byte((uint16(r) * uint16(a)) / 255)
			dst[i+3] = a
			i += 4
		}
	}
}
