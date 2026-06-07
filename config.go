package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	appDirName    = "MeapiSpace"
	oldAppDirName = "Sub2APIQuotaWidget"
)

type Config struct {
	BaseURL                string  `json:"base_url"`
	EncryptedAPIKey        string  `json:"encrypted_api_key"`
	CheckIntervalSeconds   int     `json:"check_interval_seconds"`
	LowBalanceThresholdUSD float64 `json:"low_balance_threshold_usd"`
	WalletFullReferenceUSD float64 `json:"wallet_full_reference_usd"`
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
		oldPath, oldErr := configPathForDir(oldAppDirName)
		if oldErr != nil {
			return cfg, nil
		}
		data, err = os.ReadFile(oldPath)
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		if err != nil {
			return cfg, err
		}
	} else if err != nil {
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
	return configPathForDir(appDirName)
}

func configPathForDir(name string) (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name, "config.json"), nil
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

func (c Config) APIKey() (string, error) {
	if strings.TrimSpace(c.EncryptedAPIKey) == "" {
		return "", nil
	}
	return decryptString(c.EncryptedAPIKey)
}

func (c *Config) SetAPIKey(apiKey string) error {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		c.EncryptedAPIKey = ""
		return nil
	}
	encrypted, err := encryptString(apiKey)
	if err != nil {
		return err
	}
	c.EncryptedAPIKey = encrypted
	return nil
}

func encryptString(value string) (string, error) {
	plain := []byte(value)
	protected, err := cryptProtect(plain)
	if err != nil {
		return "", fmt.Errorf("加密访问密钥：%w", err)
	}
	return base64.StdEncoding.EncodeToString(protected), nil
}

func decryptString(value string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return "", err
	}
	plain, err := cryptUnprotect(raw)
	if err != nil {
		return "", fmt.Errorf("解密访问密钥：%w", err)
	}
	return string(plain), nil
}

func cryptProtect(data []byte) ([]byte, error) {
	in := blobFromBytes(data)
	var out windows.DataBlob
	err := windows.CryptProtectData(
		&in,
		nil,
		nil,
		0,
		nil,
		windows.CRYPTPROTECT_UI_FORBIDDEN,
		&out,
	)
	if err != nil {
		return nil, err
	}
	defer freeBlob(out)
	return bytesFromBlob(out), nil
}

func cryptUnprotect(data []byte) ([]byte, error) {
	in := blobFromBytes(data)
	var out windows.DataBlob
	err := windows.CryptUnprotectData(
		&in,
		nil,
		nil,
		0,
		nil,
		windows.CRYPTPROTECT_UI_FORBIDDEN,
		&out,
	)
	if err != nil {
		return nil, err
	}
	defer freeBlob(out)
	return bytesFromBlob(out), nil
}

func blobFromBytes(data []byte) windows.DataBlob {
	if len(data) == 0 {
		return windows.DataBlob{}
	}
	return windows.DataBlob{
		Size: uint32(len(data)),
		Data: &data[0],
	}
}

func bytesFromBlob(blob windows.DataBlob) []byte {
	if blob.Size == 0 || blob.Data == nil {
		return nil
	}
	return append([]byte(nil), unsafe.Slice(blob.Data, blob.Size)...)
}

func freeBlob(blob windows.DataBlob) {
	if blob.Data == nil {
		return
	}
	_, _ = windows.LocalFree(windows.Handle(uintptr(unsafe.Pointer(blob.Data))))
}
