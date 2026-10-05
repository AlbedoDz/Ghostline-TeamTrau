# Ghostline — Engine vượt DPI zapret2: Thiết kế

- **Ngày:** 2026-10-05
- **Trạng thái:** Chờ duyệt spec
- **Phạm vi:** Thêm zapret2 (`winws2`) làm engine vượt DPI thứ hai bên cạnh GoodbyeDPI, nâng GoodbyeDPI lên 0.2.3rc3, danh sách chiến lược có chữ ký, chuẩn bị interface cho Linux/macOS.
- **Dựa trên:** [Giai đoạn 1](2026-10-04-ghostline-phase1-design.md), [Giai đoạn 2A](2026-10-04-ghostline-phase2a-design.md), [Giai đoạn 2B](2026-10-05-ghostline-phase2b-design.md). Mọi thứ không nhắc lại ở đây giữ nguyên.

---

## 1. Mục tiêu

- Có một engine vượt DPI **mạnh hơn GoodbyeDPI**: gói giả với TTL tự tính, multisplit/multidisorder, seqovl, và **QUIC/UDP**.
- **Cập nhật chiến lược không cần phát hành bản mới**: khi nhà mạng đổi cách chặn, chỉ cần đẩy một danh sách chiến lược mới có chữ ký.
- Chuẩn bị cho **Linux và macOS**: `app` và giao diện không phụ thuộc vào một engine cụ thể.
- Giữ GoodbyeDPI làm engine thứ hai và làm **đường lui** khi zapret2 bị diệt virus chặn.

### Tiêu chí thành công

1. Cài mới, Connect: các trang trong `ProbeSites` mở được bằng zapret2 với chiến lược mặc định, kể cả YouTube qua QUIC trên Chrome.
2. Người dùng v0.2.x nâng cấp lên **không thấy hành vi nào thay đổi**: engine vẫn là GoodbyeDPI (đã lên 0.2.3rc3), preset và tham số tuỳ chỉnh giữ nguyên.
3. zapret2 bị Defender chặn hoặc sai hash → Connect vẫn có vượt DPI bằng GoodbyeDPI, trạng thái SUY_GIẢM kèm hướng dẫn. Cài đặt engine không bị đổi.
4. **Danh sách chiến lược tải về không thể khiến `winws2` chạy code Lua tuỳ ý**, kể cả khi khoá ký bị lộ. Kiểm chứng bằng test.
5. Kill Ghostline thì `winws2` chết theo; watchdog gỡ dịch vụ `WinDivert` trong ≤ 3 giây.
6. Mọi chuỗi giao diện mới có đủ tiếng Việt và tiếng Anh.

## 2. Phạm vi

### Có

| Nhóm | Nội dung |
|---|---|
| Engine | Interface `Engine` trong `internal/dpi`; hai engine `goodbyedpi` và `zapret2`. Một `Manager` chung chạy tối đa một engine |
| zapret2 | Nhúng bản phát hành chính thức `winws2` v1.0.5.2 x86_64 (ghim SHA-256), dựng argv từ chiến lược, phạm vi Tất cả/Danh sách, tự phát hiện trang bị chặn (`--hostlist-auto`) |
| GoodbyeDPI | Nâng lên 0.2.3rc3 (WinDivert 2.2), cho phép `--fake-with-sni` trong tham số tuỳ chỉnh |
| Chiến lược | `strategies.json` ký ed25519 như `servers.json`, bản dự phòng nhúng trong app, cập nhật theo lịch, bộ kiểm tra allow-list |
| Đường lui | Connect và autotune tự quay về GoodbyeDPI khi zapret2 bị chặn hoặc sai hash |
| Giao diện | Chọn engine, chiến lược theo engine, công tắc tự phát hiện, banner gợi ý cho người cũ |

### Không có

