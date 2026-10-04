# Ghostline — Giai đoạn 2A: Thiết kế

- **Ngày:** 2026-10-04
- **Trạng thái:** Chờ duyệt spec
- **Phạm vi:** Giai đoạn 2A — proxy HTTP/HTTPS/SOCKS (dùng cho máy này và chia sẻ LAN), Fragment cho lưu lượng web, rules theo domain/CIDR và danh sách cộng đồng. Giai đoạn 2B (DoH server cục bộ, Fake SNI, chứng chỉ gốc) có spec riêng.
- **Dựa trên:** [Giai đoạn 1](2026-10-04-ghostline-phase1-design.md). Mọi thứ không nhắc lại ở đây giữ nguyên như giai đoạn 1.

---

## 1. Mục tiêu

Mở rộng Ghostline từ "chỉ DNS mã hoá" thành công cụ vượt chặn đầy đủ hơn, theo hướng tương đương DNSveil, mà **không cần driver** (WinDivert) và **không cần cài chứng chỉ gốc**:

- **Proxy cục bộ** HTTP / HTTPS-CONNECT / SOCKS4/4a/5 trên một cổng duy nhất, dùng được cho trình duyệt trên máy này (qua System Proxy của Windows) và cho thiết bị khác trong LAN.
- **Fragment cho lưu lượng web**: cắt ClientHello của kết nối TLS đi qua proxy để DPI không đọc được SNI, tự bật khi phát hiện bị chặn.
- **Rules**: chặn, cho phép, DNS giả, bật/tắt fragment, đẩy qua upstream proxy — theo domain, keyword, regexp, CIDR. Nhập bằng bảng hoặc text, import file và tự tải danh sách cộng đồng từ GitHub theo lịch.

### Tiêu chí thành công

1. Bật proxy + "Dùng cho máy này", Connect: trình duyệt trên máy mở được một trang **bị chặn theo SNI** (GoodbyeDPI tắt) mà không cấu hình gì thêm. Lần đầu vào trang chậm thêm tối đa 3 giây, các lần sau không chậm thêm.
2. **Proxy không bao giờ gây rò DNS plain.** Mọi tên miền proxy phân giải đều đi qua DNS engine mã hoá, hoặc được chuyển nguyên cho upstream proxy. Kiểm tra bằng test (resolver giả có bẫy) và Wireshark lúc phát hành.
3. **App crash hoặc bị kill khi đang dùng system proxy thì system proxy và luật firewall được trả về trong ≤ 3 giây**, cùng cơ chế với DNS. Mất điện thì trả về ở lần đăng nhập kế tiếp.
4. Điện thoại trong cùng mạng Private dùng được proxy bằng cách quét mã QR hoặc nhập địa chỉ hiển thị trong app.
5. Import được danh sách từ link GitHub của các repo phổ biến (StevenBlack, hagezi, OISD, AdGuard DNS filter, hostsVN, blackmatrix7, v2fly) mà không phải chọn định dạng bằng tay.
6. Người dùng giai đoạn 1 nâng cấp lên **không thấy hành vi nào thay đổi** cho tới khi tự bật tính năng mới. Mọi chuỗi giao diện mới có đủ tiếng Việt và tiếng Anh.

## 2. Phạm vi

### Có trong giai đoạn 2A

| Nhóm | Nội dung |
|---|---|
| Proxy | HTTP CONNECT, HTTP forward, SOCKS4/4a/5 trên một cổng. Chạy cùng Connect. Tuỳ chọn System Proxy (chụp và khôi phục an toàn). Chia sẻ LAN chỉ cho mạng riêng, kèm luật firewall và mã QR. Upstream proxy SOCKS5/HTTP |
| Fragment web | Tự động khi bị chặn (có ghi nhớ theo mạng), luôn bật, hoặc tắt. Hai kiểu cắt: TCP quanh SNI và nhiều bản ghi TLS, hoặc kết hợp |
| Rules | Hành động `block`, `allow`, `ip=`, `fragment=`, `upstream=`. Mẫu khớp domain, `=domain`, `*.domain`, keyword, regexp, CIDR. Bảng ↔ text. Danh sách từ file hoặc URL, tải theo lịch, đọc nhiều định dạng cộng đồng |

### Không có trong giai đoạn 2A

- **Giai đoạn 2B:** DoH server cục bộ, **Fake SNI** (giải mã TLS), tạo/cài/gỡ chứng chỉ gốc. Fake SNI là **tính năng nâng cao**: mặc định tắt, chỉ có ở chế độ Nâng cao, có trang cảnh báo rủi ro bắt buộc đọc và xác nhận, banner cảnh báo cố định khi đang bật, nút gỡ chứng chỉ, tự gỡ khi gỡ cài đặt. Rules của 2A chừa sẵn trường `sni=` cho việc này (mục 7.1).
- **Proxy độc lập** (chạy khi không Connect) và **chế độ "chỉ proxy"** (không đổi DNS card mạng): bỏ, vì proxy độc lập buộc phải dùng DNS hệ thống. Xem lại khi có nhu cầu thật.
- Mật khẩu cho proxy cục bộ, danh sách IP cho phép, SOCKS `BIND` / `UDP ASSOCIATE`, fragment cho HTTP plain.
- File danh sách nhị phân (`geosite.dat`, `.srs`, `.mrs`).

## 3. Các quyết định đã chốt

| Chủ đề | Quyết định |
|---|---|
| Cách làm proxy | **Tự viết** bằng Go (`internal/proxy`), không dùng `go-socks5` / `goproxy` / sing-box. Lý do: logic "thử lại có fragment" phải nằm giữa dòng dữ liệu, thư viện chỉ cho hook ở bước dial; phần đọc giao thức nhỏ và ổn định; một đường đi chung cho rules, thống kê và bảo đảm DNS. Bù lại bằng fuzz test, giới hạn cứng và chống SSRF (mục 5) |
| Quan hệ với Connect | Proxy là **pha phụ chạy sau khi DNS đã được bảo vệ**. Connect bật tất cả, Disconnect tắt tất cả. Pha proxy lỗi → SUY_GIẢM, DNS vẫn được bảo vệ |
| Phân giải tên miền | Luôn qua `engine.Resolve` (trong process), hoặc chuyển nguyên cho upstream proxy. **Không bao giờ dùng DNS hệ thống** |
| System Proxy | Tuỳ chọn "Dùng cho máy này". Chụp vào `state.json` trước khi đổi, khôi phục ở cả 4 lớp an toàn. Không giành lại khi app khác đổi |
| Chia sẻ LAN | Tắt mặc định. Bật thì nghe mọi địa chỉ nhưng chỉ nhận IP riêng; luật firewall chỉ profile Private. Không mật khẩu |
| Fragment web | Mặc định `auto`: đi thẳng, bị reset/timeout trước byte đầu tiên của server thì thử lại có fragment và ghi nhớ domain theo mạng. Kiểu cắt mặc định `both` |
| Rules | Rule viết tay xét trước, rule đầu tiên khớp được dùng; sau đó tới các danh sách theo thứ tự. Một bộ khớp dùng chung cho engine và proxy |
| Danh sách | Tự nhận diện 10 họ định dạng cộng đồng. Link GitHub tự chuyển sang raw, dự phòng jsDelivr |
| Cổng mặc định | `8080`, đổi được |

