# Giai đoạn 4: Microsoft Graph và CalDAV (không đổi lõi)

> **Dành cho agent thực thi:** BẮT BUỘC dùng superpowers:subagent-driven-development (khuyến nghị) hoặc superpowers:executing-plans. Mỗi bước dùng checkbox `- [ ]`. Đọc `2026-11-01-mailcal-00-overview.md` trước. **Điều kiện bắt đầu:** giai đoạn 3 đã gộp vào `master` và cổng của nó xanh.

**Mục tiêu:** Thêm hai nhà cung cấp mới (Microsoft 365/Outlook qua Graph; lịch qua CalDAV) mà **không sửa** engine luật, chốt chặn, dịch vụ, ABI hay luồng gửi. Đó chính là tiêu chí thành công số 4 của spec: một plugin mới đạt bộ tuân thủ ABI mà không phải sửa lõi.

**Kiến trúc:** Dùng lại `provkit` của giai đoạn 3. Graph là `provkit.Backend` + `netio` + OAuth (cùng đường `acton_oauth` như Gmail, nhà cung cấp `microsoft`). CalDAV dùng mật khẩu ứng dụng nên cần một đường thông tin đăng nhập **mới nhưng cùng khuôn**: module host `acton_account.get_credentials` có kiểm sở hữu và quyền `permissions.credentials`, mật khẩu nằm trong vault. Thêm `icskit` (đọc/ghi iCalendar giữ nguyên thuộc tính lạ).

**Tech stack:** Go (wasip1), Microsoft Graph v1.0 REST, CalDAV (RFC 4791) + iCalendar (RFC 5545), `encoding/xml`.

**Spec:** `docs/superpowers/specs/2026-11-01-mail-calendar-plugins-design.md` (mục 4, 9, 10, 14, 16).

## Global Constraints

- Hợp đồng khoá C1-C8. Giai đoạn này **chỉ thêm**: quyền `permissions.credentials`, module host `acton_account`, plugin mới, gói `icskit`.
- **Cổng "không đổi lõi" là kiểm tra máy móc:** `git diff --stat wip/mailcal-ref-p3 HEAD -- internal/mailcal/writeops.go internal/mailcal/tools_write.go internal/mailcal/tools_read.go internal/mailcal/policy internal/mailcal/abi internal/tools` phải **rỗng** khi kết thúc giai đoạn. Ngoại lệ duy nhất, cố ý và được liệt kê: `internal/mailcal/service.go` chỉ được thêm trường siêu dữ liệu `ProviderInfo.AuthKind` (để giao diện biết hiện biểu mẫu nào); kiểm bằng `git diff wip/mailcal-ref-p3 HEAD -- internal/mailcal/service.go | grep '^[+-]' | grep -v '^+++\|^---'` chỉ cho ra các dòng nói về `AuthKind`.
- Mật khẩu chỉ nằm trong vault (khoá `mailcal.account.<id>.password`); bản ghi tài khoản chỉ giữ **tên khoá**; API không bao giờ trả lại mật khẩu; xoá tài khoản xoá mật khẩu.
- Graph: scope tối thiểu `offline_access Mail.ReadWrite Mail.Send Calendars.ReadWrite` (không `.All`, không `.Shared`). CalDAV: URL bộ lịch phải `https`, không nhúng thông tin đăng nhập.
- Plugin không bao giờ xoá vĩnh viễn thư: Graph `delete` = chuyển vào Deleted Items.
- Con trỏ phân trang của plugin là **số thứ tự thuần** (không bao giờ là URL của nhà cung cấp), để mô hình không thể bắt plugin gọi một host khác.
- Giao diện: khoá `en` và `vi`; không chuỗi cứng; không emoji. Không `Co-Authored-By`.

## Review Focus

