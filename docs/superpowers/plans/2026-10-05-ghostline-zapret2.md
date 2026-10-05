# Engine vượt DPI zapret2 — Kế hoạch triển khai

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Thêm zapret2 (`winws2`) làm engine vượt DPI thứ hai cạnh GoodbyeDPI (nâng lên 0.2.3rc3), có danh sách chiến lược ký ed25519 và đường lui về GoodbyeDPI.

**Architecture:** `internal/dpi` trở thành bộ chạy engine chung (`Manager` + interface `Engine`); mỗi engine là một package con (`dpi/goodbyedpi`, `dpi/zapret2`) chỉ lo dựng argv và kiểm tra tham số. `dpi/zapret2/strategies` đọc và kiểm tra danh sách chiến lược. `app` chọn engine theo cài đặt, quay về GoodbyeDPI khi zapret2 bị chặn.

**Tech Stack:** Go 1.26, Wails v3, React + vitest, testify, `crypto/ed25519`.

**Spec:** `docs/superpowers/specs/2026-10-05-ghostline-zapret2-design.md`

## Global Constraints

- zapret2 **v1.0.5.2**, chỉ `binaries/windows-x86_64/` + `lua/zapret-lib.lua`, `lua/zapret-antidpi.lua`, `lua/zapret-auto.lua`; hash phải khớp `sha256sum.txt` của bản phát hành.
- GoodbyeDPI **0.2.3rc3** (`goodbyedpi-0.2.3rc3-2.zip`, `x86_64/`): `goodbyedpi.exe 8d412b094bb9c137ff25ba9a794d1122ecc84bb776debff6c249723a13cc31cd`, `WinDivert.dll 6110bfa44667405179c3e15e12af1b62037e447ed59b054b19042032995e6c7e`, `WinDivert64.sys e69b5ba3f0cd6cfb2983e442636e7f0b342b61b15264b0328317d4559c82cf50`.
- Hai engine không bao giờ chạy cùng lúc; `Manager` gỡ mọi dịch vụ tên bắt đầu bằng `WinDivert` trước mỗi lần khởi động.
- Engine ID đúng chuỗi `goodbyedpi` và `zapret2`.
- Danh sách chiến lược ký bằng **cùng khoá ed25519** với `servers.json` (`brand.ServerListPublicKeyHex`, `servers.Sign`/`servers.VerifySigned`).
- Giới hạn danh sách: ≤ 32 chiến lược, ≤ 16 tham số mỗi phần `tcp`/`quic`, `id` khớp `^[a-z0-9-]{1,32}$`.
- Người cũ (file `settings.json` có `version` < 3) giữ `engine: "goodbyedpi"`; cài mới `engine: "zapret2"`.
- Mọi chuỗi giao diện mới có cả `vi.json` và `en.json`.
- Commit message không có dòng `Co-Authored-By`.

## Review Focus

1. **Đường dẫn có dấu tiếng Việt hoặc khoảng trắng** (`C:\Users\Đức Hạnh\…`) cho thư mục dữ liệu: winws2 (Cygwin) phải chạy được → mọi file danh sách được copy vào thư mục engine và truyền bằng tên tương đối (Task 2, test `TestManager_ListsCopiedRelative`).
2. **Dịch vụ `WinDivert` do engine kia hoặc bản cũ (`WinDivert1.4`) để lại** khi khởi động: engine mới phải nạp đúng driver của nó → Task 2, test `TestManager_StartRemovesStaleDriverServices`.
3. **`settings.json` cũ không có trường `engine`**: không được rơi vào mặc định `zapret2` của cài mới → Task 7, test `TestLoadSettings_V2KeepsGoodbyeDPI`.
4. **Chiến lược đã lưu biến mất khỏi danh sách mới tải về**: Connect vẫn chạy với chiến lược đầu tiên, không lỗi → Task 8, test `TestStartDPI_UnknownStrategyFallsBackToFirst`.
5. **Danh sách tải về hợp lệ chữ ký nhưng chứa `luaexec` hoặc giá trị có `@`/`:`**: bị bỏ cả file, giữ bản nhúng → Task 5, test `TestSelect_RemoteWithForbiddenArgRejected`.

---

## File Structure

| File | Trách nhiệm |
|---|---|
| `internal/dpi/engine.go` (mới) | `Engine`, `Plan`, `Strategy`, `Scope`, lỗi chung, `BlacklistEntries`, tokenizer dùng chung |
| `internal/dpi/manager.go` (sửa) | Chạy tối đa một engine, giải nén/hash theo `Engine.Files()`, copy danh sách, gỡ driver |
| `internal/dpi/preset.go` (xoá) | Chuyển sang `dpi/goodbyedpi` |
| `internal/dpi/goodbyedpi/engine.go` (mới) | Preset, `ValidateCustom`, `--fake-with-sni`, `--fake-gen` |
| `internal/dpi/zapret2/validate.go` (mới) | Allow-list `--lua-desync`, `ValidateArgs`, `ValidateCustom` |
| `internal/dpi/zapret2/strategies/strategies.go` (mới) | `List`, `Parse`, `Select` |
| `internal/dpi/zapret2/engine.go` (mới) | Dựng argv winws2 |
| `assets/goodbyedpi/*` (sửa) | File 0.2.3rc3 |
| `assets/zapret2/*` (mới) | File zapret2 + giấy phép + `embed.go` |
| `assets/strategies/strategies.json` (mới) | Bản dự phòng nhúng; `lists/strategies.json` + `.sig` là bản tải về |
| `tools/fetchdpi/main.go` (mới) | Tải và đối chiếu hash file nhị phân vào `assets/` |
| `internal/store/settings.go`, `state.go`, `paths.go` (sửa) | v3, `engine`, đường dẫn mới |
| `internal/app/dpi.go` (mới, tách từ `autotune.go`) | `startDPI` + đường lui, `RestartDPI`, `SetDPIEnabled` |
| `internal/app/autotune.go` (sửa) | Autotune theo engine, đổi engine khi bị AV chặn |
| `internal/app/service.go`, `deps.go`, `errors.go`, `status.go` (sửa) | Binding, interface `DPI`, mã lỗi |
| `internal/shell/shell.go`, `updates.go`, `stopdpi.go`, `main.go` (sửa) | Lắp engine, tải danh sách chiến lược |
| `frontend/src/modes/advanced/pages/Dpi.tsx` (sửa), `frontend/src/i18n/*.json` | Giao diện |