## 4. Kiến trúc

### 4.1 Package mới

| Package | Nhiệm vụ |
|---|---|
| `internal/proxy` | Listener một cổng, đoán giao thức theo byte đầu (`0x04` SOCKS4/4a, `0x05` SOCKS5, còn lại HTTP). Đọc request, kiểm soát nguồn và giới hạn kết nối, gọi `Dialer`, chuyển dữ liệu hai chiều, thống kê |
| `internal/proxy/dialer` | Đường đi chung của mọi kết nối: rules → phân giải → chống SSRF → kết nối thẳng hoặc qua upstream → fragment, kèm logic thử lại có fragment và `frag-cache` |
| `internal/tlsfrag` | `SplitClientHello` (chuyển từ `fragdoh`), thêm kiểu cắt bản ghi TLS và kiểu kết hợp. `IsClientHello`, `SNI`. `fragdoh` chuyển sang dùng package này |
| `internal/rules` | Model rule, đọc/ghi định dạng text, bộ khớp đã biên dịch, đọc các định dạng danh sách cộng đồng, chuẩn hoá link GitHub, tải danh sách theo lịch |
| `internal/sysproxy` | Chụp, đặt, khôi phục và theo dõi System Proxy (WinINET per-connection options). Logic tách khỏi Win32 bằng interface `API` |
| `internal/qr` | Sinh ma trận mã QR (byte mode, mức sửa lỗi M) cho chuỗi địa chỉ proxy; frontend vẽ bằng SVG. Viết tay, không dependency |

### 4.2 Thay đổi ở package cũ

| Package | Thay đổi |
|---|---|
| `engine` | Thêm `Resolve(ctx, host) ([]netip.Addr, error)` gọi thẳng vào upstream đang dùng (qua cache của dnsproxy), không đi qua Windows. `RequestHandler` áp dụng `block` / `allow` / `ip=` từ bộ khớp hiện tại |
| `app` | Pha proxy P1–P4 (mục 6.1), lý do SUY_GIẢM dạng tập hợp, thứ tự Disconnect mới, chạy lại pha P khi đổi cài đặt, binding cho trang Proxy và Rules, lập lịch tải danh sách |
| `store` | `settings.json` v2 (khối `proxy`, `dnsBlockMode`), `rules.json`, `frag-cache.json`, thư mục `lists/`. `state.json` v2 (`sysproxy`, `firewall`) |
| `watchdog` | Khôi phục system proxy và xoá luật firewall trước khi khôi phục DNS |
| `winutil` | Thêm/xoá luật firewall bằng `netsh advfirewall firewall`, đọc profile mạng hiện tại, liệt kê IP LAN, DPAPI (`CryptProtectData`) cho mật khẩu upstream |
| `fragdoh` | Dùng `tlsfrag` thay cho code cắt riêng |

### 4.3 Ranh giới

- `proxy` không biết `engine`, `rules` hay `store`. Nó nhận các interface:
  ```go
  type Resolver interface { Resolve(ctx context.Context, host string) ([]netip.Addr, error) }
  type Matcher  interface { Match(host string, ip netip.Addr) rules.Decision }
  type FragCache interface { Has(host string) bool; Add(host string) }
  ```
- `rules` không phụ thuộc package nào khác của Ghostline.
- `app` vẫn là nơi duy nhất nối các module và là module duy nhất được Wails bind.
- Bộ khớp hiện hành nằm trong một `atomic.Pointer[rules.Compiled]` do `app` giữ; engine và proxy cùng đọc từ đó nên kết quả luôn nhất quán.

### 4.4 Thư mục dữ liệu (thêm)

```
rules.json             rule viết tay + metadata danh sách
frag-cache.json        domain cần fragment, theo từng mạng
lists/<id>.txt         bản cache nội dung danh sách đã tải
```

## 5. Luồng một kết nối qua proxy

```
client ─► 1 Nhận ─► 2 Bắt tay ─► 3 Rules ─► 4 Phân giải ─► 5 Chống SSRF ─► 6 Kết nối + Fragment ─► 7 Chuyển dữ liệu
```

### 5.1 Nhận kết nối

- Luôn nhận từ loopback. Khi bật Chia sẻ LAN thì nhận thêm `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, `169.254.0.0/16`, `fc00::/7`, `fe80::/10`. Nguồn khác bị đóng ngay, không trả gì.
- Tối đa **64** kết nối đồng thời trên mỗi IP nguồn và **1024** kết nối tổng; vượt thì đóng kết nối mới.

### 5.2 Bắt tay

- Timeout bắt tay **10 giây**, header tối đa **8 KB**.
- Chấp nhận:
  - SOCKS4 `CONNECT`, SOCKS4a (tên miền), SOCKS5 `CONNECT` với phương thức `NO AUTH`, địa chỉ IPv4/IPv6/tên miền.
  - HTTP `CONNECT host:port`.
  - HTTP forward (`GET http://host/…`): proxy viết lại request-line sang dạng đường dẫn, bỏ các header hop-by-hop (`Proxy-Connection`, `Proxy-Authorization`…), giữ keep-alive cho cùng host; host khác trên cùng kết nối thì mở kết nối upstream mới.
- Từ chối kèm mã lỗi đúng chuẩn: SOCKS4 `0x5B`, SOCKS5 `0x07` (lệnh không hỗ trợ) / `0x08` (loại địa chỉ) / `0x02` (rule chặn), HTTP `405` / `400` / `403`.
- Kết quả: đích `host:port` (host là tên miền hoặc IP) và loại client (loopback hay LAN).

### 5.3 Rules

Gọi `Matcher.Match(host, ip)` (mục 7.4). Kết quả là một `Decision`: `block`, `allow`, `ip`, `fragment` (`auto|on|off|unset`), `upstream`, cùng nguồn khớp (rule số mấy hoặc danh sách nào) để hiển thị và thống kê.

- Domain được khớp trước. Nếu đích là IP, hoặc sau khi phân giải ra IP, khớp tiếp các mẫu CIDR **chỉ cho các trường chưa được đặt**.
- `block` → từ chối ngay.

### 5.4 Phân giải

- Có `ip=` → dùng IP đó.
- Có `upstream=` → **không phân giải tại máy**, chuyển nguyên tên miền cho upstream.
- Còn lại → `Resolver.Resolve`. Thử các địa chỉ IPv4 trước, nếu không kết nối được trong 3 giây thì thử IPv6.
- `Resolver` lỗi → đóng kết nối với mã lỗi "host unreachable". **Không có đường dự phòng nào qua DNS hệ thống.**

