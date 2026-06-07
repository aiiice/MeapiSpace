package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/zalando/go-keyring"
)

const (
	expandedWidth   = 270
	expandedHeight  = 84
	collapsedWidth  = 42
	collapsedHeight = 104
	settingsWidth   = 336
	settingsHeight  = 170

	keyringService = "MeapiSpace"
	keyringAccount = "api-key"
)

type Config struct {
	BaseURL                string  `json:"base_url"`
	CheckIntervalSeconds   int     `json:"check_interval_seconds"`
	LowBalanceThresholdUSD float64 `json:"low_balance_threshold_usd"`
	WalletFullReferenceUSD float64 `json:"wallet_full_reference_usd"`
}

type QuotaService struct {
	mu        sync.Mutex
	cfg       Config
	apiKey    string
	client    *APIClient
	display   DisplayState
	fetching  bool
	collapsed bool
	settings  bool
	phase     float64

	app    *application.App
	window application.Window
	tray   *application.SystemTray
}

func NewQuotaService() *QuotaService {
	cfg, err := LoadConfig()
	if err != nil {
		cfg = DefaultConfig()
	}
	apiKey, err := loadAPIKey()
	if err != nil {
		apiKey = ""
	}
	return &QuotaService{
		cfg:     cfg,
		apiKey:  apiKey,
		client:  NewAPIClient(),
		display: DisplayNoAPIKey(),
	}
}

func DefaultConfig() Config {
	return Config{
		BaseURL:                "https://meapi.space",
		CheckIntervalSeconds:   60,
		LowBalanceThresholdUSD: 5,
		WalletFullReferenceUSD: 20,
	}
}

func LoadConfig() (Config, error) {
	cfg := DefaultConfig()
	path, err := configPath()
	if err != nil {
		return cfg, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	cfg.normalize()
	return cfg, nil
}

func SaveConfig(cfg Config) error {
	cfg.normalize()
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func configPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, appName, "config.json"), nil
}

func (c *Config) normalize() {
	c.BaseURL = strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if c.BaseURL == "" {
		c.BaseURL = DefaultConfig().BaseURL
	}
	if c.CheckIntervalSeconds < 15 {
		c.CheckIntervalSeconds = DefaultConfig().CheckIntervalSeconds
	}
	if c.LowBalanceThresholdUSD <= 0 {
		c.LowBalanceThresholdUSD = DefaultConfig().LowBalanceThresholdUSD
	}
	if c.WalletFullReferenceUSD <= c.LowBalanceThresholdUSD {
		c.WalletFullReferenceUSD = DefaultConfig().WalletFullReferenceUSD
	}
}

func (s *QuotaService) attach(app *application.App, window application.Window, tray *application.SystemTray) {
	s.app = app
	s.window = window
	s.tray = tray
	s.updateTray()
}

func (s *QuotaService) start() {
	go s.startup()
	go s.refreshLoop()
	go s.trayAnimationLoop()
}

func (s *QuotaService) startup() {
	time.Sleep(650 * time.Millisecond)
	for i := 0; i < 8; i++ {
		s.ShowMain()
		if s.window != nil && s.window.IsVisible() {
			break
		}
		time.Sleep(150 * time.Millisecond)
	}
	if strings.TrimSpace(s.apiKey) != "" {
		s.refreshNow("刷新成功")
	}
}

func (s *QuotaService) Initial() FrontendState {
	return s.frontendState("")
}

func (s *QuotaService) Refresh() FrontendState {
	return s.refreshNow("刷新成功")
}

func (s *QuotaService) SaveAPIKey(apiKey string) FrontendState {
	apiKey = strings.TrimSpace(apiKey)
	if err := saveAPIKey(apiKey); err != nil {
		s.mu.Lock()
		s.display = DisplayForError(fmt.Errorf("保存访问密钥失败：%w", err))
		s.fetching = false
		s.mu.Unlock()
		return s.frontendState("保存失败")
	}
	s.mu.Lock()
	s.apiKey = apiKey
	s.display = DisplayNoAPIKey()
	s.mu.Unlock()
	if apiKey == "" {
		s.emitUpdate("已清空密钥")
		s.updateTray()
		return s.frontendState("已清空密钥")
	}
	return s.refreshNow("已保存")
}

func (s *QuotaService) SetCollapsed(collapsed bool) {
	s.mu.Lock()
	if s.collapsed == collapsed && !s.settings {
		s.mu.Unlock()
		return
	}
	oldW := expandedWidth
	if s.collapsed {
		oldW = collapsedWidth
	}
	s.collapsed = collapsed
	s.settings = false
	s.mu.Unlock()
	s.resizeWindow(collapsed, false, oldW)
}

func (s *QuotaService) SetSettingsOpen(open bool) {
	s.mu.Lock()
	oldW := expandedWidth
	if s.collapsed {
		oldW = collapsedWidth
	}
	if s.settings {
		oldW = settingsWidth
	}
	s.settings = open
	if open {
		s.collapsed = false
	}
	collapsed := s.collapsed
	s.mu.Unlock()
	s.resizeWindow(collapsed, open, oldW)
}

func (s *QuotaService) ShowMain() {
	if s.tray != nil {
		s.tray.ShowWindow()
		return
	}
	if s.window != nil {
		s.window.Show().Focus()
	}
}

