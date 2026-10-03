# Ghostline Giai đoạn 1 — Kế hoạch triển khai

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Mục tiêu:** Xây Ghostline — Secure DNS Client cho Windows: một nút Connect mã hoá toàn bộ DNS của máy (engine dnsproxy chạy trong process), tự khôi phục DNS khi có sự cố, vượt DPI bằng GoodbyeDPI, giao diện Neon Terminal song ngữ có 2 chế độ Đơn giản và Nâng cao.

**Kiến trúc:** Một file `ghostline.exe` (Go + Wails v3) chạy quyền admin, có 4 chế độ chạy (UI, `--autostart`, `--watchdog`, `--restore`). Lõi Go chia thành các package một nhiệm vụ trong `internal/`. Chỉ `internal/app` điều phối các package khác thông qua interface. Frontend React + TS nhận trạng thái qua sự kiện Wails và gọi xuống Go qua binding được Wails sinh tự động.

**Tech stack:** Go 1.27 · Wails `v3.0.0-beta.27` · `github.com/AdguardTeam/dnsproxy v0.86.0` (miekg/dns v1.1.72, dnsstamps v1.0.3) · `golang.org/x/sys` · `github.com/jedisct1/go-minisign` · `golang.org/x/mod/semver` · testify · React 18 + TypeScript + Vite · Zustand · i18next/react-i18next · Vitest + Testing Library · GoodbyeDPI 0.2.2 (x86_64) · NSIS.

**Spec:** `docs/superpowers/specs/2026-10-04-ghostline-phase1-design.md` — người thực thi đọc spec trước. Khi kế hoạch và spec mâu thuẫn, spec thắng; ghi chú lại mâu thuẫn trong báo cáo task.

## Ràng buộc chung