1. **Plugin xin mật khẩu của tài khoản không phải của mình**, hoặc plugin không khai `permissions.credentials` (`TestCredentialsNeverGoToAnotherPlugin`, `TestCredentialRefusals`, `TestAWasmProviderReceivesCredentialsOnlyWithThePermission`); mật khẩu bị che khỏi log của plugin.
2. **Mật khẩu lọt vào bản ghi tài khoản hoặc phản hồi API** (`TestPasswordsLiveInTheVaultAndNeverInTheAccountRecord`, `TestMailcalPasswordGoesToTheVaultAndIsNeverEchoed`).
3. **Graph không có khoá idempotency cho `sendMail`**: dùng thuộc tính mở rộng `ActonIdem` gắn lên thư; gửi lại sau khi mất phản hồi tìm thấy thư đã gửi, và một **bản nháp còn sót** từ lần chết giữa chừng được gửi nốt chứ không tạo bản thứ hai (`TestALostAnswerAfterSendingIsFoundByTheRetry`, `TestADraftLeftByACrashedAttemptIsFinishedNotDuplicated`); tạo sự kiện dùng `transactionId`.
4. **Tiêm OData** qua `thread_id` (dấu nháy được nhân đôi, `TestGetAsksForTextBodiesAndThreadsMapReplies`) và **toán tử KQL** trong tìm kiếm (`TestSearchDropsOperators`); **con trỏ giả là URL** bị từ chối (`TestListIsNewest...`).
5. **CalDAV**: id sự kiện không được thoát khỏi bộ lịch (`../`, `/`); sửa/xoá **một lần xuất hiện** của sự kiện lặp bị từ chối thay vì phá cả chuỗi; cập nhật dùng `If-Match` nên không đè thay đổi của người khác; tạo lại cùng khoá không đè bản người dùng đã sửa (`If-None-Match: *`); không tạo sự kiện có người tham dự mà không cho phép gửi lời mời, vì máy chủ tự gửi (`TestInvitationsAreNeverSentBehindTheCallersBack`).
6. **iCalendar độc**: giá trị chứa xuống dòng không bao giờ sinh dòng mới (`TestSetReplacesAndSerializeNeverEmitsInjectedLines`), đầu vào méo bị từ chối, `FuzzParseNeverPanics`.

## Mã tham chiếu đã kiểm chứng

Nhánh `wip/mailcal-ref-p4` (tách khỏi `wip/mailcal-ref-p3`). Tệp mới: `git checkout wip/mailcal-ref-p4 -- <tệp>`; tệp đã có: `git diff wip/mailcal-ref-p3 wip/mailcal-ref-p4 -- <tệp> | git apply --index`. Plugin Graph và CalDAV được kiểm với máy chủ giả qua HTTP thật, và cả hai biên dịch ra WebAssembly (~6 MB) và nạp qua `plugin.WasmLoader`. **Chưa có** mã tham chiếu cho nối dây `main.go`, web, e2e; và chưa kiểm trên Microsoft/CalDAV thật (xem Task 6).

## Không được đụng tới

Mọi thứ trong danh sách "cổng không đổi lõi" ở trên, cộng `acton_oauth` (chỉ đọc), `OAuthFlow`, `TokenBroker`, và golden ABI.

## Cấu trúc tệp

| Việc | Đường dẫn |
|:---|:---|
| Thông tin đăng nhập mật khẩu | `internal/plugin/host_api_account.go` (+test); sửa `types.go` (`Permissions.Credentials`), `security_gate.go` (`CheckCredentialsAccess`), `host_api.go` (module `acton_account`), `manager.go` (`SetCredentialBroker`) |
| Lõi mailcal | `internal/mailcal/credentials.go` (+test) |
| API | `internal/server/api_mailcal_password.go` (+test); sửa `router.go`, `api_mailcal.go` |
| Guest | `provkit/netio/netio.go` (`Login`, `Credentials`), `provkit/wasmguest/credentials.go`, `refplugin/netguest/main.go` (+ `wasmnet_test.go`) |
| iCalendar | `internal/mailcal/provkit/icskit/{icskit.go,icskit_test.go}` |
| Graph | `internal/mailcal/plugins/graph/{graph.go,calendar.go,manifest.json,graph_test.go,calendar_fake_test.go,calendar_test.go,wasm/main.go}`, `graphwasm_test.go` |
| CalDAV | `internal/mailcal/plugins/caldav/{caldav.go,manifest.json,caldav_test.go,wasm/main.go}`, `caldavwasm_test.go` |
| Nối dây, web, tài liệu | `cmd/actond/main.go`, `web/src/pages/Mailcal/AccountModal.tsx` (+test), locales, `Makefile`, `docs/PLUGINS-MAILCAL.md` |

