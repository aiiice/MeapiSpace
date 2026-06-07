package main

import (
	"context"
	"fmt"
	"image"
	"math"
	"net/url"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"

	"github.com/lxn/walk"
	"github.com/lxn/win"
)

type App struct {
	cfg    Config
	apiKey string
	client *APIClient

	mw      *walk.MainWindow
	tray    *walk.NotifyIcon
	icon    *walk.Icon
	running bool

	quotaWindow     *LayeredWindow
	toastWindow     *LayeredWindow
	tipWindow       *LayeredWindow
	settingsWindow  *LayeredWindow
	settingsInput   string
	settingsMsg     string
	settingsFocus   bool
	settingsSelect  bool
	quotaPositioned bool
	quotaCollapsed  bool
	quotaMsg        string
	quotaMsgUntil   time.Time
	tipMsg          string
	tipUntil        time.Time

	mu        sync.Mutex
	display   DisplayState
	wasLow    bool
	fetching  bool
	iconPhase float64
}

const (
	quotaRefreshX    = quotaWidth - 30
	quotaRefreshY    = 9
	quotaRefreshSize = 20

	settingsInputX  = 22
	settingsInputY  = 62
	settingsInputW  = settingsWidth - 44
	settingsInputH  = 40
	settingsGetX    = 96
	settingsGetY    = 136
	settingsGetW    = 94
	settingsGetH    = 22
	settingsCancelX = 200
	settingsCancelY = 136
	settingsCancelW = 58
	settingsCancelH = 22
	settingsSaveX   = 266
	settingsSaveY   = 136
	settingsSaveW   = 50
	settingsSaveH   = 22

	quotaMenuSettings = 4101
	quotaMenuHide     = 4102
	quotaMenuExit     = 4103
)

func main() {
	cfg, err := LoadConfig()
	if err != nil {
		walk.MsgBox(nil, appName, "读取配置失败："+err.Error(), walk.MsgBoxIconError)
		cfg = DefaultConfig()
	}
	apiKey, err := cfg.APIKey()
	if err != nil {
		walk.MsgBox(nil, appName, "解密访问密钥失败，请重新设置。", walk.MsgBoxIconWarning)
		apiKey = ""
		cfg.EncryptedAPIKey = ""
		_ = SaveConfig(cfg)
	}

	app := &App{
		cfg:     cfg,
		apiKey:  apiKey,
		client:  NewAPIClient(),
		display: DisplayNoAPIKey(),
	}
	if err := app.Run(); err != nil {
		walk.MsgBox(nil, appName, err.Error(), walk.MsgBoxIconError)
	}
}

func (a *App) Run() error {
	var err error
	a.mw, err = walk.NewMainWindow()
	if err != nil {
		return err
	}
	a.mw.SetTitle(appName)
	a.mw.SetVisible(false)

	if err := a.createQuotaWindow(); err != nil {
		return err
	}
	if err := a.createToastWindow(); err != nil {
		return err
	}
	if err := a.createTipWindow(); err != nil {
		return err
	}
	if err := a.createSettingsWindow(); err != nil {
		return err
	}
	if err := a.createTray(); err != nil {
		return err
	}
	defer a.dispose()

	a.running = true
	a.showQuotaWindow(true)
	a.showStartupTip()
	if strings.TrimSpace(a.apiKey) == "" {
		a.applyDisplay(DisplayNoAPIKey())
		a.showSettings()
	} else {
		a.refreshAsync()
	}
	go a.refreshLoop()
	go a.iconAnimationLoop()

	a.mw.Run()
	return nil
}

