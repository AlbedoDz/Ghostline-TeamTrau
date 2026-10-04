# Ghostline

**Secure DNS Client cho Windows** — một nút bấm để mã hoá toàn bộ DNS của máy và vượt chặn DNS/SNI, kèm chế độ Nâng cao cho người dùng kỹ thuật.

> English summary below.

## Tính năng (giai đoạn 1)

- **Mã hoá DNS**: DNS server cục bộ (127.0.0.1 / ::1) chuyển tiếp qua DoH, DoT, DoQ, DNSCrypt — dùng [dnsproxy](https://github.com/AdguardTeam/dnsproxy).
- **Tự chọn máy chủ nhanh nhất**: quét song song, phát hiện DNS bị đầu độc, nhớ kết quả theo từng mạng.
- **Không bao giờ để mất mạng**: lưu DNS gốc của từng card mạng trước khi đổi; 4 lớp khôi phục (ngắt sạch, watchdog, khi mở lại app, tác vụ lúc đăng nhập).
- **Vượt DPI**: GoodbyeDPI 0.2.2 (preset, tự dò, danh sách đen) và Fragment cho kết nối DoH.
- **Xác minh không rò rỉ**: sau khi kết nối, Ghostline kiểm tra truy vấn thật sự đi qua nó.
- **Giao diện Neon Terminal**, song ngữ Việt/Anh, chế độ Đơn giản và Nâng cao, icon khay.
- **Không telemetry.** Không ghi tên miền bạn truy cập xuống đĩa.

## Tải về và kiểm tra

Tải từ trang [Releases](https://github.com/hashcott/ghostline/releases):

- `ghostline-amd64-installer.exe` — bản cài đặt (tự cài WebView2 nếu thiếu).
- `Ghostline-<phiên bản>-portable.zip` — bản portable: giải nén và chạy, dữ liệu nằm trong thư mục `data\` cạnh exe.

Kiểm tra file bằng mã SHA-256 trong `SHA256SUMS`:

```powershell
Get-FileHash .\Ghostline-0.1.0-portable.zip -Algorithm SHA256
```

### Cảnh báo SmartScreen

Bản phát hành chưa được ký số, nên Windows SmartScreen sẽ cảnh báo "Windows protected your PC". Chọn **More info → Run anyway** sau khi đã kiểm tra SHA-256.

### Antivirus và WinDivert

GoodbyeDPI dùng driver **WinDivert**, thường bị antivirus báo nhầm. Ghostline kiểm tra SHA-256 của GoodbyeDPI trước mỗi lần chạy. Nếu bị chặn, hãy thêm thư mục Ghostline vào danh sách loại trừ.

### Vì sao cần quyền admin

Đổi DNS của card mạng, cài driver WinDivert và tạo tác vụ khởi động đều cần quyền quản trị. Khi khởi động cùng Windows, Ghostline chạy qua Task Scheduler nên không hỏi UAC.

> Lưu ý: nếu bạn nâng quyền UAC bằng **một tài khoản admin khác**, dữ liệu của Ghostline nằm trong `%APPDATA%` của tài khoản admin đó.

## Build

Yêu cầu: Go 1.27, Node 24, `wails3` v3.0.0-beta.27, NSIS (để tạo installer).

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.27
wails3 dev                     # chạy thử (không cần admin; kết nối thật cần admin)
wails3 build                   # bin/ghostline.exe
wails3 package                 # installer NSIS
wails3 task windows:portable   # zip portable
go test ./... && (cd frontend && npm test)
```

Test cần quyền admin: `go test -tags integration ./internal/sysdns/... ./internal/startup/...` trong terminal admin.

### Danh sách máy chủ có chữ ký

`lists/servers.json` được sinh lại và ký (ed25519) bởi workflow **servers** (chạy hằng tuần hoặc bấm tay), dùng secret `SERVERLIST_SIGNING_KEY`, rồi commit lên `main`. App tải bản này mỗi ngày. Workflow **release** không sinh lại danh sách: bản phát hành nhúng đúng file đang có trong repo lúc gắn tag.

## License

MIT — xem [LICENSE](LICENSE) và [NOTICE](NOTICE).

---

## English

Ghostline is a Windows secure DNS client: one click encrypts all system DNS through a local dnsproxy engine (DoH/DoT/DoQ/DNSCrypt), with automatic fastest-server selection, leak verification, GoodbyeDPI-based DPI bypass with auto-tuning, and a four-layer safety net that always restores your original DNS. Neon-terminal UI in Vietnamese and English, simple and advanced modes, tray icon. No telemetry; visited domains are never written to disk. Releases are unsigned for now — verify `SHA256SUMS` and expect a SmartScreen prompt. Licensed MIT.