## Task 1: Thông tin đăng nhập mật khẩu (`acton_account.get_credentials`)

**Files:** như bảng "Thông tin đăng nhập mật khẩu", "Lõi mailcal", "API", "Guest".

**Interfaces:**
- Produces: `plugin.CredentialBroker{Credentials(ctx, pluginID, accountID) (Credentials, error)}`, `plugin.Credentials{Username, Password}`, `ErrCredentialsForbidden`, `(*Manager).SetCredentialBroker`, `HostContext.Credentials`, mã `CredErrNoHost=-1`, `CredErrDenied=-2`, `CredErrUnavailable=-4`; WASM import `acton_account.get_credentials(ptr,len) -> i32` (độ dài JSON `{"username","password"}`, đọc bằng `acton_sys.read_response`); `mailcal.CredentialBroker` (`NewCredentialBroker(accounts, SecretStore)`, `Credentials`, `SetPassword`, `Forget`), `mailcal.PasswordKey(accountID)`; `PUT /api/mailcal/accounts/{id}/password {password}` ⇒ `{configured:true}`; `netio.Login`, `netio.Credentials`, `wasmguest.Credentials`.
- Quy tắc: `CheckCredentialsAccess` chạy **trước** khi đánh thức broker; tài khoản của plugin khác hoặc không tồn tại cho cùng kết quả `ErrCredentialsForbidden`; tên người dùng lấy từ `config.username`, nếu thiếu thì từ `email`; mật khẩu qua `rememberSecret`; `SetPassword` từ chối chuỗi rỗng; tài khoản `auth_expired` không được cấp.

- [ ] **Step 1: Test trước (đỏ)**

```bash
git checkout wip/mailcal-ref-p4 -- internal/plugin/host_api_account_test.go internal/mailcal/credentials_test.go internal/server/api_mailcal_password_test.go
go vet ./internal/plugin ./internal/mailcal ./internal/server 2>&1 | head -5
```
Expected: `undefined: CredentialBroker` v.v.

- [ ] **Step 2: Cài đặt**

```bash
git checkout wip/mailcal-ref-p4 -- internal/plugin/host_api_account.go internal/mailcal/credentials.go internal/server/api_mailcal_password.go internal/mailcal/provkit/wasmguest/credentials.go
git diff wip/mailcal-ref-p3 wip/mailcal-ref-p4 -- internal/plugin/types.go internal/plugin/security_gate.go internal/plugin/host_api.go internal/plugin/manager.go internal/server/router.go internal/server/api_mailcal.go internal/mailcal/provkit/netio/netio.go internal/mailcal/refplugin/netguest/main.go | git apply --index
git diff wip/mailcal-ref-p3 wip/mailcal-ref-p4 -- internal/mailcal/wasmnet_test.go | git apply --index
go build ./... && go test ./internal/plugin ./internal/mailcal ./internal/server -count=1 -run 'Credential|Password|AWasm' -v 2>&1 | grep -E '^(---|ok|FAIL)'
```
Expected: PASS: `TestCredentialsAreHandedOnlyToThePluginsOwnAccountsWithPermission`, `TestCredentialRefusals`, `TestPasswordsLiveInTheVaultAndNeverInTheAccountRecord`, `TestCredentialsNeverGoToAnotherPlugin`, `TestSetPasswordRejectsEmptyAndForgetDeletesTheSecret`, `TestMailcalPasswordGoesToTheVaultAndIsNeverEchoed`, `TestAWasmProviderReceivesCredentialsOnlyWithThePermission` (plugin WASM thật nhận tên người dùng và **chỉ độ dài** mật khẩu).