func (a *App) createQuotaWindow() error {
	w, err := NewLayeredWindow(appName+" 额度", quotaWidth, quotaHeight)
	if err != nil {
		return err
	}
	a.quotaWindow = w
	w.render = func() *image.RGBA {
		a.mu.Lock()
		state := a.display
		phase := a.iconPhase
		fetching := a.fetching
		collapsed := a.quotaCollapsed
		a.mu.Unlock()
		if collapsed {
			return renderer().RenderCollapsed(state)
		}
		return renderer().RenderQuota(state, phase, fetching)
	}
	w.mouseDown = func(x, y int) bool {
		if pointInRectInt(x, y, quotaRefreshX, quotaRefreshY, quotaRefreshSize, quotaRefreshSize) {
			a.refreshAsync()
			return true
		}
		return false
	}
	w.mouseRightUp = func(x, y int) bool {
		a.showQuotaContextMenu()
		return true
	}
	w.mouseDouble = func(x, y int) bool {
		a.toggleQuotaCollapsed()
		return true
	}
	return nil
}

func (a *App) createToastWindow() error {
	w, err := NewLayeredWindow(appName+" 提示", toastWidth, toastHeight)
	if err != nil {
		return err
	}
	a.toastWindow = w
	w.render = func() *image.RGBA {
		a.mu.Lock()
		msg := a.quotaMsg
		state := a.display
		a.mu.Unlock()
		return renderer().RenderToast(msg, state)
	}
	return nil
}

func (a *App) createTipWindow() error {
	w, err := NewLayeredWindow(appName+" 操作提示", tipWidth, tipHeight)
	if err != nil {
		return err
	}
	a.tipWindow = w
	w.render = func() *image.RGBA {
		a.mu.Lock()
		msg := a.tipMsg
		a.mu.Unlock()
		return renderer().RenderTip(msg)
	}
	return nil
}

func (a *App) createSettingsWindow() error {
	w, err := NewLayeredWindow(appName+" 设置", settingsWidth, settingsHeight)
	if err != nil {
		return err
	}
	a.settingsWindow = w
	w.render = func() *image.RGBA {
		a.mu.Lock()
		input := a.settingsInput
		msg := a.settingsMsg
		focused := a.settingsFocus
		selected := a.settingsSelect
		a.mu.Unlock()
		return renderer().RenderSettings(input, msg, focused, selected)
	}
	w.mouseDown = func(x, y int) bool {
		switch {
		case pointInRectInt(x, y, settingsWidth-36, 18, 23, 23):
			a.settingsWindow.Hide()
			return true
		case pointInRectInt(x, y, settingsInputX, settingsInputY, settingsInputW, settingsInputH):
			a.mu.Lock()
			a.settingsFocus = true
			a.settingsSelect = false
			a.settingsMsg = ""
			a.mu.Unlock()
			win.SetFocus(a.settingsWindow.Handle())
			_ = a.settingsWindow.Update()
			return true
		case pointInRectInt(x, y, settingsGetX, settingsGetY, settingsGetW, settingsGetH):
			a.openAPIKeyPage()
			return true
		case pointInRectInt(x, y, settingsCancelX, settingsCancelY, settingsCancelW, settingsCancelH):
			a.settingsWindow.Hide()
			return true
		case pointInRectInt(x, y, settingsSaveX, settingsSaveY, settingsSaveW, settingsSaveH):
			a.saveSettingsFromCustomWindow()
			return true
		}
		a.mu.Lock()
		a.settingsFocus = false
		a.settingsSelect = false
		a.mu.Unlock()
		_ = a.settingsWindow.Update()
		return false
	}
	w.mouseDouble = func(x, y int) bool {
		if pointInRectInt(x, y, settingsInputX, settingsInputY, settingsInputW, settingsInputH) {
			a.mu.Lock()
			a.settingsFocus = true
			a.settingsSelect = strings.TrimSpace(a.settingsInput) != ""
			a.settingsMsg = ""
			a.mu.Unlock()
			win.SetFocus(a.settingsWindow.Handle())
			_ = a.settingsWindow.Update()
			return true
		}
		return false
	}
	w.keyDown = func(vk uintptr) bool {
		a.mu.Lock()
		focused := a.settingsFocus
		selected := a.settingsSelect
		a.mu.Unlock()
		if !focused {
			return false
		}
		switch vk {
		case win.VK_ESCAPE:
			a.settingsWindow.Hide()
			return true
		case win.VK_RETURN:
			a.saveSettingsFromCustomWindow()
			return true
		case win.VK_BACK:
			a.mu.Lock()
			if a.settingsSelect {
				a.settingsInput = ""
				a.settingsSelect = false
				a.settingsMsg = ""
			} else if len([]rune(a.settingsInput)) > 0 {
				r := []rune(a.settingsInput)
				a.settingsInput = string(r[:len(r)-1])
				a.settingsMsg = ""
			}
			a.mu.Unlock()
			_ = a.settingsWindow.Update()
			return true
		case win.VK_DELETE:
			if selected {
				a.mu.Lock()
				a.settingsInput = ""
				a.settingsSelect = false
				a.settingsMsg = ""
				a.mu.Unlock()
				_ = a.settingsWindow.Update()
			}
			return true
		case uintptr('A'):
			if win.GetKeyState(win.VK_CONTROL) < 0 {
				a.mu.Lock()
				a.settingsSelect = strings.TrimSpace(a.settingsInput) != ""
				a.settingsMsg = ""
				a.mu.Unlock()
				_ = a.settingsWindow.Update()
				return true
			}
		case uintptr('V'):
			if win.GetKeyState(win.VK_CONTROL) < 0 {
				if text := readClipboardText(a.settingsWindow.Handle()); text != "" {
					a.mu.Lock()
					if a.settingsSelect {
						a.settingsInput = strings.TrimSpace(text)
					} else {
						a.settingsInput += strings.TrimSpace(text)
					}
					a.settingsSelect = false
					a.settingsMsg = ""
					a.mu.Unlock()
					_ = a.settingsWindow.Update()
				}
				return true
			}
		}
		return false
	}
	w.charInput = func(r rune) bool {
		a.mu.Lock()
		focused := a.settingsFocus
		a.mu.Unlock()
		if !focused {
			return false
		}
		if r < 32 || r == 127 {
			return true
		}
		a.mu.Lock()
		if len([]rune(a.settingsInput)) < 500 {
			if a.settingsSelect {
				a.settingsInput = string(r)
			} else {
				a.settingsInput += string(r)
			}
			a.settingsSelect = false
			a.settingsMsg = ""
		}
		a.mu.Unlock()
		_ = a.settingsWindow.Update()
		return true
	}
	w.focusChange = func(focused bool) {
		a.mu.Lock()
		a.settingsFocus = focused
		if !focused {
			a.settingsSelect = false
		}
		a.mu.Unlock()
		_ = a.settingsWindow.Update()
	}
	return nil
}