- Module path `github.com/hashcott/ghostline`. Tên "Ghostline", repo, URL, khoá công khai chỉ nằm ở `internal/brand` (frontend: `frontend/src/brand.ts`).
- Windows 10 build 19041+ và Windows 11, **chỉ amd64**. Mọi file dùng Win32 có hậu tố `_windows.go`; logic thuần nằm trong file không có hậu tố để test bằng fake.
- Engine **chỉ** lắng nghe `127.0.0.1:53` và `[::1]:53`. **Không bao giờ** dùng resolver hệ thống (`net.DefaultResolver`) cho bootstrap; `upstream.Options.Bootstrap` không bao giờ được để nil.
- Mỗi lần gọi `upstream.AddressToUpstream` dùng một `*upstream.Options` **mới** (thư viện sửa `opts` tại chỗ).
- `Fallbacks` của dnsproxy luôn nil (không có fallback plain DNS).
- **Không telemetry.** Giao diện không tải tài nguyên từ mạng (font nhúng).
- **Không ghi tên miền xuống đĩa.** Chế độ "hiện truy vấn" chỉ giữ tối đa 500 dòng trong RAM.
- **Không bao giờ tự kill process hoặc dừng service** của bên khác nếu người dùng chưa đồng ý rõ ràng (ngoại lệ: process `goodbyedpi.exe` và service `WinDivert` do Ghostline tạo).
- Không dùng `wmic`, không dùng UPX. Không chạy `wails3 update build-assets` (lệnh này ghi đè manifest).
- Mọi chuỗi giao diện có trong cả `vi.json` và `en.json`. Go chỉ trả **mã + tham số** (lỗi, nhật ký, bước kết nối); file log viết tiếng Anh.
- File JSON trong thư mục dữ liệu ghi nguyên tử (file tạm → fsync → rename).
- Giá trị mặc định (spec §9): `maxUpstreams=5`, quét 16 worker / timeout 3s / quét nhanh dừng ở 5 server hoặc 20s, cache quét 24h, kiểm tra sức khoẻ 30s / suy giảm sau 15s, trang mẫu timeout 5s × 2 lần, Fragment 5 mảnh / 5ms, GoodbyeDPI coi là chạy được sau 2s.
- Hash GoodbyeDPI 0.2.2 x86_64 (ghim): `goodbyedpi.exe` `331ac6c1d22ba5a0a217f3f27d0d823051869cafc8b8ef7f2002fa2accebc74e`, `WinDivert.dll` `a97859785a2df1d4462e7d48d33ccbd89fedd40dac4970f4afd89e63f59ee1ec`, `WinDivert64.sys` `53ab28ec00be6e6f8aefa9ee76fc2735e94d7f3f9dbc06eb2b7ac8cd3084a6af`.
- **Môi trường thực thi:** terminal của phiên làm việc **không có quyền admin**. Test cần admin gắn build tag `integration` (`//go:build windows && integration`); người thực thi vẫn phải viết chúng, chạy những test không cần admin, và ghi rõ trong báo cáo các test `integration` đang chờ người dùng chạy bằng terminal admin. Smart App Control đôi khi chặn exe test vừa build ("An Application Control policy has blocked this file"): chạy lại một lần; nếu vẫn bị chặn thì báo cáo, **không** tắt cơ chế bảo mật.
- Không thêm dependency ngoài danh sách ở Tech stack (và các devDependency frontend nêu trong task) nếu không ghi lý do trong báo cáo task.
- Commit sau mỗi task, message dạng Conventional Commits, kết thúc bằng dòng `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.

## Trọng tâm review

Những đầu vào spec ngầm đòi hỏi mà dễ bị bỏ sót nhất, đã gắn test vào task sở hữu:

1. **Bấm Connect hai lần liên tiếp, hoặc Connect trong lúc đang Disconnect** → các lệnh được tuần tự hoá, lệnh thứ hai không làm gì; không chụp DNS hai lần. Test `TestConnect_ConcurrentCallsAreSerialized` (Task 17).
2. **`state.json` bị cắt cụt do mất điện** → không đọc được bản chụp; app vẫn phải đưa các card mạng đang trỏ về loopback về DHCP rồi đánh dấu `clean`, kèm cảnh báo. Test `TestRestore_CorruptStateFallsBackToLoopbackAdapters` (Task 13).
3. **Card mạng bị đổi tên hoặc có tên tiếng Việt** (ví dụ "Kết nối mạng cục bộ") → khôi phục theo GUID, và đường dự phòng `netsh` dùng **IfIndex** chứ không dùng tên. Test `TestRestore_KeysByGUIDAndNetshUsesIfIndex` (Task 10).
4. **Đường dẫn dữ liệu có dấu cách và chữ có dấu** (`C:\Users\Đức Hạnh\AppData\Roaming\Ghostline\dpi-blacklist.txt`) → truyền cho GoodbyeDPI thành **một** phần tử argv. Test `TestArgs_BlacklistPathWithSpacesIsSingleArg` (Task 14).
5. **Bootstrap DNS bị chặn hoặc treo** (cổng 53 ra ngoài bị nhà mạng nuốt) → mỗi lần kiểm tra server vẫn kết thúc trong timeout 3s và báo lỗi rõ ràng, không treo cả lượt quét. Test `TestBuild_BlackholeBootstrapTimesOut` (Task 6).

---

## Bản đồ file

```
main.go                                 chọn chế độ chạy, gọi shell/watchdog
internal/brand/brand.go                 hằng số thương hiệu, URL, khoá công khai, Version
internal/cli/cli.go                     Parse(os.Args) → Mode
internal/model/model.go                 Server, Protocol, Source, AdapterSnapshot, FamilyDNS, DNSMode
internal/store/{paths,jsonfile,settings,state,meta}.go
internal/logx/rotate.go                 writer xoay vòng 3 × 5 MB
lists/{seed.json,servers.json,embed.go} danh sách nhúng (package lists)
internal/servers/{list,stamp,dnscrypt,sign,minisign,custom,merge}.go
internal/testutil/minisign.go           helper ký minisign cho test
tools/genservers/main.go (+ generate.go)
internal/fragdoh/{split,conn,upstream}.go
internal/upstreams/factory.go
internal/engine/engine.go
internal/scanner/{check,scan,cache,ip}.go
internal/winutil/{admin,ports,process,job,mutex,service,exec}_windows.go
internal/sysdns/{manager,types}.go, api_windows.go, watch_windows.go, debounce.go
internal/startup/{xml.go,tasks_windows.go}
internal/watchdog/{recover,run}.go
assets/goodbyedpi/{embed.go,goodbyedpi.exe,WinDivert.dll,WinDivert64.sys,LICENSE-*.txt}
internal/dpi/{preset,validate,assets,manager}.go, runner_windows.go
internal/probe/probe.go
internal/updater/{release,lists}.go
internal/app/{status,errors,deps,steps,orchestrator,health,autotune,picker,service,events,logbuf}.go
internal/icon/icon.go                   vẽ icon vòng tròn (khay + app icon), không phụ thuộc Wails
internal/shell/{shell,window,tray,wndproc,webview2}.go
frontend/src/{brand.ts,main.tsx,App.tsx}
frontend/src/app/{store.ts,bridge.ts,api.ts}
frontend/src/i18n/{index.ts,vi.json,en.json,parity.test.ts}
frontend/src/styles/{tokens.css,global.css}; frontend/src/assets/fonts/*.woff2
frontend/src/components/neon/*.tsx (+ .module.css, .test.tsx)
frontend/src/modes/simple/SimpleView.tsx; frontend/src/modes/advanced/{AdvancedView.tsx,pages/*.tsx}
build/windows/{wails.exe.manifest,wails.dev.manifest,Taskfile.yml,nsis/project.nsi}
.github/workflows/{ci,release,servers}.yml
LICENSE NOTICE README.md docs/release-checklist.md
```

---

## Mốc 0 — Khung dự án

### Task 1: Khung Wails v3 + chế độ chạy + manifest

**Files:**
- Create: toàn bộ khung từ `wails3 init`, `internal/brand/brand.go`, `internal/cli/cli.go`, `internal/cli/cli_test.go`, `build/windows/wails.dev.manifest`, `LICENSE`
- Modify: `main.go`, `build/windows/wails.exe.manifest`, `build/windows/Taskfile.yml`, `.gitignore`, `frontend/package.json`

**Interfaces:**
- Produces:
  - `brand`: `const AppName = "Ghostline"`, `AppID = "ghostline"`, `RepoOwner = "hashcott"`, `RepoName = "ghostline"`, `TaskAutostart = "Ghostline"`, `TaskRecovery = "Ghostline Recovery"`, `StateMutex = "Local\\Ghostline-State"`, `SingleInstanceID = "io.github.hashcott.ghostline"`, `ServerListURL`, `ServerListSigURL`, `ReleasesAPI = "https://api.github.com/repos/hashcott/ghostline/releases/latest"`, `DNSCryptMinisignKey = "RWQf6LRCGA9i53mlYecO4IzT51TGPpvWucNSCh1CBM0QTaLn73Y7GFO3"`; `var DNSCryptListURLs = []string{"https://download.dnscrypt.info/resolvers-list/v3/public-resolvers.md", "https://raw.githubusercontent.com/DNSCrypt/dnscrypt-resolvers/master/v3/public-resolvers.md"}`; `var Version = "dev"` (ghi đè qua `-ldflags -X`); `var ServerListPublicKeyHex = ""` (điền ở Task 4).
  - `cli`: `type Kind int` (`KindUI`, `KindAutostart`, `KindWatchdog`, `KindRestore`); `type Mode struct { Kind Kind; ParentPID uint32; ParentStart time.Time }`; `func Parse(args []string) (Mode, error)`.

- [ ] **Step 1: Cài CLI và sinh khung vào thư mục tạm**

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.27
wails3 init -n ghostline -t react -d "$TEMP/gl-init" -mod github.com/hashcott/ghostline -productname Ghostline -productcompany Ghostline -productidentifier io.github.hashcott.ghostline -productversion 0.1.0
```
`wails3 init` đòi thư mục đích rỗng nên sinh ra thư mục tạm rồi chép vào repo (giữ `docs/`, `.gitignore` hiện có). Xoá `greetservice.go` và các thư mục `build/darwin`, `build/linux`, `build/ios`, `build/android`, `build/docker`. Ghim `@wailsio/runtime` thành `3.0.0-beta.27` trong `frontend/package.json`. Thêm `.gotmp/`, `bin/`, `frontend/node_modules/`, `frontend/dist/` vào `.gitignore`.

- [ ] **Step 2: Viết test cho `cli.Parse`**

```go
func TestParse(t *testing.T) {
	for args, kind := range map[string]cli.Kind{"": cli.KindUI, "--autostart": cli.KindAutostart, "--restore": cli.KindRestore} {
		m, err := cli.Parse(strings.Fields(args)); require.NoError(t, err); require.Equal(t, kind, m.Kind)
	}
	m, err := cli.Parse([]string{"--watchdog", "--parent", "4242", "--parent-start", "1759561331512000000"})
	require.NoError(t, err); require.Equal(t, cli.KindWatchdog, m.Kind); require.Equal(t, uint32(4242), m.ParentPID)
	require.True(t, m.ParentStart.Equal(time.Unix(0, 1759561331512000000)))
}
func TestParse_WatchdogWithoutParentFails(t *testing.T) {
	_, err := cli.Parse([]string{"--watchdog"}); require.Error(t, err)
}
```

- [ ] **Step 3: Chạy test — phải FAIL** — `go test ./internal/cli/...` → lỗi biên dịch "undefined: cli.Parse".

- [ ] **Step 4: Hiện thực `cli.Parse` và `brand`**; `--parent-start` là Unix nano (int64). Viết `main.go` tối thiểu: `mode, err := cli.Parse(os.Args[1:])`; tạm thời mọi chế độ khác UI thoát với mã 0, chế độ UI giữ nguyên code mẫu của template (một cửa sổ). **Phải** rẽ nhánh trước khi gọi `application.New` (single-instance nằm bên trong `New`).

- [ ] **Step 5: Manifest.** `build/windows/wails.exe.manifest`: `requestedExecutionLevel level="requireAdministrator"`. Tạo `build/windows/wails.dev.manifest` là bản sao với `level="asInvoker"`. Trong `build/windows/Taskfile.yml`, task sinh `.syso` chọn `wails.dev.manifest` khi `DEV=true`, ngược lại dùng `wails.exe.manifest`. Thêm `-X github.com/hashcott/ghostline/internal/brand.Version={{.VERSION}}` vào ldflags của bản production (biến `VERSION` mặc định `0.1.0`).

- [ ] **Step 6: Kiểm chứng**
  - `go test ./...` → PASS.
  - `wails3 build` → có `bin/ghostline.exe`.
  - `wails3 dev` mở được cửa sổ mẫu từ terminal không có quyền admin (dùng dev manifest). Ghi lại trong báo cáo.

- [ ] **Step 7: Commit** — `LICENSE` (MIT, "Copyright (c) 2026 Harry Nguyen") cùng toàn bộ khung: `git commit -m "chore: scaffold Wails v3 app with run modes and admin manifest"`.

---

## Mốc 1 — Nền tảng (Go thuần)

### Task 2: model, store, logx

**Files:**
- Create: `internal/model/model.go`, `internal/store/{paths,jsonfile,settings,state}.go` (+ `_test.go`), `internal/logx/rotate.go` (+ test)

**Interfaces:**
- Produces:
  - `model`: `type Protocol string` (`ProtoDoH="doh"`, `ProtoDoT="dot"`, `ProtoDoQ="doq"`, `ProtoDNSCrypt="dnscrypt"`); `type Source string` (`SourceBuiltin="builtin"`, `SourceRemote="remote"`, `SourceDNSCrypt="dnscrypt"`, `SourceCustom="custom"`); `type Server struct { ID, Name, Provider string; Protocol Protocol; Address string; IPs []string; Tags []string; Source Source }` (json tag đúng như spec §6.1); `type DNSMode string` (`DNSModeDHCP="dhcp"`, `DNSModeStatic="static"`); `type FamilyDNS struct { Mode DNSMode; Servers []string }`; `type AdapterSnapshot struct { GUID string; LUID uint64; IfIndex uint32; Alias string; IPv4, IPv6 FamilyDNS }`.
  - `store`: `type Paths struct { DataDir, Settings, State, Meta, ScanCache, ServersRemote, ServersRemoteSig, ServersDNSCrypt, ServersDNSCryptSig, ServersCustom, DPIBlacklist, LogDir, BinDir string; Portable bool }`; `func ResolvePaths(exePath, appData string) Paths`; `func WriteJSONAtomic(path string, v any) error`; `func ReadJSON(path string, v any) error`; `type Settings struct{…}` (§9); `func DefaultSettings() Settings`; `func LoadSettings(path string) (s Settings, recovered bool, err error)`; `func SaveSettings(path string, s Settings) error`; `type Phase string` (`PhaseClean="clean"`, `PhaseDNSSet="dns_set"`); `type State struct { Version int; Phase Phase; PID uint32; PIDStartTime, StartedAt time.Time; Snapshot []model.AdapterSnapshot; DPI DPIState }`; `type DPIState struct { Running bool; PID int }`; `var ErrStateCorrupt`; `type Locker interface { Lock() error; Unlock() error }`; `type StateStore struct`; `func NewStateStore(path string, l Locker) *StateStore`; `func (s *StateStore) Load() (State, error)`; `func (s *StateStore) Update(fn func(*State) error) error`.
  - `logx`: `func NewRotating(dir, base string, maxBytes int64, keep int) (*Rotating, error)` triển khai `io.WriteCloser`.

Quyết định nằm ngoài spec: `Settings.Adapters string` (`"auto"` | `"manual"`) cộng thêm `Settings.AdapterGUIDs []string` (json `adapterGuids`) thay cho một trường hỗn hợp. `Paths.Meta` là `meta.json`, nơi lưu các mốc "1 lần/ngày" (Task 16). File danh sách đen là `dpi-blacklist.txt` trong `DataDir`.

- [ ] **Step 1: Viết test**

```go
func TestResolvePaths_PortableWhenMarkerExists(t *testing.T) {
	dir := t.TempDir(); require.NoError(t, os.WriteFile(filepath.Join(dir, "portable"), nil, 0o644))
	p := store.ResolvePaths(filepath.Join(dir, "ghostline.exe"), `C:\AppData`)
	require.True(t, p.Portable); require.Equal(t, filepath.Join(dir, "data"), p.DataDir)
	require.Equal(t, filepath.Join(dir, "data", "logs"), p.LogDir)
}
func TestResolvePaths_InstalledUsesAppData(t *testing.T) {
	p := store.ResolvePaths(filepath.Join(t.TempDir(), "ghostline.exe"), `C:\AppData`)
	require.False(t, p.Portable); require.Equal(t, filepath.Join(`C:\AppData`, "Ghostline"), p.DataDir)
}
func TestWriteJSONAtomic_RoundTripLeavesNoTemp(t *testing.T) // ghi → đọc lại bằng; thư mục chỉ còn đúng 1 file
func TestDefaultSettings_MatchSpec(t *testing.T) {
	s := store.DefaultSettings()
	require.Equal(t, "vi", s.Language); require.Equal(t, "simple", s.Mode); require.True(t, s.CloseToTray)
	require.Equal(t, "auto", s.Adapters); require.Equal(t, "www.google.com", s.TestDomain)
	require.Equal(t, []string{"1.1.1.1:53", "8.8.8.8:53"}, s.Bootstrap); require.Equal(t, 5, s.MaxUpstreams)
	require.Equal(t, []string{"no-filter"}, s.IncludeTags)
	require.Equal(t, []string{"youtube.com", "discord.com", "telegram.org", "x.com"}, s.ProbeSites)
	require.Equal(t, store.DPISettings{Enabled: false, Preset: "light", CustomArgs: "", Scope: "all"}, s.DPI)
	require.Equal(t, store.FragmentSettings{Enabled: false, Chunks: 5, DelayMs: 5}, s.FragmentDNS)
	require.Equal(t, store.UpdateSettings{CheckApp: true, UpdateServerList: true}, s.Updates)
	require.Equal(t, store.WindowSize{Width: 1000, Height: 660}, s.AdvancedWindow)
	require.False(t, s.StartWithWindows); require.False(t, s.AutoConnect); require.False(t, s.PinnedOnly)
}
func TestLoadSettings_MissingReturnsDefaults(t *testing.T) // recovered == false
func TestLoadSettings_CorruptRenamesToBak(t *testing.T)    // ghi "{oops" → recovered == true, tồn tại settings.json.bak, s == DefaultSettings()
func TestLoadState_MissingIsClean(t *testing.T)            // Phase == PhaseClean, Version == 1
func TestLoadState_CorruptReturnsErrStateCorrupt(t *testing.T) // ghi `{"phase":"dns_` → errors.Is(err, store.ErrStateCorrupt)
func TestStateStore_UpdateLocksAroundReadWrite(t *testing.T) // fake Locker ghi lại ["lock","unlock"]; fn trả lỗi → không ghi file, vẫn unlock
func TestRotating_RotatesAtMaxAndKeepsN(t *testing.T) // max 100B, keep 3; ghi 450B → base.log, base.1.log, base.2.log; không có base.3.log
```

- [ ] **Step 2: Chạy test — phải FAIL** — `go test ./internal/store/... ./internal/logx/...` → lỗi "undefined".

- [ ] **Step 3: Hiện thực.** `WriteJSONAtomic`: `os.CreateTemp(dir, ".tmp-*")` → encode (indent 2 dấu cách) → `Sync` → `Close` → `os.Rename`. Khi `LoadSettings` đọc thiếu trường, các trường thiếu nhận giá trị mặc định: decode chồng lên `DefaultSettings()`.

- [ ] **Step 4: Chạy test — phải PASS** — cùng lệnh.

- [ ] **Step 5: Commit** — `git commit -m "feat(store): data paths, atomic JSON, settings, state store, rotating log"`

### Task 3: servers — danh sách, stamp, chữ ký, DNSCrypt, server tự thêm

**Files:**
- Create: `lists/embed.go`, `lists/servers.json` (tạm thời `{"generatedAt":"2026-10-04T00:00:00Z","servers":[]}`), `internal/servers/{list,stamp,dnscrypt,sign,minisign,custom,merge}.go` (+ tests), `internal/testutil/minisign.go`

**Interfaces:**
- Consumes: `model.Server`, các hằng `model.Proto*` và `model.Source*`.
- Produces:
  - `lists`: `//go:embed servers.json` `var BuiltinJSON []byte`.
  - `servers`: `type List struct { GeneratedAt time.Time; Servers []model.Server }`; `func ParseList(b []byte) (List, error)`; `func Sign(data []byte, priv ed25519.PrivateKey) []byte` (trả về base64 chuẩn của chữ ký); `func VerifySigned(data, sig []byte, pub ed25519.PublicKey) error`; `var ErrBadSignature`; `func FromStamp(stamp string, src model.Source) (model.Server, error)`; `func FromAddress(addr string, src model.Source) (model.Server, error)`; `var ErrUnencrypted`; `func ParseDNSCryptMarkdown(md []byte) ([]model.Server, error)`; `func VerifyMinisign(data, sig []byte, pubKey string) error`; `func ParseImport(text []byte) (servers []model.Server, bad []string)`; `func Merge(builtin, remote List, dnscrypt, custom []model.Server) []model.Server`; `func Filter(all []model.Server, includeTags []string) []model.Server`.
  - `testutil`: `func SignMinisign(t testing.TB, data []byte) (pubKey string, sigFile []byte)` (thuật toán "Ed", key ID 8 byte, có trusted comment và global signature theo định dạng minisign).

Quy tắc cần thực hiện đúng:
- **Protocol lấy từ stamp:** DNSCrypt→`dnscrypt`, DoH→`doh`, DoT→`dot`, DoQ→`doq`. Stamp plain DNS → `ErrUnencrypted`. ODoH và relay → bỏ qua.
- **Nhãn lấy từ cờ thuộc tính của stamp:** `ServerInformalPropertyNoFilter`→`no-filter`, `NoLog`→`no-log`, `DNSSEC`→`dnssec`.
- **IP trong stamp** được đưa vào `IPs`, bỏ cổng.
- **ID:**
  - Server DNSCrypt: `dnscrypt:<tên section>`. Section có nhiều stamp thì stamp thứ hai trở đi có thêm hậu tố `#2`, `#3`, …
  - Server tự thêm: `custom:` + 8 ký tự hex đầu của sha1(address).
- **`FromAddress`** nhận `https://`→doh, `tls://`→dot, `quic://`→doq, `sdns://`→stamp. `udp://`, `tcp://` hoặc IP trơn → `ErrUnencrypted`.
- **`Merge`:**
  - Chỉ dùng `remote` khi `remote.GeneratedAt` mới hơn `builtin.GeneratedAt`.
  - Loại trùng theo `Address`, thứ tự ưu tiên custom > remote/builtin > dnscrypt.
  - Kết quả sắp xếp ổn định theo `ID`.
- **`Filter`** giữ server có ít nhất một nhãn nằm trong `includeTags`. Server `SourceCustom` luôn được giữ.

- [ ] **Step 1: Viết test**

```go
func TestFromAddress(t *testing.T) // bảng: https→doh, tls→dot, quic→doq, "udp://1.1.1.1" và "1.1.1.1" → errors.Is(err, servers.ErrUnencrypted)
func TestParseDNSCryptMarkdown_OneServerPerStamp(t *testing.T) {
	md := []byte("# public-resolvers\n\n--\n\n## a-and-a\n\nNon-filtering.\n\n" +
		"sdns://AgcAAAAAAAAADTIxNy4xNjkuMjAuMjIADWRucy5hYS5uZXQudWsKL2Rucy1xdWVyeQ\n" +
		"sdns://AgcAAAAAAAAADTIxNy4xNjkuMjAuMjMADWRucy5hYS5uZXQudWsKL2Rucy1xdWVyeQ\n")
	got, err := servers.ParseDNSCryptMarkdown(md); require.NoError(t, err); require.Len(t, got, 2)
	require.Equal(t, "dnscrypt:a-and-a", got[0].ID); require.Equal(t, "dnscrypt:a-and-a#2", got[1].ID)
	require.Equal(t, model.ProtoDoH, got[0].Protocol); require.Equal(t, []string{"217.169.20.22"}, got[0].IPs)
	require.ElementsMatch(t, []string{"no-filter", "no-log", "dnssec"}, got[0].Tags) // props 0x07
}
func TestVerifySigned_RoundTripAndTamper(t *testing.T) // ed25519.GenerateKey; Sign → Verify ok; sửa 1 byte → ErrBadSignature
func TestVerifyMinisign_RoundTripAndTamper(t *testing.T) // testutil.SignMinisign; đúng → nil; dữ liệu bị sửa → lỗi; khoá khác → lỗi
func TestParseImport_SkipsBlankAndCommentsReportsBad(t *testing.T) // "# c\n\nhttps://a/dns-query\nudp://1.1.1.1\n" → 1 server, bad == ["udp://1.1.1.1"]
func TestMerge_RemoteOnlyWhenNewerCustomWins(t *testing.T)
func TestFilter_IncludeTagsCustomAlwaysKept(t *testing.T)
func TestBuiltinListParses(t *testing.T) // servers.ParseList(lists.BuiltinJSON) không lỗi
```

- [ ] **Step 2: Chạy test — phải FAIL** — `go test ./internal/servers/... ./lists/...`

- [ ] **Step 3: Hiện thực.** Dùng `github.com/ameshkov/dnsstamps` (`NewServerStampFromString`) và `github.com/jedisct1/go-minisign` (`NewPublicKey`, `DecodeSignature`, `pk.Verify`). Bộ đọc markdown: tách theo dòng `## `; mỗi dòng bắt đầu bằng `sdns://` trong một section là một stamp.

- [ ] **Step 4: Chạy test — phải PASS**

- [ ] **Step 5: Commit** — `git commit -m "feat(servers): list model, stamps, ed25519 + minisign verification, import, merge"`

### Task 4: tools/genservers + seed.json + khoá ký

**Files:**
- Create: `tools/genservers/main.go`, `tools/genservers/generate.go`, `tools/genservers/generate_test.go`, `lists/seed.json`
- Modify: `lists/servers.json` (sinh lại), `internal/brand/brand.go` (`ServerListPublicKeyHex`)

**Interfaces:**
- Consumes: `servers.List`, `servers.Sign`, `servers.FromStamp`, `model.Server`.
- Produces: `func Generate(seed []model.Server, resolve func(host string) ([]string, error), now time.Time) (servers.List, error)`. Các cờ CLI: `-seed`, `-out`, `-sign-env` (tên biến môi trường chứa khoá bí mật ed25519 dạng base64; có thì ghi thêm `<out>.sig`), `-genkey` (in ra `public=<hex>` và `private=<base64>`).

Nội dung `seed.json` (chỉ ghi `id`, `name`, `provider`, `protocol`, `address`, `tags`; `ips` do generator điền):

| id | address | tags |
|---|---|---|
| cloudflare-doh | `https://cloudflare-dns.com/dns-query` | no-filter |
| cloudflare-dot | `tls://one.one.one.one` | no-filter |
| quad9-unfiltered-doh | `https://dns10.quad9.net/dns-query` | no-filter |
| quad9-unfiltered-dot | `tls://dns10.quad9.net` | no-filter |
| google-doh | `https://dns.google/dns-query` | no-filter |
| google-dot | `tls://dns.google` | no-filter |
| adguard-unfiltered-doh | `https://unfiltered.adguard-dns.com/dns-query` | no-filter |
| adguard-unfiltered-dot | `tls://unfiltered.adguard-dns.com` | no-filter |
| adguard-unfiltered-doq | `quic://unfiltered.adguard-dns.com` | no-filter |
| adguard-doh | `https://dns.adguard-dns.com/dns-query` | adblock |
| adguard-family-doh | `https://family.adguard-dns.com/dns-query` | family |
| mullvad-doh | `https://dns.mullvad.net/dns-query` | no-filter |
| mullvad-adblock-doh | `https://adblock.dns.mullvad.net/dns-query` | adblock |
| dnssb-doh | `https://doh.dns.sb/dns-query` | no-filter |
| controld-free-doh | `https://freedns.controld.com/p0` | no-filter |
| controld-free-dot | `tls://p0.freedns.controld.com` | no-filter |

- [ ] **Step 1: Viết test**

```go
func TestGenerate_FillsIPsSetsTimeAndSorts(t *testing.T) {
	seed := []model.Server{{ID: "b", Protocol: model.ProtoDoH, Address: "https://b.example/dns-query", Tags: []string{"no-filter"}},
		{ID: "a", Protocol: model.ProtoDoT, Address: "tls://a.example", Tags: []string{"no-filter"}}}
	now := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	l, err := genservers.Generate(seed, func(h string) ([]string, error) { return []string{"192.0.2.10"}, nil }, now)
	require.NoError(t, err); require.Equal(t, now, l.GeneratedAt); require.Equal(t, "a", l.Servers[0].ID)
	require.Equal(t, []string{"192.0.2.10"}, l.Servers[1].IPs); require.Equal(t, model.SourceBuiltin, l.Servers[0].Source)
}
func TestGenerate_RejectsUnencrypted(t *testing.T) // Address "udp://1.1.1.1" → lỗi
func TestGenerate_ResolveFailureIsError(t *testing.T) // không cho phép danh sách thiếu IP một cách âm thầm
```
Package `tools/genservers` là `package main`; đặt `Generate` trong `generate.go` và test bằng `package main`.

- [ ] **Step 2: Chạy test — phải FAIL** — `go test ./tools/genservers/...`

- [ ] **Step 3: Hiện thực.** `resolve` thật dùng `net.Resolver{PreferGo: true, Dial: …}` quay số tới `1.1.1.1:53`, lấy cả A lẫn AAAA (chạy lúc build, không phải lúc app chạy).

- [ ] **Step 4: Chạy test — phải PASS**

- [ ] **Step 5: Sinh khoá và danh sách**
  - Chạy `go run ./tools/genservers -genkey`. Dán giá trị `public=` vào `brand.ServerListPublicKeyHex`.
  - **Không** commit khoá bí mật. Ghi trong báo cáo: người dùng phải lưu nó vào GitHub secret `SERVERLIST_SIGNING_KEY`.
  - Chạy `go run ./tools/genservers -seed lists/seed.json -out lists/servers.json`. Phải sinh đủ 16 server, mỗi server có ít nhất một IP.

- [ ] **Step 6: Commit** — `git commit -m "feat(genservers): curated seed list, generator, signing keypair"`

---

## Mốc 2 — Lõi DNS

### Task 5: fragdoh — upstream DoH cắt nhỏ ClientHello

**Files:**
- Create: `internal/fragdoh/{split,conn,upstream}.go`, `internal/fragdoh/{split,upstream}_test.go`

**Interfaces:**
- Produces: `func SplitClientHello(record []byte, chunks int) [][]byte`; `type Options struct { Bootstrap upstream.Resolver; Chunks int; Delay time.Duration; Timeout time.Duration; RootCAs *x509.CertPool }`; `type Upstream struct` (hiện thực `upstream.Upstream`); `func New(address string, o Options) (*Upstream, error)`.

Thuật toán `SplitClientHello`:
- Nếu `record[0] != 0x16` hoặc không tìm thấy extension SNI (type 0x0000) thì trả về `[][]byte{record}`.
- Ngược lại, lần lượt bỏ qua các trường của ClientHello: header record 5 byte, header handshake 4 byte, version 2 byte, random 32 byte, session id, cipher suites, compression methods, rồi duyệt danh sách extension để tìm offset bắt đầu và độ dài hostname.
- Kết quả gồm 3 phần:
  - 1 đoạn: từ đầu record tới hết byte đứng trước hostname;
  - `chunks` đoạn: hostname chia đều, đoạn cuối nhận phần dư;
  - 1 đoạn: phần còn lại của record.
- Ghép tất cả các đoạn lại phải ra đúng `record`.

`conn`: bọc `net.Conn`. Lần `Write` đầu tiên được chia bằng `SplitClientHello` và ghi từng đoạn, giữa các đoạn ngủ `Delay`. Các lần `Write` sau ghi thẳng.

`Upstream.Exchange`:
- Gửi `POST` với `Content-Type: application/dns-message` theo RFC 8484, đặt `ID=0` trước khi pack và khôi phục ID cũ sau khi nhận.
- Dùng `http.Transport` có `DialContext` lấy IP từ `Bootstrap.LookupNetIP`, quay số TCP rồi bọc bằng `conn`, cùng `ForceAttemptHTTP2: true` và `TLSClientConfig{ServerName: host, RootCAs: o.RootCAs}`.

- [ ] **Step 1: Viết test**

```go
func captureClientHello(t *testing.T, host string) []byte // tls.Client trên net.Pipe; goroutine đọc lần Write đầu rồi đóng
func TestSplitClientHello_SplitsHostnameIntoChunks(t *testing.T) {
	rec := captureClientHello(t, "dns.example.test")
	segs := fragdoh.SplitClientHello(rec, 5)
	require.Len(t, segs, 7); require.Equal(t, rec, bytes.Join(segs, nil))
	require.Equal(t, "dns.example.test", string(bytes.Join(segs[1:6], nil)))
}
func TestSplitClientHello_NonHandshakePassthrough(t *testing.T) // []byte{0x17,3,3,0,1,0} → 1 đoạn, giữ nguyên
func TestUpstream_ExchangeOverFragmentedTLS(t *testing.T) {
	// httptest.NewUnstartedServer với handler DoH trả A 192.0.2.53; EnableHTTP2 = true; StartTLS.
	// Listener của server được bọc để đếm số lần Read trước khi bắt tay TLS xong.
	// New("https://example.com:<port>/dns-query", Options{Bootstrap: upstream.StaticResolver{netip.MustParseAddr("127.0.0.1")},
	//     Chunks: 5, Delay: 20 * time.Millisecond, Timeout: 3 * time.Second, RootCAs: pool(srv.Certificate())})
	// Exchange(A example.org) → Answer[0] là A 192.0.2.53; số lần Read ở server trong lúc bắt tay ≥ 3.
}
```

- [ ] **Step 2: Chạy test — phải FAIL** — `go test ./internal/fragdoh/...`
- [ ] **Step 3: Hiện thực** theo thuật toán trên.
- [ ] **Step 4: Chạy test — phải PASS**
- [ ] **Step 5: Commit** — `git commit -m "feat(fragdoh): DoH upstream with fragmented ClientHello"`

### Task 6: upstreams — factory và quy tắc bootstrap

**Files:**
- Create: `internal/upstreams/factory.go`, `internal/upstreams/factory_test.go`

**Interfaces:**
- Consumes: `model.Server`; `fragdoh.New`, `fragdoh.Options`.
- Produces: `type Options struct { Bootstrap []string; Timeout time.Duration; Fragment *FragmentOptions; RootCAs *x509.CertPool }`; `type FragmentOptions struct { Chunks int; Delay time.Duration }`; `type Factory struct`; `func NewFactory(o Options) (*Factory, error)`; `func (f *Factory) Build(s model.Server) (upstream.Upstream, error)`.

Quy tắc:
- `NewFactory` trả lỗi khi `Bootstrap` rỗng.
- Resolver bootstrap dùng chung là `upstream.ParallelResolver{upstream.NewCachingResolver(r1), …}`, mỗi `r` tạo bằng `upstream.NewUpstreamResolver("1.1.1.1:53", &upstream.Options{Timeout: o.Timeout})`.
- `Build` tạo một `&upstream.Options{Timeout, RootCAs}` **mới** mỗi lần gọi:
  - `s.IPs` không rỗng → `Bootstrap = upstream.StaticResolver{…}`;
  - còn lại → resolver bootstrap dùng chung.
- `Fragment != nil` và `s.Protocol == model.ProtoDoH` và `s.Address` bắt đầu bằng `https://` → dùng `fragdoh.New`. Mọi trường hợp khác dùng `upstream.AddressToUpstream`.

- [ ] **Step 1: Viết test**

```go
func TestBuild_UsesPinnedIPs(t *testing.T) // DoH httptest TLS ("example.com"); server.IPs=["127.0.0.1"]; Bootstrap=["127.0.0.1:1"] (không dùng được) → Exchange OK
func TestBuild_UsesBootstrapNotSystem(t *testing.T) {
	// Server plain DNS giả (miekg dns.Server trên 127.0.0.1:0): trả lời example.com → 127.0.0.1 và đếm số truy vấn.
	// Server không có IPs, Bootstrap=[fakeAddr] → Exchange OK và server giả đã nhận ≥ 1 truy vấn.
}
func TestBuild_BlackholeBootstrapTimesOut(t *testing.T) { // Trọng tâm review #5
	// Listener UDP trên 127.0.0.1:0 đọc gói nhưng không bao giờ trả lời. Bootstrap=[addr], Timeout=3s, server không có IPs.
	start := time.Now(); _, err := u.Exchange(context.Background(), q("example.org"))
	require.Error(t, err); require.Less(t, time.Since(start), 3500*time.Millisecond)
}
func TestBuild_FragmentOnlyForDoH(t *testing.T) // Fragment bật: server DoH → *fragdoh.Upstream; server DoT → không phải
func TestNewFactory_EmptyBootstrapFails(t *testing.T)
```

- [ ] **Step 2: Chạy test — phải FAIL** — `go test ./internal/upstreams/...`
- [ ] **Step 3: Hiện thực** theo quy tắc trên. Nếu sau khi đã truyền context có deadline mà test #5 vẫn vượt 3,5s, bọc `Exchange` để cắt đúng tổng timeout.
- [ ] **Step 4: Chạy test — phải PASS**
- [ ] **Step 5: Commit** — `git commit -m "feat(upstreams): factory with pinned-IP and plain-DNS bootstrap, never system resolver"`

### Task 7: engine — DNS server cục bộ

**Files:**
- Create: `internal/engine/engine.go`, `internal/engine/engine_test.go`

**Interfaces:**
- Consumes: `proxy.New(*proxy.Config) (*proxy.Proxy, error)`, `(*Proxy).Start/Shutdown(ctx)`, `(*Proxy).Resolve(ctx, *proxy.DNSContext)`, `proxy.HandlerFunc`, `proxy.UpstreamModeParallel`, `proxy.ProtoUDP`, `(*Proxy).Addr(proto)`.
- Produces: `type Config struct { ListenV4, ListenV6 netip.AddrPort; Upstreams []upstream.Upstream; CacheEnabled bool; Logger *slog.Logger }` (`ListenV6` bằng giá trị rỗng thì không lắng nghe v6); `type QueryEvent struct { Time time.Time; Domain, Type, Upstream string; Latency time.Duration; Err string; Cached bool }`; `type UpstreamStat struct { Queries, Errors uint64; AvgLatency time.Duration }`; `type Stats struct { Queries uint64; AvgLatency time.Duration; PerUpstream map[string]UpstreamStat }`; `type Engine struct`; `func New(onQuery func(QueryEvent)) *Engine`; `Start(ctx, Config) error`; `Swap(ctx, []upstream.Upstream) error`; `Stop(ctx) error`; `SelfTest(ctx) error`; `ExpectVerify(nonce string)`; `SawVerify(nonce string) bool`; `ListenAddr() netip.AddrPort`; `Stats() Stats`; `const VerifySuffix = "verify.ghostline.test."`; `var VerifyAnswer = netip.MustParseAddr("192.0.2.1")`.

Chi tiết hiện thực:
- `Config` của proxy gồm: `UpstreamMode: proxy.UpstreamModeParallel`, `CacheEnabled` lấy từ `cfg`, `Fallbacks: nil`, và `RequestHandler` là một `HandlerFunc`:
  - tên miền có đuôi `VerifySuffix` → trả lời tại chỗ bằng `A VerifyAnswer` (TTL 0) và ghi nhận nonce, **không** chuyển tiếp;
  - tên miền khác → gọi `p.Resolve`, rồi cập nhật thống kê từ `dctx.QueryStatistics().Main()` và gọi `onQuery`.
- `Swap`: `Shutdown` proxy cũ (thao tác này cũng đóng các upstream cũ), rồi tạo proxy mới trên đúng các địa chỉ đang lắng nghe.
- `SelfTest`: gửi truy vấn `selftest-<rand>.verify.ghostline.test.` qua `dns.Client` tới `ListenAddr()`, timeout 3s.

- [ ] **Step 1: Viết test** — dùng upstream giả trong bộ nhớ, tự hiện thực `upstream.Upstream`, trả A 192.0.2.80 và đếm số lần được gọi. Engine lắng nghe `127.0.0.1:0`.

```go
func TestEngine_ResolvesThroughUpstream(t *testing.T)   // dns.Client tới ListenAddr → A 192.0.2.80
func TestEngine_AnswersVerifyLocally(t *testing.T) {
	e.ExpectVerify("n1"); r := query(t, e, "n1.verify.ghostline.test.")
	require.Equal(t, "192.0.2.1", r.Answer[0].(*dns.A).A.String()); require.True(t, e.SawVerify("n1"))
	require.Zero(t, fake.calls.Load())
}
func TestEngine_SelfTest(t *testing.T)                 // nil khi đang chạy; lỗi sau khi Stop
func TestEngine_SwapReplacesUpstreams(t *testing.T)    // sau Swap(up2) → câu trả lời đến từ up2; up1 đã bị Close
func TestEngine_StatsAndOnQuery(t *testing.T)          // 3 truy vấn → Stats().Queries == 3; onQuery nhận Domain "example.org."
```

- [ ] **Step 2: Chạy test — phải FAIL** — `go test ./internal/engine/...`
- [ ] **Step 3: Hiện thực** theo chi tiết trên.
- [ ] **Step 4: Chạy test — phải PASS**
- [ ] **Step 5: Commit** — `git commit -m "feat(engine): loopback dnsproxy with parallel upstreams, local verify answers, hot swap"`

### Task 8: scanner — kiểm tra, quét, cache theo mạng

**Files:**
- Create: `internal/scanner/{ip,check,scan,cache}.go` (+ tests)

**Interfaces:**
- Consumes: `model.Server`; `upstream.Upstream`.
- Produces:
  - `func IsPublicIP(a netip.Addr) bool`
  - `type Result struct { ServerID string; OK bool; Latency time.Duration; Reason string; CheckedAt time.Time }` với `Reason` thuộc `"" | "timeout" | "poisoned" | "rcode:<NAME>" | "empty" | "error"`
  - `type Checker interface { Check(ctx context.Context, s model.Server) Result }`
  - `type DNSChecker struct { Build func(model.Server) (upstream.Upstream, error); TestDomain string; Timeout time.Duration; Now func() time.Time }`
  - `type Options struct { Workers, Want int; Budget time.Duration; OnProgress func(done, total int, r Result) }`
  - `func Scan(ctx context.Context, list []model.Server, c Checker, o Options) []Result`
  - `func Order(list []model.Server, everOK map[string]bool, r *rand.Rand) []model.Server`
  - `type Cache struct { Entries map[string]CacheEntry }`, `type CacheEntry struct { ScannedAt time.Time; Results []Result }`
  - `func NetworkKey(gatewayIP, gatewayMAC string) string`
  - `func (c *Cache) Fresh(key string, now time.Time, ttl time.Duration) ([]Result, bool)`
  - `func (c *Cache) Put(key string, now time.Time, rs []Result)`
  - `func (c *Cache) EverOK() map[string]bool`
  - `func LoadCache(path string) (*Cache, error)` (file thiếu hoặc hỏng → cache rỗng), `func SaveCache(path string, c *Cache) error`

Quy tắc:
- **`IsPublicIP`** trả false với: RFC1918, loopback, link-local, 100.64/10, 0/8, multicast, 192.0.0/24, 192.0.2/24, 198.18/15, 198.51.100/24, 203.0.113/24, 240/4, broadcast, cùng các dải IPv6 tương đương (`::1`, `fc00::/7`, `fe80::/10`, `2001:db8::/32`, `::/128`).
- **`DNSChecker`** truy vấn A cho `TestDomain` 2 lần, độ trễ là thời gian của lần thứ 2. Mỗi lần dùng `context.WithTimeout(Timeout)`. Gọi `Close` cho upstream sau khi kiểm tra xong.
- **`Scan`:**
  - `Workers` goroutine cùng lấy từ một hàng đợi.
  - `Want > 0` → huỷ ngay khi đã có đủ `Want` kết quả OK. `Budget > 0` → dừng khi hết thời gian, server chưa kiểm tra thì không có trong kết quả.
  - Kết quả sắp xếp: OK trước, sau đó theo độ trễ tăng dần.
- **`Order`:** những server có trong `everOK` đứng trước, theo thứ tự ổn định theo ID; các server còn lại được xáo bằng `r`.
- **`NetworkKey`:** hex sha256 của `gatewayIP + "|" + gatewayMAC`.

- [ ] **Step 1: Viết test**

```go
func TestIsPublicIP(t *testing.T) // bảng: 8.8.8.8 true; 10.0.0.1, 127.0.0.1, 100.64.0.1, 192.0.2.1, ::1, fd00::1 false
func TestDNSChecker_ClassifiesAnswers(t *testing.T) // upstream giả: A công khai→OK; A 10.0.0.1→"poisoned"; NXDOMAIN→"rcode:NXDOMAIN"; không có A→"empty"; context bị huỷ→"timeout"
func TestScan_StopsAtWant(t *testing.T)      // 40 server, checker giả OK sau 10ms → Want 5: số kết quả OK == 5, tổng lượt kiểm tra < 40
func TestScan_RespectsWorkerLimit(t *testing.T) // checker giả ghi nhận độ song song tối đa → ≤ 16
func TestScan_BudgetStops(t *testing.T)      // checker giả chặn tới khi ctx bị huỷ; Budget 200ms → Scan trả về trong < 400ms
func TestScan_SortsOKByLatency(t *testing.T)
func TestOrder_EverOKFirst(t *testing.T)
func TestCache_FreshTTLAndPerNetwork(t *testing.T) // Put(k1, now); Fresh(k1, now+23h, 24h) ok; Fresh(k1, now+25h) không ok; Fresh(k2) không ok
```

- [ ] **Step 2: Chạy test — phải FAIL** — `go test ./internal/scanner/...`
- [ ] **Step 3: Hiện thực** theo quy tắc trên.
- [ ] **Step 4: Chạy test — phải PASS**
- [ ] **Step 5: Commit** — `git commit -m "feat(scanner): parallel checks, poisoning detection, early stop, per-network cache"`

---

## Mốc 3 — Tích hợp Windows

### Task 9: winutil — quyền admin, chủ sở hữu cổng, process, Job, mutex, service

**Files:**
- Create: `internal/winutil/{admin,ports,process,job,mutex,service,exec}_windows.go`, `internal/winutil/ports_parse.go` (phần đọc bảng, không phụ thuộc OS) cùng test

**Interfaces:**
- Produces:
  - `func IsAdmin() bool`
  - `type PortOwner struct { PID uint32; Name, Service, Proto string }`; `func PortOwners(port uint16) ([]PortOwner, error)`
  - `func ProcessStartTime(pid uint32) (time.Time, error)`; `func ProcessAlive(pid uint32, start time.Time) bool`
  - `type Job struct`; `func NewKillOnCloseJob() (*Job, error)`; `func (j *Job) Assign(p *os.Process) error`; `func (j *Job) Close() error`
  - `type NamedMutex struct`; `func NewNamedMutex(name string) (*NamedMutex, error)` — hiện thực `store.Locker`
  - `func ServiceForPID(pid uint32) (string, error)` (trả `""` nếu không có); `func ServiceRunning(name string) (bool, error)`; `func StopService(name string, wait time.Duration) error`; `func DeleteService(name string) error`
  - `func HiddenCmd(exe string, args []string, dir string) *exec.Cmd` (`SysProcAttr{HideWindow: true, CreationFlags: CREATE_NO_WINDOW}`)
  - `func parseUDP4Table(b []byte) []udpRow`, `func parseTCP4Table(b []byte) []tcpRow` (và bản v6)

Cách hiện thực:
- `PortOwners` gọi `GetExtendedUdpTable` (`UDP_TABLE_OWNER_PID=1`) và `GetExtendedTcpTable` (`TCP_TABLE_OWNER_PID_LISTENER=3`) cho cả `AF_INET=2` và `AF_INET6=23`, qua `windows.NewLazySystemDLL("iphlpapi.dll")`.
  - Gọi lần đầu để lấy kích thước bộ đệm (kết quả trả về là 122, tức không đủ chỗ), rồi gọi lại.
  - Cổng nằm ở 2 byte thấp theo thứ tự big-endian.
  - Chỉ giữ địa chỉ loopback và wildcard (`127.0.0.1`, `0.0.0.0`, `::1`, `::`).
- `ProcessAlive` so sánh `GetProcessTimes` creation time với `start`, sai lệch tối đa ≤ 1ms.
- `ServiceForPID` dùng `windows.EnumServicesStatusEx` với `SERVICE_WIN32`.
- `StopService` và `DeleteService` dùng `golang.org/x/sys/windows/svc/mgr`.

- [ ] **Step 1: Viết test** (đều chạy được không cần admin)

```go
func TestParseUDP4Table(t *testing.T) // bộ đệm dựng tay: 2 dòng, cổng 0x0035 big-endian → port 53, pid 4321
func TestPortOwners_FindsOwnListener(t *testing.T) {
	c, _ := net.ListenPacket("udp4", "127.0.0.1:0"); defer c.Close()
	port := uint16(c.LocalAddr().(*net.UDPAddr).Port); owners, err := winutil.PortOwners(port)
	require.NoError(t, err); require.Contains(t, pids(owners), uint32(os.Getpid()))
}
func TestProcessAlive(t *testing.T) // pid của chính mình + start đúng → true; start sai 1s → false; pid 0xFFFFFFF0 → false
func TestJob_KillsChildOnClose(t *testing.T) // HiddenCmd("cmd", {"/c","ping -n 30 127.0.0.1"}) Start → Assign → Close → Wait trả về trong 5s
func TestNamedMutex_Exclusive(t *testing.T) // 2 goroutine; Lock thứ hai chờ tới khi Unlock
func TestIsAdmin_DoesNotPanic(t *testing.T)
```

- [ ] **Step 2: Chạy test — phải FAIL** — `go test ./internal/winutil/...`
- [ ] **Step 3: Hiện thực** theo cách đã nêu.
- [ ] **Step 4: Chạy test — phải PASS**
- [ ] **Step 5: Commit** — `git commit -m "feat(winutil): admin check, port owners, process identity, job objects, named mutex, services"`

### Task 10: sysdns — logic Manager (giả lập API)

**Files:**
- Create: `internal/sysdns/types.go`, `internal/sysdns/manager.go`, `internal/sysdns/debounce.go`, `internal/sysdns/manager_test.go`, `internal/sysdns/fakeapi_test.go`

**Interfaces:**
- Consumes: `model.AdapterSnapshot`, `model.FamilyDNS`, `model.DNSMode*`.
- Produces:
  - `type Adapter struct { GUID string; LUID uint64; IfIndex uint32; Alias string; IfType uint32; Up, HasGateway, HasIPv6 bool }`
  - `type API interface { Adapters() ([]Adapter, error); GetDNS(guid string, v6 bool) ([]string, error); SetDNS(guid string, v6 bool, servers []string) error; NetshSetDNS(ifIndex uint32, v6 bool, servers []string) error; Flush() error }` — `servers` rỗng nghĩa là "trở về DHCP"
  - `type RestoreError struct { GUID, Alias string; Err error }`
  - `type Manager struct`; `func NewManager(api API, sleep func(time.Duration)) *Manager`
  - `func (m *Manager) Select(mode string, guids []string) ([]Adapter, error)`
  - `func (m *Manager) Snapshot(ads []Adapter) ([]model.AdapterSnapshot, error)`
  - `func (m *Manager) ApplyLoopback(snaps []model.AdapterSnapshot, v6 bool) error`
  - `func (m *Manager) Restore(snaps []model.AdapterSnapshot) []RestoreError`
  - `func (m *Manager) LoopbackAdapters() ([]Adapter, error)`
  - `func (m *Manager) Flush() error`
  - `func Debounce(d time.Duration, f func()) (trigger func(), stop func())`

Quy tắc:
- **`Select("auto")`** chọn card có `IfType` thuộc {6, 71}, `Up` và `HasGateway`. `Select("manual", guids)` chọn theo GUID, bỏ qua GUID không còn tồn tại.
- **`Snapshot`**: danh sách server rỗng → `dhcp`, ngược lại → `static` kèm danh sách. `IPv6` chỉ được đọc khi card có `HasIPv6`.
- **`ApplyLoopback`** đặt `127.0.0.1` cho v4 và `::1` cho v6 (khi `v6` là true và card có IPv6).
- **`Restore`**, làm cho từng snapshot và tìm card theo **GUID**:
  - card không còn tồn tại → bỏ qua, không tính là lỗi;
  - `SetDNS` thử 3 lần, giữa các lần ngủ 200ms;
  - vẫn lỗi thì gọi `NetshSetDNS(IfIndex, …)`;
  - vẫn lỗi thì `NetshSetDNS(IfIndex, v6, nil)` để đưa về DHCP;
  - vẫn lỗi thì thêm một `RestoreError`.
  - Cuối cùng gọi `Flush()`.
- **`LoopbackAdapters`**: các card mà DNS đúng bằng `["127.0.0.1"]` hoặc `["::1"]`.

- [ ] **Step 1: Viết test** (`fakeAPI` lưu DNS theo GUID, có thể cài lỗi cho từng hàm và ghi lại các lần gọi)

```go
func TestSelectAuto(t *testing.T) // Ethernet Up có gateway ✓; Wi-Fi Down ✗; loopback (24) ✗; Ethernet không gateway ✗
func TestSnapshotApplyRestore_RoundTrip(t *testing.T) // v4 dhcp + v6 static [2001:db8::1] → Apply → v4=[127.0.0.1], v6=[::1] → Restore → v4 [] (dhcp), v6 [2001:db8::1]
func TestRestore_KeysByGUIDAndNetshUsesIfIndex(t *testing.T) { // Trọng tâm review #3
	// snapshot ghi Alias "Kết nối mạng cục bộ", IfIndex 12; adapter hiện tại cùng GUID nhưng Alias "Ethernet 2"; SetDNS luôn lỗi
	errs := m.Restore(snaps); require.Empty(t, errs)
	require.Equal(t, []string{"netsh:12:v4:"}, api.netshCalls) // dùng chỉ số; danh sách rỗng → dhcp
}
func TestRestore_RetriesThenFallsBackThenReports(t *testing.T) // SetDNS lỗi 3 lần, netsh static lỗi, netsh dhcp lỗi → 1 RestoreError; sleep được gọi 2 lần
func TestRestore_MissingAdapterSkipped(t *testing.T)
func TestRestore_Idempotent(t *testing.T) // Restore hai lần → trạng thái không đổi, không có lỗi
func TestLoopbackAdapters(t *testing.T)
func TestDebounce_CoalescesBursts(t *testing.T) // 5 lần trigger trong vòng 50ms với d=100ms → f chạy đúng 1 lần
```

- [ ] **Step 2: Chạy test — phải FAIL** — `go test ./internal/sysdns/...`
- [ ] **Step 3: Hiện thực** theo quy tắc trên.
- [ ] **Step 4: Chạy test — phải PASS**
- [ ] **Step 5: Commit** — `git commit -m "feat(sysdns): snapshot/apply/restore with GUID keying and netsh-by-index fallback"`

### Task 11: sysdns — API Windows thật + theo dõi card mạng

**Files:**
- Create: `internal/sysdns/api_windows.go`, `internal/sysdns/watch_windows.go`, `internal/sysdns/api_windows_test.go`, `internal/sysdns/api_integration_test.go` (`//go:build windows && integration`)

**Interfaces:**
- Consumes: `sysdns.API` (Task 10); `windows.GetAdaptersAddresses`, `windows.NotifyIpInterfaceChange`, `windows.CancelMibChangeNotify2`, `windows.GUIDFromString`.
- Produces: `func NewWindowsAPI() API`; `func Watch(onChange func()) (stop func(), err error)` (đã debounce 2s).

Chi tiết:
- Struct mirror, 64 byte:

```go
type dnsInterfaceSettings struct {
	Version uint32; _ [4]byte; Flags uint64
	Domain, NameServer, SearchList *uint16
	RegistrationEnabled, RegisterAdapterName, EnableLLMNR, QueryAdapterName uint32
	ProfileNameServer *uint16
}
```
- Hằng: `Version=1`, `DNS_SETTING_IPV6=0x1`, `DNS_SETTING_NAMESERVER=0x2`.
- **GUID truyền bằng con trỏ:** `proc.Call(uintptr(unsafe.Pointer(&guid)), uintptr(unsafe.Pointer(&s)))`.
- **Get:** chỉ đặt `Version` (cộng cờ IPV6 nếu là v6), tách `NameServer` theo dấu phẩy hoặc dấu cách, rồi gọi `FreeInterfaceDnsSettings`.
- **Set:** `Flags = NAMESERVER (| IPV6)`, `NameServer` = chuỗi UTF-16 nối bằng dấu phẩy. Danh sách rỗng thì gửi chuỗi rỗng để trở về DHCP.
- **`Adapters()`:** `GetAdaptersAddresses(AF_UNSPEC, GAA_FLAG_INCLUDE_GATEWAYS=0x80, …)`, thử lại khi gặp `ERROR_BUFFER_OVERFLOW`. `HasIPv6` là true khi `Ipv6IfIndex != 0`.
- **`NetshSetDNS`:** `netsh interface ipv4|ipv6 set dnsservers name=<IfIndex> source=static address=<ip> register=primary validate=no`, hoặc `source=dhcp` khi danh sách rỗng. Chạy bằng `winutil.HiddenCmd`.
- **`Flush`:** gọi `DnsFlushResolverCache` trong `dnsapi.dll`.
- **`Watch`:** callback gom sự kiện qua `Debounce(2*time.Second, onChange)`. **Không** gọi `CancelMibChangeNotify2` từ bên trong callback (sẽ deadlock).

- [ ] **Step 1: Viết test**

```go
// api_windows_test.go — chỉ đọc, không cần admin
func TestWindowsAPI_AdaptersHaveGUIDs(t *testing.T) // ≥1 card; GUID nào cũng parse được bằng windows.GUIDFromString
func TestWindowsAPI_GetDNSReadOnly(t *testing.T)    // GetDNS cho từng card không lỗi
func TestSettingsStructSize(t *testing.T)           // unsafe.Sizeof(dnsInterfaceSettings{}) == 64
// api_integration_test.go — CẦN ADMIN
func TestIntegration_ApplyAndRestoreRealAdapter(t *testing.T) // Select auto → Snapshot → ApplyLoopback → GetDNS == [127.0.0.1] → Restore → GetDNS khớp snapshot
func TestIntegration_SetEmptyRevertsToDHCP(t *testing.T)      // xác minh hành vi "danh sách rỗng = DHCP", spec ghi là chưa được kiểm chứng
```

- [ ] **Step 2: Chạy test — phải FAIL** — `go test ./internal/sysdns/...`
- [ ] **Step 3: Hiện thực** theo chi tiết trên.
- [ ] **Step 4: Chạy test — phải PASS** (test không cần admin). Ghi trong báo cáo: `go test -tags integration ./internal/sysdns/...` đang chờ người dùng chạy bằng terminal admin. Nếu test DHCP thất bại, đổi đường "trở về DHCP" thành `netsh … source=dhcp` và ghi lại quyết định đó.
- [ ] **Step 5: Commit** — `git commit -m "feat(sysdns): Win32 IP Helper DNS settings, netsh fallback, change watcher"`

### Task 12: startup — tác vụ Task Scheduler

**Files:**
- Create: `internal/startup/xml.go`, `internal/startup/xml_test.go`, `internal/startup/tasks_windows.go`, `internal/startup/tasks_integration_test.go` (`integration`)

**Interfaces:**
- Produces: `type Task struct { Name, Exe, Args, UserID string; TimeLimit string }`; `func TaskXML(t Task) string`; `func Create(t Task) error`; `func Delete(name string) error` (không có tác vụ thì trả nil); `func Exists(name string) bool`; `func CurrentUserID() (string, error)` (dạng `DOMAIN\user`); `func AutostartTask(exe string) Task` (`--autostart`, `TimeLimit "PT0S"`); `func RecoveryTask(exe string) Task` (`--restore`, `TimeLimit "PT5M"`).

Chi tiết:
- XML là task schema 1.2 gồm:
  - `<LogonTrigger><UserId>`;
  - `<Principal><UserId>…</UserId><LogonType>InteractiveToken</LogonType><RunLevel>HighestAvailable</RunLevel>`;
  - `<Settings>` với `<DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>`, `<StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>`, `<ExecutionTimeLimit>` lấy từ `TimeLimit`, `<MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>`;
  - `<Exec><Command>` và `<Arguments>`, escape XML bằng `encoding/xml`.
- `Create` ghi XML ra file tạm dạng UTF-16LE có BOM, rồi chạy `schtasks /Create /TN <name> /XML <file> /F`.
- `Delete` chạy `schtasks /Delete /TN <name> /F`.

- [ ] **Step 1: Viết test**

```go
func TestTaskXML(t *testing.T) {
	x := startup.TaskXML(startup.Task{Name: "Ghostline", Exe: `C:\Program Files\Ghostline\ghostline.exe`, Args: "--autostart", UserID: `PC\Đức`, TimeLimit: "PT0S"})
	require.Contains(t, x, "<RunLevel>HighestAvailable</RunLevel>"); require.Contains(t, x, "<LogonTrigger>")
	require.Contains(t, x, `<Command>C:\Program Files\Ghostline\ghostline.exe</Command>`)
	require.Contains(t, x, "<Arguments>--autostart</Arguments>"); require.Contains(t, x, `<UserId>PC\Đức</UserId>`)
	require.NoError(t, xml.Unmarshal([]byte(x), new(any)))
}
func TestRecoveryTask(t *testing.T) // Args "--restore", TimeLimit "PT5M", Name == brand.TaskRecovery
func TestIntegration_CreateExistsDelete(t *testing.T) // CẦN ADMIN, dùng tên "Ghostline Test"
```

- [ ] **Step 2: Chạy test — phải FAIL** — `go test ./internal/startup/...`
- [ ] **Step 3: Hiện thực** theo chi tiết trên.
- [ ] **Step 4: Chạy test — phải PASS** (test không cần admin). Ghi lại test `integration` đang chờ người dùng chạy.
- [ ] **Step 5: Commit** — `git commit -m "feat(startup): logon tasks for autostart and recovery via schtasks XML"`

### Task 13: watchdog — khôi phục khi process chủ đã chết

**Files:**
- Create: `internal/watchdog/recover.go`, `internal/watchdog/run.go`, `internal/watchdog/recover_test.go`

**Interfaces:**
- Consumes: `store.StateStore`, `store.ErrStateCorrupt`, `model.AdapterSnapshot`.
- Produces:
  - `type Restorer interface { Restore([]model.AdapterSnapshot) []sysdns.RestoreError; LoopbackAdapters() ([]sysdns.Adapter, error) }`
  - `type Deps struct { States *store.StateStore; DNS Restorer; StopDPI func() error; Alive func(pid uint32, start time.Time) bool; Log *slog.Logger }`
  - `type Outcome int` (`NothingToDo`, `Restored`, `RestoredFromCorrupt`, `OwnerAlive`)
  - `func RestoreIfOrphaned(d Deps) (Outcome, error)`
  - `func RunWatchdog(parentPID uint32, parentStart time.Time, wait func(pid uint32) error, d Deps) error`
  - `func RunRestore(d Deps) error`

Logic của `RestoreIfOrphaned`, toàn bộ chạy bên trong `States.Update`:
1. **State bị hỏng** (`ErrStateCorrupt`):
   - Lấy `LoopbackAdapters()`, ép DNS của chúng về DHCP: tạo snapshot giả có `Mode=dhcp` cho cả v4 và v6, rồi gọi `Restore`.
   - Gọi `StopDPI`.
   - Ghi state mới `clean`, trả `RestoredFromCorrupt`.
2. **`Phase == clean`** → `NothingToDo`.
3. **`Alive(PID, PIDStartTime)`** → `OwnerAlive`.
4. **Còn lại:**
   - `Restore(Snapshot)`;
   - gọi `StopDPI` nếu `DPI.Running`;
   - ghi `Phase = clean`;
   - trả `Restored`, kèm lỗi gộp nếu có `RestoreError`.

`RunWatchdog` chờ process cha thoát qua `wait` (trên Windows, `wait` mở handle với quyền `SYNCHRONIZE` và gọi `WaitForSingleObject`), rồi gọi `RestoreIfOrphaned`.

- [ ] **Step 1: Viết test**

```go
func TestRestore_CleanDoesNothing(t *testing.T)
func TestRestore_OwnerAliveDoesNothing(t *testing.T)
func TestRestore_PIDReusedStillRestores(t *testing.T) // Alive trả false vì start time khác → Restored
func TestRestore_RestoresSnapshotStopsDPIMarksClean(t *testing.T)
func TestRestore_CorruptStateFallsBackToLoopbackAdapters(t *testing.T) { // Trọng tâm review #2
	os.WriteFile(statePath, []byte(`{"phase":"dns_`), 0o644)
	out, err := watchdog.RestoreIfOrphaned(d); require.NoError(t, err); require.Equal(t, watchdog.RestoredFromCorrupt, out)
	require.Equal(t, model.DNSModeDHCP, fakeDNS.restored[0].IPv4.Mode); st, _ := d.States.Load(); require.Equal(t, store.PhaseClean, st.Phase)
}
func TestRunWatchdog_RestoresAfterParentExit(t *testing.T) // wait giả trả về ngay; state dns_set, chủ đã chết → snapshot được khôi phục
```

- [ ] **Step 2: Chạy test — phải FAIL** — `go test ./internal/watchdog/...`
- [ ] **Step 3: Hiện thực**, rồi nối chế độ `--watchdog` và `--restore` trong `main.go`:
  - tạo `store.Paths`, `NewStateStore` với `winutil.NewNamedMutex(brand.StateMutex)`, `sysdns.NewManager(sysdns.NewWindowsAPI(), time.Sleep)`;
  - `StopDPI` dùng `dpi` ở Task 14. Trong lúc chờ Task 14, truyền một hàm chỉ dừng và xoá service `WinDivert` qua `winutil`, rồi thay thế ở Task 14.
- [ ] **Step 4: Chạy test — phải PASS**
- [ ] **Step 5: Commit** — `git commit -m "feat(watchdog): orphan-aware restore, corrupt-state fallback, watchdog/restore run modes"`

### Task 14: dpi — GoodbyeDPI

**Files:**
- Create: `assets/goodbyedpi/embed.go` (+ 3 file nhị phân + `LICENSE-goodbyedpi.txt`, `LICENSE-windivert.txt`), `internal/dpi/{preset,validate,assets,manager}.go`, `internal/dpi/runner_windows.go`, tests
- Modify: `main.go` (thay hàm `StopDPI` tạm ở Task 13)

**Interfaces:**
- Consumes: `winutil.HiddenCmd`, `winutil.NewKillOnCloseJob`, `winutil.ServiceRunning`, `winutil.StopService`, `winutil.DeleteService`.
- Produces:
  - `type Preset string` (`light`, `medium`, `high`, `extreme`, `mode1`…`mode6`, `custom`); `var AutotuneOrder = []Preset{"light", "medium", "high", "extreme"}`
  - `type Scope string` (`ScopeAll="all"`, `ScopeBlacklist="blacklist"`)
  - `func Args(p Preset, custom string, scope Scope, blacklistPath string) ([]string, error)`
  - `func ValidateCustom(s string) ([]string, error)`; `var ErrForbiddenFlag`
  - `var Pinned map[string]string` (3 hash ở phần Ràng buộc chung)
  - `func Extract(src fs.FS, dir string) error`; `func Verify(dir string) error`; `var ErrHashMismatch`
  - `type Process interface { PID() int; Exited() bool; Kill() error }`
  - `type Runner interface { Start(exe string, args []string, dir string) (Process, error) }`
  - `type Services interface { Running(name string) (bool, error); Stop(name string) error; Delete(name string) error }`
  - `type Manager struct`; `func NewManager(dir string, assets fs.FS, r Runner, s Services, sleep func(time.Duration)) *Manager`
  - `func (m *Manager) Start(ctx context.Context, args []string) (int, error)`; `func (m *Manager) Stop() error`; `func (m *Manager) Running() bool`
  - `var ErrStartFailed`, `ErrBlockedByAV`
  - `func NewWindowsRunner() Runner`; `func NewWindowsServices() Services`

Tham số preset dùng **đúng** bảng ở spec §7.1, lưu dạng `[]string` (ví dụ light = `{"-p","-r","-s","-m","-e","40","-w","--native-frag"}`, `modeN` = `{"-N"}`). `ScopeBlacklist` thêm hai phần tử riêng `"--blacklist", path`.

Danh sách cờ hợp lệ cho `ValidateCustom`:

| Loại | Cờ |
|---|---|
| Không có tham số | `-p -r -s -m -n -a -w -1 -2 -3 -4 -5 -6 --native-frag --reverse-frag --wrong-chksum --wrong-seq --allow-no-sni` |
| Cần một tham số số | `-f -k -e --port --set-ttl --min-ttl --max-payload` |
| Cần một tham số khác | `--ip-id <id>`; `--auto-ttl` nhận thêm `A-B-C` nếu token kế tiếp có dạng đó |

Ngoài các cờ trên, `--blacklist`, mọi cờ `--dns-*` và mọi cờ lạ khác → `ErrForbiddenFlag`. Tách token theo dấu cách, có tôn trọng dấu nháy kép.

Hành vi `Manager`:
- **`Start`:**
  - gọi `Verify`; nếu lỗi thì `Extract` rồi `Verify` lại; vẫn lỗi → `ErrHashMismatch`;
  - `Runner.Start(dir\goodbyedpi.exe, args, dir)`, rồi `sleep(2s)`;
  - process đã thoát → `ErrBlockedByAV` nếu file exe hoặc `WinDivert64.sys` đã biến mất hay bị khoá, ngược lại → `ErrStartFailed`;
  - `Services.Running("WinDivert")` là false → `ErrStartFailed`.
- **`Stop`:** `Kill`, `Stop("WinDivert")` rồi `Delete("WinDivert")`. Lỗi "không tồn tại" được bỏ qua.
- **Runner Windows:** `winutil.HiddenCmd` cộng `Job.Assign`.

- [ ] **Step 1: Đặt file nhị phân.**
  - Tải `goodbyedpi-0.2.2.zip` bằng `gh release download 0.2.2 -R ValdikSS/GoodbyeDPI`.
  - Chép 3 file trong thư mục `x86_64/` cùng `licenses/LICENSE-goodbyedpi.txt` và `licenses/LICENSE-windivert.txt` vào `assets/goodbyedpi/`.
  - Chạy `sha256sum` và đối chiếu với các hash đã ghim; **lệch thì dừng lại và báo cáo**.
  - `embed.go`: `//go:embed goodbyedpi.exe WinDivert.dll WinDivert64.sys` `var FS embed.FS`.
  - Nếu Windows Defender cách ly file, ghi lại trong báo cáo (đây là rủi ro đã nêu ở spec §14).

- [ ] **Step 2: Viết test**

```go
func TestArgs_Presets(t *testing.T) // medium == light + {"--auto-ttl","1-4-10","--min-ttl","3"}; extreme đúng spec; mode3 == {"-3"}; không preset nào chứa "--dns-addr"
func TestArgs_BlacklistPathWithSpacesIsSingleArg(t *testing.T) { // Trọng tâm review #4
	p := `C:\Users\Đức Hạnh\AppData\Roaming\Ghostline\dpi-blacklist.txt`
	a, err := dpi.Args("light", "", dpi.ScopeBlacklist, p); require.NoError(t, err)
	require.Equal(t, []string{"--blacklist", p}, a[len(a)-2:])
}
func TestValidateCustom(t *testing.T) // "-p -e 40 --auto-ttl 1-4-10" ok; "--dns-addr 1.1.1.1" → ErrForbiddenFlag; "--blacklist x" → ErrForbiddenFlag; "-e" thiếu số → lỗi; "--evil" → ErrForbiddenFlag
func TestExtractVerify(t *testing.T) {
	// Extract/Verify ủy quyền cho extractWith(src, dir, pins) / verifyWith(dir, pins) (không export) để test tiêm pins riêng.
	pins := map[string]string{"a.bin": sha256hex("hello")}
	src := fstest.MapFS{"a.bin": {Data: []byte("hello")}}
	require.NoError(t, extractWith(src, dir, pins)); require.NoError(t, verifyWith(dir, pins))
	os.WriteFile(filepath.Join(dir, "a.bin"), []byte("evil"), 0o644)
	require.ErrorIs(t, verifyWith(dir, pins), ErrHashMismatch)
	// extractWith ghi đè file sai; file đúng thì không ghi lại (mtime không đổi)
}
func TestManager_StartSuccess(t *testing.T)        // runner giả còn sống, service giả đang chạy → pid
func TestManager_ExitedImmediatelyIsStartFailed(t *testing.T)
func TestManager_StopKillsAndRemovesWinDivert(t *testing.T) // các lần gọi == ["kill","svc.stop:WinDivert","svc.delete:WinDivert"]
```

- [ ] **Step 3: Chạy test — phải FAIL** — `go test ./internal/dpi/...`
- [ ] **Step 4: Hiện thực** theo hành vi trên, và thay hàm `StopDPI` tạm trong `main.go` bằng `dpi.Manager.Stop`.
- [ ] **Step 5: Chạy test — phải PASS**
- [ ] **Step 6: Commit** — `git commit -m "feat(dpi): GoodbyeDPI 0.2.2 presets, flag validation, hash-pinned extraction, job-bound runner"`

### Task 15: probe — thử truy cập trang mẫu

**Files:**
- Create: `internal/probe/probe.go`, `internal/probe/probe_test.go`

**Interfaces:**
- Produces:
  - `type Stage string` (`StageOK="ok"`, `StageDNS="dns"`, `StageTCP="tcp"`, `StageTLS="tls"`, `StageHTTP="http"`)
  - `type Result struct { Site string; Stage Stage; Latency time.Duration; Err string }`
  - `type Prober struct { Resolve func(ctx context.Context, host string) ([]netip.Addr, error); Dial func(ctx context.Context, network, addr string) (net.Conn, error); Timeout time.Duration; RootCAs *x509.CertPool; Port string }` (`Port` mặc định `"443"`)
  - `func (p Prober) Probe(ctx context.Context, site string) Result`
  - `func (p Prober) ProbeAll(ctx context.Context, sites []string) []Result`
  - `func DPIBlocked(first, second []Result) []string`

`Probe` chạy 4 tầng nối tiếp, dừng ở tầng đầu tiên bị lỗi:
1. resolve;
2. TCP dial `ip:Port`;
3. `tls.Client` handshake với SNI là `site`;
4. `GET /` qua chính kết nối TLS đó, bằng `http.Transport{DialTLSContext}` hoặc tự ghi request.

Bất kỳ phản hồi HTTP nào cũng tính là `ok`. Toàn bộ chịu chung một timeout. `DPIBlocked` trả về các site có `Stage == StageTLS` ở **cả hai** lần thử.

- [ ] **Step 1: Viết test**

```go
func TestProbe_OK(t *testing.T)        // httptest TLS server; Resolve→127.0.0.1; site "example.com"; RootCAs của server → StageOK
func TestProbe_DNSFailure(t *testing.T) // Resolve trả lỗi → StageDNS
func TestProbe_TCPRefused(t *testing.T) // cổng đã đóng → StageTCP
func TestProbe_TLSReset(t *testing.T)   // listener nhận kết nối, đọc ClientHello rồi đóng → StageTLS
func TestDPIBlocked_RequiresBothAttempts(t *testing.T) // a: tls/tls → bị chặn; b: tls/ok → không; c: tcp/tcp → không
```

- [ ] **Step 2: Chạy test — phải FAIL** — `go test ./internal/probe/...`
- [ ] **Step 3: Hiện thực** theo 4 tầng trên.
- [ ] **Step 4: Chạy test — phải PASS**
- [ ] **Step 5: Commit** — `git commit -m "feat(probe): staged site probing and DPI-block classification"`

### Task 16: updater — bản mới, danh sách server tải về, danh sách DNSCrypt

**Files:**
- Create: `internal/updater/{release,lists}.go` (+ tests), `internal/store/meta.go` (+ test)

**Interfaces:**
- Consumes: `servers.ParseList`, `servers.VerifySigned`, `servers.VerifyMinisign`, `servers.ParseDNSCryptMarkdown`, `testutil.SignMinisign`.
- Produces:
  - `store`: `type Meta struct { LastUpdateCheck, LastServerList, LastDNSCrypt time.Time }`; `func LoadMeta(path string) Meta`; `func SaveMeta(path string, m Meta) error`
  - `updater`:
    - `type Release struct { Tag, URL string }`
    - `func Latest(ctx context.Context, c *http.Client, apiURL string) (Release, error)` (đọc `tag_name` và `html_url`)
    - `func Newer(current, tag string) bool` (dùng `golang.org/x/mod/semver`; nếu `current == "dev"` hoặc tag không hợp lệ thì trả false)
    - `func Due(last, now time.Time) bool` (khoảng cách ≥ 24h)
    - `func FetchServerList(ctx context.Context, c *http.Client, url, sigURL string, pub ed25519.PublicKey) (list servers.List, raw, sig []byte, err error)`
    - `func FetchDNSCrypt(ctx context.Context, c *http.Client, urls []string, minisignKey string) (md, sig []byte, err error)` (thử lần lượt từng URL; với mỗi URL tải `md` và `md + ".minisig"`, rồi kiểm tra chữ ký)

- [ ] **Step 1: Viết test**

```go
func TestNewer(t *testing.T) // ("0.1.0","v0.2.0") true; ("0.2.0","v0.2.0") false; ("dev","v9.0.0") false; ("0.1.0","garbage") false
func TestLatest(t *testing.T) // httptest trả {"tag_name":"v0.2.0","html_url":"https://x/r"} → Release{"v0.2.0","https://x/r"}
func TestFetchServerList_BadSignature(t *testing.T) // errors.Is(err, servers.ErrBadSignature)
func TestFetchDNSCrypt_FallsBackToSecondURL(t *testing.T) // URL đầu trả 500, URL thứ hai trả md + chữ ký đúng → ok
func TestFetchDNSCrypt_BadSignatureEverywhere(t *testing.T) // → lỗi
func TestDue(t *testing.T); func TestMeta_RoundTrip(t *testing.T)
```

- [ ] **Step 2: Chạy test — phải FAIL** — `go test ./internal/updater/... ./internal/store/...`
- [ ] **Step 3: Hiện thực** theo interfaces trên.
- [ ] **Step 4: Chạy test — phải PASS**
- [ ] **Step 5: Commit** — `git commit -m "feat(updater): release check, signed server list and minisign-verified DNSCrypt fetch"`

---

## Mốc 4 — Bộ điều phối

### Task 17: app — máy trạng thái, Connect/Disconnect/Cancel

**Files:**
- Create: `internal/app/{status,errors,deps,steps,orchestrator}.go`, `internal/app/orchestrator_test.go`, `internal/app/fakes_test.go`

**Interfaces:**
- Consumes: `engine.Config`, `store.StateStore`, `store.Settings`, `model.*`, `sysdns.Adapter`, `sysdns.RestoreError`, `winutil.PortOwner`, `watchdog.Outcome`.
- Produces:
  - `type Status string` (`StatusDisconnected="disconnected"`, `StatusConnecting="connecting"`, `StatusProtected="protected"`, `StatusDegraded="degraded"`, `StatusDisconnecting="disconnecting"`, `StatusError="error"`)
  - `type AppError struct { Code string; Params map[string]any }`; mã lỗi gồm các mã ở spec §10 cộng `CodeAutotuneNoPreset = "AUTOTUNE_NO_PRESET"`, `CodeInternal = "INTERNAL"` (params `{step}`, dùng cho lỗi không có mã riêng như watchdog/task) và mã cảnh báo `CodeSettingsReset = "SETTINGS_RESET"`
  - `type Snapshot struct { Status Status; Step int; Error *AppError; Warnings []AppError; Servers []string; Since time.Time; LatencyMs int; Queries uint64; DPI DPIStatus; BlockedSites []string }`; `type DPIStatus struct { Enabled, Running bool; Preset string }`
  - `func (o *Orchestrator) AddWarning(e AppError)`; `func (o *Orchestrator) ClearWarning(code string)` — cảnh báo cố định (`RESTORE_FAILED`, `SETTINGS_RESET`) nằm trong `Snapshot.Warnings` tới khi được xoá
  - Interface phụ thuộc trong `deps.go`:
    - `Engine { Start(context.Context, engine.Config) error; Swap(context.Context, []upstream.Upstream) error; Stop(context.Context) error; SelfTest(context.Context) error; ExpectVerify(string); SawVerify(string) bool; Stats() engine.Stats }`
    - `DNS { Select(string, []string) ([]sysdns.Adapter, error); Snapshot([]sysdns.Adapter) ([]model.AdapterSnapshot, error); ApplyLoopback([]model.AdapterSnapshot, bool) error; Restore([]model.AdapterSnapshot) []sysdns.RestoreError; Flush() error }`
    - `DPI { Start(context.Context, []string) (int, error); Stop() error; Running() bool }`
    - `Safety { StartWatchdog(pid uint32, start time.Time) (stop func() error, err error); CreateRecoveryTask() error; DeleteRecoveryTask() error }`
    - `System { IsAdmin() bool; PortOwners(uint16) ([]winutil.PortOwner, error); SelfPID() (uint32, time.Time); IPv6Available() bool }`
    - `Picker { Pick(ctx context.Context, onProgress func(done, total int)) ([]model.Server, error) }`
    - `Builder { Build(model.Server) (upstream.Upstream, error) }`
    - `Resolver { LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error) }` (resolver hệ thống, chỉ dùng cho bước xác minh)
    - `Recover func() (watchdog.Outcome, error)`
    - `Sink { State(Snapshot); Log(LogEvent) }`; `type LogEvent struct { Time time.Time; Source, Code string; Params map[string]any }`
  - `type Deps struct { Engine; DNS; DPI; Safety; System; Picker; Builder; Resolver; Recover; Sink; States *store.StateStore; Settings func() store.Settings; Now func() time.Time }`
  - `func New(d Deps) *Orchestrator`; `func (o *Orchestrator) Connect(ctx context.Context) error`; `func (o *Orchestrator) Cancel()`; `func (o *Orchestrator) Disconnect(ctx context.Context) error`; `func (o *Orchestrator) Snapshot() Snapshot`
  - `steps.go`: `type step struct { name string; do, undo func(context.Context) error }`; `func runSteps(ctx context.Context, steps []step, onStep func(int)) error`. Khi bước thứ i lỗi, `runSteps` gọi `undo` của các bước i−1…0 và log các lỗi undo, sau đó trả lỗi ban đầu.

Các bước Connect, đúng spec §5.2:

| # | Bước | Do | Undo |
|---|---|---|---|
| 01 | preflight | `IsAdmin` (sai → `NOT_ADMIN`). `Recover()` nếu state chưa `clean`. `PortOwners(53)` không rỗng → `PORT53_BUSY{pid, name, service}` | — |
| 02 | pick | Gọi `Pick`. Trả rỗng → `NO_SERVERS` với `{checked, elapsed}` lấy từ lỗi của picker | — |
| 03 | engine | `Build` từng server, `Start` với `127.0.0.1:53` (thêm `[::1]:53` nếu `IPv6Available`), rồi `SelfTest`. Lỗi → `ENGINE_SELFTEST_FAILED` | `Stop` |
| 04 | snapshot | `Select(settings.Adapters, settings.AdapterGUIDs)` → `Snapshot` → `States.Update` ghi `Phase=dns_set`, `PID`, `PIDStartTime`, `StartedAt`, `Snapshot` | `States.Update(Phase=clean)` |
| 05 | safety | `StartWatchdog`, rồi `CreateRecoveryTask` | `DeleteRecoveryTask`, gọi `stop` của watchdog |
| 06 | apply | `ApplyLoopback(snap, IPv6Available)` rồi `Flush` | `Restore(snap)` rồi `Flush` |
| 07 | verify | nonce ngẫu nhiên; `ExpectVerify`; `Resolver.LookupNetIP(ctx, "ip4", nonce+".verify.ghostline.test")`. Phải nhận đúng `192.0.2.1` **và** `SawVerify(nonce)`, ngược lại → `VERIFY_LEAK` | — |

Quy tắc chung:
- `Connect` giữ một mutex `opMu` trong suốt quá trình chạy.
- Nếu `Connect` được gọi khi trạng thái **không phải** `disconnected` hoặc `error` thì trả nil ngay, không làm gì.
- `Cancel` huỷ context của lần Connect đang chạy. Bị huỷ → hoàn tác các bước, trạng thái về `disconnected` (không phải `error`).

Disconnect, đúng spec §5.3: `Restore(snapshot)` → `Flush` → `DPI.Stop` (nếu đang chạy) → `Engine.Stop` → `States.Update(clean)` → gọi `stop` của watchdog → `DeleteRecoveryTask`. Mọi bước trạng thái đều được phát qua `Sink.State`.
- Nếu `Restore` trả `RestoreError`, mỗi lỗi thành một `AddWarning(RESTORE_FAILED{adapter: Alias})`. Disconnect **vẫn** làm tiếp các bước còn lại, nhưng **không** đánh dấu `clean`, để lớp 3 và lớp 4 khôi phục lại sau.
- Lỗi của watchdog hoặc của tác vụ Task Scheduler trong lúc Connect dùng mã `INTERNAL{step}`.

- [ ] **Step 1: Viết test.** Các fake dùng chung một `rec *[]string`. Engine giả: `SawVerify` trả true, Resolver giả trả `192.0.2.1`.

```go
func TestConnect_HappyPathOrder(t *testing.T) {
	require.NoError(t, o.Connect(ctx))
	require.Equal(t, []string{"sys.admin", "sys.ports", "pick", "build", "engine.start", "engine.selftest", "dns.select", "dns.snapshot",
		"state.dns_set", "safety.watchdog", "safety.task.create", "dns.apply", "dns.flush", "engine.expect", "resolve", "engine.saw"}, *rec)
	require.Equal(t, app.StatusProtected, o.Snapshot().Status)
}
func TestConnect_FailureAtEachStepRollsBack(t *testing.T) {
	cases := []struct{ fail string; code string; undo []string }{
		{"pick", "NO_SERVERS", nil},
		{"engine.selftest", "ENGINE_SELFTEST_FAILED", []string{"engine.stop"}},
		{"dns.snapshot", "SET_DNS_FAILED", []string{"engine.stop"}},
		{"safety.watchdog", "INTERNAL", []string{"state.clean", "engine.stop"}},
		{"dns.apply", "SET_DNS_FAILED", []string{"safety.task.delete", "safety.watchdog.stop", "state.clean", "engine.stop"}},
		{"engine.saw", "VERIFY_LEAK", []string{"dns.restore", "dns.flush", "safety.task.delete", "safety.watchdog.stop", "state.clean", "engine.stop"}},
	}
	// mỗi case: lỗi được cài ở cases.fail → Snapshot().Status == StatusError, Error.Code == code,
	// các lần gọi sau điểm lỗi khớp đúng undo theo thứ tự; state.json kết thúc ở PhaseClean.
	// Picker giả trả đúng 1 server nên "build" chỉ xuất hiện 1 lần.
}
func TestDisconnect_RestoresBeforeEngineStop(t *testing.T) // đuôi các lần gọi == ["dns.restore","dns.flush","engine.stop","state.clean","safety.watchdog.stop","safety.task.delete"]
func TestDisconnect_RestoreFailureWarnsAndKeepsStateDirty(t *testing.T) // Restore trả 1 RestoreError → Warnings[0].Code == "RESTORE_FAILED"; state vẫn dns_set; engine vẫn được Stop
func TestConnect_CancelDuringPickRollsBackToDisconnected(t *testing.T)
func TestConnect_ConcurrentCallsAreSerialized(t *testing.T) { // Trọng tâm review #1
	// Pick bị chặn bởi channel; gọi Connect ở 2 goroutine; mở channel → Pick đúng 1 lần, dns.snapshot đúng 1 lần, cả hai trả nil
}
func TestConnect_NotAdmin(t *testing.T)          // Code NOT_ADMIN, không có lời gọi nào sau sys.admin
func TestConnect_Port53Busy(t *testing.T)        // Params["pid"]==1234, ["name"]=="svchost.exe", ["service"]=="SharedAccess"
func TestConnect_DirtyStateRecoversFirst(t *testing.T) // state dns_set → Recover được gọi trước "sys.ports"
```

- [ ] **Step 2: Chạy test — phải FAIL** — `go test ./internal/app/...`
- [ ] **Step 3: Hiện thực** theo bảng bước và quy tắc trên.
- [ ] **Step 4: Chạy test — phải PASS** — chạy thêm `go test -race ./internal/app/...`.
- [ ] **Step 5: Commit** — `git commit -m "feat(app): connect saga with rollback, ordered disconnect, cancel and serialization"`

### Task 18: app — picker, sức khoẻ, đổi mạng, thức dậy, trang mẫu, tự dò

**Files:**
- Create: `internal/app/{picker,health,autotune}.go` (+ tests)
- Modify: `internal/app/orchestrator.go`, `internal/app/deps.go`

**Interfaces:**
- Consumes: `scanner.Scan`, `scanner.Order`, `scanner.Cache`, `scanner.Checker`, `servers.Filter`, `probe.Prober`, `probe.DPIBlocked`, `dpi.Args`, `dpi.AutotuneOrder`.
- Produces:
  - `type ScanPicker struct { Catalog func() []model.Server; Checker scanner.Checker; Cache *scanner.Cache; SaveCache func(*scanner.Cache) error; NetKey func() string; Settings func() store.Settings; Now func() time.Time; Rand *rand.Rand }`, hiện thực `Picker`
  - `type NoServersError struct { Checked int; Elapsed time.Duration }`
  - `deps.go` thêm vào `Deps`:
    - `Prober interface { ProbeAll(ctx context.Context, sites []string) []probe.Result }`
    - `Ticker func(d time.Duration) (<-chan time.Time, func())`
    - `BlacklistPath string`
    - `SaveSettings func(store.Settings) error`
  - Phương thức mới của `Orchestrator`:
    - `func (o *Orchestrator) OnNetworkChange(ctx context.Context)`
    - `func (o *Orchestrator) OnResume(ctx context.Context)`
    - `func (o *Orchestrator) Autotune(ctx context.Context, onProgress func(preset string, i, n int)) error`
    - `func (o *Orchestrator) ProbeNow(ctx context.Context) []probe.Result`
    - `func (o *Orchestrator) SetDPIEnabled(ctx context.Context, on bool) error`
    - `func (o *Orchestrator) Rescan(ctx context.Context, onProgress func(done, total int, r scanner.Result)) ([]scanner.Result, error)` (quét toàn bộ, ghi cache)
    - `func (o *Orchestrator) ScanResults() []scanner.Result`

`ScanPicker.Pick`:
- `PinnedOnly` → chỉ quét các server đã ghim, `Want = MaxUpstreams`.
- Ngược lại:
  - lọc server bằng `Filter(Catalog(), IncludeTags)`;
  - `Cache.Fresh(NetKey(), now, 24h)` có ít nhất `MaxUpstreams` kết quả OK → lấy top N, **không quét**;
  - không thì `Scan(Order(…, Cache.EverOK(), Rand), Workers 16, Want MaxUpstreams, Budget 20s)`, `Put` vào cache rồi `SaveCache`.
- Không có server OK nào → `*NoServersError`.

Sức khoẻ (goroutine chạy khi trạng thái là ĐÃ_BẢO_VỆ):
- mỗi tick 30s gọi `Engine.SelfTest` **và** kiểm tra `Stats().PerUpstream` để phát hiện upstream lỗi;
- khi tất cả upstream lỗi liên tục ≥ 15s → `degraded` → `Pick` → `Build` → `Engine.Swap` → `protected`.

Đổi mạng:
- `Select` ra những GUID chưa có trong snapshot → `Snapshot` các card mới;
- `States.Update` thêm vào snapshot **trước**, rồi mới `ApplyLoopback`.

Thức dậy: gọi `SelfTest`; lỗi → chạy ngay luồng suy giảm.

Sau khi Connect thành công, chạy ở nền:
- `ProbeAll` hai lần → `DPIBlocked`;
- kết quả không rỗng → đặt `Snapshot.BlockedSites` và phát state.

Tự dò, theo đúng spec §7.2:
- Với lần lượt từng preset trong `AutotuneOrder`:
  - `DPI.Stop()`; `DPI.Start(Args(preset, "", Scope, BlacklistPath))`; chờ 2s; `ProbeAll(BlockedSites)`;
  - nếu không còn site nào ở `StageTLS` → lưu `DPI.Enabled=true`, `Preset=preset` qua `SaveSettings`, giữ GoodbyeDPI chạy, xoá `BlockedSites`, rồi trả nil.
- Không preset nào được → `DPI.Stop()` và trả lỗi `AUTOTUNE_NO_PRESET`.

Đổi lỗi DPI thành mã, dùng chung cho `SetDPIEnabled` và tự dò: `dpi.ErrHashMismatch` → `DPI_HASH_MISMATCH`; `dpi.ErrBlockedByAV` → `DPI_BLOCKED_BY_AV`; các lỗi khởi động khác → `DPI_START_FAILED`. Khi tự dò gặp `ErrHashMismatch` hoặc `ErrBlockedByAV` thì dừng ngay và trả đúng mã đó, không thử preset kế tiếp.

- [ ] **Step 1: Viết test**

```go
func TestPicker_FreshCacheSkipsScan(t *testing.T)      // checker giả không được gọi lần nào
func TestPicker_StaleCacheQuickScansAndSaves(t *testing.T)
func TestPicker_PinnedOnly(t *testing.T)
func TestPicker_NoneOKReturnsNoServersError(t *testing.T)
func TestHealth_DegradesAfter15sAndSwaps(t *testing.T) // ticker giả + Now giả; lỗi lúc t=0,30 → không đổi; t=45 → Swap được gọi, status quay về protected
func TestNetworkChange_NewAdapterSnapshottedBeforeApply(t *testing.T) // rec: "state.append" đứng trước "dns.apply:{guid}"
func TestResume_SelfTestFailureTriggersSwap(t *testing.T)
func TestPostConnectProbe_SetsBlockedSitesOnlyForTLS(t *testing.T)
func TestAutotune_StopsAtFirstWorkingPreset(t *testing.T) // light vẫn TLS, medium ok → Preset "medium" được lưu, DPI đang chạy, onProgress ("light",1,4),("medium",2,4)
func TestAutotune_NoneWork(t *testing.T) // Code AUTOTUNE_NO_PRESET; DPI.Stop là lời gọi cuối
func TestAutotune_BlockedByAVStopsImmediately(t *testing.T) // Start trả dpi.ErrBlockedByAV → Code DPI_BLOCKED_BY_AV, chỉ thử 1 preset
func TestSetDPIEnabled_MapsHashMismatch(t *testing.T)       // → Code DPI_HASH_MISMATCH
```

- [ ] **Step 2: Chạy test — phải FAIL** — `go test ./internal/app/...`
- [ ] **Step 3: Hiện thực** theo các mô tả trên.
- [ ] **Step 4: Chạy test — phải PASS**, kèm `-race`.
- [ ] **Step 5: Commit** — `git commit -m "feat(app): scan picker, health/degraded hot swap, network change, resume, probing, DPI autotune"`

### Task 19: app — Service cho Wails, sự kiện, bộ đệm log

**Files:**
- Create: `internal/app/{service,events,logbuf}.go` (+ tests)

**Interfaces:**
- Consumes: `Orchestrator` (Task 17–18), `servers.ParseImport`, `store.SaveSettings`, `engine.QueryEvent`.
- Produces:
  - `type Emitter interface { Emit(name string, data any) }`
  - Tên sự kiện: `EventState="state"`, `EventStats="stats"`, `EventLog="log"`, `EventQuery="query"`, `EventScan="scan:progress"`, `EventAutotune="dpi:autotune"`, `EventUpdate="update"`, đăng ký trong `init()` bằng `application.RegisterEvent[T]`
  - `type StatsEvent struct { Queries uint64; LatencyMs int }`
  - `type ServerRow struct { Server model.Server; Result *scanner.Result; InUse, Pinned bool }`
  - `type AppInfo struct { Version string; Portable bool; UpdateTag, UpdateURL string }`
  - `type LogBuffer struct` (vòng tròn): `func NewLogBuffer(n int) *LogBuffer`, `Add(LogEvent)`, `All() []LogEvent`
  - `type Service struct`; `func NewService(o *Orchestrator, x ServiceDeps) *Service`, với `type ServiceDeps struct { Emitter Emitter; Paths store.Paths; Catalog func() []model.Server; LoadCustom func() ([]model.Server, error); SaveCustom func([]model.Server) error; ListAdapters func() ([]sysdns.Adapter, error); StopService func(name string) error; SetMode func(mode string); RestoreNow func() error; Info func() AppInfo }`
  - `RestoreNow` chạy `sysdns.Restore` theo snapshot trong `state.json`. Nếu không có snapshot, nó đưa các card đang trỏ về loopback về DHCP. Khôi phục thành công thì gọi `ClearWarning("RESTORE_FAILED")`.
  - Phương thức xuất ra cho frontend:
    - kết nối: `GetSnapshot() Snapshot`, `Connect() error`, `Disconnect() error`, `CancelConnect()`
    - cài đặt: `GetSettings() store.Settings`, `SaveSettings(s store.Settings) error`, `SetMode(mode string) error`
    - máy chủ: `ListServers() []ServerRow`, `ScanAll() error`, `CancelScan()`, `AddServers(text string) (int, []string)`, `RemoveCustomServer(id string) error`, `SetPinned(id string, pinned bool) error`
    - vượt DPI: `SetDPIEnabled(on bool) error`, `StartAutotune() error`, `CancelAutotune()`, `ProbeNow() []probe.Result`, `GetDPIBlacklist() (string, error)`, `SaveDPIBlacklist(text string) error`, `PreviewDPIArgs(preset, custom, scope string) ([]string, error)` (gọi thẳng `dpi.Args`, để frontend không phải tự dựng cờ)
    - cảnh báo: `DismissWarning(code string)`
    - hệ thống: `RestoreDNSNow() error`, `StopConflictingService(name string) error`, `ListAdapters() []sysdns.Adapter`, `AppInfo() AppInfo`
    - nhật ký: `SetQueryLog(on bool)`, `GetLogs() []LogEvent`

Quy tắc:
- `Sink` của `Orchestrator` phát `EventState` và `EventLog`, đồng thời lưu `LogEvent` vào `LogBuffer` (1000 dòng).
- `SetQueryLog(true)` mới bắt đầu chuyển `engine.QueryEvent` sang `EventQuery`. Các truy vấn **chỉ** nằm trong một `LogBuffer` riêng 500 dòng, không bao giờ ghi vào file log.
- Khi trạng thái là ĐÃ_BẢO_VỆ, phát `EventStats` mỗi giây.
- `SaveSettings` kiểm tra hợp lệ trước khi lưu: `MaxUpstreams` từ 1 đến 10, `Bootstrap` không rỗng và mỗi phần tử là `ip:port` hợp lệ, `DPI.CustomArgs` đi qua `dpi.ValidateCustom`.
- `StopConflictingService` chỉ được gọi khi người dùng bấm nút trên giao diện, sau khi đã thấy cảnh báo. Trên Go, đây là một hàm riêng không bao giờ được gọi tự động.

- [ ] **Step 1: Viết test**

```go
func TestLogBuffer_RingKeepsLastN(t *testing.T)
func TestService_QueryLogOffByDefaultAndRAMOnly(t *testing.T) // khi chưa SetQueryLog không phát EventQuery; khi đã bật: tối đa 500, file log không chứa tên miền
func TestService_SaveSettingsValidates(t *testing.T) // MaxUpstreams 0 → lỗi; Bootstrap ["nope"] → lỗi; CustomArgs "--dns-addr x" → lỗi
func TestService_AddServers(t *testing.T) // trả về (1, ["udp://1.1.1.1"]); file custom được lưu
func TestService_ListServersMarksInUseAndPinned(t *testing.T)
func TestService_EmitsStatsEverySecondWhileProtected(t *testing.T)
```

- [ ] **Step 2: Chạy test — phải FAIL** — `go test ./internal/app/...`
- [ ] **Step 3: Hiện thực** theo quy tắc trên.
- [ ] **Step 4: Chạy test — phải PASS**
- [ ] **Step 5: Commit** — `git commit -m "feat(app): Wails-bound service, typed events, RAM-only query log"`

### Task 20: shell — nối mọi thứ, cửa sổ, khay, wndproc

**Files:**
- Create: `internal/icon/icon.go` (+ `icon_test.go`), `internal/shell/{shell,window,tray,wndproc,webview2}.go` (+ `wndproc_test.go`)
- Modify: `main.go`

**Interfaces:**
- Consumes: mọi thứ ở trên; Wails `application.New`, `application.Options{Name, Services, Assets, SingleInstance, Windows: application.WindowsOptions{WndProcInterceptor, DisableQuitOnLastWindowClosed: true}, OnShutdown}`, `app.Window.NewWithOptions(application.WebviewWindowOptions{…})`, `win.RegisterHook(events.Common.WindowClosing, …)`, `app.SystemTray.New()`, `tray.SetIcon/SetTooltip/SetMenu/OnClick/OnRightClick/OpenMenu`, `application.NewMenu()`, `menu.Add(...).OnClick(...)`, `menu.AddCheckbox`, `menu.AddSeparator`, `app.Event.Emit`.
- Produces:
  - `func Run(mode cli.Mode, assets fs.FS) error`
  - `icon.Ring(c color.RGBA, size int) []byte` (PNG `size`×`size`, nền trong suốt, vòng tròn màu `c` dày `size/10`, chấm tròn ở giữa)
  - `func webView2Installed() bool` (đọc giá trị `pv` của khoá `SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}` trong HKLM, rồi tới cùng khoá trong HKCU; có giá trị khác rỗng và khác `0.0.0.0` thì là đã cài)
  - `type wmAction int` (`wmNone`, `wmEndSession`, `wmResume`); `func classify(msg uint32, wParam uintptr) wmAction` (`WM_QUERYENDSESSION=0x11` và `WM_ENDSESSION=0x16` có `wParam != 0` → `wmEndSession`; `WM_POWERBROADCAST=0x218` với `PBT_APMRESUMEAUTOMATIC=0x12` → `wmResume`)

Thứ tự khởi động trong `Run`:
0. Nếu `!webView2Installed()` → `MessageBoxW` song ngữ có link `https://go.microsoft.com/fwlink/p/?LinkId=2124703`, rồi thoát (trường hợp bản portable trên máy thiếu WebView2)
1. `ResolvePaths`
2. logger: `logx.NewRotating(LogDir, "ghostline", 5<<20, 3)` + `slog`
3. `LoadSettings` (nếu `recovered` thì sau khi tạo Orchestrator gọi `AddWarning(SETTINGS_RESET)`)
4. `StateStore`, rồi **khôi phục lớp 3**: `watchdog.RestoreIfOrphaned`
5. tạo `Factory`, `Engine`, `sysdns.Manager`, `dpi.Manager`, `ScanPicker`, `Prober`, `Orchestrator`, `Service`
6. `application.New` với:
   - `SingleInstance{UniqueID: brand.SingleInstanceID, OnSecondInstanceLaunch: hiện cửa sổ và focus}`
   - `OnShutdown`: `Disconnect` với timeout 10s
7. cửa sổ:
   - `Name "main"`, `Frameless: true`, 380×580
   - `DisableResize: true`, `MaximiseButtonState: application.ButtonDisabled`, `BackgroundColour: application.NewRGB(5, 7, 10)`
   - `Hidden` khi `mode.Kind == KindAutostart`
8. hook đóng cửa sổ: `closeToTray` → `Hide` + `e.Cancel()`; ngược lại → `app.Quit()`
9. khay:
   - icon theo trạng thái: tắt `#3a4a44`, đang chạy `#00d0ff`, đã bảo vệ `#00ffa3`, suy giảm `#ffb020`, lỗi `#ff4d6d`
   - `SetTooltip("Ghostline · <trạng thái>")`
   - menu: Kết nối/Ngắt, checkbox Vượt DPI, Mở Ghostline, dòng phiên bản (kèm "có bản mới ↗" nếu có), Thoát
   - `OnRightClick(tray.OpenMenu)` để tránh lỗi #6161
   - `OnClick` hiện cửa sổ
10. `SetMode`:
    - `"simple"`: 380×580, `SetResizable(false)`
    - `"advanced"`: `AdvancedWindow` (tối thiểu 900×600), `SetResizable(true)`
    - giữ nguyên tâm cửa sổ bằng `Position()`/`SetPosition()` nếu beta.27 có; nếu không có thì dùng `Center()` và ghi lại trong báo cáo
    - kích thước Nâng cao được lưu lại khi người dùng đổi kích thước
11. `WndProcInterceptor`:
    - `wmEndSession` → `Disconnect` đồng bộ (timeout 5s), trả `(1, true)` cho `WM_QUERYENDSESSION`
    - `wmResume` → `go OnResume`
12. `sysdns.Watch(o.OnNetworkChange)`
13. bộ kiểm tra cập nhật chạy nền:
    - `updater` theo mốc `Due`: kiểm tra bản mới, tải danh sách server, tải danh sách DNSCrypt
    - lưu `Meta`
    - phát `EventUpdate`
    - lỗi chỉ ghi log
14. khởi động cùng Windows: khi `settings.StartWithWindows` thay đổi → `startup.Create(AutostartTask(exe))` hoặc `Delete`
15. `mode.Kind == KindAutostart && settings.AutoConnect` → `Connect`

Resolver của hệ thống (`net.DefaultResolver`) chỉ được dùng ở hai chỗ: bước xác minh 07 và `Prober.Resolve`. Ở cả hai chỗ, mục đích là đi đúng con đường của trình duyệt, nên dùng resolver hệ thống là cố ý. Bootstrap thì **không bao giờ** dùng nó.

`Safety` thật:
- `StartWatchdog` chạy `os.Executable()` với `--watchdog --parent <pid> --parent-start <unixnano>` qua `winutil.HiddenCmd`, **không** gắn vào Job;
- hàm `stop` kill watchdog;
- `CreateRecoveryTask` và `DeleteRecoveryTask` dùng `startup.RecoveryTask`.

- [ ] **Step 1: Viết test**

```go
func TestRing(t *testing.T) // icon.Ring(c, 32): giải mã PNG, kích thước 32×32, điểm ảnh (16,1) mang màu c, góc (0,0) trong suốt
func TestClassify(t *testing.T) // (0x11,1)→wmEndSession; (0x16,0)→wmNone; (0x218,0x12)→wmResume; (0x218,0x4)→wmNone
```

- [ ] **Step 2: Chạy test — phải FAIL** — `go test ./internal/icon/... ./internal/shell/...`
- [ ] **Step 3: Hiện thực** theo thứ tự khởi động ở trên.
- [ ] **Step 4: Kiểm chứng**
  - `go test ./...` → PASS.
  - `wails3 dev` từ terminal không có quyền admin: cửa sổ 380×580 hiện ra, khay có icon, menu chuột phải mở được, bấm ✕ thì ẩn xuống khay, Thoát thì đóng hẳn. Bấm Connect phải nhận lỗi `NOT_ADMIN` (đúng như mong đợi).
  - Ghi lại các bước kiểm tra kết nối thật cần admin cho người dùng: `wails3 build` rồi chạy `bin\ghostline.exe` bằng quyền admin.
- [ ] **Step 5: Commit** — `git commit -m "feat(shell): wire app, frameless window modes, tray, end-session and resume handling"`

---

## Mốc 5 — Frontend

### Task 21: Nền tảng frontend — token, font, i18n, store, thanh tiêu đề

**Files:**
- Create: `frontend/src/brand.ts`, `frontend/src/styles/{tokens.css,global.css}`, `frontend/src/assets/fonts/JetBrainsMono-{Regular,Bold}.woff2`, `frontend/src/i18n/{index.ts,vi.json,en.json,parity.test.ts}`, `frontend/src/app/{api.ts,store.ts,bridge.ts,store.test.ts}`, `frontend/src/components/neon/TitleBar.tsx` (+ `.module.css`, `.test.tsx`), `frontend/vitest.config.ts`, `frontend/src/test/setup.ts`
- Modify: `frontend/src/{main.tsx,App.tsx}`, `frontend/package.json`

**Interfaces:**
- Consumes: binding sinh bởi `wails3 generate bindings -ts` tại `frontend/bindings/github.com/hashcott/ghostline/internal/app`; `Events.On(name, ev => ev.data)` từ `@wailsio/runtime`.
- Produces:
  - `api.ts`: re-export `Service` và các kiểu `Snapshot`, `Settings`, `ServerRow`, `LogEvent`
  - `store.ts` (Zustand): `useGhost` với `{ snapshot, settings, servers, logs, queries, latency: number[] /* 60 giá trị */, scan, autotune, update, mode }` cùng các action `setSnapshot`, `pushStats`, `pushLog`, `pushQuery`, `setSettings`, …
  - `bridge.ts`: `startBridge(): () => void`, đăng ký `Events.On` cho 7 tên sự kiện và nạp dữ liệu ban đầu qua `Service.GetSnapshot()`, `GetSettings()`, `GetLogs()`
  - `i18n/index.ts`:
    - `initI18n(lang: "vi" | "en")`
    - `tCode(code: string, params?)`: tra theo `errors.<CODE>`, `log.<CODE>` hoặc `step.<n>`, nếu thiếu khoá thì trả về chính `code`
  - `TitleBar` có props `{ mode, onMode, lang, onLang }`. Thanh tiêu đề có `--wails-draggable: drag`, các nút đặt `no-drag`.

Quy định:
- **Token** lấy từ spec §8.3: `--bg #05070a`, `--green #00ffa3`, `--cyan #00d0ff`, `--amber #ffb020`, `--red #ff4d6d`, `--text #b8ffe4`, `--dim rgba(184,255,228,.45)`, lưới nền 22px, `font-family: "JetBrains Mono", "Cascadia Code", Consolas, monospace`.
- **Font:** lấy `JetBrainsMono-Regular.woff2` và `JetBrainsMono-Bold.woff2` từ bản phát hành chính thức `JetBrains/JetBrainsMono` (license OFL-1.1), khai báo bằng `@font-face` trỏ tới file cục bộ.
- **devDependency:** `vitest`, `@testing-library/react`, `@testing-library/jest-dom`, `jsdom`. **Dependency:** `zustand`, `i18next`, `react-i18next`. Thêm script `"test": "vitest run"`.
- **Bộ khoá `vi.json`** (dịch tương ứng sang `en.json`): `status.*` (disconnected/connecting/protected/degraded/disconnecting/error), `step.1`…`step.7`, `errors.*` cho mọi mã ở Task 17 kèm `errors.*.action.*`, `log.*`, `simple.*`, `nav.*`, `servers.*`, `dpi.*`, `logs.*`, `settings.*`, `tray.*`, `common.*`.
- Câu cố định do spec quy định: `simple.errorUnchanged = "DNS của máy vẫn như cũ, không có gì bị thay đổi"` và `simple.tapToConnect = "bấm để kết nối"`.

