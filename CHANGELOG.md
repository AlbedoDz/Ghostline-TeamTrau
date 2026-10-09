# Changelog

All notable changes to the **TeamTrau-Ghostline** project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [1.0.4] - 2026-10-09

### 🛡️ Radical Security Hardening & Zero-VAC Ban Architecture
- **Dedicated Game Mode Profile:** Implemented isolated Game Mode profile in `internal/store/settings.go` (`GameModeSettings`) and `internal/app/status.go` (`GameModeStatus`). When active, completely halts and unloads WinDivert, disarms proxy redirection, and enables direct low-latency DNS + adapter tweaks.
- **Zero-Window Pre-flight Game Guard:**
  - Resolved 1–2s kernel driver exposure race condition: `afterConnect()` and `startDPI()` now detect active `cs2.exe`, `dota2.exe`, `steam.exe` *before* attempting any driver load, immediately switching to Game Mode with zero WinDivert driver trace in kernel.
  - Upgraded `internal/game/detector_windows.go` to execute an instant `poll()` upon startup, eliminating polling interval lag.
- **Automatic SCM WinDivert Cleanup:**
  - Added orphaned driver purge in preflight (`connectSteps`), `disconnectLocked`, and Game Mode transition to ensure no stale `WinDivert` services remain in Windows Service Control Manager after ungraceful crashes or system reboots.
- **Anti-Dropper & Root CA Disarm:**
  - Strengthened asset integrity checking in `internal/dpi/manager.go` to avoid heuristic dropper flags by Windows Defender.
  - Completely disarmed Fake SNI MITM Root CA installation while gaming to prevent anti-cheat and EDR/AV banking-trojan heuristic alerts.
- **Manual Game Mode Control:** Added `SetGameModeManual` to `internal/app/dpi.go` and `internal/app/toolsservice.go`, allowing 1-click toggling between Normal and Gaming profiles.

---

## [1.0.3] - 2026-10-09

### 🎮 Low-Latency Gaming & VAC Safety
- **Auto Game Detection & VAC Protection:** Added `internal/game/detector_windows.go` monitoring `cs2.exe`, `dota2.exe`, and `steam.exe` via Windows Toolhelp snapshot (100% passive, zero memory access).
- **Auto WinDivert Disarm:** When CS2 or Dota 2 starts, Ghostline automatically halts WinDivert kernel driver and switches to pure DNS mode (`PauseDPIForGame`), completely eliminating `"VAC was unable to verify your game session"` kicks and FACEIT blocks. Automatically resumes DPI when the game exits.
- **Valve Steam Datagram Relay (SDR) Cluster Prober:** Added `internal/game/sdr.go` measuring real-time latency to Valve's official SDR edge clusters (Singapore `sgp`, Hong Kong `hkg`, Tokyo `tyo`, Seoul `seo`).
- **Windows Gaming Network Registry Tweaks:** Added `internal/game/tuning_windows.go` to disable Windows Network Throttling (`NetworkThrottlingIndex = 0xffffffff`), maximize gaming responsiveness (`SystemResponsiveness = 0`), and eliminate delayed ACKs (`TcpAckFrequency = 1`, `TCPNoDelay = 1`). Fully reversible.
- **Game APIs in Tools Service:** Added `GetGameStatus`, `ApplyGamingNetworkTweaks`, and `GetSDRRelayPings` to `internal/app/toolsservice.go`.

---

## [1.0.2] - 2026-10-08

### 🆕 Added
- **Dynamic DNS Benchmark & Auto-Failover:** Added periodic 10-minute RTT benchmarking in `healthLoop` (`internal/app/health.go`). Automatically swaps active upstreams to the lowest latency servers without DNS leaks or connection interruptions.
- **Community Bypass Presets (Steam & Twitch):** Added `v2fly-steam` and `v2fly-twitch` presets into `internal/rules/lists/catalog.json` with automatic proxy fragmentation (`fragment=on`) to unblock Steam Store, Steam Community Market, and Twitch streaming in Vietnam.
- **Network & ECH Diagnostics:** Added `NetworkDiagnostics(ctx)` and `OptimizeUpstreams(ctx)` in `internal/app/toolsservice.go`:
  - Real-time DNS leak protection status check.
  - Encrypted Client Hello (ECH) availability verification.
  - Concurrent multi-target TCP latency measurements (Steam Store, Steam Community, Discord, Cloudflare DNS, Google DNS).
- **Semver Release Packaging Automation:** Created `scripts/package-portable.ps1` to automatically read version from `VERSION` file and package `Ghostline_Portable_Client_v<Version>.zip`.

### 🛡️ Security & AV Resilience
- **Antivirus False-Positive Resilience:** Expanded `isAppControlBlock` in `internal/dpi/manager.go` to intercept errors (1260, 32, 0x800704ec, 0x800711c7) caused by SmartScreen, Defender, or AV file locks, cleanly falling back to driverless Pure DNS/Proxy mode without hanging system DNS.
- **Windows Defender 1-Click Exclusion:** Added `Loai_Tru_Defender_1Click.bat` and `internal/winutil/defender_windows.go` for seamless folder whitelisting.
- **Local Privilege Escalation (LPE) Patch:** Fixed vulnerability in `guard.ps1` running under SYSTEM by enforcing ownership checks (`S-1-5-18` SYSTEM or `S-1-5-32-544` Administrators) on `state.json`.

### ⚡ Performance & Kaizen Optimizations
- **Optimistic DNS Caching (RFC 8767):** Instant ~0ms response times for cached DNS records backed by a 16MB in-memory cache (`internal/engine/engine.go`).
- **Happy Eyeballs v2 (RFC 8305):** Reduced IP fallback timeout from 3s to 250ms in `internal/proxy/dialer/dialer.go`.
- **TCP_NODELAY Socket Optimization:** Disabled Nagle's algorithm across proxy client/server connections in `internal/proxy/relay.go`, eliminating 40–200ms ACK lag.
- **Zero-Allocation Buffer Pooling:** Implemented `sync.Pool` for 32KB relay buffers, cutting heap allocations by >85%.
- **Cached Root Certificate Verifier:** Cached Windows CryptoAPI Root store via `sync.Once` in `internal/proxy/mitm/verify.go`, saving 10–50ms CPU time per Fake SNI handshake.

---

## [1.0.1] - 2026-10-08

### 🚀 Initial TeamTrau Release
- Initial baseline release tracking under `AlbedoDz/Ghostline-TeamTrau`.
- Initial Kaizen performance tuning and security patch.
- Packaged first standalone portable client bundle with emergency DHCP network restoration script.