func (a *App) createTray() error {
	var err error
	a.tray, err = walk.NewNotifyIcon(a.mw)
	if err != nil {
		return err
	}

	show := walk.NewAction()
	_ = show.SetText("显示额度")
	show.Triggered().Attach(func() { a.showQuotaWindow(true) })
	_ = a.tray.ContextMenu().Actions().Add(show)

	settings := walk.NewAction()
	_ = settings.SetText("设置访问密钥")
	settings.Triggered().Attach(func() { a.showSettings() })
	_ = a.tray.ContextMenu().Actions().Add(settings)

	refresh := walk.NewAction()
	_ = refresh.SetText("刷新")
	refresh.Triggered().Attach(func() { a.refreshAsync() })
	_ = a.tray.ContextMenu().Actions().Add(refresh)
	_ = a.tray.ContextMenu().Actions().Add(walk.NewSeparatorAction())

	exit := walk.NewAction()
	_ = exit.SetText("退出")
	exit.Triggered().Attach(func() {
		a.running = false
		walk.App().Exit(0)
	})
	_ = a.tray.ContextMenu().Actions().Add(exit)

	a.tray.MouseUp().Attach(func(_, _ int, button walk.MouseButton) {
		if button == walk.LeftButton {
			a.showQuotaWindow(true)
		}
	})

	a.applyDisplay(a.display)
	return a.tray.SetVisible(true)
}

func (a *App) showQuotaWindow(focus bool) {
	if a.quotaWindow == nil {
		return
	}
	a.mu.Lock()
	positioned := a.quotaPositioned
	if !a.quotaPositioned {
		a.quotaPositioned = true
	}
	a.mu.Unlock()
	if !positioned {
		x, y := topRightPosition(quotaWidth, quotaHeight, 18)
		a.quotaWindow.SetPosition(x, y)
	}
	a.quotaWindow.Show()
	a.quotaWindow.BringToFront(focus)
}