---

### Task 1: Interface `Engine` và engine GoodbyeDPI

**Files:**
- Create: `internal/dpi/engine.go`, `internal/dpi/goodbyedpi/engine.go`, `internal/dpi/goodbyedpi/engine_test.go`
- Delete: `internal/dpi/preset.go` (nội dung chuyển đi)
- Modify: `internal/dpi/dpi_test.go` (chuyển các test preset/custom sang `goodbyedpi/engine_test.go`)

**Interfaces:**
- Produces (package `dpi`):
  ```go
  type Scope string            // ScopeAll = "all", ScopeBlacklist = "blacklist"
  type Plan struct {
      Strategy     string // preset/strategy id, or "custom"
      Custom       string
      Scope        Scope
      Blacklist    string // Manager passes a name relative to the engine dir
      AutoHostlist string // "" = off; relative name when set (zapret2 only)
  }
  type Strategy struct { ID string `json:"id"`; Name map[string]string `json:"name"` }
  type Engine interface {
      ID() string
      Files() map[string]string // slash path relative to engine dir → SHA-256
      Exe() string              // slash path relative to engine dir
      Args(p Plan) ([]string, error)
      Strategies() []Strategy   // autotune order, lightest first
      ValidateCustom(s string) error
      HotReloadsLists() bool
  }
  var ErrForbiddenFlag = errors.New("dpi: flag not allowed")
  func Tokenize(s string) ([]string, error)   // moved from preset.go, unchanged behaviour
  func BlacklistEntries(text string) int      // moved, unchanged
  ```
- Produces (package `goodbyedpi`): `func New() dpi.Engine` with `ID() == "goodbyedpi"`, `Exe() == "goodbyedpi.exe"`, `Files()` = the three 0.2.3rc3 hashes from Global Constraints, `HotReloadsLists() == false`, `Strategies()` = `light, medium, high, extreme` (names `{"vi":"Nhẹ","en":"Light"}`, `Vừa/Medium`, `Mạnh/Strong`, `Rất mạnh/Extreme`).

- [ ] **Step 1: Move the existing preset tests** from `internal/dpi/dpi_test.go` (`TestArgs_*`, `TestValidateCustom*`, tokenizer tests) into `goodbyedpi/engine_test.go`, rewritten against `New().Args(dpi.Plan{...})`. Expected argv values stay byte-for-byte identical. Blacklist scope now emits `--blacklist`, `<p.Blacklist>` verbatim.

- [ ] **Step 2: Add new tests**

```go
func TestValidateCustom_FakeWithSNI(t *testing.T) {
    e := New()
    require.NoError(t, e.ValidateCustom(`--fake-with-sni www.w3.org --fake-gen 5`))
    require.Error(t, e.ValidateCustom(`--fake-with-sni`))
    require.Error(t, e.ValidateCustom(`--fake-with-sni "a b"`))
    require.Error(t, e.ValidateCustom(`--fake-with-sni -p`))
    require.Error(t, e.ValidateCustom(`--fake-gen 31`))
    require.Error(t, e.ValidateCustom(`--fake-gen 0`))
}
func TestStrategies_AutotuneOrder(t *testing.T) {
    ids := []string{}
    for _, s := range New().Strategies() { ids = append(ids, s.ID) }
    require.Equal(t, []string{"light", "medium", "high", "extreme"}, ids)
}
```

- [ ] **Step 3: Run** `go test ./internal/dpi/...` — Expected: FAIL (package `goodbyedpi` missing).

- [ ] **Step 4: Implement** `engine.go` and `goodbyedpi/engine.go`. Domain for `--fake-with-sni` must match `^([a-z0-9-]{1,63}\.)+[a-z]{2,63}$` (lower-cased). `--fake-gen` takes 1–30. `Args` for `Strategy == "custom"` uses `ValidateCustom` result. Delete `preset.go`.

- [ ] **Step 5: Run** `go test ./internal/dpi/...` — Expected: PASS. (`internal/app` will not compile until Task 8; run only `./internal/dpi/...` here.)

- [ ] **Step 6: Commit** `refactor(dpi): Engine interface; GoodbyeDPI presets move to dpi/goodbyedpi`

---

### Task 2: `Manager` chạy nhiều engine

**Files:**
- Modify: `internal/dpi/manager.go`, `internal/dpi/dpi_test.go`

**Interfaces:**
- Consumes: `dpi.Engine`, `dpi.Plan` (Task 1).
- Produces:
  ```go
  type Installed struct { Engine Engine; Assets fs.FS }
  func NewManager(binDir string, engines []Installed, r Runner, s Services, sleep func(time.Duration)) *Manager
  func (m *Manager) Start(ctx context.Context, engine string, p Plan) (int, error) // p.Blacklist / p.AutoHostlist are ABSOLUTE paths here
  func (m *Manager) Stop() error
  func (m *Manager) Running() bool
  func (m *Manager) Engine() string                // "" when not running
  func (m *Manager) RefreshLists(p Plan) error     // copies the blacklist into the running engine's dir
  func (m *Manager) Get(engine string) (Engine, bool)
  var ErrUnknownEngine = errors.New("dpi: unknown engine")
  ```
  `ErrHashMismatch`, `ErrStartFailed`, `ErrBlockedByAV` keep their names; messages say "DPI engine" instead of "GoodbyeDPI".

