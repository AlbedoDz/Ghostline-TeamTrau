# Hướng dẫn phát hành (release) Ghostline

Tài liệu cho người bảo trì (maintainer). Mô tả toàn bộ quy trình: từ chuẩn bị, gắn tag, theo dõi build, kiểm tra bản phát hành, cho tới sửa lỗi và thu hồi.

## Mục lục

1. [Tổng quan quy trình](#1-tổng-quan-quy-trình)
2. [Thiết lập một lần](#2-thiết-lập-một-lần)
3. [Đánh số phiên bản](#3-đánh-số-phiên-bản)
4. [Các bước phát hành](#4-các-bước-phát-hành)
5. [Sau khi phát hành](#5-sau-khi-phát-hành)
6. [Bản vá khẩn (hotfix)](#6-bản-vá-khẩn-hotfix)
7. [Thu hồi một bản phát hành lỗi](#7-thu-hồi-một-bản-phát-hành-lỗi)
8. [Danh sách máy chủ có chữ ký](#8-danh-sách-máy-chủ-có-chữ-ký)
9. [Build bản phát hành trên máy (không qua CI)](#9-build-bản-phát-hành-trên-máy-không-qua-ci)
10. [Xử lý khi workflow release lỗi](#10-xử-lý-khi-workflow-release-lỗi)

---

## 1. Tổng quan quy trình

```
main (CI xanh) ──► kiểm tra trên máy thật ──► nâng số phiên bản ──► git tag vX.Y.Z ──► git push tag
                                                                                          │
                       GitHub Actions: workflow "release" (.github/workflows/release.yml) ◄┘
                         1. cài Go, Node, wails3, NSIS
                         2. sinh bindings, build frontend, chạy toàn bộ test (frontend + Go)
                         3. wails3 package VERSION=X.Y.Z        → bin/ghostline-amd64-installer.exe
                         4. wails3 task windows:portable        → bin/Ghostline-X.Y.Z-portable.zip
                         5. tính SHA256SUMS
                         6. gh release create vX.Y.Z (kèm 3 file, ghi chú tự sinh từ commit)
```

- Release được kích hoạt **chỉ bằng tag bắt đầu bằng `v`**. Push lên `main` không tạo release.
- Nếu bất kỳ test nào hỏng, workflow dừng và **không** tạo trang Release.
- Trang Release được tạo **công khai ngay**. App của người dùng hỏi `releases/latest` mỗi ngày, nên khi bản mới lên, người dùng sẽ thấy thông báo "có bản mới" trong vòng 24 giờ.

## 2. Thiết lập một lần

Làm một lần cho repo, trước bản phát hành đầu tiên.

**2.1. Secret ký danh sách máy chủ.** Workflow `servers` cần secret này để ký `lists/servers.json`. Release không cần, nhưng nếu thiếu, danh sách máy chủ sẽ không bao giờ được cập nhật.

```bash
gh secret set SERVERLIST_SIGNING_KEY < "$USERPROFILE/ghostline-serverlist-signing-key.txt"
gh secret list        # phải thấy SERVERLIST_SIGNING_KEY
```

> ⚠️ Khoá riêng **không bao giờ** được commit, dán vào issue hay gửi qua chat. Hãy sao lưu file khoá ở nơi an toàn (trình quản lý mật khẩu). Mất khoá đồng nghĩa phải phát hành bản app mới với public key mới trong `internal/brand`.

**2.2. Bật báo lỗ hổng riêng tư** (link trong `SECURITY.md` cần tính năng này):

```bash
gh api -X PUT repos/hashcott/ghostline/private-vulnerability-reporting
```

**2.3. Quyền của GitHub Actions.** Vào **Settings → Actions → General → Workflow permissions**, chọn **Read and write permissions**. Workflow `release` cần quyền này để tạo Release, workflow `servers` cần để push danh sách mới.

**2.4. Bảo vệ tag (khuyến nghị).** Vào **Settings → Rules → Rulesets → New tag ruleset**, áp dụng cho `v*` và chỉ cho phép maintainer tạo, cập nhật hoặc xoá tag. Như vậy không ai khác tạo được bản phát hành.

**2.5. Sửa thông tin trong file exe.** `build/windows/info.json` vẫn còn chữ mẫu của Wails (`"A ghostline application"`, `"© 2026, My Company"`, `"This is a comment"`). Những chữ này hiện trong **Properties → Details** của file exe, nên sửa lại trước bản đầu tiên, ví dụ:

```json
"FileDescription": "Ghostline – Secure DNS client",
"LegalCopyright": "© 2026 Harry Nguyen. MIT License.",
"Comments": "https://github.com/hashcott/ghostline"
```

## 3. Đánh số phiên bản

Dùng [Semantic Versioning](https://semver.org/lang/vi/): `vMAJOR.MINOR.PATCH`.

| Thay đổi | Tăng | Ví dụ |
| --- | --- | --- |
| Chỉ sửa lỗi | PATCH | `v0.1.0` → `v0.1.1` |
| Thêm tính năng, vẫn tương thích | MINOR | `v0.1.1` → `v0.2.0` |
| Thay đổi phá vỡ tương thích (định dạng cài đặt, dữ liệu…) | MAJOR | `v0.9.0` → `v1.0.0` |

Lưu ý:
- Trước `v1.0.0`, bản MINOR được phép chứa thay đổi lớn.
- App so sánh phiên bản bằng semver (`internal/updater`). Tag phải đúng dạng `vX.Y.Z` thì người dùng mới nhận được thông báo có bản mới.
- Nếu muốn phát hành thử (`v0.2.0-beta.1`), sau khi workflow chạy xong hãy đánh dấu Release đó là **pre-release** (`gh release edit v0.2.0-beta.1 --prerelease`). API `releases/latest` bỏ qua pre-release, nên người dùng bản ổn định sẽ không nhận thông báo.

**Số phiên bản nằm ở đâu:**

| Nơi | Cách cập nhật |
| --- | --- |
| Phiên bản hiển thị trong app (Cài đặt → *phiên bản*, menu khay) | **Tự động** từ tag, qua `-ldflags -X …/brand.Version` |
| Tên file zip portable | **Tự động** từ tag |
| Thông tin file exe (`build/windows/info.json`: `file_version`, `ProductVersion`) | **Sửa tay** trước khi gắn tag |
| Phiên bản trong *Apps & features* của Windows (`build/windows/nsis/wails_tools.nsh`: `INFO_PRODUCTVERSION`) | **Sửa tay** trước khi gắn tag |

## 4. Các bước phát hành

Ví dụ phát hành `v0.2.0`.

### Bước 1: Đảm bảo `main` sạch và CI xanh

```bash
git checkout main
git pull
git status                      # không còn thay đổi chưa commit
gh run list --branch main --limit 3   # lần chạy "ci" mới nhất phải là "success"
```

### Bước 2: Chạy test trên máy

```bash
go test ./...
cd frontend && npm test && npx tsc --noEmit && cd ..
```

Mở một terminal **admin** để chạy test tích hợp (các test này thay đổi DNS thật của máy):

```bash
go test -tags integration ./internal/sysdns/... ./internal/startup/...
```

### Bước 3: Kiểm tra trên máy thật

Đi qua toàn bộ [`docs/release-checklist.md`](release-checklist.md). **Mục nào hỏng thì không phát hành.** Quan trọng nhất là các mục bảo đảm DNS luôn được trả lại:

- Kết nối rồi ngắt kết nối: DNS về đúng như ban đầu.
- `taskkill /F /IM ghostline.exe` khi đang kết nối: DNS về như cũ trong ≤ 3 giây.
- Rút điện hoặc tắt cứng máy khi đang kết nối: sau khi đăng nhập lại, DNS được khôi phục.
- Gỡ cài đặt khi đang kết nối: DNS về như cũ, tác vụ và driver WinDivert bị xoá.
- Bật và tắt GoodbyeDPI: `sc query WinDivert1.4` cho thấy service biến mất sau khi tắt.

Nên thử trên ít nhất một máy **Windows 11 cài mới** (hoặc máy ảo), dùng chính bản build từ bước 9.

### Bước 4: Nâng số phiên bản

Sửa `0.1.0` thành `0.2.0` ở:

- `build/windows/info.json`: `file_version` và `ProductVersion`.
- `build/windows/nsis/wails_tools.nsh`: `!define INFO_PRODUCTVERSION "0.2.0"`.

Rồi commit:

```bash
git add build/windows/info.json build/windows/nsis/wails_tools.nsh
git commit -m "chore(release): v0.2.0"
git push origin main
```

Chờ CI của commit này xanh (`gh run watch`).

### Bước 5: Gắn tag và đẩy lên

Dùng **annotated tag** (có ghi chú), không dùng tag nhẹ:

```bash
git tag -a v0.2.0 -m "Ghostline v0.2.0"
git push origin v0.2.0
```

Ngay khi tag được đẩy lên, workflow `release` bắt đầu chạy.

### Bước 6: Theo dõi build

```bash
gh run list --workflow release --limit 1
gh run watch <run-id> --exit-status
```

Hoặc mở tab **Actions → release** trên GitHub. Một lần chạy mất khoảng 10–15 phút (phần lớn là cài NSIS và chạy test).

### Bước 7: Kiểm tra bản phát hành

```bash
gh release view v0.2.0
```

Trang Release phải có đúng 3 file:

| File | Kiểm tra |
| --- | --- |
| `ghostline-amd64-installer.exe` | Cài được, mở được, **Cài đặt** hiện `phiên bản 0.2.0` |
| `Ghostline-0.2.0-portable.zip` | Giải nén có `ghostline.exe` và file đánh dấu `portable` |
| `SHA256SUMS` | Khớp với file đã tải về |

Tải về và đối chiếu mã băm:

```bash
mkdir /tmp/rel && cd /tmp/rel
gh release download v0.2.0
sha256sum -c SHA256SUMS
```

Sau đó cài bản vừa tải lên một máy sạch, bấm kết nối, bật GoodbyeDPI rồi gỡ cài đặt. Bạn đang kiểm tra **đúng file người dùng sẽ tải**, không phải bản build trên máy mình.

### Bước 8: Viết ghi chú phát hành

Workflow tạo ghi chú tự động từ danh sách commit. Nên sửa lại cho người dùng dễ đọc:

```bash
gh release edit v0.2.0 --notes-file notes.md
```

Mẫu `notes.md`:

```markdown
## Có gì mới
- Tự dò preset vượt DPI nhanh hơn.

## Sửa lỗi
- Công tắc GoodbyeDPI không còn tự tắt khi đang khởi động.

## Lưu ý
- Bản chưa ký số: Windows SmartScreen sẽ cảnh báo, chọn More info → Run anyway sau khi kiểm tra SHA-256.

**SHA-256:** xem file `SHA256SUMS` bên dưới.
```

## 5. Sau khi phát hành

- **Kiểm tra thông báo cập nhật:** mở một bản cũ hơn đang chạy. Trong vòng 24 giờ, app phải hiện *có bản mới v0.2.0 ↗*.
- **Thông báo:** đăng link Release lên nơi bạn chia sẻ dự án.
- **Theo dõi issue** trong vài ngày đầu, nhất là các báo cáo về mất mạng hoặc DNS không được khôi phục. Đây là lỗi nghiêm trọng nhất, cần vá ngay (xem phần 6).

## 6. Bản vá khẩn (hotfix)

Khi bản vừa phát hành có lỗi nghiêm trọng:

1. Sửa lỗi trên `main`, kèm test tái hiện lỗi đó (test phải hỏng trước khi sửa).
2. Chờ CI xanh, chạy lại các mục liên quan trong checklist.
3. Tăng **PATCH** (`v0.2.0` → `v0.2.1`), sửa hai file phiên bản (bước 4), rồi gắn tag và đẩy lên (bước 5–7).

Không sửa hay ghi đè một tag đã phát hành; luôn phát hành số mới. Người dùng và app so sánh theo số phiên bản, nên tag bị ghi đè sẽ gây nhầm lẫn.

## 7. Thu hồi một bản phát hành lỗi

Chỉ làm khi bản đó **gây hại** (ví dụ làm mất mạng) và chưa kịp có bản vá:

```bash
gh release delete v0.2.0 --cleanup-tag --yes   # xoá Release và tag trên GitHub
git tag -d v0.2.0                              # xoá tag trên máy
```

- Sau khi xoá, `releases/latest` quay về bản trước, nên người dùng mới sẽ tải bản cũ.
- Người đã cài bản lỗi **không** được tự hạ cấp. Hãy phát hành bản vá với số **cao hơn** (`v0.2.1`) càng sớm càng tốt, và ghi rõ trong ghi chú.
- Một cách nhẹ tay hơn xoá: giữ lại Release nhưng đánh dấu pre-release (`gh release edit v0.2.0 --prerelease`) và thêm cảnh báo ở đầu ghi chú.

## 8. Danh sách máy chủ có chữ ký

Danh sách máy chủ **không đi theo bản phát hành**:

- Workflow `servers` chạy **mỗi thứ Hai lúc 03:00 UTC** (hoặc bấm tay: `gh workflow run servers`). Nó sinh lại `lists/servers.json` từ `lists/seed.json`, ký bằng `SERVERLIST_SIGNING_KEY`, rồi commit lên `main`.
- App tải file này mỗi ngày và kiểm tra chữ ký bằng public key trong `internal/brand`. File có chữ ký sai sẽ bị bỏ qua.
- Bản phát hành nhúng sẵn `lists/servers.json` **đúng như trong repo lúc gắn tag**, để app mới cài vẫn có danh sách khi chưa tải được bản mới.

Muốn thêm hoặc bớt máy chủ: sửa `lists/seed.json`, commit, rồi chạy `gh workflow run servers`. Không cần phát hành bản app mới.

## 9. Build bản phát hành trên máy (không qua CI)

Dùng để thử installer trước khi gắn tag. **Không** dùng để phát hành chính thức: bản chính thức phải do CI build để ai cũng kiểm chứng được nguồn gốc.

Cần: Go, Node 24, `wails3` v3.0.0-beta.27, NSIS (`makensis` có trong PATH).

```bash
cd frontend && npm ci && cd ..
wails3 generate bindings -clean=true -ts -i
wails3 package VERSION=0.2.0               # bin/ghostline-amd64-installer.exe
wails3 task windows:portable VERSION=0.2.0 # bin/Ghostline-0.2.0-portable.zip
```

Kiểm tra phiên bản được nhúng: mở app → **Cài đặt** → góc phải hiện `phiên bản 0.2.0`.

## 10. Xử lý khi workflow release lỗi

Xem log lỗi:

```bash
gh run view <run-id> --log-failed
```

| Lỗi | Cách xử lý |
| --- | --- |
| Test hỏng | Chưa có Release nào được tạo. Sửa lỗi trên `main`, rồi **gắn lại tag** (xem bên dưới) |
| `choco install nsis` lỗi hoặc mạng chập chờn | Chạy lại: `gh run rerun <run-id>` |
| `gh release create` báo Release đã tồn tại | Tag này đã từng phát hành. Xoá Release cũ (phần 7) hoặc dùng số phiên bản mới |
| `Resource not accessible by integration` | Thiếu quyền ghi cho Actions (xem 2.3) |

**Gắn lại tag khi Release chưa được tạo** (ví dụ test hỏng):

```bash
git tag -d v0.2.0
git push origin :refs/tags/v0.2.0     # xoá tag trên GitHub
# ...sửa lỗi, commit, push main, chờ CI xanh...
git tag -a v0.2.0 -m "Ghostline v0.2.0"
git push origin v0.2.0
```

Chỉ gắn lại tag khi **chưa có Release công khai** cho tag đó. Nếu Release đã lên, hãy phát hành số mới (phần 6).

---

### Tóm tắt nhanh

```bash
git checkout main && git pull && gh run list --branch main --limit 1   # CI xanh?
go test ./... && (cd frontend && npm test)                              # test xanh?
# → đi qua docs/release-checklist.md trên máy thật
# → sửa phiên bản trong info.json và wails_tools.nsh, commit, push, chờ CI
git tag -a v0.2.0 -m "Ghostline v0.2.0" && git push origin v0.2.0
gh run watch $(gh run list --workflow release --limit 1 --json databaseId -q '.[0].databaseId') --exit-status
gh release view v0.2.0                                                 # đủ 3 file?
```