func (a *App) toggleQuotaCollapsed() {
	if a.quotaWindow == nil {
		return
	}
	x, y, currentW, _ := a.quotaWindow.Bounds()
	a.mu.Lock()
	collapsed := !a.quotaCollapsed
	a.quotaCollapsed = collapsed
	a.mu.Unlock()
	newW := collapsedWidth
	newH := collapsedHeight
	if !collapsed {
		newW = quotaWidth
		newH = quotaHeight
	}
	right := x + currentW
	nx := right - newW
	if nx < 8 {
		nx = x
	}
	a.quotaWindow.SetBounds(nx, y, newW, newH)
	a.quotaWindow.BringToFront(false)
}

func (a *App) showStartupTip() {
	if a.tipWindow == nil || a.quotaWindow == nil {
		return
	}
	a.mu.Lock()
	a.tipMsg = "提示：双击可展开/收起"
	a.tipUntil = time.Now().Add(10 * time.Second)
	a.mu.Unlock()
	x, y := a.tipPosition()
	a.tipWindow.SetPosition(x, y)
	a.tipWindow.Show()
	a.tipWindow.BringToFront(false)
	_ = a.tipWindow.Update()
}

func (a *App) showQuotaToast(msg string, duration time.Duration) {
	if a.toastWindow == nil || strings.TrimSpace(msg) == "" {
		return
	}
	a.mu.Lock()
	a.quotaMsg = msg
	a.quotaMsgUntil = time.Now().Add(duration)
	a.mu.Unlock()
	x, y := a.toastPosition()
	a.toastWindow.SetPosition(x, y)
	a.toastWindow.Show()
	a.toastWindow.BringToFront(false)
	_ = a.toastWindow.Update()
}

func (a *App) toastPosition() (int, int) {
	if a.quotaWindow == nil {
		return topRightPosition(toastWidth, toastHeight, 18)
	}
	qx, qy := a.quotaWindow.Position()
	a.mu.Lock()
	collapsed := a.quotaCollapsed
	a.mu.Unlock()
	qw := quotaWidth
	if collapsed {
		qw = collapsedWidth
	}
	qh := quotaHeight
	if collapsed {
		qh = collapsedHeight
	}
	x := qx + qw - toastWidth - 12
	y := qy + qh + 6
	return clampWindowPosition(x, y, toastWidth, toastHeight, 4)
}

func (a *App) tipPosition() (int, int) {
	if a.quotaWindow == nil {
		return topRightPosition(tipWidth, tipHeight, 18)
	}
	qx, qy := a.quotaWindow.Position()
	a.mu.Lock()
	collapsed := a.quotaCollapsed
	a.mu.Unlock()
	qw := quotaWidth
	qh := quotaHeight
	if collapsed {
		qw = collapsedWidth
		qh = collapsedHeight
	}
	x := qx + qw - tipWidth - 8
	y := qy + qh + 6
	return clampWindowPosition(x, y, tipWidth, tipHeight, 4)
}

func (a *App) syncFloatingOverlays() {
	if a.toastWindow != nil && a.toastWindow.Visible() {
		if time.Now().After(a.quotaMsgUntil) {
			a.toastWindow.Hide()
		} else {
			x, y := a.toastPosition()
			a.toastWindow.SetPosition(x, y)
			_ = a.toastWindow.Update()
		}
	}
	if a.tipWindow != nil && a.tipWindow.Visible() {
		if time.Now().After(a.tipUntil) {
			a.tipWindow.Hide()
		} else {
			x, y := a.tipPosition()
			a.tipWindow.SetPosition(x, y)
			_ = a.tipWindow.Update()
		}
	}
}