func (s *QuotaService) OpenSettings() {
	if s.window == nil {
		return
	}
	s.SetSettingsOpen(true)
	if s.tray != nil {
		s.tray.ShowWindow()
	} else {
		s.window.Show().Focus()
	}
	s.window.ExecJS("window.meapiOpenSettings && window.meapiOpenSettings()")
}

func (s *QuotaService) HideWindow() {
	if s.window != nil {
		s.window.Hide()
	}
}

func (s *QuotaService) QuitApp() {
	if s.app != nil {
		s.app.Quit()
	}
}

func (s *QuotaService) OpenAPIKeyPage() FrontendState {
	if err := openURL("https://meapi.space"); err != nil {
		return s.frontendState("打开网页失败")
	}
	return s.frontendState("")
}

func (s *QuotaService) refreshLoop() {
	for {
		s.mu.Lock()
		interval := s.cfg.CheckIntervalSeconds
		s.mu.Unlock()
		if interval < 15 {
			interval = DefaultConfig().CheckIntervalSeconds
		}
		time.Sleep(time.Duration(interval) * time.Second)
		if strings.TrimSpace(s.apiKey) != "" {
			s.refreshNow("刷新成功")
		}
	}
}

func (s *QuotaService) refreshNow(message string) FrontendState {
	s.mu.Lock()
	if s.fetching {
		state := s.frontendStateLocked("正在刷新")
		s.mu.Unlock()
		return state
	}
	apiKey := strings.TrimSpace(s.apiKey)
	cfg := s.cfg
	if apiKey == "" {
		s.display = DisplayNoAPIKey()
		state := s.frontendStateLocked("等待密钥")
		s.mu.Unlock()
		s.emitState(state)
		s.updateTray()
		return state
	}
	s.fetching = true
	loadingState := s.frontendStateLocked("刷新中")
	s.mu.Unlock()
	s.emitState(loadingState)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	resp, err := s.client.FetchUsage(ctx, cfg.BaseURL, apiKey)

	s.mu.Lock()
	s.fetching = false
	if err != nil {
		s.display = DisplayForError(err)
		message = "刷新失败"
	} else {
		s.display = DisplayFromUsage(resp, cfg, time.Now())
	}
	state := s.frontendStateLocked(message)
	s.mu.Unlock()
	s.emitState(state)
	s.updateTray()
	return state
}

func (s *QuotaService) frontendState(message string) FrontendState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.frontendStateLocked(message)
}

func (s *QuotaService) frontendStateLocked(message string) FrontendState {
	state := FrontendState{
		DisplayState: s.display,
		AmountText:   amountText(s.display),
		Percent:      percentText(s.display),
		Status:       activeStatus(s.display),
		APIKey:       s.apiKey,
		Fetching:     s.fetching,
		Message:      message,
	}
	state.TodayCostText = metricValue(state.TodayCostText, "今日消耗")
	state.TodayTokenText = metricValue(state.TodayTokenText, "今日令牌")
	state.UpdatedText = compactUpdateText(state.DisplayState)
	return state
}

func (s *QuotaService) emitUpdate(message string) {
	s.emitState(s.frontendState(message))
}

func (s *QuotaService) emitState(state FrontendState) {
	if s.app != nil {
		s.app.Event.Emit("quota:update", state)
	}
}

func (s *QuotaService) resizeWindow(collapsed, settings bool, oldW int) {
	if s.window == nil {
		return
	}
	newW, newH := expandedWidth, expandedHeight
	if settings {
		newW, newH = settingsWidth, settingsHeight
	} else if collapsed {
		newW, newH = collapsedWidth, collapsedHeight
	}
	x, y := s.window.Position()
	s.window.SetSize(newW, newH)
	s.window.SetPosition(x+oldW-newW, y)
}

func (s *QuotaService) trayAnimationLoop() {
	ticker := time.NewTicker(132 * time.Millisecond)
	defer ticker.Stop()
	for range ticker.C {
		s.mu.Lock()
		s.phase = mathMod(time.Now().UnixNano(), int64(2400*time.Millisecond))
		s.mu.Unlock()
		s.updateTray()
	}
}

func (s *QuotaService) updateTray() {
	if s.tray == nil {
		return
	}
	s.mu.Lock()
	state := s.display
	phase := s.phase
	s.mu.Unlock()
	s.tray.SetIcon(trayIconPNG(state, phase))
	s.tray.SetTooltip(state.Tooltip)
}

func loadAPIKey() (string, error) {
	value, err := keyring.Get(keyringService, keyringAccount)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", nil
	}
	return value, err
}

func saveAPIKey(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		err := keyring.Delete(keyringService, keyringAccount)
		if errors.Is(err, keyring.ErrNotFound) {
			return nil
		}
		return err
	}
	return keyring.Set(keyringService, keyringAccount, value)
}

func openURL(raw string) error {
	if _, err := url.ParseRequestURI(raw); err != nil {
		return err
	}
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", raw).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", raw).Start()
	default:
		return exec.Command("xdg-open", raw).Start()
	}
}

func mathMod(v, period int64) float64 {
	if period <= 0 {
		return 0
	}
	return float64(v%period) / float64(period)
}