- [ ] **Step 3: Đột biến:** bỏ `h.Gate.CheckCredentialsAccess()` ⇒ test từ chối đỏ; trong `Credentials` bỏ `acct.PluginID != pluginID` ⇒ test sở hữu đỏ; trong `SetPassword` lưu mật khẩu vào `Config` ⇒ test vault đỏ. Hoàn nguyên.
- [ ] **Step 4: Commit** `feat(plugin): hand a plugin the login of its own password accounts`.

## Task 2: Nối dây, giao diện mật khẩu và tài liệu

**Files:** Modify `cmd/actond/main.go`, `web/src/lib/api/mailcal.ts`, `web/src/pages/Mailcal/AccountModal.tsx` (+test), `web/src/locales/{en,vi}/mailcal.json`, `docs/API.md`, `.agents/rules/source-registry.md`.

- [ ] **Step 1: `main.go`** (mã tham chiếu chưa có). Cạnh broker token của giai đoạn 3, **trước** khi nạp plugin:

```go
	var mailcalCreds *mailcal.CredentialBroker
	if mailcalSvc != nil {
		mailcalCreds = mailcal.NewCredentialBroker(mailcalSvc.Accounts(), vault)
		if pluginMgr != nil {
			pluginMgr.SetCredentialBroker(mailcalCreds)
		}
	}
```
và thêm `MailcalCredentials: mailcalCreds` vào `server.Config{...}`. Kiểm: `grep -n "SetCredentialBroker\|ScanAndLoadAll" cmd/actond/main.go` cho `SetCredentialBroker` đứng trước `ScanAndLoadAll` (cùng lý do với broker token: `HostContext` sao chép broker lúc nạp plugin).

- [ ] **Step 2: Web (test trước).** `MailcalProvider` thêm `auth_kind?: 'oauth' | 'password'` (lấy từ `ProviderInfo.AuthKind`, đã có trong nhánh tham chiếu: `"oauth"` khi manifest có khối `oauth`, `"password"` khi `permissions.credentials` là true; lấy bằng `git diff wip/mailcal-ref-p3 wip/mailcal-ref-p4 -- internal/mailcal/service.go | git apply --index` và `git checkout wip/mailcal-ref-p4 -- internal/mailcal/authkind_test.go`; `TestProvidersTellTheUIWhichSignInFormToShow`). Trong `AccountModal`, khi `auth_kind === 'password'`, hiển thị các ô **không phải bí mật**: `calendar_url`, `email`, `username` (đi vào `config`), và một ô `password` kiểu `password` **không** nằm trong `config` mà gọi `mailcalApi.putPassword(id, password)` sau khi tạo tài khoản. Thêm `putPassword: (id, password) => fetchJSON<{configured:boolean}>(`/mailcal/accounts/${encodeURIComponent(id)}/password`, {method:'PUT', body: JSON.stringify({password})})`. Test vitest: (a) với plugin mật khẩu, bấm Thêm gọi `createAccount` mà `config` **không** chứa khoá `password`, rồi `putPassword`; (b) sau khi lưu, ô mật khẩu bị xoá khỏi DOM; (c) `calendar_url` không phải `https` hiện lỗi trước khi gửi. Khoá locale `account.password*`, `account.calendarUrl*` ở `en` và `vi`.
- [ ] **Step 3: Kiểm**

```bash
go build ./... && go test ./cmd/... ./internal/server ./internal/mailcal -count=1
cd web && npx tsc --noEmit && npm run lint && npm run check:i18n && npm run audit:i18n && npx vitest run src/pages/Mailcal
```
- [ ] **Step 4: Tài liệu API** (`PUT /accounts/{id}/password`) + registry. **Commit** `feat(web): add a password sign-in for CalDAV style accounts`.

## Task 3: Plugin Microsoft Graph

**Files:** Create `internal/mailcal/plugins/graph/*` và `internal/mailcal/graphwasm_test.go` (xem bảng); Modify `Makefile` (`plugin-graph`).