- [ ] **Step 1: Viết test**

```ts
// parity.test.ts
import vi from "./vi.json"; import en from "./en.json";
const keys = (o: any, p = ""): string[] => Object.entries(o).flatMap(([k, v]) => typeof v === "object" ? keys(v, p + k + ".") : [p + k]);
test("vi and en have identical keys", () => { expect(keys(en).sort()).toEqual(keys(vi).sort()); });
test("every Go error code has a message", () => {
  for (const c of ["NOT_ADMIN","PORT53_BUSY","NO_SERVERS","ENGINE_SELFTEST_FAILED","SET_DNS_FAILED","VERIFY_LEAK","RESTORE_FAILED",
    "DPI_START_FAILED","DPI_BLOCKED_BY_AV","DPI_HASH_MISMATCH","SERVERLIST_BAD_SIGNATURE","UPDATE_CHECK_FAILED","AUTOTUNE_NO_PRESET",
    "INTERNAL","SETTINGS_RESET"])
    expect((vi as any).errors[c]?.message).toBeTruthy();
});
// store.test.ts
test("pushStats keeps last 60 latency points", () => { for (let i = 0; i < 70; i++) useGhost.getState().pushStats({ Queries: i, LatencyMs: i }); expect(useGhost.getState().latency).toHaveLength(60); expect(useGhost.getState().latency[0]).toBe(10); });
// TitleBar.test.tsx
test("language toggle calls onLang", ...); test("drag region attribute present", ...)
```