### 5.5 Chống SSRF và vòng lặp

- Client từ LAN: chặn đích thuộc loopback, `0.0.0.0/8`, `169.254.0.0/16`, `fe80::/10`, và mọi IP của chính máy này.
- Mọi client: chặn đích trùng cổng proxy hoặc cổng 53 của engine trên địa chỉ của máy này.

### 5.6 Kết nối và Fragment

1. Mở kết nối TCP tới đích (hoặc qua upstream: SOCKS5 `CONNECT` với tên miền, hoặc HTTP `CONNECT`). Bật `TCP_NODELAY`.
2. Với HTTP CONNECT trả `200 Connection established`; với SOCKS trả thành công. Từ đây là luồng byte trong suốt.
3. Đọc bản ghi TLS đầu tiên từ client (chờ tối đa 5 giây, tối đa 16 KB). Nếu không phải ClientHello thì chuyển thẳng sang bước 7, không fragment.
4. Quyết định fragment: rule (`on`/`off`) được ưu tiên; nếu rule không đặt thì dùng cài đặt chung `proxy.fragment.mode`:
   - `never` → gửi nguyên bản.
   - `always` → gửi bản đã cắt.
   - `auto` → nếu domain (SNI, hoặc host của request) có trong `frag-cache` của mạng hiện tại thì gửi bản đã cắt; nếu không thì **thử**:
     1. Gửi nguyên bản, chờ byte đầu tiên từ server tối đa `autoTimeoutMs` (mặc định **3000**).
     2. Có byte → thành công, chuyển dữ liệu (byte đã đọc được chuyển cho client trước).
     3. Bị reset, EOF hoặc hết giờ mà chưa có byte nào → đóng, **mở kết nối mới**, gửi bản đã cắt. Thành công thì `FragCache.Add(domain)`.
     4. Bản đã cắt cũng thất bại → đóng kết nối client, tăng bộ đếm `blockedEvenFragmented`. Giao diện gợi ý bật GoodbyeDPI.
   - Fragment qua upstream proxy vẫn áp dụng (cắt trên luồng tới upstream); mặc định rule `upstream=` không kèm `fragment` thì coi là `off`, vì upstream thường đã ở ngoài vùng bị chặn.
5. Kiểu cắt (`proxy.fragment.method`):
   - `tcp`: như `fragdoh` hiện có — phần trước SNI 1 mảnh, SNI cắt `chunks` mảnh, phần còn lại 1 mảnh, nghỉ `delayMs` giữa các mảnh.
   - `record`: cắt ClientHello thành nhiều bản ghi TLS (mỗi bản ghi có header riêng, độ dài hợp lệ), điểm cắt đầu tiên rơi vào giữa SNI, tổng `chunks` bản ghi; gửi trong một lần ghi.
   - `both` (mặc định): cắt `record` trước, rồi gửi mỗi bản ghi thành một segment TCP riêng có nghỉ `delayMs`.

### 5.7 Chuyển dữ liệu

- Copy hai chiều, hỗ trợ đóng nửa chiều (`CloseWrite`).
- Không có dữ liệu theo cả hai chiều **5 phút** → đóng.
- Khi pha proxy dừng: đóng listener, chờ tối đa 2 giây để kết nối tự kết thúc rồi đóng hết.

### 5.8 Thống kê và quyền riêng tư

- Thống kê (gửi lên giao diện mỗi giây qua sự kiện `proxy:stats`): số kết nối đang mở, tổng byte vào/ra, số lượt theo hành động (`direct`, `fragmented`, `blocked`, `upstream`, `blockedEvenFragmented`), số client LAN đang dùng.
- Danh sách 500 kết nối gần nhất (thời điểm, client, đích, hành động, nguồn rule) **chỉ giữ trong RAM**, chỉ gửi lên giao diện khi bật "hiện truy vấn".
- File log **không bao giờ ghi tên miền hoặc IP đích**, chỉ ghi bộ đếm và lỗi theo mã.

### 5.9 Upstream proxy

- Khai báo trong `settings.proxy.upstreams`: `id` (chữ thường, số, `-`), `type` (`socks5` | `http`), `addr` (`host:port`), `user`, `passEnc`.
- Mật khẩu mã hoá bằng **DPAPI** (phạm vi người dùng hiện tại) rồi base64 vào `passEnc`. Không giải mã được (ví dụ chép settings sang máy khác) → upstream báo `UPSTREAM_PROXY_FAILED` và yêu cầu nhập lại.
- `addr` là tên miền thì phân giải qua `engine.Resolve`.
- Rule tham chiếu `upstream=<id>` không tồn tại → rule đó bị từ chối khi lưu (`RULES_PARSE`); danh sách tham chiếu id không tồn tại → danh sách bị tắt kèm ghi chú.
- Nút "Kiểm tra" trên giao diện: mở `CONNECT` tới `www.google.com:443` qua upstream, đạt khi bắt tay TLS xong trong 5 giây.

## 6. Tích hợp Connect/Disconnect và an toàn hệ thống

### 6.1 Pha proxy

Saga DNS 01→07 của giai đoạn 1 **giữ nguyên**. Khi đạt bước 07 trạng thái là ĐÃ_BẢO_VỆ; nếu `proxy.enabled`, app chạy tiếp pha P (dùng lại `runSteps`):

| # | Bước | Chi tiết | Undo |
|---|---|---|---|
| P1 | Mở proxy | Lắng nghe `127.0.0.1:<port>` (và `[::1]`), hoặc `0.0.0.0` + `[::]` khi chia sẻ LAN. Tự kiểm tra: mở listener echo tạm trên loopback, cho proxy `CONNECT` tới nó, đạt khi echo đúng trong 2 giây | Tắt proxy |
| P2 | Chụp system proxy | Chỉ khi `systemProxy`. Đọc 4 giá trị per-connection và ghi vào `state.json` (`sysproxy.snapshot`, `set=false`) **trước khi** đổi. Nếu máy đang có proxy hoặc PAC khác → hỏi xác nhận (`SYSPROXY_EXISTING`); người dùng từ chối thì bỏ qua P2 và P4, pha P vẫn thành công | Xoá `sysproxy` khỏi `state.json` |
| P3 | Luật firewall | Chỉ khi `shareLan`. Ghi `firewall.rule` vào `state.json` trước, rồi tạo luật `Ghostline Proxy` (mục 6.4) | Xoá luật, xoá `firewall` khỏi `state.json` |
| P4 | Đặt system proxy | Đặt `PROXY_SERVER=127.0.0.1:<port>`, `PROXY_BYPASS=<local>;localhost;127.*;10.*;172.16.*;…;172.31.*;192.168.*;[::1]`, `FLAGS=DIRECT|PROXY`, gửi `INTERNET_OPTION_SETTINGS_CHANGED` và `INTERNET_OPTION_REFRESH`, **đọc lại để xác nhận**. Ghi `sysproxy.set=true`, `sysproxy.ours` | Khôi phục bản chụp |