**Interfaces:** `graph.Backend{HTTP, Tokens, APIBase}` implements `provkit.Backend`; hằng `idemProperty = "String {8c3e4a5f-3c2b-4f0e-9a43-5a1c2b7d9e11} Name ActonIdem"`.

Bảng ánh xạ (đã cài, đã kiểm):

| ABI | Graph | Ghi chú |
|:---|:---|:---|
| `mail_list` | `GET /me/mailFolders/{wellknown}/messages?$top=N+1&$skip=K&$orderby=receivedDateTime desc` | `N+1` hàng để biết có trang sau; con trỏ = `K+N` dạng số; **`unread_only` dùng `$filter=isRead eq false` và không có `$orderby`** (Graph từ chối kết hợp) rồi sắp xếp phía client |
| thư mục | `inbox, sentitems, drafts, deleteditems, junkemail, archive` | tên khác ⇒ `invalid` |
| `mail_search` | `GET /me/messages?$search="từ thường"` | một trang (Graph không cho `$skip` với `$search`); bỏ toán tử KQL, `NOT`, `-`, `*`, `:` |
| `mail_get` | `GET /me/messages/{id}` + `Prefer: outlook.body-content-type="text"` | Microsoft đổi HTML sang văn bản phía máy chủ |
| `mail_get_thread` | `GET /me/messages?$filter=conversationId eq '...'` | dấu `'` nhân đôi; `in_reply_to` ánh xạ từ header `In-Reply-To` sang id Graph trong cùng thread |
| `mail_send` | (1) tìm trong `sentitems` theo thuộc tính `ActonIdem` = khoá ⇒ replay; (2) tìm trong `drafts` ⇒ gửi nốt; (3) `POST /me/messages` (hoặc `createReply` rồi `PATCH` để ghi đè đúng nội dung đã duyệt) kèm thuộc tính; (4) `POST /me/messages/{id}/send` | **bắt buộc có khoá**; tiêu đề một dòng; dùng `Prefer: IdType="ImmutableId"` để id không đổi khi thư chuyển thư mục |
| `mail_mark`, `move` | `PATCH {isRead}`, `POST /move {destinationId}` | |
| `mail_delete` | `POST /move {destinationId:"deleteditems"}` | không bao giờ `DELETE` |
| `calendar_list` | `GET /me/calendarView?startDateTime&endDateTime` + `Prefer: outlook.timezone="UTC"` | đã khai triển sự kiện lặp; bỏ `isCancelled` |
| `calendar_create` | `POST /me/events` + `transactionId = khoá` | Graph tự khử trùng |
| `calendar_update`, `respond` | `PATCH`; `POST /events/{id}/accept|decline|tentativelyAccept {sendResponse:true}` | |
| `calendar_delete` | tổ chức viên có người tham dự và `send_updates` ⇒ `POST /cancel`; còn lại `DELETE` | |
| `calendar_free_busy` | `POST /me/calendar/getSchedule` | bỏ khoảng `free`; chuẩn hoá email |

- [ ] **Step 1: Test trước**

```bash
git checkout wip/mailcal-ref-p4 -- internal/mailcal/plugins/graph/graph_test.go internal/mailcal/plugins/graph/calendar_fake_test.go internal/mailcal/plugins/graph/calendar_test.go internal/mailcal/plugins/graph/manifest.json internal/mailcal/graphwasm_test.go
go vet ./internal/mailcal/plugins/graph 2>&1 | head -3
```
- [ ] **Step 2: Cài đặt và chạy**