- [ ] **Step 2: Chạy test — phải FAIL** — `cd frontend && npm test`
- [ ] **Step 3: Hiện thực** theo các quy định trên. Trong test, mock `@wailsio/runtime` và module binding bằng `vi.mock`.
- [ ] **Step 4: Chạy test — phải PASS**. Kiểm chứng thêm `wails3 dev`: nền lưới, font mono, nút VI/EN đổi được ngôn ngữ.
- [ ] **Step 5: Commit** — `git commit -m "feat(ui): design tokens, bundled font, i18n vi/en with parity test, store and event bridge"`

### Task 22: Bộ component Neon

**Files:**
- Create: trong `frontend/src/components/neon/`: `PowerButton`, `TerminalPanel`, `Toggle`, `Banner`, `DataTable`, `Sparkline`, `Chip`, `Sidebar` (mỗi component gồm `.tsx` + `.module.css` + `.test.tsx`)

**Interfaces:**
- Produces:
  - `PowerButton { state: "off" | "busy" | "on" | "err"; size?: number; onClick; label }`
  - `TerminalPanel { rows: { k: string; v: ReactNode; tone?: "ok" | "dim" | "warn" | "err" }[] } | { lines: ReactNode[] }`
  - `Toggle { checked; onChange; label }` (`role="switch"`, có `aria-checked`)
  - `Banner { tone: "warn" | "err"; children; actions: { label; onClick; primary? }[] }`
  - `DataTable<T> { columns: { key; label; sort?: (a: T, b: T) => number; render }[]; rows: T[]; rowKey }` (bấm vào tiêu đề cột để đổi chiều sắp xếp)
  - `Sparkline { points: number[]; width; height }` (SVG polyline cộng vùng tô gradient)
  - `Chip { active?; onClick?; children }`
  - `Sidebar { items: { id; label }[]; active; onSelect; footer }`