func (a *App) showSettings() {
	if a.settingsWindow == nil {
		return
	}
	a.mu.Lock()
	a.settingsInput = a.apiKey
	a.settingsMsg = ""
	a.settingsFocus = true
	a.settingsSelect = false
	a.mu.Unlock()
	x, y := settingsPosition(a.quotaWindow, settingsWidth, settingsHeight, 18)
	a.settingsWindow.SetPosition(x, y)
	a.settingsWindow.Show()
	a.settingsWindow.BringToFront(true)
	_ = a.settingsWindow.Update()
}

func (a *App) showQuotaContextMenu() {
	if a.quotaWindow == nil {
		return
	}
	menu := win.CreatePopupMenu()
	if menu == 0 {
		return
	}
	defer win.DestroyMenu(menu)

	insertPopupMenuItem(menu, 0, quotaMenuSettings, "设置访问密钥")
	insertPopupMenuItem(menu, 1, quotaMenuHide, "隐藏浮窗")
	insertPopupSeparator(menu, 2)
	insertPopupMenuItem(menu, 3, quotaMenuExit, "退出应用")

	var pt win.POINT
	if !win.GetCursorPos(&pt) {
		return
	}
	hwnd := a.quotaWindow.Handle()
	win.SetForegroundWindow(hwnd)
	cmd := win.TrackPopupMenu(
		menu,
		win.TPM_RIGHTBUTTON|win.TPM_RETURNCMD|win.TPM_NOANIMATION,
		pt.X,
		pt.Y,
		0,
		hwnd,
		nil,
	)
	switch cmd {
	case quotaMenuSettings:
		a.showSettings()
	case quotaMenuHide:
		a.quotaWindow.Hide()
	case quotaMenuExit:
		a.running = false
		walk.App().Exit(0)
	}
}

func insertPopupMenuItem(menu win.HMENU, pos uint32, id uint32, text string) {
	ptr := syscall.StringToUTF16Ptr(text)
	info := win.MENUITEMINFO{
		CbSize:     uint32(unsafe.Sizeof(win.MENUITEMINFO{})),
		FMask:      win.MIIM_ID | win.MIIM_STRING | win.MIIM_FTYPE,
		FType:      win.MFT_STRING,
		WID:        id,
		DwTypeData: ptr,
		Cch:        uint32(len([]rune(text))),
	}
	win.InsertMenuItem(menu, pos, true, &info)
}

func insertPopupSeparator(menu win.HMENU, pos uint32) {
	info := win.MENUITEMINFO{
		CbSize: uint32(unsafe.Sizeof(win.MENUITEMINFO{})),
		FMask:  win.MIIM_FTYPE,
		FType:  win.MFT_SEPARATOR,
	}
	win.InsertMenuItem(menu, pos, true, &info)
}

func (a *App) saveSettingsFromCustomWindow() {
	a.mu.Lock()
	input := strings.TrimSpace(a.settingsInput)
	cfg := a.cfg
	a.mu.Unlock()

	cfg.BaseURL = DefaultConfig().BaseURL
	if _, err := url.ParseRequestURI(cfg.BaseURL); err != nil {
		a.setSettingsMessage("服务地址配置异常。")
		return
	}
	if err := cfg.SetAPIKey(input); err != nil {
		a.setSettingsMessage("保存访问密钥失败。")
		return
	}
	cfg.normalize()
	if err := SaveConfig(cfg); err != nil {
		a.setSettingsMessage("写入配置失败。")
		return
	}
	apiKey, err := cfg.APIKey()
	if err != nil {
		a.setSettingsMessage("读取访问密钥失败。")
		return
	}
	a.mu.Lock()
	a.cfg = cfg
	a.apiKey = apiKey
	a.settingsMsg = "已保存，正在刷新。"
	a.mu.Unlock()
	_ = a.settingsWindow.Update()
	a.refreshAsync()
	time.AfterFunc(700*time.Millisecond, func() {
		if a.mw != nil {
			a.mw.Synchronize(func() {
				if a.settingsWindow != nil {
					a.settingsWindow.Hide()
				}
			})
		}
	})
}

func (a *App) setSettingsMessage(msg string) {
	a.mu.Lock()
	a.settingsMsg = msg
	a.mu.Unlock()
	if a.settingsWindow != nil {
		_ = a.settingsWindow.Update()
	}
}