```bash
git checkout wip/mailcal-ref-p4 -- internal/mailcal/plugins/graph/graph.go internal/mailcal/plugins/graph/calendar.go internal/mailcal/plugins/graph/wasm
go test ./internal/mailcal/plugins/graph -count=1 -v 2>&1 | grep -E '^(---|ok|FAIL)'
go test ./internal/mailcal -count=1 -run Graph -v 2>&1 | grep -E '^(---|ok|FAIL)'
```
Expected: PASS, gồm `TestSendingTheSameKeyTwiceSendsOnce`, `TestALostAnswerAfterSendingIsFoundByTheRetry`, `TestADraftLeftByACrashedAttemptIsFinishedNotDuplicated`, `TestMarkMoveDeleteNeverDestroy`, `TestCreateWithTheSameKeyMakesOneEventAndTimesGoOutAsUTC`, `TestDeleteCancelsOrganizedMeetingsOnlyWhenAttendeesAreToBeNotified`, `TestGraphPassesTheShapeConformanceSuite`, `TestGraphManifestAsksForOnlyWhatItNeeds`, `TestGraphWasmDeclaresItsCapabilities`.

- [ ] **Step 3: Đột biến (đã làm khi dựng thử, đều bị bắt):** bỏ nhân đôi dấu nháy trong `odataString` ⇒ `TestGetAsksForTextBodiesAndThreadsMapReplies` đỏ; đổi khoá tìm trong `sentitems` thành `key+"x"` ⇒ hai test idempotency đỏ. Hoàn nguyên.
- [ ] **Step 4: Makefile** `plugin-graph` (cùng mẫu `plugin-gmail`). **Commit** `feat(mailcal): add the Microsoft Graph provider plugin`.

## Task 4: `icskit` và plugin CalDAV

**Files:** Create `internal/mailcal/provkit/icskit/*`, `internal/mailcal/plugins/caldav/*`, `internal/mailcal/caldavwasm_test.go`; Modify `Makefile` (`plugin-caldav`).

**Interfaces:**
- `icskit.Parse(text) (*Component, error)`, `(*Component).Serialize() []byte` (CRLF, gập dòng 75 octet không cắt giữa ký tự UTF-8, **bỏ qua** mọi giá trị chứa `\r`/`\n` và tên thuộc tính chứa `\r \n : ;`), `Find/All/Get/Values/Set/Remove`, `Prop.Param`, `Escape/Unescape`, `ParseTime(Prop) (t, allDay, err)` (tôn trọng `TZID`, múi giờ lạ là lỗi, **không** lặng lẽ coi là UTC), `UTC(t)`, `Date(t)`; nhúng `time/tzdata` để `TZID` phân giải được trong WASM; giới hạn độ sâu 8 và 200 000 dòng.
- `caldav.Backend{HTTP netio.Doer, Creds netio.Credentials}` implements `provkit.Backend`; cấu hình tài khoản: `calendar_url` (bắt buộc, `https`), `email` (địa chỉ của chính người dùng, cần để trả lời lời mời), `username` (tuỳ chọn). Khai: `calendar@1` với `list/get/create/update/respond/delete`; **không** có `free_busy` và **không** có thư (trả `unsupported`).

Quy tắc đã cài và có test: `REPORT calendar-query` với `<c:expand>` trong khoảng thời gian (máy chủ khai triển sự kiện lặp), id sự kiện = tên tài nguyên; mỗi lần xuất hiện của sự kiện lặp có id `<tài nguyên>@<RECURRENCE-ID|DTSTART UTC>` và `get/update/delete` trên id đó bị từ chối (`invalid`); `create` dùng UID `acton-`+sha256(khoá)[:32], `PUT` với `If-None-Match: *` và đọc lại sự kiện (kể cả khi đã tồn tại, nên không đè bản người dùng sửa); `update` đọc `ETag`, giữ nguyên báo thức/múi giờ/PARTSTAT người khác, `PUT` với `If-Match`, `412` ⇒ `transient` ("đọc lại"); `respond` chỉ đổi PARTSTAT của đúng `config.email`; có người tham dự mà `send_updates=false` ⇒ `invalid` (máy chủ tự gửi lời mời theo RFC 6638, plugin không thể giữ im lặng).

- [ ] **Step 1: Test trước**