- **Pha P lỗi** → hoàn tác các bước P đã làm, giữ nguyên DNS, chuyển sang **SUY_GIẢM** với lý do tương ứng (mục 9) và nút "Thử lại proxy".
- **SUY_GIẢM có nhiều lý do**: `reasons` là một tập (`upstreams`, `proxy`). Trạng thái chỉ quay lại ĐÃ_BẢO_VỆ khi tập rỗng. Giao diện hiện lý do cụ thể.
- **Đổi cài đặt proxy khi đang kết nối** (bật/tắt, cổng, system proxy, chia sẻ LAN): hoàn tác pha P rồi chạy lại. Đổi rules, fragment hoặc upstream thì **không** chạy lại pha P (đọc nóng).
- **Health check** (mục 5.6 giai đoạn 1) thêm: mỗi 30 giây kiểm tra listener còn sống; nếu chết thì chạy lại pha P một lần, không được thì SUY_GIẢM.

### 6.2 Disconnect (thứ tự mới)

1. Khôi phục system proxy (nếu `sysproxy.set` và chưa bị thay — mục 6.3).
2. Xoá luật firewall.
3. Tắt proxy.
4. Trả DNS về, xoá cache DNS.
5. Dừng GoodbyeDPI.
6. Tắt engine.
7. Đặt `state.json` thành `clean`, kill watchdog, xoá tác vụ `Ghostline Recovery`.

Bất biến: không lúc nào system proxy trỏ vào proxy đã tắt, và không lúc nào DNS trỏ vào engine đã tắt.

### 6.3 System proxy

- **Bản chụp** gồm `INTERNET_PER_CONN_FLAGS`, `INTERNET_PER_CONN_PROXY_SERVER`, `INTERNET_PER_CONN_PROXY_BYPASS`, `INTERNET_PER_CONN_AUTOCONFIG_URL` của kết nối mặc định (LAN, `pszConnection = NULL`), đọc và ghi bằng `InternetQueryOptionW` / `InternetSetOptionW` với `INTERNET_OPTION_PER_CONNECTION_OPTION`.
- **Theo dõi** bằng `RegNotifyChangeKeyValue` trên `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings` (và `Connections`). Khi có thay đổi, đọc lại; nếu `PROXY_SERVER` hoặc `FLAGS` không còn là của Ghostline thì:
  - đánh dấu `sysproxy.takenOver=true` trong `state.json`,
  - phát thông báo `SYSPROXY_TAKEN_OVER`,
  - **không** giành lại, và khi ngắt **không** khôi phục đè lên.
- **Khôi phục** chỉ khi giá trị hiện tại vẫn đúng là `sysproxy.ours`. Nguyên tắc này giống bản sửa `15a8852` cho card mạng.
- **Hạn chế đã biết:** UAC bằng tài khoản admin khác thì `HKCU` là của tài khoản đó, system proxy không áp dụng cho người dùng đang đăng nhập. Ghi vào README cùng hạn chế `%APPDATA%` của giai đoạn 1. Trình duyệt tự cấu hình proxy riêng (Firefox với cài đặt riêng) không bị ảnh hưởng.

### 6.4 Firewall

- Tạo: `netsh advfirewall firewall add rule name="Ghostline Proxy" dir=in action=allow protocol=TCP localport=<port> program="<exe>" profile=private remoteip=localsubnet`.
- Xoá: `netsh advfirewall firewall delete rule name="Ghostline Proxy"` — idempotent, không lỗi khi không có luật.
- Nếu profile mạng hiện tại là **Public**: giao diện hiện gợi ý "Mạng đang là Public, thiết bị khác không vào được. Đổi sang Private trong Cài đặt Windows để chia sẻ". **Không** tự đổi profile mạng.

### 6.5 Khôi phục khi crash

- Watchdog, khôi phục lúc mở app, và `--restore` chạy theo thứ tự: **system proxy** (chỉ khi `set && !takenOver` và giá trị hiện tại vẫn là `ours`) → **xoá luật firewall** (nếu `firewall.rule`) → DNS → GoodbyeDPI → `clean`.
- Vẫn **idempotent** và chỉ chạy khi process chủ của `state.json` đã chết (pid + thời điểm khởi động), như giai đoạn 1.
- Trình gỡ cài đặt NSIS đã gọi `--restore`, nên system proxy và firewall được dọn theo; thêm một lệnh xoá luật `Ghostline Proxy` dự phòng.

### 6.6 `state.json` phiên bản 2

```json
{
  "version": 2,
  "phase": "clean | dns_set",
  "pid": 1234,
  "pidStartTime": "…",
  "startedAt": "…",
  "snapshot": [ … ],
  "dpi": { "running": false, "pid": 0 },
  "sysproxy": {
    "set": true,
    "takenOver": false,
    "ours": "127.0.0.1:8080",
    "snapshot": { "flags": 1, "server": "", "bypass": "", "autoconfigUrl": "" }
  },
  "firewall": { "rule": "Ghostline Proxy" }
}
```

- File version 1 đọc lên: `sysproxy` và `firewall` rỗng, nghĩa là không có gì cần khôi phục. Ghi lại luôn dùng version 2.
- `sysproxy` và `firewall` được ghi khi `phase` đã là `dns_set` (pha P luôn chạy sau bước 04), nên mọi lớp khôi phục đều thấy chúng.

## 7. Rules

### 7.1 Định dạng text

Mỗi dòng một rule: `<mẫu> <hành động…>`, `#` bắt đầu chú thích, dòng trống bỏ qua.

```
youtube.com          fragment=on        # youtube.com và mọi subdomain
=example.com         block              # chỉ đúng example.com
*.doubleclick.net    block              # chỉ subdomain
~adservice           block              # keyword (chuỗi con)
/^ad[0-9]+\./        block              # regexp (RE2)
bank.vn              allow              # chữa chặn nhầm của danh sách
myrouter.lan         ip=192.168.1.1     # DNS giả
example.org          ip=1.2.3.4 fragment=off
10.0.0.0/8           upstream=corp      # CIDR, chỉ ở proxy
*.onion              upstream=tor
```

- **Mẫu khớp:** `domain` (chính nó và mọi subdomain) · `=domain` (chính xác) · `*.domain` (chỉ subdomain) · `~keyword` · `/regexp/` · CIDR v4/v6. Domain được chuẩn hoá: chữ thường, bỏ dấu chấm cuối, IDN chuyển sang punycode.
- **Hành động:**
  - `block` — DNS trả sinkhole, proxy từ chối. Không đi kèm hành động khác.
  - `allow` — đi thẳng, dừng xét các danh sách phía sau. Không đi kèm `block`.
  - `ip=<IPv4|IPv6>` — có thể lặp lại để có cả A và AAAA.
  - `fragment=auto|on|off`
  - `upstream=<id>`
  - `sni=<tên>` — **chừa cho 2B**: parser chấp nhận và lưu, nhưng 2A bỏ qua và giao diện ghi "cần giai đoạn 2B".