Behaviour that tests pin:
- Engine files live in `<binDir>/<engine ID>/`; `Files()` keys may contain `/` (e.g. `lua/zapret-lib.lua`) → create subdirs.
- `Start`: stop a running engine first; remove every service from `Services.Find("WinDivert")` (Stop + Delete) **before** launching; copy `p.Blacklist` → `<dir>/blacklist.txt` and `p.AutoHostlist` (if the source exists) → `<dir>/autohostlist.txt`, then call `Engine.Args` with those relative names. AutoHostlist set but the source missing → create an empty `autohostlist.txt`.
- `Stop`: if the running plan had `AutoHostlist`, copy `<dir>/autohostlist.txt` back to that absolute path before killing (ignore a missing file).
- `driverRunning()`: `Services.Running("WinDivert")` only (exact name).

- [ ] **Step 1: Write failing tests** in `dpi_test.go` with the existing fake `Runner`/`Services` style, two fake engines (`fakeEngine{id, files}`):
  - `TestManager_StartRemovesStaleDriverServices`: fake services pre-seeded with `WinDivert1.4` and `WinDivert`; after `Start("zapret2", …)` both were Stopped+Deleted before `Runner.Start` was called (record call order).
  - `TestManager_ListsCopiedRelative`: blacklist at `t.TempDir()+"/Đức Hạnh/dpi-blacklist.txt"`; the args given to the engine contain `blacklist.txt` (not the absolute path); file content copied.
  - `TestManager_AutoHostlistRoundTrip`: start with autohostlist source containing `a.com`; the fake process "writes" `b.com` into `<dir>/autohostlist.txt`; `Stop()` → source contains `b.com`.
  - `TestManager_SwitchEngine`: start `goodbyedpi`, then `zapret2` → first process killed, `Engine() == "zapret2"`.
  - `TestManager_UnknownEngine`: `Start("x", …)` → `ErrUnknownEngine`.
  - `TestManager_NestedFilesExtracted`: engine file `lua/a.lua` extracted to `<dir>/lua/a.lua` and verified.
  - Existing hash-mismatch / blocked-by-AV / driver-not-running tests are ported to the new constructor.

- [ ] **Step 2: Run** `go test ./internal/dpi/` — Expected: FAIL.

- [ ] **Step 3: Implement** in `manager.go`. Remove `Pinned`, `localBlacklist` (replaced by the copy above). Keep `isAppControlBlock`.

- [ ] **Step 4: Run** `go test ./internal/dpi/...` — Expected: PASS.

- [ ] **Step 5: Commit** `feat(dpi): Manager runs one of several engines`

---

### Task 3: File nhị phân — GoodbyeDPI 0.2.3rc3 và zapret2 v1.0.5.2

**Files:**
- Create: `tools/fetchdpi/main.go`, `tools/fetchdpi/main_test.go`, `assets/zapret2/embed.go`, `assets/zapret2/embed_test.go`, `assets/zapret2/LICENSE-zapret2.txt`, `LICENSE-cygwin.txt`, `LICENSE-windivert.txt`
- Modify: `assets/goodbyedpi/*` (binaries, `embed.go` comment), `assets/goodbyedpi/embed_test.go`, `Taskfile.yml`, `NOTICE`

**Interfaces:**
- Consumes: `goodbyedpi.New().Files()` (Task 1).
- Produces: `assets/goodbyedpi.FS` (unchanged name), `assets/zapret2.FS embed.FS` containing `winws2.exe cygwin1.dll WinDivert.dll WinDivert64.sys lua/zapret-lib.lua lua/zapret-antidpi.lua lua/zapret-auto.lua`.

> **Trước khi bắt đầu:** Windows Defender trên máy phát triển xoá zip zapret2 (`Trojan:Win32/Suschil!rfn`). Người thực hiện phải nhờ chủ repo thêm loại trừ Defender cho `<repo>\assets\zapret2`, `<repo>\bin` và thư mục tạm của `fetchdpi` trước khi chạy bước 3. Không tự tắt Defender.

- [ ] **Step 1: Write failing test** `tools/fetchdpi/main_test.go`: `TestParseSHA256Sums` parses a `sha256sum.txt` body (`<hex> *<path>` and `<hex>  <path>` forms) into `map[string]string`; `TestPick_RejectsHashMismatch` — `pick(files map[string][]byte, want map[string]string) error` fails when one file differs.

- [ ] **Step 2: Implement** `tools/fetchdpi`: flags `-what zapret2|goodbyedpi -version <v> -out <dir>`. zapret2: download `https://github.com/bol-van/zapret2/releases/download/<v>/zapret2-<v>.tar.gz` and `sha256sum.txt`, verify the archive, extract the 7 files (stripping `zapret2-<v>/binaries/windows-x86_64/` and `zapret2-<v>/`), write to `-out`. goodbyedpi: download `goodbyedpi-0.2.3rc3-2.zip`, extract `x86_64/*`, verify against the pins hard-coded in the tool (same values as Global Constraints). Use `.tar.gz` for zapret2 (streamed, no zip on disk).

- [ ] **Step 3: Run** `go test ./tools/fetchdpi/` — PASS. Then `go run ./tools/fetchdpi -what goodbyedpi -version 0.2.3rc3 -out assets/goodbyedpi` and `go run ./tools/fetchdpi -what zapret2 -version v1.0.5.2 -out assets/zapret2`. Copy the zapret2 `docs/LICENSE.txt` → `LICENSE-zapret2.txt`; Cygwin LGPLv3 text → `LICENSE-cygwin.txt`; WinDivert LGPLv3 → `LICENSE-windivert.txt`.

- [ ] **Step 4: Add Taskfile tasks** `assets:goodbyedpi` and `assets:zapret2` that run the two commands above.