Quy định:
- Màu và hiệu ứng của `PowerButton` khớp mockup Phần 4 (glow, vòng nét đứt xoay 18s, dạng `busy` xoay 2s dotted).
- `@media (prefers-reduced-motion: reduce)` tắt mọi animation.
- Mọi phần tử bấm được đều có `:focus-visible` với `outline: 2px solid var(--cyan)`.

- [ ] **Step 1: Viết test**

```tsx
test("PowerButton exposes state via data attribute and aria-label", () => { render(<PowerButton state="busy" label="Đang kết nối" onClick={() => {}} />); const b = screen.getByRole("button", { name: "Đang kết nối" }); expect(b).toHaveAttribute("data-state", "busy"); });
test("Toggle is a switch reflecting checked", ...);
test("DataTable sorts by clicked column and toggles direction", ...);
test("Sparkline renders polyline with N points", () => { const { container } = render(<Sparkline points={[1,2,3]} width={300} height={56} />); expect(container.querySelector("polyline")!.getAttribute("points")!.trim().split(" ")).toHaveLength(3); });
test("Banner renders actions as buttons", ...);
```

- [ ] **Step 2: Chạy test — phải FAIL** — `npm test`
- [ ] **Step 3: Hiện thực** các component theo props và quy định trên.
- [ ] **Step 4: Chạy test — phải PASS**
- [ ] **Step 5: Commit** — `git commit -m "feat(ui): neon component kit with reduced-motion and focus styles"`