- Code chạy trên Linux/macOS (`nfqws2`, `tpws`). Chỉ có interface sẵn sàng (mục 4.4).
- Tự build `winws2` từ mã nguồn. Dùng bản chính thức để ai cũng đối chiếu được hash.
- Chạy hai engine cùng lúc.
- Người dùng tự viết hoặc nạp file Lua.
- Windows ARM64 (winws2 không có bản ARM; WinDivert ARM64 chưa ký).

## 3. Các quyết định đã chốt

| Chủ đề | Quyết định |
|---|---|
| Tầng can thiệp | Mức gói tin (WinDivert), áp dụng cho mọi ứng dụng — không phải chỉ proxy |
| Engine mới | **zapret2**, không phải zapret1 (EOL), DPIBreak (ít kỹ thuật, không có macOS) hay SpoofDPI (không chạy trên Windows, cần Npcap cho gói giả) |
| GoodbyeDPI | **Giữ**, làm engine thứ hai và đường lui. Nâng lên 0.2.3rc3 trong spec này |
| Mặc định | Người cũ giữ GoodbyeDPI; cài mới dùng zapret2 |
| Nguồn chiến lược | Bản nhúng + danh sách tải về có chữ ký ed25519, cùng khoá với `servers.json` |
| Nguồn file nhị phân | Bản phát hành chính thức, ghim SHA-256. Bị diệt virus chặn → đường lui GoodbyeDPI + hướng dẫn loại trừ + gửi Microsoft xem xét báo nhầm mỗi lần nâng phiên bản |
| macOS sau này | `tpws` của zapret1 qua `pf` (chỉ TCP, không gói giả) vì zapret2 không hỗ trợ macOS |

## 4. Kiến trúc

### 4.1 Cấu trúc package

```
internal/dpi/
  engine.go          interface Engine, Plan, Strategy, các lỗi chung
  manager.go         chạy tối đa 1 engine: giải nén, hash, job object, chờ, driver, dọn dịch vụ
  runner_windows.go  (giữ nguyên)
  goodbyedpi/        preset light..extreme, mode1..6, kiểm tra tham số tuỳ chỉnh (chuyển từ preset.go)
  zapret2/           dựng argv, kiểm tra chiến lược và tham số, đường dẫn hostlist
  strategies/        đọc, kiểm tra chữ ký, chọn bản mới nhất giữa bản nhúng và bản tải về
assets/goodbyedpi/   goodbyedpi.exe 0.2.3rc3, WinDivert.dll, WinDivert64.sys, giấy phép
assets/zapret2/      winws2.exe, cygwin1.dll, WinDivert.dll, WinDivert64.sys, lua/*.lua, giấy phép
assets/strategies/   strategies.json (bản dự phòng nhúng)
```

### 4.2 Interface

```go
// Engine describes one DPI bypass program. It builds argv; Manager runs it.
type Engine interface {
    ID() string                     // "goodbyedpi" | "zapret2"
    Files() map[string]string       // relative path → pinned SHA-256
    Exe() string                    // relative path of the executable
    Args(p Plan) ([]string, error)
    Strategies() []Strategy         // in autotune order, lightest first
}

type Plan struct {
    Strategy     string  // preset/strategy id, or "custom"
    Custom       string  // user-typed args when Strategy == "custom"
    Scope        Scope   // all | blacklist
    Blacklist    string  // absolute path of the user's blacklist
    AutoHostlist string  // absolute path; "" = off (zapret2 only)
}

type Strategy struct {
    ID   string
    Name map[string]string // "vi", "en"
}
```

- `Manager` nhận `map[string]Engine` và các file nhúng của từng engine. `Start(ctx, engineID, plan)` giải nén vào `bin/<engineID>/`, kiểm tra hash, **gỡ dịch vụ `WinDivert` còn sót**, chạy, chờ 2 giây, kiểm tra process còn sống và driver đang chạy.
- Interface `DPI` trong `app` đổi thành:
  ```go
  type DPI interface {
      Start(ctx context.Context, engine string, p dpi.Plan) (int, error)
      Stop() error
      Running() bool
      Engine() string // engine đang chạy, "" nếu không chạy
  }
  ```