- **Lỗi cú pháp:** báo `RULES_PARSE{line,msg}` cho từng dòng lỗi. **Không lưu gì** cho tới khi mọi dòng hợp lệ.
- Rule viết tay tối đa **10.000** dòng.

### 7.2 Bảng ↔ Text

- Trang Rules có hai tab dùng chung dữ liệu: **Bảng** (thêm, sửa, xoá, bật/tắt từng rule, kéo để đổi thứ tự, cột chú thích) và **Text**.
- Rule bị tắt trong bảng xuất ra text dưới dạng `#! <rule>` để giữ được qua vòng chuyển đổi.
- Lưu ở tab Text: parse đạt → cập nhật bảng và lưu; không đạt → giữ nguyên, đánh dấu dòng lỗi.
- Danh sách quản lý ở bảng riêng, không nằm trong text.

### 7.3 Danh sách

#### Model

Mỗi danh sách: `id`, `name`, `source` (`file` | `url`), `url` hoặc đường dẫn file gốc, `format` (`auto` hoặc tên định dạng), `action` (`block` | `allow` | `fragment=on` | `upstream=<id>` | `fromFile`), `enabled`, `updateHours` (mặc định 24; `0` = không tự cập nhật), và metadata `lastUpdated`, `etag`, `lastModified`, `detected`, `counts` (theo loại khớp), `skipped`, `skippedSamples` (tối đa 5 dòng), `lastError`.

`fromFile` chỉ có với hosts và dnsmasq: IP sinkhole → `block`, IP thật → `ip=`.

#### Định dạng hỗ trợ

| Định dạng | Ví dụ dòng | Repo tiêu biểu | Ánh xạ |
|---|---|---|---|
| hosts | `0.0.0.0 ads.com` · `1.2.3.4 site.com` | StevenBlack/hosts, bigdargon/hostsVN | Khớp chính xác. IP sinkhole (`0.0.0.0`, `127.0.0.1`, `::`, `::1`) → chặn; IP thật → `ip=` khi `fromFile` |
| Domain trơn | `ads.com` · `*.ads.com` | hagezi (domains), OISD (domainswild) | Domain + subdomain, hoặc chỉ subdomain |
| AdBlock / AdGuard DNS | `\|\|ads.com^` · `@@\|\|ok.com^` · `\|\|x.com^$important` | AdGuard DNS filter, hagezi (adblock), OISD | `\|\|d^` → domain + subdomain. `@@` → ngoại lệ trong chính danh sách. Chấp nhận modifier `$important`, `$all`; dòng có modifier khác, quy tắc cosmetic (`##`) hoặc dạng URL bị bỏ qua |
| dnsmasq | `address=/ads.com/0.0.0.0` · `server=/ads.com/` · `local=/ads.com/` | nhiều repo blocklist | `address` sinkhole hoặc `server=/d/`, `local=/d/` → chặn domain + subdomain; `address` IP thật → `ip=` khi `fromFile` |
| Unbound | `local-zone: "ads.com" always_nxdomain` | hagezi (unbound) | Chặn domain + subdomain (`always_nxdomain`, `always_null`, `refuse`, `static`, `redirect`) |
| RPZ | `ads.com CNAME .` · `*.ads.com CNAME .` | hagezi (rpz) | Chặn; bỏ qua bản ghi SOA/NS và `rpz-passthru.` |
| Clash / Surge / Shadowrocket | `DOMAIN-SUFFIX,google.com` · `DOMAIN,x.com` · `DOMAIN-KEYWORD,ads` · `DOMAIN-REGEX,…` · `IP-CIDR,1.0.0.0/8` · `IP-CIDR6,…` · YAML `payload:` · `+.domain` · `'.domain'` | blackmatrix7/ios_rule_script, Loyalsoldier/clash-rules | Theo đúng loại. Bỏ cột policy và tham số `no-resolve`. Loại khác (`PROCESS-NAME`, `GEOIP`, `DST-PORT`…) bị bỏ qua |
| v2ray domain-list-community | `domain:x.com` · `full:x.com` · `keyword:ads` · `regexp:…` · `include:google` · `x.com @cn` | v2fly/domain-list-community | Theo đúng loại; dòng không tiền tố = `domain:`. Bỏ thuộc tính `@…`. `include:` tải file cùng thư mục trong repo, sâu tối đa 5 cấp, chỉ cùng host, phát hiện vòng lặp |
| sing-box rule-set (nguồn JSON) | `{"version":1,"rules":[{"domain_suffix":[…]}]}` | SagerNet/sing-geosite (nhánh source) | `domain`, `domain_suffix`, `domain_keyword`, `domain_regex`, `ip_cidr`; rule logic (`type: logical`) bị bỏ qua |
| CIDR | `1.2.3.0/24` · `2001:db8::/32` · `1.2.3.4` | Telegram/Cloudflare IP list, ipverse | CIDR (IP đơn = /32 hoặc /128) |

- **Không hỗ trợ:** `geosite.dat`, `.srs`, `.mrs` và nội dung nhị phân nói chung → lỗi `LIST_UNSUPPORTED_FORMAT`, gợi ý dùng bản text/nguồn tương ứng.
- **Regexp:** RE2 của Go (không có ReDoS), tối đa **1.000** regexp mỗi danh sách; phần thừa bị bỏ qua và tính vào `skipped`.

#### Nhận diện định dạng

1. Theo phần mở rộng: `.yaml`/`.yml` → Clash YAML; `.json` → sing-box; `.rpz` → RPZ; `.conf` → dnsmasq/Unbound (theo nội dung).
2. Theo nội dung: lấy tối đa 200 dòng có nghĩa đầu tiên (bỏ chú thích `#`, `!`, `;`, dòng trống), chấm điểm từng định dạng theo số dòng khớp mẫu đặc trưng; chọn định dạng điểm cao nhất nếu ≥ 60% số dòng mẫu, nếu không thì báo `LIST_UNSUPPORTED_FORMAT`.
3. Người dùng có thể chọn tay định dạng; khi đó bỏ qua nhận diện.
4. Kết quả nhận diện lưu vào `detected` và hiện trên giao diện cùng `counts`, `skipped` và các dòng ví dụ bị bỏ qua.

#### Tải từ URL

