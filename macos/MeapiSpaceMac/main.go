package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// Wails uses Go's `embed` package to embed the frontend files into the binary.
// Any files in the frontend/dist folder will be embedded into the binary and
// made available to the frontend.
// See https://pkg.go.dev/embed for more information.

//go:embed all:frontend/dist
var assets embed.FS

func init() {
	application.RegisterEvent[FrontendState]("quota:update")
}

func main() {
	quota := NewQuotaService()

	app := application.New(application.Options{
		Name:        appName,
		Description: "MeapiSpace macOS 额度小组件",
		Services: []application.Service{
			application.NewService(quota),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
	})

	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:           "quota",
		Title:          appName,
		Width:          expandedWidth,
		Height:         expandedHeight,
		AlwaysOnTop:    true,
		DisableResize:  true,
		Frameless:      true,
		Hidden:         true,
		BackgroundType: application.BackgroundTypeTransparent,
		BackgroundColour: application.NewRGBA(
			0,
			0,
			0,
			0,
		),
		MinimiseButtonState:   application.ButtonHidden,
		MaximiseButtonState:   application.ButtonHidden,
		CloseButtonState:      application.ButtonHidden,
		FullscreenButtonState: application.ButtonHidden,
		Mac: application.MacWindow{
			Backdrop:                application.MacBackdropTransparent,
			TitleBar:                application.MacTitleBarHidden,
			InvisibleTitleBarHeight: expandedHeight,
			WindowLevel:             application.MacWindowLevelStatus,
			CollectionBehavior: application.MacWindowCollectionBehaviorCanJoinAllSpaces |
				application.MacWindowCollectionBehaviorTransient |
				application.MacWindowCollectionBehaviorFullScreenAuxiliary,
		},
		URL: "/",
	})

	menu := application.NewMenu()
	menu.Add("显示额度").OnClick(func(*application.Context) { quota.ShowMain() })
	menu.Add("设置访问密钥").OnClick(func(*application.Context) { quota.OpenSettings() })
	menu.Add("刷新").OnClick(func(*application.Context) { go quota.Refresh() })
	menu.AddSeparator()
	menu.Add("退出").OnClick(func(*application.Context) { app.Quit() })

	tray := app.SystemTray.New().
		SetIcon(trayIconPNG(DisplayNoAPIKey(), 0)).
		SetMenu(menu).
		AttachWindow(window).
		WindowOffset(6).
		OnClick(func() { quota.ShowMain() })
	tray.SetTooltip(appName + " 额度")

	quota.attach(app, window, tray)
	quota.start()

	err := app.Run()
	if err != nil {
		log.Fatal(err)
	}
}
