# Ghostline — Giai đoạn 1: Thiết kế

- **Ngày:** 2026-10-04
- **Trạng thái:** Chờ duyệt spec
- **Phạm vi:** Giai đoạn 1 (nhóm tính năng 1–4). Giai đoạn 2 và 3 sẽ có spec riêng.

---

## 1. Mục tiêu

Ghostline là ứng dụng **Secure DNS Client cho Windows**. App mã hoá toàn bộ truy vấn DNS của máy và giúp vượt chặn DNS/SNI của nhà mạng. Có hai kiểu người dùng:

- **Người dùng phổ thông** (chế độ Đơn giản): bấm một nút là được bảo vệ, không cần hiểu DNS là gì.
- **Người dùng kỹ thuật** (chế độ Nâng cao): chỉnh được server, quét server, cấu hình vượt DPI và xem nhật ký.

Ý tưởng lấy cảm hứng từ DNSveil (trước đây là SecureDNSClient), nhưng Ghostline viết lại từ đầu bằng Go, có giao diện hiện đại và an toàn hơn với hệ thống.

### Tiêu chí thành công

1. Trên Windows 11 x64 vừa cài mới, người dùng tải bản zip về, chạy, bấm Connect và đạt trạng thái **Đã bảo vệ trong ≤ 25 giây** ở lần đầu, và **≤ 5 giây** khi đã có kết quả quét lưu sẵn.
2. **Khi đã bảo vệ, không có truy vấn DNS dạng plain nào đi ra ngoài.** Ngoại lệ duy nhất là truy vấn bootstrap được nêu ở mục 6.4. Bước xác minh 07 phải đạt, và lúc phát hành sẽ kiểm tra thủ công thêm bằng Wireshark.
3. **App crash hoặc bị kill (`taskkill /F`) thì DNS phải được trả về đúng bản đã lưu trong ≤ 3 giây.**
4. **Máy mất điện hoặc khởi động lại khi đang kết nối thì DNS được khôi phục ở lần đăng nhập kế tiếp**, kể cả khi người dùng không mở Ghostline.
5. **Mọi chuỗi chữ trên giao diện đều có cả tiếng Việt và tiếng Anh**, mặc định là tiếng Việt.

## 2. Phạm vi

### Có trong giai đoạn 1

| Nhóm | Nội dung |
|---|---|
| 1. Server | Danh sách server có sẵn và cập nhật qua URL (có chữ ký). Thêm server bằng tay hoặc import. Quét để tìm server nhanh nhất |
| 2. Kết nối | DNS server cục bộ (engine Go), đặt và khôi phục DNS hệ thống, xác minh không rò rỉ, chống mất mạng 4 lớp |
| 3. Vượt DPI | GoodbyeDPI (preset, tham số tự nhập, Tự dò, danh sách đen) và Fragment cho kết nối DoH |
| 4. Hệ thống | Icon khay, khởi động cùng Windows, tự kết nối, kiểm tra bản mới |

### Không có trong giai đoạn 1

- **Giai đoạn 2:** proxy HTTP/HTTPS/SOCKS để chia sẻ cho thiết bị khác, Fragment và Fake SNI cho lưu lượng web, DoH server cục bộ (cần chứng chỉ gốc), rules theo domain/CIDR.
- **Giai đoạn 3:** Advanced DNS Scanner, công cụ DNS Lookup, quét IP sạch của Cloudflare, mã hoá/giải mã STAMP, import/export cài đặt.
- **Chưa làm:** tự tải và cài bản cập nhật, ký số file exe, Windows ARM64, telemetry. **Ghostline không bao giờ có telemetry.**

## 3. Các quyết định đã chốt

| Chủ đề | Quyết định |
|---|---|
| Tên | **Ghostline**. Tên, repo và URL đều nằm ở một chỗ duy nhất: package `internal/brand` |
| Nền tảng | Windows 10 bản 2004 (build 19041) trở lên và Windows 11, chỉ **x64** |
| Ngôn ngữ, khung | Go (lõi) + **Wails v3**, ghim một bản beta cụ thể. Frontend dùng **React + TypeScript + Vite** |
| Engine DNS | **`github.com/AdguardTeam/dnsproxy`** dùng như thư viện, không dùng MsmhAgnosticServer. `upstream` tự viết cho Fragment |
| Thành phần bên ngoài | Chỉ có **GoodbyeDPI** (bản chính thức x86_64 của ValdikSS, kèm WinDivert) |
| Giao diện | Phong cách **Neon Terminal**. Bố cục **Co giãn**: Đơn giản khoảng 380×580, Nâng cao khoảng 1000×660 |
| Ngôn ngữ giao diện | Tiếng Việt (mặc định) và tiếng Anh, đổi được trong app |
| Mặc định khi Connect | Chỉ bật DNS mã hoá. Nếu trang mẫu bị chặn ở tầng TLS thì gợi ý "Tự dò vượt DPI" |
| Phát hành | Có cả **installer NSIS** và **zip portable** |
| Cập nhật | **Chỉ báo có bản mới**, dựa trên GitHub Releases |
| License | **MIT**, mã nguồn mở trên GitHub. Module mặc định là `github.com/hashcott/ghostline`, đổi trong `internal/brand` |

## 4. Kiến trúc

### 4.1 Mô hình process