func (a *App) openAPIKeyPage() {
	verb := syscall.StringToUTF16Ptr("open")
	url := syscall.StringToUTF16Ptr("https://meapi.space")
	if !win.ShellExecute(a.settingsWindow.Handle(), verb, url, nil, nil, win.SW_SHOWNORMAL) {
		a.setSettingsMessage("打开网页失败。")
	}
}

func (a *App) refreshLoop() {
	for a.running {
		a.mu.Lock()
		interval := a.cfg.CheckIntervalSeconds
		a.mu.Unlock()
		if interval < 15 {
			interval = DefaultConfig().CheckIntervalSeconds
		}

		timer := time.NewTimer(time.Duration(interval) * time.Second)
		<-timer.C
		if !a.running {
			return
		}
		a.refreshAsync()
	}
}

func (a *App) iconAnimationLoop() {
	ticker := time.NewTicker(33 * time.Millisecond)
	defer ticker.Stop()
	tick := 0
	for a.running {
		<-ticker.C
		if !a.running {
			return
		}
		a.mu.Lock()
		a.iconPhase = math.Mod(float64(time.Now().UnixNano())/float64(time.Second)/2.4, 1)
		display := a.display
		a.mu.Unlock()
		if a.mw != nil {
			a.mw.Synchronize(func() {
				tick++
				if tick%4 == 0 {
					a.updateTrayIcon(display)
				}
				if a.quotaWindow != nil && a.quotaWindow.Visible() {
					_ = a.quotaWindow.Update()
				}
				if a.settingsWindow != nil && a.settingsWindow.Visible() {
					_ = a.settingsWindow.Update()
				}
				a.syncFloatingOverlays()
			})
		}
	}
}

func (a *App) refreshAsync() {
	a.mu.Lock()
	if a.fetching {
		a.mu.Unlock()
		a.showQuotaToast("正在刷新", 1200*time.Millisecond)
		return
	}
	apiKey := strings.TrimSpace(a.apiKey)
	cfg := a.cfg
	if apiKey == "" {
		a.fetching = false
		a.display = DisplayNoAPIKey()
		display := a.display
		a.mu.Unlock()
		a.showQuotaToast("等待密钥", 1600*time.Millisecond)
		a.mw.Synchronize(func() { a.applyDisplay(display) })
		return
	}
	a.fetching = true
	a.mu.Unlock()
	if a.tipWindow == nil || !a.tipWindow.Visible() {
		a.showQuotaToast("刷新中", 1800*time.Millisecond)
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		resp, err := a.client.FetchUsage(ctx, cfg.BaseURL, apiKey)
		fetchedAt := time.Now()

		a.mw.Synchronize(func() {
			a.mu.Lock()
			a.fetching = false
			var display DisplayState
			toastMsg := "刷新成功"
			toastDuration := 1800 * time.Millisecond
			if err != nil {
				display = DisplayForError(err)
				toastMsg = "刷新失败"
				toastDuration = 2400 * time.Millisecond
			} else {
				display = DisplayFromUsage(resp, cfg, fetchedAt)
			}
			a.display = display
			a.mu.Unlock()
			a.showQuotaToast(toastMsg, toastDuration)
			a.applyDisplay(display)
			a.maybeNotifyLowBalance(display)
		})
	}()
}

func (a *App) applyDisplay(state DisplayState) {
	if a.quotaWindow != nil {
		_ = a.quotaWindow.Update()
	}
	if a.tray != nil {
		_ = a.tray.SetToolTip(state.Tooltip)
		a.updateTrayIcon(state)
	}
}

func (a *App) updateTrayIcon(state DisplayState) {
	if a.tray == nil {
		return
	}
	a.mu.Lock()
	phase := a.iconPhase
	a.mu.Unlock()
	icon, err := iconForDisplay(state, a.tray.DPI(), phase)
	if err != nil {
		return
	}
	old := a.icon
	a.icon = icon
	_ = a.tray.SetIcon(icon)
	if old != nil {
		old.Dispose()
	}
}