- Phát hiện diệt virus chặn (`ErrBlockedByAV`) và sai hash (`ErrHashMismatch`) giữ nguyên logic hiện tại, áp dụng cho mọi engine.

### 4.3 Driver WinDivert

- GoodbyeDPI 0.2.3rc3 và zapret2 đều dùng WinDivert dòng 2.2 nhưng **khác bản build** (hash `WinDivert64.sys` lần lượt `e69b5ba3…` và `8da08533…`), cùng tên dịch vụ `WinDivert`.
- Mỗi engine giữ driver trong thư mục riêng. `Manager` **luôn dừng và gỡ dịch vụ `WinDivert` trước khi khởi động** engine, để engine nạp đúng driver của nó kể cả khi engine trước crash.
- `driverRunning()` chỉ tìm dịch vụ đúng tên `WinDivert` (bỏ cách tìm theo tiền tố dành cho WinDivert 1.x `WinDivert1.4`). Phần dọn dẹp lúc nâng cấp và `--restore` vẫn tìm theo tiền tố để gỡ được dịch vụ 1.4 do bản cũ để lại.

### 4.4 Chuẩn bị cho Linux/macOS

- `Engine` không chứa gì riêng của Windows. Bộ lọc WinDivert (`--wf-*`) do `zapret2` sinh trong file `args_windows.go`; sau này `args_linux.go` sinh `--qnum` và luật nftables do một interface `Interceptor` riêng cài và gỡ.
- `Manager` chỉ dùng `Runner` và `Services` (đã là interface). Trên Linux, `Services` thay bằng việc cài và gỡ luật nftables; trên macOS là luật `pf` cho `tpws`.
- Spec này không viết các file đó.

## 5. Engine zapret2

### 5.1 File nhúng

Lấy từ `zapret2-v1.0.5.2.zip` (bản phát hành chính thức), thư mục `binaries/windows-x86_64/` và `lua/`:

| File | Ghi chú |
|---|---|
| `winws2.exe` | Build bằng Cygwin |
| `cygwin1.dll` | LGPLv3, đóng gói nguyên bản |
| `WinDivert.dll`, `WinDivert64.sys` | WinDivert 2.2, LGPLv3, driver đã ký |
| `lua/zapret-lib.lua`, `lua/zapret-antidpi.lua`, `lua/zapret-auto.lua` | Nạp bằng `--lua-init` |

`mdig.exe`, `ip2net.exe`, `killall.exe`, `zapret-obfs.lua`, `zapret-pcap.lua`, `zapret-tests.lua` không được nhúng. Hash trong `zapret2.Pinned` phải khớp `sha256sum.txt` của bản phát hành; bước `task assets:zapret2` trong Taskfile tải, đối chiếu và copy.

### 5.2 Dựng argv

Thứ tự cố định, mỗi phần tử một tham số, mọi giá trị dùng dạng `--name=value`:

1. `--wf-tcp-out=80,443`, và `--wf-udp-out=443` nếu chiến lược có phần QUIC.
2. `--wf-dup-check=1`.
3. `--lua-init=@lua/zapret-lib.lua`, `--lua-init=@lua/zapret-antidpi.lua`, `--lua-init=@lua/zapret-auto.lua` (đường dẫn tương đối với thư mục làm việc `bin/zapret2`).
4. Profile TCP: `--filter-tcp=80,443`, phần lọc phạm vi (5.3), rồi các tham số `tcp` của chiến lược.
5. Profile QUIC (nếu có): `--new`, `--filter-udp=443`, `--filter-l7=quic`, phần lọc phạm vi, rồi các tham số `quic`.