- **`ghostline.exe`** chạy với quyền admin (manifest `requireAdministrator`). Một file exe này có 4 cách chạy:
  - *(không tham số)*: mở app có giao diện. Đây là cách chạy bình thường.
  - `--autostart`: Task Scheduler gọi lúc đăng nhập. App mở thẳng xuống khay, và tự kết nối nếu đã bật trong cài đặt.
  - `--watchdog --parent <pid>`: process giám sát (xem mục 5.5).
  - `--restore`: khôi phục DNS từ `state.json` nếu trạng thái chưa "sạch", rồi thoát. Không có giao diện.
- **GoodbyeDPI** là process con nằm trong một **Job Object** có cờ `KILL_ON_JOB_CLOSE`, nên process chính chết thì GoodbyeDPI cũng chết theo. Watchdog **không** nằm trong Job này, vì nó phải sống lâu hơn process chính.
- **Chỉ chạy một bản:** dùng tính năng single-instance của Wails v3. Mở lần thứ 2 thì đưa cửa sổ đang có lên trước.
- **Khi khởi động cùng Windows,** app chạy qua tác vụ Task Scheduler tên `Ghostline` với trigger lúc đăng nhập, `RunLevel=Highest` và tham số `--autostart`, nên không bị hỏi UAC.

**Phương án đã cân nhắc và bỏ:** tách thành Windows Service chạy quyền admin cộng với giao diện không có quyền admin. Cách đó bền hơn, nhưng bản portable sẽ phải cài service và độ phức tạp tăng gấp đôi. Có thể xem lại sau giai đoạn 1.

### 4.2 Các module Go

Mỗi module làm đúng một việc. Chỉ `app` biết đến các module khác. Các module còn lại giao tiếp qua interface để có thể giả lập khi test.

| Package | Nhiệm vụ |
|---|---|
| `internal/brand` | Tên app, repo GitHub, URL danh sách server, URL API phát hành, khoá công khai ed25519 |
| `internal/app` | Bộ điều phối và máy trạng thái (mục 5). Phát sự kiện cho giao diện, gom thống kê. **Là module duy nhất được Wails bind** |
| `internal/engine` | Bọc `dnsproxy`: lắng nghe trên loopback, upstream chế độ parallel, cache, `RequestHandler` để thống kê và trả lời tên miền xác minh |
| `internal/fragdoh` | Upstream DoH tự viết theo interface `upstream.Upstream`, có cắt nhỏ ClientHello |
| `internal/scanner` | Kiểm tra server song song, đo độ trễ, phát hiện DNS bị đầu độc, lưu kết quả theo từng mạng |
| `internal/servers` | Model server, danh sách nhúng sẵn, danh sách tải về (kèm kiểm tra chữ ký), server tự thêm, đọc stamp `sdns://` |
| `internal/sysdns` | Liệt kê card mạng, chụp, đặt và khôi phục DNS, xoá cache DNS, theo dõi khi card mạng thay đổi |
| `internal/dpi` | Quản lý GoodbyeDPI: giải nén và kiểm tra hash, tạo tham số từ preset, chạy và dừng, dọn service WinDivert |
| `internal/probe` | Thử truy cập các trang mẫu và phân loại lỗi (DNS / TCP / TLS) |
| `internal/watchdog` | Chạy ở chế độ `--watchdog` và `--restore` |
| `internal/store` | Đọc và ghi `settings.json`, `state.json`, `scan-cache.json` một cách nguyên tử (ghi file tạm, fsync, rồi rename). Chọn thư mục dữ liệu cho bản cài đặt hoặc portable |
| `internal/startup` | Tạo và xoá tác vụ Task Scheduler (`Ghostline`, `Ghostline Recovery`) bằng `schtasks /Create /XML` |
| `internal/updater` | Gọi `releases/latest` tối đa 1 lần/ngày và so phiên bản theo semver |
| `internal/winutil` | Hàm hỗ trợ Win32: tìm process giữ một cổng, Job Object, kiểm tra quyền admin |

### 4.3 Thư mục dữ liệu

- **Bản cài đặt:** `%APPDATA%\Ghostline\`
- **Bản portable** (có file `portable` nằm cạnh exe): `<thư mục exe>\data\`

```
settings.json          cài đặt người dùng
state.json             trạng thái kết nối + bản chụp DNS (mục 5.4)
scan-cache.json        kết quả quét, lưu theo từng mạng
servers-remote.json    danh sách tải về (+ servers-remote.json.sig)
servers-custom.json    server người dùng tự thêm
logs/ghostline.log     xoay vòng 3 × 5 MB, tiếng Anh, KHÔNG ghi tên miền
bin/goodbyedpi/        goodbyedpi.exe, WinDivert.dll, WinDivert64.sys
```

## 5. Luồng Connect, Disconnect và an toàn DNS

### 5.1 Máy trạng thái

```
NGẮT → ĐANG_KẾT_NỐI → ĐÃ_BẢO_VỆ ⇄ SUY_GIẢM → ĐANG_NGẮT → NGẮT
                 └──(lỗi)──→ LỖI → (hoàn tác) → NGẮT