### Task 23: Chế độ Đơn giản

**Files:**
- Create: `frontend/src/modes/simple/SimpleView.tsx` (+ `.module.css`, `.test.tsx`)
- Modify: `frontend/src/App.tsx`

**Interfaces:**
- Consumes: `useGhost`, `Service.Connect/Disconnect/CancelConnect/StartAutotune/SetDPIEnabled`, `PowerButton`, `TerminalPanel`, `Banner`, `tCode`.

Map trạng thái sang giao diện, theo spec §8.2 và mockup:

| `Status` | Nút nguồn | Dòng trạng thái | Bấm nút | Nội dung bên dưới |
|---|---|---|---|---|
| `disconnected` | `off` | `[ CHƯA BẢO VỆ ]` + "bấm để kết nối" | `Connect` | Bảng thông số: máy chủ "tự động", vượt DPI, thời điểm quét gần nhất |
| `connecting` | `busy` | `[ ĐANG KẾT NỐI ]` | `CancelConnect` | Log từng bước `step.1…7`: bước đã xong có ✓, bước đang chạy có ›, các bước còn lại mờ |
| `protected` | `on` | `[ ĐÃ BẢO VỆ ]` | `Disconnect` | Bảng: máy chủ (tên đầu + "+N"), độ trễ, thời gian. Nếu `BlockedSites` không rỗng thì có banner cam "N/M trang mẫu vẫn bị chặn" với nút **TỰ DÒ VƯỢT DPI** (`StartAutotune`) và **bỏ qua** (ẩn banner tới lần Connect sau) |
| `degraded` | `on` | dòng trạng thái màu cam | `Disconnect` | Giống `protected` |
| `error` | `err` | `[ LỖI ]` + `simple.errorUnchanged` | `Connect` | Banner đỏ hiện thông báo lỗi kèm nút hành động (ví dụ `NO_SERVERS`: Thử lại, Bật Fragment DNS); link "mở nhật ký ›" chuyển sang chế độ Nâng cao, trang Nhật ký |