- **Chuẩn hoá link GitHub:**
  - `github.com/<u>/<r>/blob/<ref>/<path>` và `github.com/<u>/<r>/raw/<ref>/<path>` → `raw.githubusercontent.com/<u>/<r>/<ref>/<path>`
  - `gist.github.com/<u>/<id>/raw/…` giữ nguyên; `gist.github.com/<u>/<id>` → thêm `/raw`
  - Link `cdn.jsdelivr.net/gh/<u>/<r>@<ref>/<path>` nhận như link GitHub tương ứng.
  - Link GitHub tự có **dự phòng jsDelivr**: tải `raw.githubusercontent.com` lỗi mạng hoặc timeout → thử `cdn.jsdelivr.net/gh/<u>/<r>@<ref>/<path>`.
- Chỉ HTTPS. Timeout 30 giây. Gửi `If-None-Match` / `If-Modified-Since`; `304` → chỉ cập nhật `lastUpdated`.
- `.gz` hoặc `Content-Encoding: gzip` được giải nén. Giới hạn **50 MB sau giải nén** mỗi file (chống zip bomb); tổng mọi danh sách tối đa **2 triệu** mục, vượt thì danh sách mới nhất bị tắt kèm `LIST_TOO_LARGE`.
- Lỗi (mạng, HTTP, định dạng, quá lớn) → **giữ bản cache cũ** trong `lists/<id>.txt`, ghi `lastError`, phát `LIST_FETCH_FAILED{id}`. Không làm phiền bằng hộp thoại.
- Ghi bản cache mới một cách nguyên tử (file tạm → fsync → rename, cùng cách với `store.WriteJSONAtomic`; thêm hàm `store.WriteFileAtomic` cho dữ liệu thô) rồi mới biên dịch lại bộ khớp.
- Request tải danh sách đi qua resolver của hệ thống; khi đang Connect thì đó chính là engine, nên tên miền không bị lộ.

#### Lịch cập nhật

- Lúc mở app: danh sách nào có `lastUpdated` cũ hơn `updateHours` được đưa vào hàng đợi, bắt đầu sau 30 giây (không làm chậm khởi động).
- Trong lúc chạy: kiểm tra mỗi 15 phút; danh sách tới hạn được tải với độ lệch ngẫu nhiên 0–10 phút. Tải tuần tự, mỗi lần một danh sách.
- Nút "Cập nhật ngay" cho từng danh sách và cho tất cả.

#### Thêm nhanh

- Danh mục nhúng sẵn trong `internal/rules/catalog.json`, **chỉ chứa link và metadata** (tên, mô tả, repo, license, định dạng, hành động gợi ý), không phân phối lại nội dung.
- Mục khởi đầu: hagezi Light, hagezi Pro, StevenBlack Unified, OISD Small, AdGuard DNS filter, hostsVN.
- Mỗi mục hiện license và link repo gốc; bấm "Thêm" tạo danh sách với hành động gợi ý, người dùng sửa được.

### 7.4 Biên dịch và khớp

- **Thứ tự ưu tiên:**
  1. Rule viết tay đang bật: rule **đầu tiên** khớp (theo thứ tự trong danh sách) được dùng.
  2. Không rule nào khớp → các danh sách đang bật theo thứ tự trong bảng danh sách; danh sách **đầu tiên** khớp được dùng (ngoại lệ `@@` của một danh sách làm danh sách đó coi như không khớp).
  3. Không gì khớp → `Decision` rỗng (đi thẳng, fragment theo cài đặt chung).
- **Cấu trúc:**
  - Domain/`=`/`*.`: `map[string][]entry` theo tên đầy đủ; khi tra, đi lần lượt từ tên đầy đủ lên từng hậu tố (`a.b.c.com` → `b.c.com` → `c.com` → `com`) và chọn mục có thứ tự ưu tiên nhỏ nhất.
  - Keyword: duyệt tuần tự (thường ít); regexp: duyệt tuần tự, đã biên dịch.
  - CIDR: cây tiền tố nhị phân riêng cho v4 và v6.
  - Mỗi danh sách là một bộ con riêng với cùng cấu trúc.
- **Hiệu năng mục tiêu:** tra một tên miền với 2 triệu mục đã nạp < 5 µs (không tính regexp); bộ nhớ < 250 MB cho 2 triệu mục.
- **Đổi nóng:** biên dịch ở goroutine nền, xong thì `atomic.Pointer.Store`; engine và proxy đọc bằng `Load` ở mỗi truy vấn/kết nối. Không cần kết nối lại.
- **Thử tên miền:** hàm `Explain(host)` trả về quyết định cùng nguồn khớp (rule số mấy, danh sách nào, dòng mẫu nào) cho ô "Thử tên miền" trên giao diện.

### 7.5 Áp dụng ở DNS engine

Áp dụng mọi lúc khi đã Connect, kể cả khi proxy tắt (nên có luôn tác dụng chặn quảng cáo):

- `block`: A → `0.0.0.0`, AAAA → `::`, loại khác → NODATA (`dnsBlockMode: "zero"`, mặc định); hoặc NXDOMAIN cho mọi loại (`"nxdomain"`). TTL 60 giây.
- `ip=`: A/AAAA trả các IP tương ứng (TTL 60 giây); loại không có IP tương ứng hoặc loại khác → NODATA.
- `allow`, `fragment`, `upstream`, `sni`: không tác động DNS. Mẫu CIDR không tác động DNS.
- Tên miền `*.verify.ghostline.test` **luôn bỏ qua rules**.
- Câu trả lời do rules tạo ra không đi qua upstream và không vào cache của dnsproxy; thống kê ghi nhận là `blocked` / `rewritten`.

## 8. Giao diện

### 8.1 Chế độ Nâng cao

Thanh bên: Tổng quan · Máy chủ · Vượt DPI · **Proxy** · **Rules** · Nhật ký · Cài đặt.

- **Proxy**
  - Công tắc: Bật proxy · Dùng cho máy này · Chia sẻ LAN · ô Cổng.
  - Khi chia sẻ LAN: danh sách địa chỉ LAN (`192.168.1.5:8080`…) kèm **mã QR** vẽ bằng SVG từ `internal/qr`; cảnh báo khi profile mạng là Public; hướng dẫn ngắn cấu hình trên Android/iOS.
  - Fragment web: `auto | always | never`, kiểu cắt `tcp | record | both`, `chunks`, `delayMs`, `autoTimeoutMs`; danh sách `frag-cache` của mạng hiện tại, xoá từng mục hoặc tất cả.
  - Bảng upstream proxy: thêm, sửa, xoá, "Kiểm tra".
  - Thống kê trực tiếp (mục 5.8).
- **Rules**
  - Tab Bảng ↔ Text (mục 7.2).
  - Bảng danh sách: thêm từ file, từ URL, hoặc từ "Thêm nhanh"; cột định dạng nhận được, số mục, số dòng bỏ qua (bấm để xem ví dụ), lần cập nhật cuối, lỗi gần nhất; "Cập nhật ngay"; kéo để đổi thứ tự.
  - Ô **Thử tên miền** dùng `Explain`.
  - Chọn `dnsBlockMode`.
