# MeapiSpace macOS

这是 MeapiSpace 额度小组件的 macOS 版工程，使用 Wails v3 和系统 WebView。

## Windows 上构建

需要先安装 Docker Desktop 并启动。

```powershell
cd desktop\quota-widget-mac\MeapiSpaceMac
powershell -ExecutionPolicy Bypass -File .\build-macos.ps1
```

产物：

```text
bin\MeapiSpace.app
```

Windows 交叉构建出来的 `.app` 默认不会做 Apple Developer ID 签名和公证，发给别人安装时 macOS 可能会提示来源未知。

## GitHub Actions 构建

把仓库推到 GitHub 后，在 Actions 页面手动运行 `Build MeapiSpace macOS`。

产物会包含：

```text
MeapiSpace-macOS.zip
MeapiSpace-macOS.dmg
```

这个流程跑在 macOS runner 上，会做 ad-hoc 签名，但仍不是 Apple Developer ID 签名。正式分发需要接 Apple 开发者证书和 notarization。

## 本机 macOS 构建

```bash
cd desktop/quota-widget-mac/MeapiSpaceMac
go install github.com/wailsapp/wails/v3/cmd/wails3@latest
wails3 task darwin:package:universal
open bin/MeapiSpace.app
```