Cảnh báo cố định (`Snapshot.Warnings`) hiện thành banner cam ở đầu màn hình, bất kể trạng thái nào:
- `RESTORE_FAILED` có nút **KHÔI PHỤC DNS NGAY** (`RestoreDNSNow`) và **không** có nút đóng;
- `SETTINGS_RESET` có nút "đã hiểu" (`DismissWarning`).

- [ ] **Step 1: Viết test** — mock `Service`, đặt store vào từng trạng thái:

```tsx
test("RESTORE_FAILED warning shows restore button and cannot be dismissed", ...);
test("disconnected: clicking power calls Connect", ...);
test("connecting: shows steps with current marker and click cancels", ...);
test("protected with blocked sites shows autotune banner; dismiss hides it", ...);
test("error NO_SERVERS shows unchanged-DNS reassurance and both actions", ...);
test("English locale renders English strings", ...);
```

- [ ] **Step 2: Chạy test — phải FAIL** — `npm test`
- [ ] **Step 3: Hiện thực** theo bảng trên.
- [ ] **Step 4: Chạy test — phải PASS**. Kiểm chứng thêm bằng mắt qua `wails3 dev`.
- [ ] **Step 5: Commit** — `git commit -m "feat(ui): simple mode with four states, DPI suggestion and error actions"`