```

Trong lúc ĐANG_KẾT_NỐI, bấm nút lần nữa sẽ **huỷ** và hoàn tác như trường hợp lỗi.

### 5.2 Connect

Mỗi bước có cặp `Do` / `Undo`. Nếu bước N lỗi, app gọi `Undo` của các bước N−1…1 theo thứ tự ngược lại. Nếu chính `Undo` lỗi thì ghi log và vẫn hoàn tác tiếp.

| # | Bước | Chi tiết | Undo |
|---|---|---|---|
| 01 | Kiểm tra trước | Có quyền admin. Cổng 53 UDP và TCP trên `127.0.0.1` (và `::1` nếu có IPv6) còn trống. `state.json` đang ở trạng thái `clean`, nếu không thì chạy khôi phục trước | — |
| 02 | Chọn server | Xem mục 6.3: dùng kết quả quét còn hạn, không có thì quét nhanh. Lấy `maxUpstreams` (5) server nhanh nhất. Nếu bật "chỉ dùng máy chủ đã ghim" thì chỉ kiểm tra các server đã ghim | — |
| 03 | Bật engine | Lắng nghe `127.0.0.1:53` và `[::1]:53`. Gửi thử một truy vấn vào chính nó qua loopback, phải có câu trả lời trong 3 giây | Tắt engine |
| 04 | Chụp DNS gốc | Ghi `state.json` với `phase=dns_set`, bản chụp DNS từng card mạng và pid, **trước khi** đổi bất cứ thứ gì | Đặt `phase=clean` |
| 05 | Bật lưới an toàn | Chạy watchdog. Tạo tác vụ `Ghostline Recovery` | Kill watchdog, xoá tác vụ |
| 06 | Đặt DNS | Đặt `127.0.0.1` (v4) và `::1` (v6) cho các card mạng đã chọn, rồi xoá cache DNS của Windows | Khôi phục theo bản chụp |
| 07 | Xác minh | Phân giải `<nonce>.verify.ghostline.test` **thông qua Windows** (GetAddrInfoW). Engine trả lời tên này ngay tại chỗ bằng `192.0.2.1`. Đạt khi kết quả đúng **và** engine ghi nhận đúng nonce đó. Không đạt thì báo lỗi `VERIFY_LEAK` | — |

Sau bước 07: trạng thái chuyển sang **ĐÃ_BẢO_VỆ**, rồi chạy kiểm tra trang mẫu ở nền (mục 7.3).

### 5.3 Disconnect

Disconnect được kích hoạt khi bấm nút, khi chọn Thoát trong menu khay, hoặc khi Windows tắt máy hay đăng xuất (`WM_QUERYENDSESSION`). Các bước:

1. **Trả DNS về** theo bản chụp, rồi xoá cache DNS.
2. Dừng GoodbyeDPI nếu đang chạy (mục 7.1).
3. Tắt engine.
4. Đặt `state.json` thành `phase=clean`, kill watchdog, xoá tác vụ `Ghostline Recovery`.

Thứ tự này đảm bảo không lúc nào DNS của máy trỏ vào một engine đã tắt.

### 5.4 `sysdns`: chụp, đặt và khôi phục DNS

- **Chọn card mạng.** Chế độ `auto` chọn các card có `IfType` là Ethernet (6) hoặc Wi-Fi (71), đang `OperStatus=Up` và có default gateway (v4 hoặc v6). Chế độ Nâng cao cho phép chọn bằng tay theo GUID.
- **Chụp DNS:** đọc bằng `GetInterfaceDnsSettings`, riêng cho IPv4 và IPv6.
  - Danh sách NameServer rỗng nghĩa là DNS lấy từ DHCP, ghi lại thành `mode: "dhcp"`.
  - Ngược lại ghi `mode: "static"` kèm danh sách server.
- **Đặt và khôi phục:** dùng `SetInterfaceDnsSettings`.
  - Khôi phục `dhcp` nghĩa là đặt NameServer rỗng. Khôi phục `static` thì đặt lại đúng danh sách đã chụp.
  - Nếu API lỗi thì dùng `netsh interface ipv4|ipv6 set dnsservers … validate=no`.
  - Khi khôi phục lỗi: thử lại 3 lần, sau đó dùng `netsh`, cuối cùng đặt về DHCP. Nếu vẫn lỗi thì hiện cảnh báo cố định kèm nút "Khôi phục DNS ngay". **Không bao giờ im lặng bỏ qua.**
- **Xoá cache:** `DnsFlushResolverCache` (dnsapi.dll).
- **Card mạng thay đổi.** App theo dõi bằng `NotifyIpInterfaceChange`, gom sự kiện trong 2 giây rồi mới xử lý:
  - Có card mới đạt điều kiện: chụp DNS của nó (ghi vào `state.json` trước), rồi đặt DNS.
  - Card biến mất: giữ lại bản chụp. Khi ngắt kết nối, app khôi phục những card còn tồn tại và bỏ qua những card đã mất.
  - Khi máy thức dậy từ chế độ ngủ: kiểm tra engine ngay.
- **Máy tắt IPv6:** engine chỉ lắng nghe trên v4, app bỏ qua phần DNS v6 và ghi log.

`state.json` (phiên bản 1):

```json
{
  "version": 1,
  "phase": "clean | dns_set",
  "pid": 1234,
  "pidStartTime": "2026-10-04T14:02:09.512+07:00",
  "startedAt": "2026-10-04T14:02:11+07:00",
  "snapshot": [
    {
      "guid": "{…}", "luid": 1234, "alias": "Wi-Fi",
      "ipv4": { "mode": "dhcp" },
      "ipv6": { "mode": "static", "servers": ["2001:db8::1"] }
    }
  ],
  "dpi": { "running": false, "pid": 0 }
}
```

### 5.5 Bốn lớp chống mất mạng

| Lớp | Cơ chế |
|---|---|
| 1. Ngắt sạch | Disconnect, Thoát, hoặc Windows tắt máy hay đăng xuất (mục 5.3) |
| 2. Watchdog | `ghostline.exe --watchdog --parent <pid>` mở handle của process cha với quyền `SYNCHRONIZE` rồi chờ. Khi process cha kết thúc, nếu `state.json` khác `clean` thì watchdog khôi phục DNS, dừng GoodbyeDPI và WinDivert, đặt `clean`, rồi thoát. Khi ngắt sạch, app tự kill watchdog sau khi đã đặt `clean` |
| 3. Khôi phục khi mở app | Lúc khởi động, nếu `state.json` khác `clean` thì khôi phục trước mọi việc khác, và kill các `goodbyedpi.exe` còn sót do Ghostline chạy |
| 4. Tác vụ lúc đăng nhập | `Ghostline Recovery` có trigger lúc đăng nhập, `RunLevel=Highest`, chạy `--restore`. Tác vụ được tạo ở bước 05 và xoá khi ngắt sạch |

Khôi phục luôn **idempotent**: chạy nhiều lần cho cùng kết quả, không gây hại.

**Chống tranh chấp giữa các lớp.** Ví dụ: `--restore` và `--autostart` cùng chạy lúc đăng nhập, rồi `--restore` lại khôi phục DNS sau khi app vừa kết nối xong. Để tránh những tình huống như vậy:

- `state.json` lưu thêm `pidStartTime` cùng với `pid`.
- Lớp 2, 3 và 4 **chỉ khôi phục khi process chủ của `state.json` đã chết**. Process chủ được xác định bằng pid **cùng với** thời điểm khởi động, để không bị nhầm khi Windows dùng lại số pid.
- Mọi thao tác đọc rồi ghi `state.json` đều nằm trong mutex có tên `Local\Ghostline-State`.

### 5.6 Theo dõi sức khoẻ khi đang kết nối

- **Mỗi 30 giây**, app gửi một truy vấn thử qua engine.
- Nếu **tất cả** upstream lỗi liên tục quá 15 giây, trạng thái chuyển sang **SUY_GIẢM**. App quét nhanh lại rồi **đổi nóng** danh sách upstream bằng cách tắt và bật lại proxy (dưới 100 ms). Trong lúc đó DNS vẫn trỏ về loopback nên không rò ra ngoài.
- Khi lại có upstream trả lời được thì trạng thái quay về ĐÃ_BẢO_VỆ.

## 6. Server và quét server

### 6.1 Model

```json
{
  "id": "cloudflare-doh",
  "name": "Cloudflare",
  "provider": "Cloudflare",
  "protocol": "doh | dot | doq | dnscrypt",
  "address": "https://cloudflare-dns.com/dns-query | tls://… | quic://… | sdns://…",
  "ips": ["1.1.1.1", "1.0.0.1"],
  "tags": ["no-filter", "no-log", "dnssec"],
  "source": "builtin | remote | custom"
}
```

Các nhãn dùng để lọc: `no-filter`, `adblock` và `family`. Chế độ Đơn giản chỉ dùng server có nhãn `no-filter`.

### 6.2 Nguồn danh sách

- **Danh sách nhúng sẵn:** `tools/genservers` tạo `lists/servers.json` từ 2 nguồn:
  - danh sách **DNSCrypt public-resolvers v3** (dạng máy đọc được, có stamp và cờ thuộc tính);
  - file `lists/seed.json` do dự án tự chọn lọc, gồm các nhà cung cấp lớn (Cloudflare, Quad9, Google, AdGuard, Mullvad, DNS.SB, Control D) với đủ biến thể DoH/DoT/DoQ.

  Script chỉ giữ server có mã hoá, gắn nhãn dựa trên cờ thuộc tính trong stamp, và **ghi lại license của từng nguồn vào `NOTICE`**. Nguồn nào không cho phép phân phối lại thì không nhúng. Danh sách nhúng vào exe bằng `go:embed`.
- **Danh sách tải về:** `lists/servers.json` cùng `lists/servers.json.sig` trên nhánh `main` của repo, tải qua `raw.githubusercontent.com` tối đa 1 lần/ngày.
  - Chữ ký là **ed25519**. Khoá bí mật nằm trong GitHub Actions secret `SERVERLIST_SIGNING_KEY`, khoá công khai nằm trong `internal/brand`.
  - Sai chữ ký thì bỏ qua và giữ danh sách cũ.
  - Danh sách tải về được ưu tiên hơn danh sách nhúng nếu mới hơn (so trường `generatedAt`).
- **Server tự thêm:** dán URL hoặc stamp, hoặc import từ file hay URL (mỗi dòng một server). Lưu trong `servers-custom.json`, không bị bản cập nhật ghi đè.

### 6.3 Quét server

- **Kiểm tra một server:**
  - Tạo upstream bằng `upstream.AddressToUpstream`. Nếu bật Fragment DNS và giao thức là DoH thì dùng `fragdoh` thay thế. Timeout 3 giây.
  - Gửi truy vấn A cho tên miền thử (mặc định `www.google.com`) **2 lần**. Độ trễ là thời gian của lần thứ 2.
  - **Đạt** khi rcode là NOERROR, có ít nhất một bản ghi A, và **mọi** IP trả về là IP công khai. IP bị coi là không công khai nếu thuộc RFC1918, loopback, link-local, 100.64/10, 0/8, multicast, các dải reserved hoặc TEST-NET.
- **Song song:** 16 worker dùng chung một hàng đợi. Worker nào xong thì lấy ngay server tiếp theo, không chia theo lô.
- **Quét nhanh** (khi Connect):
  - Dừng khi có 5 server đạt, hoặc sau 20 giây.
  - Thứ tự quét: những server từng đạt (ở bất kỳ mạng nào) được kiểm tra trước, các server còn lại xáo trộn ngẫu nhiên.
- **Quét toàn bộ:** dùng ở trang Máy chủ của chế độ Nâng cao. Có hiển thị tiến độ và huỷ được.
- **Lưu kết quả:** trong `scan-cache.json`, hạn 24 giờ. Khoá phân biệt mạng là hash SHA-256 của (IP + MAC của default gateway).

### 6.4 Bootstrap (phân giải hostname của server DoH/DoT/DoQ)

Khi DNS của máy đang trỏ về engine, **tuyệt đối không được dùng resolver của hệ thống để bootstrap**, vì engine sẽ tự hỏi lại chính nó và tạo thành vòng lặp. Thứ tự dùng:

1. `ips` có sẵn trong mục server, hoặc IP nằm trong stamp;
2. plain DNS tới danh sách `bootstrap` trong cài đặt (mặc định `1.1.1.1:53`, `8.8.8.8:53`).

Bước 2 là truy vấn plain duy nhất được phép đi ra ngoài. Truy vấn này chỉ để lộ hostname của server DoH, không lộ trang người dùng đang vào. `genservers` cố gắng điền sẵn `ips` để rất ít khi phải dùng tới bước 2.

## 7. Vượt DPI

### 7.1 GoodbyeDPI

- **File đi kèm:** bản chính thức mới nhất của ValdikSS, **x86_64**. Phiên bản và SHA-256 của `goodbyedpi.exe`, `WinDivert.dll` và `WinDivert64.sys` được ghim trong code. Các file này nhúng vào exe và được giải nén vào `bin/goodbyedpi/`. App kiểm tra hash trước **mỗi lần** chạy.
- **Preset.** Không dùng `--dns-addr` / `--dns-port`, vì DNS đã do engine của Ghostline xử lý:

| Preset | Tham số |
|---|---|
| Nhẹ | `-p -r -s -m -e 40 -w --native-frag` |
| Vừa | Nhẹ + `--auto-ttl 1-4-10 --min-ttl 3` |
| Mạnh | Vừa + `--wrong-seq` |
| Cực mạnh | `-p -r -s -m -f 2 -e 40 -w --auto-ttl 1-4-10 --min-ttl 3 --native-frag --wrong-chksum --wrong-seq --max-payload` |
| Mode 1–6 | `-1` … `-6` |
| Tự nhập | Tham số tự do. App kiểm tra từng cờ theo danh sách cờ hợp lệ của GoodbyeDPI và từ chối cờ lạ, kể cả `--dns-*` |

- **Phạm vi:** mặc định là **mọi kết nối**. Có thể chuyển sang **danh sách đen** (`--blacklist <data>\dpi-blacklist.txt`, sửa được trong app).
- **Chạy:** process ẩn cửa sổ, nằm trong Job Object. Coi là thành công khi sau 2 giây process vẫn còn sống **và** service `WinDivert` đang chạy. Nếu process thoát ngay mà file exe hoặc driver bị chặn, báo lỗi `DPI_BLOCKED_BY_AV` kèm trang trợ giúp.
- **Dừng:** kill process, rồi `sc stop WinDivert` và `sc delete WinDivert`. **Không dùng `wmic`.**

### 7.2 Tự dò cấu hình

- **Kích hoạt:** người dùng bấm "Tự dò vượt DPI" (từ banner gợi ý hoặc từ trang Vượt DPI).
- **Cách dò:** thử lần lượt **Nhẹ → Vừa → Mạnh → Cực mạnh**. Với mỗi preset:
  - chạy GoodbyeDPI;
  - chờ 2 giây;
  - kiểm tra lại các trang đang bị chặn.
- **Kết quả:**
  - Preset đầu tiên làm **tất cả** các trang đó vào được sẽ được lưu lại và giữ ở trạng thái bật.
  - Không preset nào được thì dừng GoodbyeDPI và báo "Không tìm được cấu hình phù hợp".
- **Giao diện:** có thanh tiến trình và huỷ được giữa chừng. Ước tính tổng thời gian ≤ 40 giây.

### 7.3 Kiểm tra trang mẫu (`probe`)

- **Danh sách trang mẫu** sửa được. Mặc định: `youtube.com`, `discord.com`, `telegram.org`, `x.com`.
- **Cách thử:** gửi `GET https://<trang>/` qua resolver của hệ thống, tức là đi qua engine, để giống hệt trình duyệt. Timeout 5 giây, thử 2 lần.
- **Phân loại lỗi:**