```bash
git checkout wip/mailcal-ref-p4 -- internal/mailcal/provkit/icskit/icskit_test.go internal/mailcal/plugins/caldav/caldav_test.go internal/mailcal/plugins/caldav/manifest.json internal/mailcal/caldavwasm_test.go
go vet ./internal/mailcal/provkit/icskit ./internal/mailcal/plugins/caldav 2>&1 | head -3
```
- [ ] **Step 2: Cài đặt, chạy, fuzz**

```bash
git checkout wip/mailcal-ref-p4 -- internal/mailcal/provkit/icskit/icskit.go internal/mailcal/plugins/caldav/caldav.go internal/mailcal/plugins/caldav/wasm
go test ./internal/mailcal/provkit/icskit ./internal/mailcal/plugins/caldav -count=1 -v 2>&1 | grep -E '^(---|ok|FAIL)'
go test ./internal/mailcal/provkit/icskit -run xxx -fuzz FuzzParseNeverPanics -fuzztime 30s
go test ./internal/mailcal -count=1 -run CalDAV -v 2>&1 | grep -E '^(---|ok|FAIL)'
```
Expected: PASS; fuzz không tìm lỗi.

- [ ] **Step 3: Đột biến (đã làm khi dựng thử):** bỏ `If-Match` ⇒ test xung đột đỏ; bỏ `If-None-Match: *` ⇒ `TestCreateIsIdempotentAndNeverLeavesTheCollection` đỏ (sau khi thêm ca "người dùng sửa rồi gửi lại"); cho `Serialize` ghi giá trị có xuống dòng ⇒ test tiêm đỏ. Hoàn nguyên.
- [ ] **Step 4: Manifest.** `manifest.json` liệt kê `caldav.fastmail.com`, `caldav.icloud.com`. Máy chủ tự dựng (Nextcloud, Radicale) cần máy chủ của chính người dùng trong `net_outbound`; vì manifest là tĩnh và có thể được ký, hướng dẫn người dùng sao chép thư mục plugin thành `mailcal-caldav-<tên>` với `id` khác, thêm host của họ vào `net_outbound`, và cài **không ký** (`ACTONOS_ALLOW_UNSIGNED_PLUGINS=1`). Ghi rõ lựa chọn này trong `docs/PLUGINS-MAILCAL.md`; không thêm `*` vào manifest chính thức.
- [ ] **Step 5: Makefile** `plugin-caldav`; **Commit** `feat(mailcal): add the CalDAV provider plugin and an iCalendar toolkit`.

## Task 5: Giao diện Microsoft

**Files:** không thêm tệp mới: `OAuthClientPanel`, `ConnectButton` của giai đoạn 3 đã tổng quát theo `oauth_provider`. Chỉ thêm khoá locale cho hướng dẫn riêng của Microsoft.

- [ ] **Step 1: Test trước:** trong `OAuthClientPanel.test.tsx` thêm ca `provider="microsoft"` hiển thị hướng dẫn đăng ký ứng dụng Entra (loại "Mobile and desktop"/"Web", `redirect_uri` hiển thị, quyền ủy quyền Graph được liệt kê) và gọi `putOAuthClient('microsoft', ...)`. Expected: FAIL.
- [ ] **Step 2: Cài đặt:** tra khoá `oauth.help.<provider>` trong locale (có `google`, `microsoft`; provider lạ rơi về hướng dẫn chung), không chuỗi cứng.
- [ ] **Step 3: Kiểm** `npm run check:i18n && npm run audit:i18n && npx vitest run src/pages/Mailcal`. **Commit** `feat(web): explain how to register a Microsoft app`.

## Task 6: Cổng giai đoạn 4

- [ ] **Step 1: Toàn bộ test và cổng "không đổi lõi"**