Đường dẫn file danh sách luôn được copy vào `bin/zapret2/` và truyền bằng tên tương đối, như `localBlacklist` hiện tại, để không phụ thuộc vào cách Cygwin xử lý đường dẫn Unicode (`C:\Users\Đức…`).

### 5.3 Phạm vi

| Cài đặt | Thêm vào mỗi profile |
|---|---|
| Tất cả, tự phát hiện tắt | (không thêm) |
| Danh sách, tự phát hiện tắt | `--hostlist=blacklist.txt` |
| Tự phát hiện bật | `--hostlist=blacklist.txt` (nếu phạm vi Danh sách) và `--hostlist-auto=autohostlist.txt` |

- Với **Tất cả + tự phát hiện bật**, chiến lược áp dụng cho mọi domain nên tự phát hiện không có tác dụng. Giao diện chỉ hiện công tắc tự phát hiện khi phạm vi là Danh sách.
- `winws2` tự nạp lại hostlist khi file đổi, nên sửa danh sách đen khi đang chạy **không** cần khởi động lại engine (khác GoodbyeDPI: vẫn khởi động lại như hiện tại).
- `autohostlist.txt` nằm trong `bin/zapret2/`. Khi dừng engine, `Manager` copy nó ra `dpi-autohostlist.txt` trong thư mục dữ liệu; khi khởi động thì copy ngược vào. Giao diện đọc file trong thư mục dữ liệu.

## 6. Chiến lược

### 6.1 Định dạng `strategies.json`

```json
{
  "version": 1,
  "zapret2": [
    {
      "id": "z-split",
      "name": { "vi": "Nhẹ", "en": "Light" },
      "tcp":  ["--lua-desync=multisplit:pos=1,midsld"],
      "quic": []
    },
    {
      "id": "z-disorder",
      "name": { "vi": "Vừa", "en": "Medium" },
      "tcp":  ["--lua-desync=multidisorder:pos=1,midsld"],
      "quic": []
    },
    {
      "id": "z-fake",
      "name": { "vi": "Mạnh", "en": "Strong" },
      "tcp":  ["--lua-desync=fake:blob=fake_default_tls:badsum", "--lua-desync=multidisorder:pos=1,midsld"],
      "quic": ["--lua-desync=fake:blob=fake_default_quic:repeats=6"]
    },
    {
      "id": "z-fake-ttl",
      "name": { "vi": "Rất mạnh", "en": "Extreme" },
      "tcp":  ["--lua-desync=fake:blob=fake_default_tls:ip_autottl=-2,3-20", "--lua-desync=fakeddisorder:pos=1,midsld"],
      "quic": ["--lua-desync=fake:blob=fake_default_quic:repeats=11"]
    }
  ]
}
```

- Thứ tự trong mảng là **thứ tự autotune**, nhẹ trước. Chiến lược đầu tiên là mặc định khi cài mới.
- Bốn chiến lược trên là bản dự phòng nhúng ban đầu. Tham số cụ thể được xác nhận ở bước test tích hợp (mục 10.2) trên ít nhất hai nhà mạng Việt Nam trước khi phát hành, và về sau chỉnh qua danh sách tải về.
- Tối đa 32 chiến lược, mỗi chiến lược tối đa 16 tham số cho mỗi phần `tcp`/`quic`, `id` khớp `^[a-z0-9-]{1,32}$`.

### 6.2 Tải về và chữ ký

- URL: `https://raw.githubusercontent.com/hashcott/ghostline/main/lists/strategies.json`, chữ ký ở `strategies.json.sig`. Ký bằng cùng khoá ed25519 và cùng hàm `servers.Sign`/`VerifySigned` như `servers.json`.
- Cập nhật cùng lịch với danh sách server (`updates.updateServerList`).
- Bản tải về được lưu thành `dpi-strategies.json` (+ `.sig`) và chỉ được dùng khi: chữ ký hợp lệ, `version` **lớn hơn** bản nhúng, và **mọi** chiến lược qua bộ kiểm tra (6.3). Một điều kiện sai → bỏ cả file, giữ bản đang dùng, ghi `STRATEGY_LIST_INVALID`.
- Chiến lược đang lưu trong cài đặt không còn trong danh sách mới → dùng chiến lược đầu tiên và ghi log.