| Lỗi ở tầng | Ý nghĩa |
|---|---|
| DNS | Lỗi phía DNS. Không phải lỗi DPI |
| TCP connect | Bị chặn theo IP. Vượt DPI không giúp được, nên app không gợi ý |
| TLS handshake (reset hoặc timeout) | **Nhiều khả năng bị chặn theo SNI hoặc DPI.** Đây là loại duy nhất được tính để gợi ý |

- **Hiện banner** "N/M trang mẫu vẫn bị chặn · Tự dò vượt DPI" nếu có ít nhất 1 trang lỗi ở tầng TLS ở **cả 2** lần thử. Bấm "bỏ qua" thì banner ẩn cho tới lần Connect sau.

### 7.4 Fragment cho kết nối DNS (`fragdoh`)

- **Cách hoạt động:** upstream DoH tự viết, dùng `net/http` với `DialTLSContext` tuỳ biến. Kết nối TCP bật `TCP_NODELAY`.
- **Cách cắt bản ghi TLS chứa ClientHello:**
  - phần trước SNI gửi thành 1 mảnh;
  - phần SNI cắt thành `chunks` mảnh (mặc định 5);
  - phần còn lại gửi thành 1 mảnh;
  - giữa các mảnh nghỉ `delayMs` (mặc định 5).