### Task 24: Chế độ Nâng cao — khung, Tổng quan, Máy chủ

**Files:**
- Create: `frontend/src/modes/advanced/AdvancedView.tsx`, `frontend/src/modes/advanced/pages/{Overview,Servers}.tsx` (+ `.module.css`, tests), `frontend/src/modes/advanced/AddServersDialog.tsx`
- Modify: `frontend/src/App.tsx` (khi đổi chế độ thì gọi `Service.SetMode`)

**Interfaces:**
- Consumes: `Service.ListServers/ScanAll/CancelScan/AddServers/RemoveCustomServer/SetPinned/SaveSettings`, `Sidebar`, `DataTable`, `Sparkline`, `Chip`.

Quy định:
- **Khung Nâng cao:** `Sidebar` có 5 mục (tổng quan, máy chủ, vượt dpi, nhật ký, cài đặt). Phần chân của sidebar hiện trạng thái, độ trễ và số máy chủ, kèm một `PowerButton` cỡ nhỏ.
- **Tổng quan:** tóm tắt giống chế độ Đơn giản, thêm `Sparkline` độ trễ 60 giây, số truy vấn và danh sách server đang dùng.
- **Máy chủ:**
  - Cột: ghim, tên, giao thức, độ trễ, trạng thái, nhãn. Mặc định sắp theo độ trễ tăng dần.
  - Chip lọc theo giao thức (doh/dot/doq/dnscrypt), theo nhãn (no-filter/adblock/family) và "chỉ đạt".
  - Các nút: "⟳ quét toàn bộ" (hiện tiến độ, bấm lại để huỷ), "+ thêm" (mở hộp thoại dán URL hoặc stamp, nhiều dòng, nút import từ file dùng `<input type=file>`; sau khi thêm hiện số server thêm được và các dòng bị từ chối), checkbox "chỉ dùng máy chủ đã ghim".
  - Chân bảng: "quét lúc HH:MM".

- [ ] **Step 1: Viết test**

```tsx
test("servers table filters by protocol chip and only-ok", ...);
test("pin toggle calls SetPinned", ...);
test("scan button shows progress from scan:progress events and cancels on second click", ...);
test("add dialog reports added count and rejected lines", ...);
test("overview sparkline receives latency points", ...);
```

- [ ] **Step 2: Chạy test — phải FAIL** — `npm test`
- [ ] **Step 3: Hiện thực** theo quy định trên.
- [ ] **Step 4: Chạy test — phải PASS**. Kiểm chứng `wails3 dev`: chuyển sang Nâng cao thì cửa sổ nở thành 1000×660; chuyển về thì thu lại 380×580.
- [ ] **Step 5: Commit** — `git commit -m "feat(ui): advanced shell, overview and servers pages"`

### Task 25: Chế độ Nâng cao — Vượt DPI, Nhật ký, Cài đặt

**Files:**
- Create: `frontend/src/modes/advanced/pages/{Dpi,Logs,Settings}.tsx` (+ `.module.css`, tests)

**Interfaces:**
- Consumes: `Service.SetDPIEnabled/StartAutotune/CancelAutotune/ProbeNow/GetDPIBlacklist/SaveDPIBlacklist/SaveSettings/SetQueryLog/GetLogs/RestoreDNSNow/StopConflictingService/ListAdapters/AppInfo`.

Quy định:
- **Vượt DPI:**
  - Khung GoodbyeDPI: `Toggle`, chọn preset (`light`…`extreme`, `mode1`…`mode6`, `custom`), ô tham số tự nhập (khi `SaveSettings` báo lỗi thì hiện lỗi ngay dưới ô), nút "⚡ tự dò" kèm tiến độ từ `dpi:autotune`, phạm vi `all`/`blacklist` cùng nút "sửa ›" mở trình soạn danh sách đen (`GetDPIBlacklist`/`SaveDPIBlacklist`), và dòng xem trước `goodbyedpi.exe …` lấy từ `Service.PreviewDPIArgs`. Frontend **không** tự dựng cờ.
  - Khung Fragment DNS: `Toggle`, số mảnh, độ trễ, và ghi chú "thừa khi GoodbyeDPI đang bật".
  - Khung trang mẫu: danh sách sửa được, nút "⟳ thử lại" gọi `ProbeNow`, kết quả ✓/✕ kèm tầng lỗi.
- **Nhật ký:**
  - Chip lọc nguồn (tất cả, engine, dpi, hệ thống); các nút tạm dừng, copy (clipboard), lưu ra file (dùng `Blob` + tải xuống của trình duyệt).
  - Công tắc "hiện truy vấn" (mặc định tắt) gọi `SetQueryLog`. Bên cạnh hiện ghi chú "chỉ giữ trong RAM, tối đa 500 dòng".
- **Cài đặt:**
  - Các trường theo spec §8.2 và §9: ngôn ngữ, khởi động cùng Windows, tự kết nối, đóng thì thu xuống khay, card mạng (auto hoặc chọn nhiều từ `ListAdapters`), tên miền thử, bootstrap (mỗi dòng một `ip:port`), số server tối đa (1–10), cập nhật danh sách server, báo có bản mới.
  - Nút **⚠ KHÔI PHỤC DNS NGAY** (`RestoreDNSNow`).
  - Khi trạng thái lỗi có `PORT53_BUSY` mà `service` khác rỗng, hiện nút "Tạm dừng dịch vụ <name>". Bấm vào thì phải qua một hộp xác nhận tự dựng ngay trong trang (không dùng `confirm()`), rồi mới gọi `StopConflictingService`.

- [ ] **Step 1: Viết test**

```tsx
test("custom args validation error from SaveSettings is shown inline", ...);
test("autotune progress renders preset steps", ...);
test("query log toggle calls SetQueryLog and shows RAM-only note", ...);
test("settings: restore DNS now calls RestoreDNSNow", ...);
test("stopping a conflicting service requires in-page confirmation", ...); // hộp xác nhận hiện ra, chỉ gọi service sau khi bấm nút xác nhận
```

- [ ] **Step 2: Chạy test — phải FAIL** — `npm test`
- [ ] **Step 3: Hiện thực** theo quy định trên.
- [ ] **Step 4: Chạy test — phải PASS**
- [ ] **Step 5: Commit** — `git commit -m "feat(ui): DPI, logs and settings pages"`

---

## Mốc 6 — Đóng gói và CI

### Task 26: Installer, portable, NOTICE, README, danh sách kiểm tra phát hành

**Files:**
- Modify: `build/windows/nsis/project.nsi`, `build/windows/Taskfile.yml`, `build/appicon.png`
- Create: `tools/genicons/main.go`, `NOTICE`, `README.md`, `docs/release-checklist.md`

Quy định:
- **Icon:**
  - `tools/genicons` gọi `icon.Ring(#00ffa3, 512)`, ghi ra `build/appicon.png`, rồi chạy `wails3 generate icons` để sinh `icon.ico`.
  - **Không** chạy `wails3 update build-assets`.
- **NSIS, khi cài:** chạy `nsExec::Exec 'taskkill /IM ghostline.exe /F'` trước khi chép file.
- **NSIS, khi gỡ** (trong `Section "uninstall"`, đặt trước `RMDir /r $INSTDIR`), theo đúng thứ tự:
  1. `nsExec::Exec 'taskkill /IM ghostline.exe /F'`
  2. `ExecWait '"$INSTDIR\ghostline.exe" --restore'`
  3. `nsExec::Exec 'schtasks /Delete /TN "Ghostline" /F'`
  4. `nsExec::Exec 'schtasks /Delete /TN "Ghostline Recovery" /F'`
  5. `nsExec::Exec 'sc stop WinDivert'`
  6. `nsExec::Exec 'sc delete WinDivert'`
- **Portable:** task `windows:portable` trong Taskfile:
  - build production;
  - tạo `bin/portable/ghostline.exe` cùng file rỗng `bin/portable/portable`;
  - nén thành `bin/Ghostline-{{.VERSION}}-portable.zip` bằng `powershell Compress-Archive`.
- **`NOTICE`** liệt kê: dnsproxy (Apache-2.0), GoodbyeDPI (Apache-2.0, kèm file license), WinDivert (LGPLv3, kèm file license), JetBrains Mono (OFL-1.1), Wails (MIT), cùng ghi chú "Danh sách DNSCrypt chỉ được tải lúc chạy, không phân phối kèm app".
- **`README.md`** (tiếng Việt, có phần tiếng Anh ngắn), gồm:
  - tính năng;
  - tải và kiểm tra SHA-256;
  - cảnh báo SmartScreen;
  - antivirus và WinDivert;
  - vì sao app cần quyền admin;
  - lưu ý khi nâng quyền UAC bằng tài khoản admin khác (`%APPDATA%` là của tài khoản đó);
  - cách build;
  - không có telemetry;
  - license.
- **`docs/release-checklist.md`** chép nguyên văn dòng "Kiểm tra thủ công trước phát hành" trong spec §11, mỗi mục là một checkbox kèm cách làm:
  - Windows 11 vừa cài mới: Connect lần đầu ≤ 25 giây, lần sau ≤ 5 giây (tiêu chí thành công 1);
  - `taskkill /F` → đo thời gian khôi phục ≤ 3 giây;
  - khởi động lại máy khi đang kết nối;
  - cổng 53 bị ICS chiếm;
  - đổi Wi-Fi sang dây mạng;
  - máy ngủ rồi thức;
  - Wireshark với filter `udp.port==53 || tcp.port==53`;
  - cài rồi gỡ.

- [ ] **Step 1: Cài NSIS nếu thiếu.** `makensis` chưa có trên máy này. Hỏi người dùng trước khi chạy `winget install NSIS.NSIS` (đây là thao tác cài phần mềm lên hệ thống).
- [ ] **Step 2: Hiện thực** các quy định trên.
- [ ] **Step 3: Kiểm chứng**
  - `wails3 package` → có `bin/ghostline-amd64-installer.exe`;
  - `wails3 task windows:portable` → có file zip; giải nén ra thì thấy `ghostline.exe` và `portable`;
  - `go test ./... && (cd frontend && npm test)` → PASS.
- [ ] **Step 4: Commit** — `git commit -m "build: NSIS uninstall cleanup, portable zip, icons, NOTICE, README, release checklist"`

### Task 27: GitHub Actions — ci, release, servers

**Files:**
- Create: `.github/workflows/ci.yml`, `.github/workflows/release.yml`, `.github/workflows/servers.yml`

Quy định:
- **Cấu hình chung cho mọi job:** `runs-on: windows-latest`; `actions/setup-go` với `go-version: '1.27'`; `actions/setup-node` với `node-version: '24'`; cài `wails3` bằng `go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.27`; `npm ci` trong `frontend`.
- **`ci.yml`** (chạy khi push hoặc PR):
  - `golangci-lint`, dùng `golangci/golangci-lint-action` bản mới nhất có hỗ trợ Go 1.27;
  - `go test ./...` (không chạy tag `integration`);
  - `npm test`;
  - `wails3 build`.
- **`release.yml`** (chạy khi đẩy tag `v*`):
  - test;
  - `wails3 package` với `VERSION=${GITHUB_REF_NAME#v}`;
  - `wails3 task windows:portable`;
  - tạo `SHA256SUMS` cho installer và zip;
  - `gh release create "$GITHUB_REF_NAME" … --generate-notes`.
  - Bản phát hành dùng `lists/servers.json` đang có trong repo, không sinh lại ở đây.
- **`servers.yml`** (chạy bằng `workflow_dispatch` và lịch hằng tuần `cron: '0 3 * * 1'`):
  - `go run ./tools/genservers -seed lists/seed.json -out lists/servers.json -sign-env SERVERLIST_SIGNING_KEY`;
  - commit `lists/servers.json` và `lists/servers.json.sig` lên `main` dưới tên `github-actions[bot]`, chỉ commit khi có thay đổi;
  - quyền `contents: write`.

- [ ] **Step 1: Viết 3 workflow** theo quy định.
- [ ] **Step 2: Kiểm chứng cú pháp** — chạy `actionlint` nếu máy có. Nếu không có thì kiểm tra bằng `gh workflow view` sau khi push. Ghi trong báo cáo: workflow chỉ chạy thật khi repo đã được đẩy lên GitHub, và người dùng phải tạo secret `SERVERLIST_SIGNING_KEY` (khoá sinh ở Task 4).
- [ ] **Step 3: Commit** — `git commit -m "ci: test, release and signed server-list workflows"`

---

## Kiểm tra cuối (sau Task 27)

- [ ] `go test ./...` và `go test -race ./internal/app/...` → PASS
- [ ] `cd frontend && npm test` → PASS
- [ ] `wails3 build` và `wails3 package` → có exe và installer
- [ ] Danh sách test `integration` (sysdns, startup) cùng các mục trong `docs/release-checklist.md`, giao cho người dùng chạy bằng terminal admin