### 6.3 Bộ kiểm tra (allow-list)

Áp dụng cho từng tham số của chiến lược (kể cả bản nhúng, trong test) và cho tham số tuỳ chỉnh người dùng nhập:

- Chỉ chấp nhận `--lua-desync=<hàm>[:k[=v]]...` và `--payload=<kiểu>[,<kiểu>]`, `--out-range=…`, `--in-range=…` (giá trị khớp cú pháp trong manual).
- `<hàm>` phải thuộc allow-list: `fake`, `multisplit`, `multidisorder`, `fakedsplit`, `fakeddisorder`, `hostfakesplit`, `tcpseg`, `oob`, `wsize`, `wssize`, `syndata`, `synack`, `synack_split`, `tls_client_hello_clone`, `http_domcase`, `http_hostcase`, `http_methodeol`, `http_unixeol`, `udplen`, `drop`, `pass`. **Mọi hàm khác bị từ chối**, đặc biệt `luaexec` (chạy Lua tuỳ ý) và các hàm debug.
- Tên tham số khớp `^[a-z0-9_]{1,32}$`. Giá trị khớp `^[A-Za-z0-9_.,+\-]{0,64}$`: không có `@`, `/`, `\`, `:`, `=` hay khoảng trắng, nên không trỏ được tới file và không chèn thêm tham số.
- Ngoại lệ cho mẫu giá trị trên: `blob=` và `seqovl_pattern=` chỉ được là blob dựng sẵn của zapret2 (`fake_default_tls`, `fake_default_http`, `fake_default_quic`) hoặc chuỗi hex `^0x[0-9a-fA-F]{2,2048}$`.
- Mọi tham số khác bị từ chối, trong đó có `--lua-init`, `--blob`, `--writable`, `--debug`, `--wf-*`, `--new`, `--filter-*`, `--hostlist*`, `--ipset*`, `@<file>`, `--pidfile`, `--chdir`, `--daemon`, `--intercept`. Chỉ Ghostline được tự thêm các flag cấu trúc ở mục 5.2.
- Tham số tuỳ chỉnh do người dùng nhập được tách bằng tokenizer có ngoặc kép (như GoodbyeDPI) rồi đưa vào profile TCP.

## 7. Engine GoodbyeDPI

- Nâng lên **0.2.3rc3** (`goodbyedpi-0.2.3rc3-2.zip`, thư mục `x86_64`). Hash mới:
  - `goodbyedpi.exe` `8d412b094bb9c137ff25ba9a794d1122ecc84bb776debff6c249723a13cc31cd`
  - `WinDivert.dll` `6110bfa44667405179c3e15e12af1b62037e447ed59b054b19042032995e6c7e`
  - `WinDivert64.sys` `e69b5ba3f0cd6cfb2983e442636e7f0b342b61b15264b0328317d4559c82cf50`
- Preset `light`, `medium`, `high`, `extreme`, `mode1..6` giữ nguyên tham số.
- Thêm `--fake-with-sni <domain>` và `--fake-gen <n>` vào danh sách flag tuỳ chỉnh được phép; giá trị domain phải là tên miền hợp lệ, `n` là số 1–30. Phần 2B dùng `--fake-with-sni` sẽ dựa vào việc này.
- Cách chạy, phạm vi Danh sách (`--blacklist blacklist.txt`) và khởi động lại khi sửa danh sách giữ như hiện tại.

## 8. Tích hợp với Connect, autotune và an toàn

### 8.1 Connect

Bước DPI của Connect (hiện là `startDPI`):

1. Lấy `engine` từ cài đặt và dựng `Plan`.
2. `DPI.Start(engine, plan)`. Thành công → như hiện tại, `DPIStatus` có thêm `Engine`.
3. Engine là `zapret2` và lỗi là `ErrBlockedByAV` hoặc `ErrHashMismatch` → thử `goodbyedpi` với preset GoodbyeDPI đang lưu (mặc định `light`), cùng phạm vi.
   - Thành công → SUY_GIẢM lý do `dpiFallback`, log `DPI_FALLBACK{from: zapret2, to: goodbyedpi, cause}`. **Cài đặt không đổi**: lần Connect sau vẫn thử zapret2 trước.
   - Thất bại → lỗi DPI như hiện tại (trả mã của lần thử zapret2).
4. Lỗi khác (`ErrStartFailed`) → không quay về, báo lỗi như hiện tại.

### 8.2 Đổi cài đặt khi đang kết nối

| Thay đổi | Hành động |
|---|---|
| Đổi engine | Dừng engine cũ, khởi động engine mới (Manager tự gỡ dịch vụ driver ở giữa) |
| Đổi chiến lược/preset, tham số tuỳ chỉnh, phạm vi, tự phát hiện | Khởi động lại engine (`RestartDPI`) |
| Sửa danh sách đen | GoodbyeDPI: khởi động lại. zapret2: chỉ ghi file vào `bin/zapret2/blacklist.txt` |
| Danh sách chiến lược mới tải về | Không khởi động lại; dùng từ lần khởi động sau |

### 8.3 Autotune

- Thử lần lượt `Strategies()` của **engine đang chọn**, giữ nguyên tiêu chí đạt (mọi trang bị chặn qua được giai đoạn TLS) và thời gian chờ `dpiSettleDelay`.
- Engine `zapret2` mà lần thử đầu lỗi `ErrBlockedByAV`/`ErrHashMismatch` → chuyển sang thử preset GoodbyeDPI. Tìm được → lưu `engine: goodbyedpi` và preset đó, báo `AUTOTUNE_ENGINE_SWITCHED`. Đây là trường hợp **duy nhất** cài đặt engine tự đổi, vì người dùng đã chủ động bấm autotune.
- Tiến độ gửi lên giao diện có thêm tên engine.

### 8.4 An toàn hệ thống

- `winws2` chạy ẩn trong job object kill-on-close, như GoodbyeDPI.
- `state.json` `dpi` thêm `engine` (để ghi log và chẩn đoán). Như hiện tại, process engine nằm trong job object kill-on-close nên chết cùng Ghostline; watchdog, khôi phục lúc mở app và `--restore` chỉ cần gọi `Manager.Stop`, vốn gỡ mọi dịch vụ có tên bắt đầu bằng `WinDivert`. Không giết process theo PID.
- `--wf-dup-check=1`: nếu đã có một `winws2` khác (người dùng tự chạy zapret) với cùng bộ lọc, `winws2` thoát ngay → `DPI_START_FAILED` với gợi ý "đóng zapret đang chạy bên ngoài Ghostline".
- Trình gỡ cài đặt xoá `bin/zapret2/` cùng các thư mục engine khác (như hiện tại với `bin/goodbyedpi/`).

## 9. Dữ liệu và giao diện

### 9.1 `settings.json` v3

```json
"dpi": {
  "enabled": true,
  "engine": "goodbyedpi",
  "preset": "light",
  "customArgs": "",
  "scope": "all",
  "zapret2": { "strategy": "z-split", "customArgs": "", "autoHostlist": false }
}
```

- `preset` và `customArgs` ở cấp `dpi` vẫn là của GoodbyeDPI (giữ tương thích). zapret2 có khối riêng. `scope` dùng chung.
- **Nâng cấp v2 → v3:** thêm `engine: "goodbyedpi"` và khối `zapret2` mặc định; mọi giá trị cũ giữ nguyên. Nếu spec 2B đã đưa `settings.json` lên v3 trước, phần này thành bước nâng cấp v3 → v4 với cùng quy tắc.
- **Cài mới:** `engine: "zapret2"`, `zapret2.strategy` = chiến lược đầu tiên.

### 9.2 Thư mục dữ liệu (thêm)

```
bin/zapret2/                 winws2 và file đi kèm (giải nén từ app)
dpi-strategies.json          danh sách chiến lược tải về
dpi-strategies.json.sig      chữ ký
dpi-autohostlist.txt         domain winws2 tự thêm
```

### 9.3 Trang Vượt DPI

- Dòng mới **Engine**: `zapret2 (khuyên dùng)` | `GoodbyeDPI`, kèm một dòng mô tả ngắn cho mỗi engine.
- Danh sách preset/chiến lược và ô tham số tuỳ chỉnh đổi theo engine đang chọn. Nút autotune giữ nguyên.
- Khi engine là zapret2 và phạm vi là Danh sách: công tắc **Tự phát hiện trang bị chặn**, kèm danh sách domain đã tự thêm (xem, xoá từng dòng, xoá hết).
- Người dùng đang ở GoodbyeDPI thấy banner đóng được: *"Thử engine mới zapret2: mạnh hơn, hỗ trợ QUIC"*. Đóng rồi thì không hiện lại (`dpi.hideEngineHint` trong cài đặt).
- Khi đang ở trạng thái `dpiFallback`: banner cảnh báo kèm hướng dẫn thêm `…\bin\zapret2` vào danh sách loại trừ của Defender và nút "Thử lại zapret2".
- Chế độ Đơn giản không đổi.

### 9.4 Mã lỗi và log

| Mã | Khi nào |
|---|---|
| `DPI_START_FAILED{engine}`, `DPI_BLOCKED_BY_AV{engine}`, `DPI_HASH_MISMATCH{engine}` | Như hiện tại, thêm trường `engine` |
| `DPI_FALLBACK{from,to,cause}` | Connect đã quay về GoodbyeDPI |
| `AUTOTUNE_ENGINE_SWITCHED{from,to,preset}` | Autotune đã đổi engine |
| `STRATEGY_LIST_INVALID{reason}` | Danh sách tải về bị bỏ (chữ ký, version, bộ kiểm tra, JSON) |
| `DPI_CUSTOM_REJECTED{arg}` | Tham số tuỳ chỉnh bị bộ kiểm tra từ chối |

### 9.5 Giấy phép

`NOTICE` thêm:
- zapret2 (github.com/bol-van/zapret2) — MIT. Đóng gói nguyên bản `winws2.exe` và các file Lua.
- Cygwin (`cygwin1.dll`) — LGPLv3. Đóng gói nguyên bản.
- WinDivert 2.2 đi kèm zapret2 và GoodbyeDPI 0.2.3rc3 — LGPLv3.
- Cập nhật dòng GoodbyeDPI thành 0.2.3rc3.

File giấy phép đặt trong `assets/zapret2/` và `assets/goodbyedpi/`.

## 10. Kiểm thử

### 10.1 Unit test (CI, không cần admin)

| Package | Nội dung |
|---|---|
| `dpi/zapret2` | Argv đúng thứ tự cho mọi tổ hợp phạm vi × tự phát hiện × có/không QUIC. Đường dẫn danh sách luôn là tên tương đối |
| `dpi/zapret2` (kiểm tra) | Bảng test: mọi hàm ngoài allow-list (đặc biệt `luaexec`), giá trị có `@`, `/`, `:`, `=`, khoảng trắng, blob lạ, flag cấu trúc, `@config`. **Fuzz** tokenizer và bộ kiểm tra: không panic, không có tham số nào lọt qua mà không thuộc dạng cho phép |
| `dpi/strategies` | Chữ ký đúng/sai, `version` thấp hơn hoặc bằng bản nhúng, JSON hỏng, một chiến lược hỏng làm hỏng cả file, quá giới hạn số lượng. Bản nhúng qua được bộ kiểm tra |
| `dpi/goodbyedpi` | Preset giữ nguyên argv như trước. `--fake-with-sni`, `--fake-gen` hợp lệ/không hợp lệ |
| `dpi` (Manager) | Gỡ dịch vụ `WinDivert` trước khi khởi động; đổi engine; sai hash thì giải nén lại; bị chặn → `ErrBlockedByAV`; copy autohostlist vào/ra. Dùng `Runner`/`Services` giả |
| `app` | Connect quay về GoodbyeDPI (SUY_GIẢM `dpiFallback`, cài đặt không đổi); `ErrStartFailed` không quay về; autotune đổi engine; đổi engine khi đang kết nối |
| `store` | Nâng cấp v2 → v3 (người cũ `goodbyedpi`), cài mới `zapret2` |
| `assets` | Hash file nhúng khớp `Pinned` của từng engine |

### 10.2 Test tích hợp (tay, Windows, admin) — thêm vào `docs/release-checklist.md`

1. Cài mới, Connect: `ProbeSites` mở được bằng zapret2; YouTube trên Chrome chạy QUIC (kiểm tra `chrome://net-internals`) vẫn xem được.
2. Autotune với zapret2 trên ít nhất hai nhà mạng (ví dụ Viettel, VNPT): tìm được chiến lược; ghi lại chiến lược thắng để chỉnh `strategies.json`.
3. Nâng cấp từ v0.2.5: engine là GoodbyeDPI 0.2.3rc3, preset cũ chạy được, dịch vụ `WinDivert1.4` cũ đã bị gỡ.
4. Đổi GoodbyeDPI → zapret2 → GoodbyeDPI khi đang kết nối: `sc qc WinDivert` trỏ đúng file `.sys` của engine đang chạy.
5. Kill `ghostline.exe` bằng Task Manager: `winws2` chết theo; dịch vụ được gỡ trong ≤ 3 giây.
6. Để Defender chặn `winws2.exe` (bỏ loại trừ, giải nén lại): Connect quay về GoodbyeDPI, banner hướng dẫn hiện ra.
7. Phạm vi Danh sách + tự phát hiện: mở một trang bị chặn không có trong danh sách ba lần → domain hiện trong danh sách tự thêm, lần sau mở được.
8. Tự chạy một `winws2` ngoài Ghostline rồi Connect: lỗi `DPI_START_FAILED` với gợi ý đúng.