```bash
go build ./... && go vet ./... && go test ./internal/... ./evals/... ./cmd/... -count=1
go test -race -count=1 ./internal/mailcal/... ./internal/plugin ./internal/auth
git diff --stat wip/mailcal-ref-p3 HEAD -- internal/mailcal/writeops.go internal/mailcal/tools_write.go internal/mailcal/tools_read.go internal/mailcal/policy internal/mailcal/abi internal/tools
cd web && npm run lint && npm run check:i18n && npm run audit:i18n && npx vitest run && cd ..
```
Expected: xanh; lệnh `git diff --stat` **không in gì**. (Nếu giai đoạn 3 chưa được gộp bằng đúng các commit của `wip/mailcal-ref-p3`, so với commit gộp giai đoạn 3 trên `master`.)

- [ ] **Step 2: Chạy bộ tuân thủ cho mọi plugin trong một lệnh:** thêm test `internal/mailcal/allplugins_test.go` duyệt thư mục `plugins/*/manifest.json`, với mỗi plugin kiểm (a) manifest không có `*` trong `net_outbound`, không xin `secrets/storage/workspace/bus_events`; (b) module WASM nạp được và `Capabilities` hợp lệ theo `conformance.RunShape` phần không cần mạng; (c) mọi export khai trong `Ops` thật sự tồn tại trong module. Test này chặn plugin tương lai vô tình xin quá quyền. Expected: PASS cho `gmail`, `graph`, `caldav`.
- [ ] **Step 3: Nghiệm thu thủ công (người dùng làm, không tự động hoá, không đưa khoá vào tệp):**
  1. **Microsoft:** đăng ký ứng dụng trong Microsoft Entra (tài khoản cá nhân/cơ quan), thêm `redirect_uri` hiển thị trong ứng dụng, nhập `client_id` (và `secret` nếu loại Web), kết nối; chạy `RunShape` với tài khoản thử; gửi một thư có duyệt: **kiểm** (a) thuộc tính mở rộng `ActonIdem` còn trên thư trong "Sent Items" (nếu không, bước tìm lại vô hiệu: đổi sang tìm theo người nhận + tiêu đề + thời điểm), (b) id thư không đổi sau khi gửi nhờ `ImmutableId`, (c) `createReply` rồi `PATCH` giữ đúng chuỗi hội thoại.
  2. **CalDAV (Fastmail hoặc iCloud với mật khẩu ứng dụng):** tạo/sửa/xoá sự kiện, và kiểm máy chủ có thật sự **gửi lời mời** khi có người tham dự (giả định của `SendUpdates`); kiểm `expand` trả đúng các lần xuất hiện.
  3. Ghi kết quả vào mô tả PR.
- [ ] **Step 4: Tài liệu:** `docs/PLUGINS-MAILCAL.md` (các mục Microsoft và CalDAV, giới hạn: CalDAV không có free/busy của người khác và không sửa được một lần xuất hiện của sự kiện lặp; Graph `$search` một trang); `docs/SECURITY.md` (đường thông tin đăng nhập mật khẩu); `CHANGELOG.md`. **Commit** `docs: document the Microsoft and CalDAV providers`.

## Hoàn tác

`git revert` Task 6 → 1. Plugin gỡ khỏi `<data>/plugins/` thì tài khoản tương ứng báo "provider plugin is not loaded", không ảnh hưởng tài khoản khác. Mật khẩu trong vault được xoá khi xoá tài khoản; nếu hoàn tác Task 1 khi còn mật khẩu, chúng còn trong vault dưới khoá `mailcal.account.*` (vô hại, xoá qua API vault).

## Điều giai đoạn sau phải biết

- **IMAP/SMTP (GĐ5) dùng `acton_account.get_credentials`** cho mật khẩu và `provkit/mimekit` cho thư; đừng thêm đường mật khẩu thứ hai.
- `ProviderInfo.AuthKind` là điểm UI dựa vào để chọn biểu mẫu; plugin IMAP/SMTP là `password`.
- Mọi plugin phải qua `allplugins_test.go`; GĐ5 thêm plugin IMAP/SMTP thì test đó phải tự bao phủ nó mà không sửa.
- `provkit.Dispatch` và `BridgeProvider` đối xứng; nếu thêm trường vào ABI, thêm vào golden **và** kiểm chúng vẫn khứ hồi (`TestKitRoundTripPassesConformance`).