- **Vượt DPI:** ghi chú rằng Fragment web chỉ áp dụng cho ứng dụng đi qua proxy; GoodbyeDPI áp dụng cho mọi ứng dụng.
- **Nhật ký:** thêm nguồn lọc `proxy` và `rules`. Tên miền chỉ hiện khi bật "hiện truy vấn" (chỉ RAM).
- **Tổng quan:** thẻ trạng thái proxy (địa chỉ, số kết nối, số client LAN).

### 8.2 Chế độ Đơn giản và khay

- Chế độ Đơn giản **không thêm công tắc nào**. Khi proxy đang chạy, bảng thông số có thêm dòng "Proxy: 127.0.0.1:8080". Proxy lỗi → banner cam SUY_GIẢM.
- Menu khay thêm "Proxy: bật/tắt" (đổi `proxy.enabled`, chạy lại pha P nếu đang kết nối).

### 8.3 Frontend

- Thêm trang `modes/advanced/pages/{proxy,rules}/`, component `QRCode` (SVG), `RulesTable`, `RulesTextEditor` (textarea có đánh số dòng và đánh dấu lỗi, không dùng thư viện editor).
- Sự kiện Wails mới: `proxy:stats`, `proxy:conn` (chỉ khi bật hiện truy vấn), `rules:compiled`, `lists:progress`.
- Toàn bộ chuỗi mới có trong `vi.json` và `en.json`.

## 9. Cài đặt (`settings.json`, phiên bản 2)

Thêm vào phiên bản 1:

```json
{
  "version": 2,
  "proxy": {
    "enabled": false,
    "port": 8080,
    "systemProxy": false,
    "shareLan": false,
    "fragment": {
      "mode": "auto",
      "method": "both",
      "chunks": 5,
      "delayMs": 5,
      "autoTimeoutMs": 3000,
      "cacheDays": 7
    },
    "upstreams": []
  },
  "dnsBlockMode": "zero"
}
```

- Nâng cấp từ v1: thêm các khối mặc định ở trên, giữ nguyên mọi trường cũ, ghi lại thành v2.
- Kiểm tra khi lưu: `port` 1024–65535 và khác 53; `chunks` 2–64; `delayMs` 0–100; `autoTimeoutMs` 1000–10000; `cacheDays` 1–90.

### `rules.json` (phiên bản 1)

```json
{
  "version": 1,
  "rules": [
    { "pattern": "youtube.com", "fragment": "on", "enabled": true, "comment": "" }
  ],
  "lists": [
    {
      "id": "hagezi-light", "name": "HaGeZi Light", "source": "url",
      "url": "https://raw.githubusercontent.com/hagezi/dns-blocklists/main/domains/light.txt",
      "format": "auto", "action": "block", "enabled": true, "updateHours": 24,
      "lastUpdated": "…", "etag": "…", "lastModified": "…", "detected": "domains",
      "counts": { "domain": 0 }, "skipped": 0, "skippedSamples": [], "lastError": ""
    }
  ]
}
```

File hỏng → đổi tên thành `rules.json.bak`, dùng rỗng, thông báo (giống `settings.json`).

### `frag-cache.json`

```json
{ "version": 1, "networks": { "<networkKey>": { "youtube.com": "2026-10-11T10:00:00Z" } } }
```

`networkKey` giống `scan-cache.json` (SHA-256 của IP + MAC default gateway). Giá trị là thời điểm hết hạn (`cacheDays`). Mục hết hạn bị dọn khi nạp.

## 10. Xử lý lỗi

| Mã | Khi nào | Hành động gợi ý |
|---|---|---|
| `PROXY_PORT_BUSY{port,pid,name}` | Cổng proxy bị chiếm | Hiện process đang giữ cổng · Đổi cổng · Thử lại proxy |
| `PROXY_SELFTEST_FAILED` | P1 không echo được | Thử lại · Mở nhật ký |
| `PROXY_FIREWALL{detail}` | Không tạo được luật firewall | Thử lại · Tắt chia sẻ LAN |
| `SYSPROXY_EXISTING{server,pac}` | Máy đang có proxy/PAC khác | Hộp xác nhận: Ghi đè (sẽ trả lại khi ngắt) · Không dùng system proxy |
| `SYSPROXY_FAILED` | Đặt hoặc đọc lại system proxy không đúng | Thử lại · Tắt "Dùng cho máy này" |
| `SYSPROXY_TAKEN_OVER` | App khác đổi system proxy khi đang kết nối | Thông báo; không giành lại, không khôi phục đè |
| `SYSPROXY_RESTORE_FAILED` | Không khôi phục được system proxy | Cảnh báo cố định + "Khôi phục proxy ngay" + hướng dẫn làm bằng tay |
| `RULES_PARSE{line,msg}` | Rule viết tay sai | Đánh dấu dòng lỗi, không lưu |
| `LIST_FETCH_FAILED{id,detail}` | Tải danh sách lỗi | Giữ bản cũ, ghi chú trong bảng |
| `LIST_UNSUPPORTED_FORMAT{id}` | Không nhận diện được hoặc là file nhị phân | Gợi ý chọn tay hoặc dùng bản text |
| `LIST_TOO_LARGE{id}` | Vượt 50 MB hoặc tổng 2 triệu mục | Tắt danh sách đó |
| `UPSTREAM_PROXY_FAILED{id}` | Upstream không kết nối được hoặc mất mật khẩu DPAPI | Đánh dấu trong bảng upstream · Nhập lại mật khẩu |

- Panic trong goroutine xử lý một kết nối proxy: bắt lại, ghi log kèm stack trace, chỉ đóng kết nối đó.
- Panic trong listener hoặc bộ lập lịch danh sách: xử lý như pha P chết (chạy lại một lần, rồi SUY_GIẢM).

## 11. Kiểm thử

Phát triển theo TDD. Test cần Windows thật gắn build tag `integration`.