func (a *App) maybeNotifyLowBalance(state DisplayState) {
	if a.tray == nil {
		return
	}
	if state.LowBalance && !a.wasLow {
		_ = a.tray.ShowWarning(appName+" 余额不足", state.RemainingText+"，已低于提醒阈值。")
	}
	a.wasLow = state.LowBalance
}

func (a *App) dispose() {
	a.running = false
	if a.tray != nil {
		_ = a.tray.SetVisible(false)
		_ = a.tray.Dispose()
	}
	if a.icon != nil {
		a.icon.Dispose()
	}
	if a.settingsWindow != nil {
		a.settingsWindow.Destroy()
	}
	if a.toastWindow != nil {
		a.toastWindow.Destroy()
	}
	if a.tipWindow != nil {
		a.tipWindow.Destroy()
	}
	if a.quotaWindow != nil {
		a.quotaWindow.Destroy()
	}
}

func topRightPosition(width, height, margin int) (int, int) {
	screenW := int(win.GetSystemMetrics(win.SM_CXSCREEN))
	screenH := int(win.GetSystemMetrics(win.SM_CYSCREEN))
	x := screenW - width - margin
	y := margin
	if x < margin {
		x = margin
	}
	if y+height > screenH-margin {
		y = screenH - height - margin
	}
	return x, y
}

func settingsPosition(quota *LayeredWindow, width, height, margin int) (int, int) {
	if quota != nil && quota.Visible() {
		qx, qy := quota.Position()
		qw := quotaWidth
		qh := quotaHeight
		if quota.width == collapsedWidth {
			qw = collapsedWidth
			qh = collapsedHeight
		}
		x := qx - (width - qw)
		y := qy + qh + 8
		return clampWindowPosition(x, y, width, height, margin)
	}
	return topRightPosition(width, height, margin)
}

func clampWindowPosition(x, y, width, height, margin int) (int, int) {
	screenW := int(win.GetSystemMetrics(win.SM_CXSCREEN))
	screenH := int(win.GetSystemMetrics(win.SM_CYSCREEN))
	if x < margin {
		x = margin
	}
	if y < margin {
		y = margin
	}
	if x+width > screenW-margin {
		x = screenW - width - margin
	}
	if y+height > screenH-margin {
		y = screenH - height - margin
	}
	if x < margin {
		x = margin
	}
	if y < margin {
		y = margin
	}
	return x, y
}

func pointInRectInt(x, y, rx, ry, rw, rh int) bool {
	return x >= rx && x < rx+rw && y >= ry && y < ry+rh
}

func readClipboardText(hwnd win.HWND) string {
	if !win.IsClipboardFormatAvailable(win.CF_UNICODETEXT) {
		return ""
	}
	if !win.OpenClipboard(hwnd) {
		return ""
	}
	defer win.CloseClipboard()
	h := win.GetClipboardData(win.CF_UNICODETEXT)
	if h == 0 {
		return ""
	}
	ptr := win.GlobalLock(win.HGLOBAL(h))
	if ptr == nil {
		return ""
	}
	defer win.GlobalUnlock(win.HGLOBAL(h))
	u16 := unsafe.Slice((*uint16)(ptr), 4096)
	n := 0
	for n < len(u16) && u16[n] != 0 {
		n++
	}
	return string(utf16.Decode(u16[:n]))
}

func amountText(state DisplayState) string {
	if state.Unlimited {
		return "无限制"
	}
	if state.RemainingUSD == nil {
		return "--"
	}
	return formatUSD(*state.RemainingUSD)
}

func percentText(state DisplayState) string {
	if state.Unlimited {
		return "100%"
	}
	if !state.HasData && state.RemainingUSD == nil {
		return "--"
	}
	return fmt.Sprintf("%d%%", int(clamp01(state.Progress)*100+0.5))
}

func metricValue(text, prefix string) string {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, prefix) {
		return strings.TrimSpace(strings.TrimPrefix(text, prefix))
	}
	if text == "" {
		return "--"
	}
	return text
}
