<div align="center">

<img src="build/appicon.png" width="96" alt="Ghostline logo">

# Ghostline

**One-click encrypted DNS and DPI bypass for Windows.**

[![CI](https://github.com/hashcott/ghostline/actions/workflows/ci.yml/badge.svg)](https://github.com/hashcott/ghostline/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/hashcott/ghostline?include_prereleases)](https://github.com/hashcott/ghostline/releases)
[![Downloads](https://img.shields.io/github/downloads/hashcott/ghostline/total)](https://github.com/hashcott/ghostline/releases)
[![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)
![Platform](https://img.shields.io/badge/platform-Windows%2010%2F11%20x64-blue)

English · [Tiếng Việt](README.vi.md)

</div>

---

Ghostline runs a local DNS server on `127.0.0.1` / `::1`, points every network adapter at it, and forwards your queries over **DoH, DoT, DoQ or DNSCrypt** to the fastest healthy resolver. When your network blocks sites by SNI, it can also run **GoodbyeDPI**. Above all, it is built to **always give your original DNS back**, even if the app crashes or the machine loses power.

<p align="center">
  <img src="docs/screenshots/simple-en.png" height="360" alt="Simple mode">
  &nbsp;
  <img src="docs/screenshots/overview-en.png" height="360" alt="Advanced mode">
</p>

## Table of contents

- [Features](#features)
- [Screenshots](#screenshots)
- [Install](#install)
- [Usage](#usage)
- [How it works](#how-it-works)
- [Privacy](#privacy)
- [Building from source](#building-from-source)
- [Project layout](#project-layout)
- [Contributing](#contributing)
- [Security](#security)
- [License and credits](#license-and-credits)

## Features

- **Encrypted DNS for the whole system:** DoH, DoT, DoQ and DNSCrypt upstreams, powered by [AdGuard dnsproxy](https://github.com/AdguardTeam/dnsproxy).
- **Automatic server choice:** scans resolvers in parallel, rejects poisoned answers, and remembers the best servers per network.
- **Never lose the internet:** each adapter's original DNS is snapshotted before any change, with four recovery layers: clean disconnect, a watchdog process, restore on next launch, and a logon recovery task.
- **Leak verification:** after connecting, Ghostline checks that queries really go through it.
- **DPI bypass:** bundled, hash-pinned GoodbyeDPI 0.2.2 with presets, auto-tune, a site blacklist, and DoH request fragmentation.
- **Signed server list:** updated daily and verified with ed25519; the DNSCrypt list is checked with minisign.
- **Simple and Advanced modes**, a tray icon, Vietnamese and English UI, and a neon-terminal look.
- **Installer or portable:** the portable build keeps all data in a `data\` folder next to the exe.
- **Update notifications only:** Ghostline tells you about a new version and never updates itself silently.

## Screenshots

| Servers | DPI bypass |
| --- | --- |
| ![Servers](docs/screenshots/servers-en.png) | ![DPI bypass](docs/screenshots/dpi-en.png) |
| **Logs** | **Settings** |
| ![Logs](docs/screenshots/logs-en.png) | ![Settings](docs/screenshots/settings-en.png) |

## Install

Download from [Releases](https://github.com/hashcott/ghostline/releases):

| File | What it is |
| --- | --- |
| `ghostline-amd64-installer.exe` | Installer (installs WebView2 if missing) |
| `Ghostline-<version>-portable.zip` | Portable: unzip and run |
| `SHA256SUMS` | Checksums for both |

Verify the download:

```powershell
Get-FileHash .\Ghostline-0.1.0-portable.zip -Algorithm SHA256
```

**Requirements:** Windows 10/11 x64 and administrator rights. Changing adapter DNS and loading the WinDivert driver both need admin. When Ghostline starts with Windows it runs through Task Scheduler, so there is no UAC prompt.

> [!NOTE]
> Releases are not code-signed yet, so SmartScreen shows "Windows protected your PC". After checking the SHA-256, choose **More info → Run anyway**.
> Some antivirus products flag the WinDivert driver used by GoodbyeDPI. Ghostline verifies GoodbyeDPI's hash before every start; if your antivirus blocks it, add the Ghostline folder to its exclusions.

## Usage

> 📖 A detailed user guide covering every screen, unblocking sites and troubleshooting: **[docs/user-guide.md](docs/user-guide.md)** ([Tiếng Việt](docs/huong-dan-su-dung.md))

1. Start Ghostline and press **Connect**. It picks a server, redirects DNS and verifies there is no leak.
2. If some sites are still blocked, open **Advanced → DPI**, turn on **GoodbyeDPI**, or press **auto-tune** to find a preset that works on your network.
3. Press **Disconnect** (or quit from the tray) to restore your original DNS.

If DNS ever looks wrong, **Settings → Restore DNS now** puts every adapter back to its saved state. From a terminal you can also run:

```powershell
ghostline.exe --restore
```

## How it works

```
apps ──► Windows DNS client ──► 127.0.0.1:53 (Ghostline / dnsproxy) ──► DoH · DoT · DoQ · DNSCrypt
                                         │
                       GoodbyeDPI (optional) rewrites outgoing TLS/HTTP to dodge SNI filtering
```

**Safety net.** Before touching an adapter, Ghostline writes a snapshot (`state.json`) of its DNS. Four layers make sure that snapshot gets restored:

1. **Clean disconnect:** the normal path.
2. **Watchdog:** a separate `--watchdog` process restores DNS within seconds if the app dies.
3. **Next launch:** a leftover snapshot is restored at startup.
4. **Logon task:** the `Ghostline Recovery` scheduled task runs `--restore` after a crash or power loss.

Design details live in [`docs/superpowers/specs`](docs/superpowers/specs).

## Privacy

- No telemetry, no accounts, no analytics.
- Visited domains are never written to disk. The optional query log lives in RAM only.
- Network access is limited to your chosen DNS resolvers, bootstrap resolution of their hostnames, the signed server list, the DNSCrypt resolver list and the GitHub release check.

## Building from source

**Prerequisites:** Go 1.26+, Node.js 24, [Wails v3](https://v3.wails.io) `v3.0.0-beta.27`, and [NSIS](https://nsis.sourceforge.io) for the installer.

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.27

wails3 dev                     # live-reload dev build (connecting for real needs admin)
wails3 build                   # bin/ghostline.exe
wails3 package                 # NSIS installer
wails3 task windows:portable   # portable zip
```

**Tests:**

```bash
go test ./...
cd frontend && npm test
```

Integration tests change real system settings, so run them in an **admin** terminal:

```bash
go test -tags integration ./internal/sysdns/... ./internal/startup/...
```

Before a release, go through [`docs/release-checklist.md`](docs/release-checklist.md). Pushing a `v*` tag builds and publishes the release through GitHub Actions.

## Project layout

| Path | Purpose |
| --- | --- |
| `internal/app` | Orchestrator: connect, disconnect, health checks, DPI, recovery |
| `internal/engine` | Local DNS server built on dnsproxy |
| `internal/sysdns` | Read, apply and restore adapter DNS (Win32 + netsh fallback) |
| `internal/watchdog`, `internal/startup` | Watchdog process and scheduled tasks |
| `internal/dpi` | GoodbyeDPI runner, presets, WinDivert service handling |
| `internal/scanner`, `internal/probe` | Server latency scan and blocked-site probes |
| `internal/servers`, `internal/upstreams` | Signed server list and DNSCrypt list |
| `internal/shell` | Window, tray and OS events (Wails) |
| `frontend/` | React + TypeScript UI |
| `lists/servers.json` | Signed server list, regenerated weekly by CI |

## Contributing

Bug reports, translations and pull requests are welcome. Please read [CONTRIBUTING.md](CONTRIBUTING.md) first.

## Security

Please do **not** open a public issue for vulnerabilities. See [SECURITY.md](SECURITY.md).

## License and credits

Ghostline is released under the [MIT License](LICENSE).

It stands on the shoulders of [dnsproxy](https://github.com/AdguardTeam/dnsproxy), [GoodbyeDPI](https://github.com/ValdikSS/GoodbyeDPI), [WinDivert](https://github.com/basil00/WinDivert) and [Wails](https://wails.io), and was inspired by [DNSveil / SecureDNSClient](https://github.com/msasanmh/SecureDNSClient). Third-party licenses are listed in [NOTICE](NOTICE).