| Phần | Test |
|---|---|
| `tlsfrag` | Ghép các mảnh `tcp` luôn ra đúng bản gốc. `record` sinh chuỗi bản ghi TLS hợp lệ mà ghép phần payload ra đúng ClientHello gốc. `both`. Dữ liệu không phải ClientHello trả nguyên. **Fuzz** |
| `proxy` (đọc request) | Test dạng bảng cho SOCKS4/4a/5 và HTTP CONNECT/forward, hợp lệ và lỗi, mã lỗi trả về đúng chuẩn. **Fuzz** mọi hàm đọc. Giới hạn 8 KB, timeout bắt tay, giới hạn kết nối theo IP và tổng, từ chối nguồn không phải IP riêng |
| `proxy/dialer` | Server TLS giả **mô phỏng DPI** (reset hoặc im lặng khi SNI nguyên vẹn nằm trong một segment/bản ghi). `auto` → thử lại có fragment → ghi cache; lần sau dùng cache. `on`/`off`/`never`/`always`. Bản cắt cũng thất bại → bộ đếm. Upstream SOCKS5/HTTP giả (kèm user/pass). Chặn SSRF và vòng lặp. `Resolver` giả kèm **bẫy**: test thất bại nếu có bất kỳ lần gọi `net.DefaultResolver` / `LookupHost` nào |
| `proxy` (đầu cuối) | Client SOCKS5 và HTTP thật (`golang.org/x/net/proxy`, `http.Transport.Proxy`) qua proxy tới server `httptest` TLS |
| `rules` (text) | Parse hợp lệ/lỗi theo dòng, chuẩn hoá domain và IDN, `#!` rule tắt, vòng parse → xuất → parse giữ nguyên |
| `rules` (định dạng) | Bộ file mẫu cho cả 10 họ định dạng trong `testdata/` (trích ngắn, có ghi nguồn), nhận diện đúng, `counts`/`skipped` đúng, `@@`, `include:` (kể cả vòng lặp và giới hạn độ sâu), gz, zip bomb, file nhị phân |
| `rules` (khớp) | Thứ tự ưu tiên rule → danh sách, `domain`/`=`/`*.`/keyword/regexp/CIDR, `allow`, `Explain`. Benchmark 2 triệu mục. Đổi nóng khi nhiều goroutine cùng đọc (`-race`) |
| `rules` (tải) | Server `httptest`: ETag/304, chuẩn hoá link GitHub (bảng), dự phòng jsDelivr, lỗi giữ cache cũ, lịch cập nhật với đồng hồ giả |
| `engine` | `block` zero/nxdomain, `ip=` A/AAAA/NODATA, tên miền xác minh bỏ qua rules, `Resolve` dùng upstream giả |
| `sysproxy` | `API` giả: chụp → đặt → khôi phục; không khôi phục khi đã bị thay hoặc giá trị không còn là của Ghostline; xác nhận đọc lại. `integration`: vòng thật trên máy |
| `qr` | So ma trận với vector chuẩn cho vài chuỗi; đọc lại bằng thư viện giải mã QR **chỉ trong test** |
| `app` | Pha P lỗi ở từng bước P1–P4 → đúng các undo, DNS vẫn được bảo vệ, SUY_GIẢM với đúng lý do. Tập lý do SUY_GIẢM. Thứ tự Disconnect mới (bất biến: system proxy trả trước khi tắt proxy). Chạy lại pha P khi đổi cài đặt; không chạy lại khi đổi rules |
| `watchdog` / `store` | Khôi phục system proxy và firewall theo đúng thứ tự, `takenOver` được tôn trọng. Nâng `state.json` và `settings.json` từ v1 lên v2. `rules.json` hỏng |
| Frontend | Trang Proxy (QR, cảnh báo Public), Rules (đồng bộ Bảng ↔ Text, đánh dấu dòng lỗi, Thử tên miền), `vi.json` và `en.json` cùng bộ khoá |
| Kiểm tra thủ công trước phát hành | Trình duyệt vào trang bị chặn theo SNI qua system proxy (GoodbyeDPI tắt). Điện thoại trong LAN dùng proxy qua mã QR. Mạng Public → gợi ý đúng. `taskkill /F` → system proxy và firewall được khôi phục ≤ 3 giây. Khởi động lại máy khi đang dùng system proxy → khôi phục lúc đăng nhập. VPN đổi system proxy giữa chừng → không bị giành lại. Wireshark: không có DNS plain do proxy gây ra. Import link GitHub từ 6 repo trong danh mục |

## 12. Đóng gói và tài liệu

- Không thêm thành phần bên ngoài, không thêm dependency Go nào cho lõi (thư viện giải mã QR chỉ dùng trong test).
- `NOTICE` không đổi (danh mục danh sách chỉ chứa link, nội dung tải lúc chạy).
- README (EN + VI) và hướng dẫn sử dụng: thêm mục Proxy, Chia sẻ LAN, Rules và danh sách, cùng hạn chế UAC bằng tài khoản admin khác.
- Phát hành dưới dạng **v0.2.0**.

## 13. Cấu trúc repo (thêm)

```
internal/
  proxy/            listener, socks4.go, socks5.go, http.go, relay.go, limits.go, stats.go
  proxy/dialer/     dialer.go, autofrag.go, upstream.go, ssrf.go
  tlsfrag/          split.go, record.go, hello.go
  rules/            rule.go, text.go, compile.go, match.go, explain.go,
                    formats/{hosts,domains,adblock,dnsmasq,unbound,rpz,clash,v2fly,singbox,cidr,detect}.go,
                    fetch.go, github.go, schedule.go, catalog.json
  sysproxy/         sysproxy.go, api_windows.go, watch_windows.go
  qr/               qr.go
frontend/src/modes/advanced/pages/{proxy,rules}/
```

## 14. Rủi ro và cách giảm thiểu

| Rủi ro | Giảm thiểu |
|---|---|
| Lỗi trong code proxy tự viết | Fuzz test mọi hàm đọc, giới hạn cứng (8 KB, timeout, số kết nối), chỉ hỗ trợ tập lệnh tối thiểu, fail closed, test đầu cuối bằng client chuẩn |
| Proxy bị dùng để dò mạng nội bộ hoặc bị lạm dụng từ ngoài | Tắt LAN mặc định; chỉ nhận IP riêng; firewall chỉ profile Private và `localsubnet`; chặn SSRF tới loopback/link-local/chính máy |
| System proxy kẹt sau crash khiến mất mạng | Chụp trước khi đổi, khôi phục ở cả 4 lớp, thứ tự Disconnect "trả proxy trước khi tắt proxy" |
| Giành system proxy với VPN/app khác | Hỏi trước khi ghi đè; theo dõi thay đổi; không giành lại; không khôi phục đè |
| Fragment `auto` làm chậm lần đầu, hoặc DPI kiểu im lặng làm mỗi trang mới chậm 3 giây | Ghi nhớ theo mạng 7 ngày; `autoTimeoutMs` chỉnh được; chế độ `always` cho mạng chặn nặng; rule `fragment=on` cho domain cụ thể |
| DPI vẫn chặn dù đã fragment | Đếm `blockedEvenFragmented`, gợi ý GoodbyeDPI (có sẵn từ giai đoạn 1) |
| Danh sách cộng đồng rất lớn tốn RAM, hoặc định dạng lạ | Giới hạn 50 MB/file và 2 triệu mục; báo `skipped` kèm ví dụ; chọn tay định dạng |
| `raw.githubusercontent.com` bị chặn | Dự phòng jsDelivr; giữ bản cache cũ |
| Danh sách chặn nhầm trang quan trọng | Hành động `allow`; ô Thử tên miền chỉ rõ danh sách và dòng gây chặn |
| Ứng dụng không tôn trọng system proxy | Ghi rõ trên giao diện rằng Fragment web chỉ áp dụng cho ứng dụng đi qua proxy; GoodbyeDPI vẫn là lựa chọn cho mọi ứng dụng |
