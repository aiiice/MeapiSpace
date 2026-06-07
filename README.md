# MeapiSpace

MeapiSpace 桌面额度小组件。

## Windows 本地构建

```powershell
.\build.ps1
```

运行：

```powershell
.\dist\MeapiSpace.exe
```

首次启动会让你输入访问秘钥。小组件会请求：

```text
https://meapi.space/v1/usage
```

请求头：

```text
Authorization: Bearer <your-api-key>
```

访问秘钥会用 Windows DPAPI 加密后保存在当前用户的配置目录。

## GitHub 自动打包

仓库里的 `Build Release` workflow 会自动构建：

- Windows：`MeapiSpace.exe`、`MeapiSpace-Windows-x64.zip`
- macOS：`MeapiSpace-macOS-universal.zip`、`MeapiSpace-macOS-universal.dmg`

在 GitHub 的 Actions 页面手动运行 `Build Release`，填入版本号，例如 `v0.1.0`，即可生成并发布 Release。

也可以推送 tag 触发：

```powershell
git tag v0.1.0
git push origin v0.1.0
```