- **Mặc định tắt.** Màn hình lỗi `NO_SERVERS` gợi ý bật nó.
- **Khi GoodbyeDPI đang bật,** giao diện ghi chú rằng Fragment này là thừa.

## 8. Giao diện

### 8.1 Cửa sổ và các chế độ

- **Cửa sổ:** không viền (frameless), thanh tiêu đề tự vẽ có vùng kéo, gồm nhãn `>_ GHOSTLINE`, nút đổi VI/EN, thu nhỏ và đóng.
- **Đơn giản:** 380×580, cố định kích thước.
- **Nâng cao:** 1000×660, kéo giãn được, nhỏ nhất 900×600, nhớ kích thước lần trước. Khi đổi chế độ, cửa sổ đổi kích thước nhưng giữ nguyên tâm.
- **Nút ✕** thu app xuống khay (mặc định bật, có thể tắt). Thoát hẳn bằng menu khay, khi đó app ngắt kết nối sạch trước.
- **Menu khay:** trạng thái, Kết nối/Ngắt, bật/tắt Vượt DPI, Mở Ghostline, phiên bản (và "có bản mới ↗" nếu có), Thoát. Icon khay đổi theo trạng thái.

### 8.2 Các màn hình

- **Đơn giản, 4 trạng thái:**

| Trạng thái | Nút nguồn | Nội dung khác |
|---|---|---|
| Ngắt | Màu mờ | Bảng thông số tóm tắt |
| Đang kết nối | Xanh dương | Log từng bước trực tiếp. Bấm lần nữa để huỷ |
| Đã bảo vệ | Xanh lá phát sáng | Bảng máy chủ, độ trễ, thời gian. Có thể có banner cam gợi ý vượt DPI |
| Lỗi | Đỏ | Câu "DNS của máy vẫn như cũ", nguyên nhân, các nút hành động, link mở nhật ký |