## 11. Phát hành

- `task assets:zapret2 VERSION=v1.0.5.2`: tải zip chính thức, đối chiếu `sha256sum.txt`, copy đúng các file ở mục 5.1 vào `assets/zapret2/`. Tương tự `task assets:goodbyedpi`. File nhị phân được commit như hiện tại.
- Mỗi lần nâng zapret2 hoặc phát hành Ghostline: gửi `winws2.exe` và `ghostline.exe` cho Microsoft xem xét báo nhầm (Defender đã báo `Trojan:Win32/Suschil!rfn` với zip zapret2 v1.0.5.2 trên máy phát triển), ghi lại trong checklist.
- `lists/strategies.json` và `.sig` được ký bằng công cụ ký `servers.json` hiện có.
- Ghi chú phát hành và hướng dẫn sử dụng (vi/en) có mục engine mới và cách thêm loại trừ Defender.

## 12. Ảnh hưởng tới spec 2B

- Mục "Gói giả mang SNI" của 2B vẫn dùng `--fake-with-sni` của GoodbyeDPI; việc nâng lên 0.2.3rc3 đã được làm ở spec này. Với engine zapret2, gói giả là một phần của chiến lược (`fake:blob=…`), nên tuỳ chọn 2B chỉ hiện khi engine là GoodbyeDPI.
- Số phiên bản `settings.json` của hai spec được gộp theo thứ tự triển khai (mục 9.1).