- [ ] **Step 5: Write `assets/zapret2/embed_test.go`** mirroring `assets/goodbyedpi/embed_test.go`: every key of the zapret2 engine's `Files()` exists in `FS` and hashes match. The zapret2 `Files()` map itself is created in Task 6 — for now the test compares against a `pins` map in the test file, and Task 6 moves the map into `zapret2.Pinned` and makes this test use it. Update `assets/goodbyedpi/embed_test.go` to compare with `goodbyedpi.New().Files()`.

- [ ] **Step 6: Update `NOTICE`**: GoodbyeDPI line → 0.2.3rc3; add zapret2 (MIT, bundled unmodified: `winws2.exe` and the three Lua files), Cygwin `cygwin1.dll` (LGPLv3, unmodified), WinDivert 2.2 shipped with zapret2 and GoodbyeDPI (LGPLv3).

- [ ] **Step 7: Run** `go test ./assets/... ./tools/...` — PASS.

- [ ] **Step 8: Commit** `build(dpi): GoodbyeDPI 0.2.3rc3 and zapret2 v1.0.5.2 binaries`

---

### Task 4: Bộ kiểm tra tham số zapret2 (allow-list)

**Files:**
- Create: `internal/dpi/zapret2/validate.go`, `internal/dpi/zapret2/validate_test.go`

**Interfaces:**
- Consumes: `dpi.Tokenize`, `dpi.ErrForbiddenFlag` (Task 1).
- Produces:
  ```go
  func ValidateArgs(args []string) error            // each element one argument
  func ValidateCustom(s string) ([]string, error)   // Tokenize then ValidateArgs
  ```

Rules (spec §6.3, verbatim):
- Allowed argument forms only: `--lua-desync=<fn>[:k[=v]]...`, `--payload=<type>[,<type>]`, `--out-range=<range>`, `--in-range=<range>`.
- `<fn>` ∈ {`fake`, `multisplit`, `multidisorder`, `fakedsplit`, `fakeddisorder`, `hostfakesplit`, `tcpseg`, `oob`, `wsize`, `wssize`, `syndata`, `synack`, `synack_split`, `tls_client_hello_clone`, `http_domcase`, `http_hostcase`, `http_methodeol`, `http_unixeol`, `udplen`, `drop`, `pass`}.
- key `^[a-z0-9_]{1,32}$`; value `^[A-Za-z0-9_.,+\-]{0,64}$`; except `blob` and `seqovl_pattern`: one of `fake_default_tls`, `fake_default_http`, `fake_default_quic`, or `^0x[0-9a-fA-F]{2,2048}$`.
- `--payload` types `^[a-z0-9_]{1,32}$` comma-separated; `--out-range`/`--in-range` value `^[nadsp]?[0-9]{0,10}(-|<)[nadsp]?[0-9]{0,10}$`.
- Anything else → `fmt.Errorf("%w: %q", dpi.ErrForbiddenFlag, arg)`.

- [ ] **Step 1: Write failing table test** `TestValidateArgs` with cases (want ok / want error):
  - ok: `--lua-desync=multisplit:pos=1,midsld`, `--lua-desync=fake:blob=fake_default_tls:badsum`, `--lua-desync=fake:blob=fake_default_tls:ip_autottl=-2,3-20`, `--lua-desync=multidisorder:pos=1,sniext+1:seqovl=midsld-1`, `--lua-desync=fake:blob=0x1603`, `--payload=tls_client_hello`, `--out-range=-n3`.
  - error: `--lua-desync=luaexec:code=os.execute`, `--lua-desync=pktdebug`, `--lua-desync=fake:blob=@C:/x.bin`, `--lua-desync=fake:blob=evil_blob`, `--lua-desync=multisplit:pos=1/2`, `--lua-desync=multisplit:pos=a:b=c=d`, `--lua-init=@x.lua`, `--blob=x:@f`, `--writable`, `--debug=@log`, `--wf-raw=@f`, `@config.txt`, `--new`, `--filter-tcp=1`, `--hostlist=x`, `--ipset=x`, `--pidfile=x`, `--chdir`, `--daemon`, `--intercept=0`, `--lua-desync=fake:blob=0x` + 2050 hex chars, `--lua-desync=` + `fake:` + `k=` + 65-char value.
- [ ] **Step 2: Write** `FuzzValidateCustom`: for any input, no panic; if it returns nil error, every returned arg has prefix `--lua-desync=`, `--payload=`, `--out-range=` or `--in-range=` and contains no `@`, `/`, `\`, space.
- [ ] **Step 3: Run** `go test ./internal/dpi/zapret2/` — FAIL.
- [ ] **Step 4: Implement** `validate.go`. Split `--lua-desync=` value on `:`; first part is `<fn>`; each further part splits on the **first** `=` into key/value (a part without `=` is a flag with empty value). A value containing `=` fails the value regex.
- [ ] **Step 5: Run** `go test ./internal/dpi/zapret2/ && go test -fuzz=FuzzValidateCustom -fuzztime=30s ./internal/dpi/zapret2/` — PASS.
- [ ] **Step 6: Commit** `feat(dpi/zapret2): allow-list validator for strategy args`

---

### Task 5: Danh sách chiến lược

**Files:**
- Create: `internal/dpi/zapret2/strategies/strategies.go`, `strategies_test.go`, `assets/strategies/strategies.json`, `assets/strategies/embed.go`, `lists/strategies.json`

**Interfaces:**
- Consumes: `zapret2.ValidateArgs` (passed in as a func to avoid an import cycle).
- Produces:
  ```go
  type Strategy struct {
      ID   string            `json:"id"`
      Name map[string]string `json:"name"`
      TCP  []string          `json:"tcp"`
      QUIC []string          `json:"quic"`
  }
  type List struct {
      Version int        `json:"version"`
      Zapret2 []Strategy `json:"zapret2"`
  }
  type Validator func(args []string) error
  var ErrInvalid = errors.New("strategies: invalid list")
  func Parse(raw []byte, v Validator) (List, error)
  // Select returns the remote list when its signature verifies, its Version is
  // greater than the builtin's and Parse accepts it; otherwise the builtin and
  // a non-nil reason (nil when there is no remote file).
  func Select(builtin, remote, sig []byte, pub ed25519.PublicKey, v Validator) (List, error)
  ```
  `assets/strategies.FS`-style: `var BuiltinJSON []byte` (`//go:embed strategies.json`).