- **Nâng cao:** thanh bên gồm Tổng quan, Máy chủ, Vượt DPI, Nhật ký, Cài đặt. Phía dưới thanh bên luôn hiện trạng thái và nút nguồn nhỏ.
  - **Tổng quan:** giống chế độ Đơn giản, thêm biểu đồ độ trễ 60 giây, số truy vấn và danh sách server đang dùng.
  - **Máy chủ:** bảng có cột tên, giao thức, độ trễ, trạng thái, nhãn. Sắp xếp, lọc theo giao thức, nhãn và "chỉ đạt". Ghim server, tuỳ chọn "chỉ dùng máy chủ đã ghim". Các nút quét toàn bộ, thêm, import.
  - **Vượt DPI:** GoodbyeDPI (bật/tắt, preset, Tự dò, phạm vi, xem trước dòng lệnh), Fragment DNS, danh sách trang mẫu và kết quả kiểm tra.
  - **Nhật ký:** luồng sự kiện kiểu terminal, lọc theo nguồn (tất cả, engine, dpi, hệ thống). Có thể tạm dừng, copy, lưu ra file. Công tắc "hiện truy vấn" mặc định tắt. Khi bật, tên miền **chỉ giữ trong RAM**, tối đa 500 dòng gần nhất, không bao giờ ghi xuống đĩa.
  - **Cài đặt:** ngôn ngữ, khởi động cùng Windows, tự kết nối khi mở, đóng thì thu xuống khay, chọn card mạng, tên miền thử, danh sách bootstrap, số server tối đa, cập nhật danh sách server, báo có bản mới, nút **"Khôi phục DNS ngay"**.

### 8.3 Frontend

- **Công nghệ:** React + TypeScript + Vite.
- **Style:** CSS Modules, với design token khai báo bằng CSS variables trong `tokens.css`. Bảng màu: nền `#05070a`, xanh lá `#00ffa3`, xanh dương `#00d0ff`, cam `#ffb020`, đỏ `#ff4d6d`.
- **Font JetBrains Mono nhúng sẵn** (OFL). **Không tải bất kỳ tài nguyên nào từ mạng.**
- **State:** một store dùng Zustand, nhận dữ liệu từ sự kiện Wails (`state`, `stats`, `log`, `scan:progress`, `dpi:autotune`). Lệnh gửi xuống Go qua các binding được Wails sinh tự động.
- **Biểu đồ độ trễ:** SVG tự vẽ, không dùng thư viện biểu đồ.
- **i18n:** `react-i18next` với `vi.json` và `en.json`.
  - Go **chỉ trả về mã kèm tham số** (lỗi, sự kiện nhật ký, bước kết nối). Frontend dịch các mã đó.
  - Log ghi ra file bằng tiếng Anh.
- **Khả năng tiếp cận:** khi người dùng chọn giảm chuyển động (`prefers-reduced-motion`), vòng xoay và hiệu ứng nhấp nháy dừng lại. Mọi thao tác dùng được bằng bàn phím và có focus ring rõ ràng.

## 9. Cài đặt (`settings.json`, phiên bản 1)

```json
{
  "version": 1,
  "language": "vi",
  "mode": "simple",
  "startWithWindows": false,
  "autoConnect": false,
  "closeToTray": true,
  "adapters": "auto",
  "testDomain": "www.google.com",
  "bootstrap": ["1.1.1.1:53", "8.8.8.8:53"],
  "maxUpstreams": 5,
  "includeTags": ["no-filter"],
  "pinned": [],
  "pinnedOnly": false,
  "probeSites": ["youtube.com", "discord.com", "telegram.org", "x.com"],
  "dpi": { "enabled": false, "preset": "light", "customArgs": "", "scope": "all" },
  "fragmentDns": { "enabled": false, "chunks": 5, "delayMs": 5 },
  "updates": { "checkApp": true, "updateServerList": true },
  "advancedWindow": { "width": 1000, "height": 660 }
}
```

