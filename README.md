# MeapiSpace

Windows tray widget for displaying the current MeapiSpace balance/remaining quota.

## Usage

Build:

```powershell
.\build.ps1
```

Run:

```powershell
.\dist\MeapiSpace.exe
```

The first launch asks for your access key. The widget queries:

```text
https://meapi.space/v1/usage
```

with:

```text
Authorization: Bearer <your-api-key>
```

The access key is encrypted with Windows DPAPI and stored under the current user's
config directory.

`build.ps1` embeds `app.manifest` into the executable so the Windows common
controls used by the tray UI are available at startup.