- [ ] **Step 1: Write failing tests**:
  - `TestParse_Limits`: 33 strategies → `ErrInvalid`; 17 tcp args → `ErrInvalid`; id `Bad_ID` → `ErrInvalid`; duplicate id → `ErrInvalid`; missing `vi` or `en` name → `ErrInvalid`; empty `tcp` → `ErrInvalid`; `version < 1` → `ErrInvalid`.
  - `TestParse_ValidatorAppliedToEveryArg`: a validator rejecting `bad` makes a list with `"quic":["bad"]` fail.
  - `TestSelect_*`: no remote → builtin, nil; bad signature → builtin, error; remote `version` equal to builtin → builtin, error; newer and valid → remote, nil; malformed JSON → builtin, error.
  - `TestSelect_RemoteWithForbiddenArgRejected` (Review Focus #5): remote signed correctly, `version` 99, one strategy `tcp: ["--lua-desync=luaexec:x=1"]` with the real `zapret2.ValidateArgs` → builtin returned, error wraps `ErrInvalid`.
  - `TestBuiltinIsValid`: `Parse(assets/strategies.BuiltinJSON, zapret2.ValidateArgs)` succeeds and the ids are `z-split, z-disorder, z-fake, z-fake-ttl` in that order.
- [ ] **Step 2: Run** — FAIL.
- [ ] **Step 3: Implement**; write `assets/strategies/strategies.json` with exactly the four strategies of spec §6.1 (`version: 1`). Copy it to `lists/strategies.json` and sign: extend `tools/genservers` is **not** needed — add flag `-sign-file <path>` to `tools/genservers/main.go` that signs an arbitrary file with `-sign-env` and writes `<path>.sig`. Commit `lists/strategies.json` unsigned if `SERVERLIST_SIGNING_KEY` is unavailable and note it in the commit body; the release checklist (Task 11) carries the signing step.
- [ ] **Step 4: Run** `go test ./internal/dpi/... ./assets/... ./tools/...` — PASS.
- [ ] **Step 5: Commit** `feat(dpi/zapret2): signed strategy list with built-in fallback`

---

### Task 6: Engine zapret2

**Files:**
- Create: `internal/dpi/zapret2/engine.go`, `engine_test.go`
- Modify: `assets/zapret2/embed_test.go` (use `zapret2.Pinned`)

**Interfaces:**
- Consumes: `dpi.Engine`, `dpi.Plan`, `strategies.List`, `ValidateArgs`/`ValidateCustom`.
- Produces:
  ```go
  var Pinned map[string]string // the 7 files of Task 3 → SHA-256
  // New returns the engine; list is read on every Args/Strategies call so a
  // newly downloaded list applies on the next start.
  func New(list func() strategies.List) dpi.Engine
  ```
  `ID() == "zapret2"`, `Exe() == "winws2.exe"`, `HotReloadsLists() == true`.

Argv (spec §5.2, exact order, `--name=value` form):
```
--wf-tcp-out=80,443
[--wf-udp-out=443]                     only if the strategy has QUIC args
--wf-dup-check=1
--lua-init=@lua/zapret-lib.lua
--lua-init=@lua/zapret-antidpi.lua
--lua-init=@lua/zapret-auto.lua
--filter-tcp=80,443  <scope> <tcp args>
[--new --filter-udp=443 --filter-l7=quic <scope> <quic args>]
```
`<scope>` = `--hostlist=<p.Blacklist>` when `Scope == blacklist`, then `--hostlist-auto=<p.AutoHostlist>` when `AutoHostlist != ""` and `Scope == blacklist`. `Strategy == "custom"` → TCP args from `ValidateCustom(p.Custom)`, no QUIC. Unknown strategy id → `fmt.Errorf("zapret2: unknown strategy %q", id)`.

- [ ] **Step 1: Write failing tests** `TestArgs_*`: all-scope with `z-split` (no QUIC → no `--wf-udp-out`, no `--new`); blacklist scope with `z-fake` (QUIC profile present, `--hostlist=blacklist.txt` appears twice, once per profile); blacklist + autohostlist (`--hostlist-auto=autohostlist.txt` in both profiles); all-scope + autohostlist → no `--hostlist-auto`; custom args; custom with `--lua-init` → `ErrForbiddenFlag`; unknown strategy → error. `TestStrategies_FollowList`: swapping the list func changes `Strategies()`.
- [ ] **Step 2: Run** — FAIL. **Step 3: Implement.** Fill `Pinned` with the hashes `fetchdpi` verified in Task 3 (read them from `sha256sum.txt` / the test map). **Step 4: Run** `go test ./internal/dpi/... ./assets/...` — PASS.
- [ ] **Step 5: Commit** `feat(dpi/zapret2): winws2 engine`

---

### Task 7: `settings.json` v3, `state.json`, đường dẫn

**Files:**
- Modify: `internal/store/settings.go`, `state.go`, `paths.go`, `store_test.go` (or new `v3_test.go`)

**Interfaces:**
- Produces:
  ```go
  type DPISettings struct {
      Enabled        bool             `json:"enabled"`
      Engine         string           `json:"engine"` // "goodbyedpi" | "zapret2"
      Preset         string           `json:"preset"`     // GoodbyeDPI
      CustomArgs     string           `json:"customArgs"` // GoodbyeDPI
      Scope          string           `json:"scope"`
      Zapret2        Zapret2Settings  `json:"zapret2"`
      HideEngineHint bool             `json:"hideEngineHint"`
  }
  type Zapret2Settings struct {
      Strategy     string `json:"strategy"`
      CustomArgs   string `json:"customArgs"`
      AutoHostlist bool   `json:"autoHostlist"`
  }
  type DPIState struct { Running bool `json:"running"`; PID int `json:"pid"`; Engine string `json:"engine,omitempty"` }
  // Paths gains: DPIStrategies ("dpi-strategies.json"), DPIStrategiesSig ("dpi-strategies.json.sig"), DPIAutoHostlist ("dpi-autohostlist.txt")
  ```
  `DefaultSettings()`: `Version: 3`, `DPI.Engine: "zapret2"`, `DPI.Zapret2.Strategy: "z-split"`.

- [ ] **Step 1: Write failing tests**:
  - `TestLoadSettings_V2KeepsGoodbyeDPI` (Review Focus #3): file `{"version":2,"dpi":{"enabled":true,"preset":"high","scope":"blacklist"}}` → `Engine == "goodbyedpi"`, `Preset == "high"`, `Scope == "blacklist"`, `Zapret2.Strategy == "z-split"`, `Version == 3`.
  - `TestLoadSettings_V1File` (no `version` key) → also `goodbyedpi`.
  - `TestLoadSettings_NoFile` → `zapret2`.
  - `TestLoadSettings_V3KeepsChoice`: `{"version":3,"dpi":{"engine":"zapret2"}}` → `zapret2`.
  - `TestLoadSettings_UnknownEngine`: `{"version":3,"dpi":{"engine":"x"}}` → `"zapret2"` (any v3 value other than the two IDs becomes the default).
- [ ] **Step 2: Run** `go test ./internal/store/` — FAIL.
- [ ] **Step 3: Implement**: in `LoadSettings`, after unmarshal read the file's original `version` (decode into `struct{ Version int }` first); if `< 3`, set `s.DPI.Engine = "goodbyedpi"`. Then normalise unknown engine, set `s.Version = 3`. Keep the existing v1→v2 behaviour and `oldProbeSites` migration.
- [ ] **Step 4: Run** — PASS. **Step 5: Commit** `feat(store): settings v3 with the DPI engine choice`

---

### Task 8: `app` — chọn engine, đường lui, autotune, binding

**Files:**
- Create: `internal/app/dpi.go`, `internal/app/dpi_engine_test.go`
- Modify: `internal/app/autotune.go` (keep `Autotune`, move `dpiErr`/`startDPI`/`RestartDPI`/`SetDPIEnabled` to `dpi.go`), `deps.go`, `errors.go`, `status.go`, `service.go`, `health.go` (unchanged call), `fakes_test.go`, existing tests that build `DPI` fakes

**Interfaces:**
- Consumes: `dpi.Manager` methods (Task 2), `store.DPISettings` (Task 7).
- Produces:
  ```go
  // deps.go
  type DPI interface {
      Start(ctx context.Context, engine string, p dpi.Plan) (int, error)
      Stop() error
      Running() bool
      Engine() string
      RefreshLists(p dpi.Plan) error
      Get(engine string) (dpi.Engine, bool)
  }
  // Deps gains AutoHostlistPath string (next to BlacklistPath)
  // status.go
  type DPIStatus struct {
      Enabled  bool   `json:"enabled"`
      Running  bool   `json:"running"`
      Engine   string `json:"engine"`   // engine actually running ("" when stopped)
      Preset   string `json:"preset"`   // strategy/preset actually running
      Fallback bool   `json:"fallback"` // running GoodbyeDPI because zapret2 failed
  }
  // errors.go
  CodeDPIFallback            = "DPI_FALLBACK"
  CodeAutotuneEngineSwitched = "AUTOTUNE_ENGINE_SWITCHED"
  CodeStrategyListInvalid    = "STRATEGY_LIST_INVALID"
  CodeDPICustomRejected      = "DPI_CUSTOM_REJECTED"
  ReasonDPIFallback          = "dpiFallback"
  // dpi.go
  func (o *Orchestrator) planFor(s store.Settings, engine string) dpi.Plan
  // service.go (bindings)
  func (s *Service) DPIStrategies(engine string) ([]dpi.Strategy, error)
  func (s *Service) PreviewDPIArgs(engine, strategy, custom, scope string, autoHostlist bool) ([]string, error)
  func (s *Service) GetDPIAutoHostlist() ([]string, error)
  func (s *Service) SaveDPIAutoHostlist(domains []string) error // restarts a running zapret2 so the file is reloaded from disk
  func (s *Service) RetryZapret2(ctx context.Context) error      // restarts DPI with the configured engine
  ```

Behaviour (spec §8):
- `planFor`: GoodbyeDPI → `Strategy: s.DPI.Preset`, `Custom: s.DPI.CustomArgs`; zapret2 → `Strategy: s.DPI.Zapret2.Strategy` (if not in the engine's `Strategies()` and not `"custom"` → first strategy, log `DPI_STRATEGY_RESET`), `Custom: s.DPI.Zapret2.CustomArgs`, `AutoHostlist: o.d.AutoHostlistPath` when `Zapret2.AutoHostlist`. `Blacklist: o.d.BlacklistPath`, `Scope` shared.
- `startDPI`: start the configured engine. If it is `zapret2` and the error is `ErrBlockedByAV` or `ErrHashMismatch` → start `goodbyedpi` with `planFor(s, "goodbyedpi")`. Success → `DPIStatus{Fallback: true, Engine: "goodbyedpi", ...}`, `addReason(ReasonDPIFallback)`, log `DPI_FALLBACK` with `from`, `to`, `cause` (the original code). Failure → return the **zapret2** error. Settings are never written here.
- Clearing: a successful start of the configured engine calls `clearReason(ReasonDPIFallback)`.
- `dpiErr(err, engine)`: the three existing DPI codes gain param `engine`.
- `RestartDPI`: unchanged contract, uses the configured engine (so a fallback state retries zapret2).
- `SaveSettings` restart trigger: also when `Engine`, `Zapret2.Strategy`, `Zapret2.CustomArgs`, `Zapret2.AutoHostlist` change. Validation: `GoodbyeDPI.ValidateCustom` for `DPI.CustomArgs` (as today) and `zapret2` `ValidateCustom` for `Zapret2.CustomArgs`; rejections return `appErr(CodeDPICustomRejected, err, "arg", <text>)`. `Engine` must be one of the two IDs.
- `SaveDPIBlacklist`: running engine with `HotReloadsLists()` → `RefreshLists` only; otherwise `RestartDPI` (as today).
- `Autotune`: iterate `Get(configured).Strategies()`. If the configured engine is `zapret2` and the **first** start returns `ErrBlockedByAV`/`ErrHashMismatch`, switch to iterating GoodbyeDPI's strategies; on success save `Engine: "goodbyedpi"` + `Preset`, log `AUTOTUNE_ENGINE_SWITCHED` (`from`, `to`, `preset`). On success with zapret2 save `Zapret2.Strategy`. `onProgress` gains the engine: `func(engine, preset string, i, n int)` — update the Wails event emitter accordingly.

- [ ] **Step 1: Update fakes** (`fakes_test.go`): `fakeDPI` records `(engine, plan)` per `Start`, can be told to fail per engine with a given error, and returns engines from a map of `goodbyedpi.New()` and `zapret2.New(func() strategies.List { builtin })`.
- [ ] **Step 2: Write failing tests** in `dpi_engine_test.go`:
  - `TestStartDPI_Zapret2BlockedFallsBack`: zapret2 → `ErrBlockedByAV`; second Start is `goodbyedpi` with `Strategy == s.DPI.Preset`; snapshot `Reasons` contains `dpiFallback`, `DPI.Fallback`; settings file unchanged (`Engine` still `zapret2`).
  - `TestStartDPI_HashMismatchFallsBack`: same with `ErrHashMismatch`.
  - `TestStartDPI_StartFailedNoFallback`: zapret2 → `ErrStartFailed` → no second Start, error code `DPI_START_FAILED`.
  - `TestStartDPI_FallbackAlsoFails`: both fail → returned code is zapret2's (`DPI_BLOCKED_BY_AV`).
  - `TestStartDPI_UnknownStrategyFallsBackToFirst` (Review Focus #4): `Zapret2.Strategy == "gone"` → Start plan `Strategy == "z-split"`.
  - `TestRestartDPI_RetriesConfiguredEngine`: after a fallback, `RestartDPI` starts zapret2 first and, on success, clears `dpiFallback`.
  - `TestAutotune_SwitchesEngineWhenZapret2Blocked`: zapret2 blocked → goodbyedpi `light` passes probes → saved `Engine == "goodbyedpi"`, `Preset == "light"`, log has `AUTOTUNE_ENGINE_SWITCHED`.
  - `TestAutotune_Zapret2SavesStrategy`: second zapret2 strategy passes → `Zapret2.Strategy == "z-disorder"`.
  - `TestSaveSettings_EngineChangeRestarts`, `TestSaveSettings_Zapret2CustomRejected` (`--lua-init=@x` → `DPI_CUSTOM_REJECTED`), `TestSaveDPIBlacklist_Zapret2NoRestart` (`RefreshLists` called, no Stop/Start).
- [ ] **Step 3: Run** `go test ./internal/app/` — FAIL.
- [ ] **Step 4: Implement.** Port every existing test in `internal/app` that uses the old `DPI` interface or `dpi.Args`/`dpi.AutotuneOrder`.
- [ ] **Step 5: Run** `go test ./internal/...` — PASS.
- [ ] **Step 6: Commit** `feat(app): choose the DPI engine, fall back to GoodbyeDPI when zapret2 is blocked`

---

### Task 9: Lắp ráp trong `shell` và tải danh sách chiến lược

**Files:**
- Modify: `internal/shell/shell.go`, `internal/shell/updates.go`, `internal/shell/catalog.go` (or new `internal/shell/strategies.go`), `internal/brand/brand.go`, `stopdpi.go`, `main.go`, `internal/shell/*_test.go` where wiring is tested

**Interfaces:**
- Consumes: `dpi.NewManager`, `goodbyedpi.New`, `zapret2.New`, `strategies.Select`, `assets/zapret2.FS`, `assets/strategies.BuiltinJSON`.
- Produces:
  ```go
  // brand
  StrategyListURL    = "https://raw.githubusercontent.com/hashcott/ghostline/main/lists/strategies.json"
  StrategyListSigURL = StrategyListURL + ".sig"
  // shell
  type strategyBox struct{ /* mutex + current strategies.List */ }
  func newStrategyBox(p store.Paths, log *slog.Logger) *strategyBox // Select(builtin, remote file, sig file)
  func (b *strategyBox) get() strategies.List
  func (b *strategyBox) reload()
  func newDPIManager(paths store.Paths, list func() strategies.List) *dpi.Manager // shared by shell.Run and headless stopdpi.go
  ```
  `shell.Options.DPIAssets` is replaced by `GoodbyeDPIAssets fs.FS` and `Zapret2Assets fs.FS`.

- [ ] **Step 1: Write failing test** `TestStrategyBox_IgnoresTamperedRemote`: remote file + wrong sig on disk → `get()` equals builtin; log line contains `STRATEGY_LIST_INVALID`.
- [ ] **Step 2: Implement** `strategyBox`; `newDPIManager` builds `[]dpi.Installed{{goodbyedpi.New(), goodbyedpiFS}, {zapret2.New(box.get), zapret2FS}}` under `paths.BinDir`. Pass `AutoHostlistPath: paths.DPIAutoHostlist` into `app.Deps`. In `updates.go`, alongside the server list (same `UpdateServerList` switch, same `updater.Due` cadence with a new `Meta.LastStrategyList`), fetch the strategy list and its sig with a small `updater.FetchSigned(ctx, client, url, sigURL, pub) (raw, sig []byte, err error)` (extract the download+verify part of `FetchServerList` into it and have `FetchServerList` call it), write `paths.DPIStrategies`/`DPIStrategiesSig`, then `box.reload()`.
- [ ] **Step 3:** `stopdpi.go` uses the shared constructor; watchdog `StopDPI` is still `Manager.Stop` (the kill-on-close job already ends `winws2`; `Stop` removes every `WinDivert*` service). `state.json` records `Engine` via `recordDPI`.
- [ ] **Step 4: Run** `go test ./...` and `go build ./...` — PASS.
- [ ] **Step 5: Commit** `feat(shell): wire both DPI engines and the strategy list updates`

---

### Task 10: Giao diện trang Vượt DPI

**Files:**
- Modify: `frontend/src/modes/advanced/pages/Dpi.tsx`, `frontend/src/i18n/vi.json`, `frontend/src/i18n/en.json`, `frontend/src/app/api.ts` if it re-exports types, `frontend/src/modes/advanced/pages25.test.tsx` / `dpi-blacklist.test.tsx` (port), new `frontend/src/modes/advanced/dpi-engine.test.tsx`
- Regenerate: `frontend/bindings/**` via `wails3 task common:generate:bindings` (or `task common:generate:bindings`)

**Interfaces:**
- Consumes: bindings from Task 8 (`DPIStrategies`, `PreviewDPIArgs(engine, strategy, custom, scope, autoHostlist)`, `GetDPIAutoHostlist`, `SaveDPIAutoHostlist`, `RetryZapret2`), `settings.dpi.engine`, `settings.dpi.zapret2`, `snapshot.dpi.{engine,fallback}`.

UI (spec §9.3):
- Engine row: two chips `zapret2 (khuyên dùng)` / `GoodbyeDPI` + one-line description each. Changing it calls `saveSettings({dpi: {...dpi, engine}})`.
- Preset row lists `Service.DPIStrategies(engine)` names in the current language, plus `custom`. GoodbyeDPI keeps `mode1..mode6` (they are not in `Strategies()` because autotune does not try them — keep the existing constant for them).
- Custom args box edits `dpi.customArgs` (GoodbyeDPI) or `dpi.zapret2.customArgs` (zapret2).
- When `engine == "zapret2" && scope == "blacklist"`: toggle **Tự phát hiện trang bị chặn** (`dpi.zapret2.autoHostlist`), and under it the auto-added domains with per-row remove and "xoá hết" (`SaveDPIAutoHostlist`).
- Banner (dismissible, sets `dpi.hideEngineHint`) when `engine == "goodbyedpi" && !hideEngineHint`.
- Warning banner when `snapshot.dpi.fallback`: Defender exclusion hint showing the `bin\zapret2` folder path and a "Thử lại zapret2" button (`RetryZapret2`).
- The `DPI_START_FAILED` message, when `params.engine == "zapret2"`, adds the hint "close any zapret/winws2 running outside Ghostline" (spec §8.4 `--wf-dup-check`).
- New i18n keys under `dpi.engine.*`, `dpi.autoHostlist.*`, `dpi.fallback.*`, and error codes `DPI_FALLBACK`, `AUTOTUNE_ENGINE_SWITCHED`, `STRATEGY_LIST_INVALID`, `DPI_CUSTOM_REJECTED` in both languages.

- [ ] **Step 1: Write failing vitest tests** in `dpi-engine.test.tsx` (mock `Service` like the existing page tests): engine switch saves `engine`; strategy list comes from `DPIStrategies("zapret2")` and shows Vietnamese names when language is `vi`; auto-hostlist toggle only rendered for zapret2 + blacklist; fallback banner rendered when `snapshot.dpi.fallback` and its button calls `RetryZapret2`; engine hint hidden when `hideEngineHint`. Add an i18n parity test (or extend the existing one) asserting every new key exists in both files.
- [ ] **Step 2: Run** `cd frontend && npx vitest run` — FAIL.
- [ ] **Step 3: Regenerate bindings, implement** the page changes.
- [ ] **Step 4: Run** `cd frontend && npx vitest run && npx tsc --noEmit` — PASS.
- [ ] **Step 5: Commit** `feat(ui): DPI engine choice, zapret2 strategies and auto-detected sites`

---

### Task 11: Tài liệu và checklist phát hành

**Files:**
- Modify: `docs/release-checklist.md`, `docs/user-guide.md`, `docs/huong-dan-su-dung.md`, `docs/superpowers/specs/2026-10-05-ghostline-phase2b-design.md` (note in §2/§4.2 `dpi` row: GoodbyeDPI upgrade done by the zapret2 spec; `--fake-with-sni` only for engine GoodbyeDPI)

- [ ] **Step 1:** Add to the release checklist the eight manual checks of spec §10.2 verbatim, plus: "Ký `lists/strategies.json` bằng `go run ./tools/genservers -sign-file lists/strategies.json -sign-env SERVERLIST_SIGNING_KEY`", and "Gửi `winws2.exe` và `ghostline.exe` lên https://www.microsoft.com/wdsi/filesubmission; ghi lại mã gửi".
- [ ] **Step 2:** User guides (vi + en): DPI section gains the engine choice, auto-detect, and how to add a Defender exclusion for the `bin\zapret2` folder.
- [ ] **Step 3: Commit** `docs: zapret2 engine in the guides and the release checklist`

---

## Final verification

- [ ] `go vet ./... && go test ./...` — PASS
- [ ] `cd frontend && npx vitest run && npx tsc --noEmit` — PASS
- [ ] `task build` produces `bin/ghostline.exe`; launching it as admin and connecting with engine zapret2 runs `winws2.exe` (check Task Manager) — manual, on the developer machine with the Defender exclusion in place.