Nếu đọc `settings.json` bị lỗi, app đổi tên file thành `settings.json.bak`, dùng cài đặt mặc định và hiện thông báo.

## 10. Xử lý lỗi

| Mã | Khi nào | Hành động gợi ý trên giao diện |
|---|---|---|
| `NOT_ADMIN` | Không có quyền admin, thường do gọi exe trực tiếp mà bỏ qua manifest | Mở lại với quyền admin |
| `PORT53_BUSY{pid,name,service?}` | Cổng 53 trên loopback bị chiếm | Hiện tên process/service. Nếu đó là service Windows (ví dụ `SharedAccess`), có nút "Tạm dừng dịch vụ" kèm cảnh báo rõ ràng. **Không bao giờ tự ý kill hay dừng** |
| `NO_SERVERS{checked,elapsed}` | Quét nhanh không đủ server đạt | Thử lại · Bật Fragment DNS · Mở trang Máy chủ |
| `ENGINE_SELFTEST_FAILED` | Engine không tự trả lời được | Thử lại · Mở nhật ký |
| `SET_DNS_FAILED{adapter}` | Không đặt được DNS | Hoàn tác, chọn card mạng khác |
| `VERIFY_LEAK` | Bước xác minh 07 không đạt (VPN, tool DNS khác hoặc chính sách hệ thống đang chiếm DNS) | Hoàn tác, giải thích các nguyên nhân thường gặp |
| `RESTORE_FAILED{adapter}` | Khôi phục lỗi sau mọi phương án dự phòng | Cảnh báo cố định + "Khôi phục DNS ngay" + hướng dẫn làm bằng tay |
| `DPI_START_FAILED` / `DPI_BLOCKED_BY_AV` | GoodbyeDPI không chạy được hoặc bị antivirus chặn | Trang trợ giúp về WinDivert và antivirus |
| `DPI_HASH_MISMATCH` | File GoodbyeDPI bị thay đổi | Giải nén lại từ exe. Nếu vẫn sai thì không chạy |
| `SERVERLIST_BAD_SIGNATURE` | Danh sách tải về sai chữ ký | Giữ danh sách cũ, ghi log, không làm phiền người dùng |
| `UPDATE_CHECK_FAILED` | Không gọi được GitHub API | Bỏ qua, chỉ ghi log |

- **Panic** trong goroutine: app bắt lại, ghi log kèm stack trace và xử lý như engine bị lỗi (chuyển SUY_GIẢM hoặc LỖI). Crash ở cấp cao nhất thì watchdog lo phần khôi phục.

## 11. Kiểm thử

Phát triển theo TDD. Test Go chạy bằng `go test ./...`. Test cần Windows thật được gắn build tag `integration`.

| Phần | Test |
|---|---|
| `app` | Giả lập engine, sysdns, dpi, store, startup. **Test dạng bảng: lỗi ở từng bước 01–07** thì đúng các `Undo` được gọi, đúng thứ tự ngược lại. Bất biến "chụp DNS (ghi state) trước khi đặt DNS" và "trả DNS trước khi tắt engine". Huỷ giữa chừng khi đang kết nối. Chuyển SUY_GIẢM → đổi nóng upstream → ĐÃ_BẢO_VỆ |
| `sysdns` | Logic tách khỏi Win32 bằng interface `AdapterAPI`. Vòng chụp → đặt → khôi phục cho các kiểu dhcp/static × v4/v6, card mạng biến mất, khôi phục idempotent, chuỗi dự phòng khi lỗi. Có thêm `integration`: chạy vòng thật trên một card mạng |
| `engine` | Dựng server DoH giả có TLS bằng `httptest`. Engine chạy trên cổng ngẫu nhiên. Kiểm tra truy vấn thật, chế độ parallel, câu trả lời cho tên miền xác minh, thống kê |
| `fragdoh` | Server TLS giả ghi lại từng lần `Read`. Kiểm tra ClientHello đến đúng số mảnh, SNI bị cắt đúng chỗ, câu trả lời DoH giải mã đúng |
| `scanner` | Upstream giả có độ trễ, timeout, IP bị đầu độc. Kiểm tra phân loại, dừng sớm ở 5, giới hạn 16 worker, budget 20 giây, thứ tự ưu tiên, khoá cache theo mạng |
| `servers` | Đọc stamp DoH/DoT/DoQ/DNSCrypt, gộp các nguồn, chữ ký đúng/sai/file bị sửa, server tự thêm không bị ghi đè |
| `dpi` | Preset → tham số, kiểm tra tham số tự nhập (từ chối `--dns-*` và cờ lạ), kiểm tra hash, quản lý process dùng exe giả |
| `probe` | Phân loại lỗi DNS/TCP/TLS bằng các server giả |
| `watchdog` | Process cha giả thoát khi `state` khác `clean` → khôi phục. `clean` → không làm gì. Process chủ còn sống, hoặc pid trùng nhưng thời điểm khởi động khác → không khôi phục nhầm |
| `store` | Ghi nguyên tử, xử lý file hỏng, chọn thư mục portable hay cài đặt |
| Frontend | Vitest + React Testing Library: hiển thị 4 trạng thái, banner, bảng máy chủ. **Test `vi.json` và `en.json` có cùng bộ khoá** |
| Kiểm tra thủ công trước phát hành | Connect/Disconnect. `taskkill /F` → khôi phục trong ≤ 3 giây. Khởi động lại máy khi đang kết nối → khôi phục lúc đăng nhập. Cổng 53 bị ICS chiếm. Đổi Wi-Fi sang dây mạng. Máy ngủ rồi thức. Wireshark: không có UDP/TCP 53 đi ra ngoài trừ bootstrap. Installer: cài và gỡ (gỡ phải khôi phục DNS và xoá tác vụ) |

## 12. Đóng gói, phát hành và cập nhật

- **Build:** `wails3` + `Taskfile.yml`. Exe khai báo manifest `requireAdministrator`. **Không dùng UPX.**
- **Installer NSIS** (dùng template của Wails v3):
  - cài vào `Program Files\Ghostline`;
  - tự cài WebView2 Runtime nếu thiếu;
  - tạo shortcut ở Start Menu;
  - khi gỡ: chạy `ghostline.exe --restore`, xoá tác vụ `Ghostline` và `Ghostline Recovery`, dừng và xoá service `WinDivert`.
- **Portable:** `Ghostline-<ver>-portable.zip` chứa `ghostline.exe` và file `portable`. Nếu máy thiếu WebView2 thì hiện hướng dẫn cài.
- **CI** (`.github/workflows/`, chạy trên `windows-latest`):
  - `ci.yml`, chạy cho mỗi push và PR: `golangci-lint`, `go test ./...`, `npm test`, build thử.
  - `release.yml`, chạy khi đẩy tag `v*`: chạy `genservers`, ký `servers.json`, build installer và zip, tạo `SHA256SUMS`, tạo GitHub Release.
- **Ký số file exe:** chưa làm ở giai đoạn 1. README ghi rõ là SmartScreen sẽ cảnh báo và hướng dẫn kiểm tra SHA-256. Sau này sẽ xin SignPath Foundation (ký miễn phí cho mã nguồn mở).
- **Kiểm tra bản mới:** gọi `GET https://api.github.com/repos/hashcott/ghostline/releases/latest` tối đa 1 lần/ngày, so semver. Có bản mới thì báo trên giao diện và trong menu khay, kèm link tới trang Release.
- **License và ghi công:** `LICENSE` (MIT). `NOTICE` liệt kê các thành phần bên thứ ba: dnsproxy (Apache-2.0), GoodbyeDPI (Apache-2.0), WinDivert (LGPLv3), JetBrains Mono (OFL-1.1) và license của các nguồn danh sách server.

## 13. Cấu trúc repo

```
/                         module github.com/hashcott/ghostline
  main.go                 chọn cách chạy: ui | --autostart | --watchdog | --restore
  Taskfile.yml
  build/                  icon, manifest, cấu hình NSIS
  internal/
    brand/ app/ engine/ fragdoh/ scanner/ servers/ sysdns/
    dpi/ probe/ watchdog/ store/ startup/ updater/ winutil/
  assets/goodbyedpi/      exe, dll, sys + SHA256 (nhúng bằng go:embed)
  lists/                  seed.json, servers.json, servers.json.sig
  tools/genservers/
  frontend/
    src/
      app/                store, cầu nối sự kiện Wails
      modes/simple/
      modes/advanced/pages/{overview,servers,dpi,logs,settings}/
      components/neon/    PowerButton, TerminalPanel, Toggle, Table, Banner, Sparkline…
      i18n/{vi,en}.json
      styles/tokens.css
  docs/
  .github/workflows/{ci,release}.yml
  LICENSE  NOTICE  README.md
```

## 14. Rủi ro và cách giảm thiểu

| Rủi ro | Giảm thiểu |
|---|---|
| Wails v3 vẫn đang beta | Ghim đúng một phiên bản. Chỉ `main.go` và lớp bind trong `internal/app` phụ thuộc vào Wails. Nếu buộc phải quay về Wails v2 thì chỉ phải viết lại lớp bind và phần icon khay |
| Antivirus báo nhầm (WinDivert, đổi DNS, exe chưa ký) | Không dùng UPX. Có `SHA256SUMS`. Có trang trợ giúp. Gửi mẫu cho Microsoft phân tích. Sau này ký số qua SignPath |
| Cổng 53 bị chiếm (ICS, Hyper-V, Docker, tool DNS khác) | Phát hiện và cho biết process hoặc service đang giữ cổng. Chỉ dừng service khi người dùng đồng ý rõ ràng |
| Bootstrap tự hỏi lại chính engine | Không bao giờ dùng resolver hệ thống để bootstrap (mục 6.4) |
| VPN hoặc phần mềm khác chiếm DNS | Bước xác minh 07 phát hiện được, báo `VERIFY_LEAK` và hoàn tác |
| GoodbyeDPI làm hỏng một số trang hoặc game | Mặc định tắt. Tự dò chọn preset nhẹ nhất có tác dụng. Có chế độ danh sách đen. Bật/tắt nhanh từ khay |
| Dùng UAC bằng tài khoản admin khác ("over-the-shoulder") khiến `%APPDATA%` là của tài khoản admin | Chấp nhận ở giai đoạn 1 và ghi vào README. Dữ liệu vẫn nhất quán vì mọi process của Ghostline đều chạy dưới cùng một tài khoản đã nâng quyền |
| Máy tắt IPv6 | Engine chỉ lắng nghe trên v4 và bỏ qua phần DNS v6 (mục 5.4) |
