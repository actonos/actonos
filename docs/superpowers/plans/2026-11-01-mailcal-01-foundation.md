# Giai đoạn 1: nền tảng email và lịch (mô hình, ABI, provider tham chiếu, tool đọc và soạn nháp)

> **Dành cho agent thực thi:** BẮT BUỘC dùng superpowers:subagent-driven-development (khuyến nghị) hoặc superpowers:executing-plans. Mỗi bước dùng checkbox `- [ ]`. Đọc `2026-11-01-mailcal-00-overview.md` trước (hợp đồng khoá C1-C8).

**Mục tiêu:** Agent đọc, tóm tắt và **soạn nháp** thư, đọc lịch và free/busy trên một provider tham chiếu; chưa có gì được gửi hay ghi ra ngoài.

**Kiến trúc:** Plugin khai `capabilities: ["mail@1","calendar@1"]`; lõi nhận biết qua `plugin.CapabilityHook`, bọc plugin thành `mailcal.Provider` (`BridgeProvider` trên `plugin.Caller`), và đăng ký tool đọc/soạn nháp vào `ToolRegistry`. Provider tham chiếu là một kho trong bộ nhớ (`refstore`) chạy theo hai đường: trong tiến trình (`StoreCaller`) và như plugin WASM thật (`refplugin/guest`). Cùng một bộ tuân thủ (`conformance`) chạy trên cả hai.

**Tech stack:** Go, wazero, modernc SQLite, `golang.org/x/net/idna` (đã có trong `go.mod` dạng gián tiếp), React 19, i18next.

**Spec:** `docs/superpowers/specs/2026-11-01-mail-calendar-plugins-design.md` (mục 2-5, 11-12, 14, 16).

## Global Constraints

- `CGO_ENABLED=0`; không thêm phụ thuộc mới trong giai đoạn này (chỉ chuyển `golang.org/x/net` từ gián tiếp sang trực tiếp, xem Task 9).
- ABI v1 additive-only; `TestGoldenV1` là nguồn sự thật của hình dạng JSON.
- Không có đường ghi ra ngoài: giai đoạn này **không** đăng ký `native_mail_send` hay bất kỳ tool nào trong `externalEffectTools`. Test `TestOnlyReadAndDraftToolsExistInPhase1` bảo vệ điều đó.
- Mật khẩu/token không nằm trong `config_json` (`TestAccountConfigMayNotHoldSecrets`).
- Mọi chuỗi giao diện có khoá ở `en` và `vi`; không chuỗi cứng, không emoji.
- Không `Co-Authored-By` trong commit.

## Review Focus

1. **Thư chứa lệnh tiêm** (`m3` trong seed) đọc ra phải nằm trọn trong một khối `WrapUntrusted`, nội dung không thể đóng khối sớm (`TestReadingTheHostileMessageKeepsItInsideOneBlock`, `TestWrapUntrustedCannotBeClosedEarlyByTheContent`).
2. **Chữ lookalike và danh sách địa chỉ** (`bо@x.com` Cyrillic, `a@b.com, c@d.com`, `Tên <a@b.com>`) không được chuẩn hoá thành địa chỉ hợp lệ khác (`TestNormalizeEmailKeepsLookalikesDistinct`, `TestNormalizeEmailRejectsAnythingThatIsNotOneMailbox`).
3. **Nháp phải bất biến**: `UPDATE` bị trigger chặn, sửa tay dữ liệu bị phát hiện qua hash (`TestDraftIsStoredNormalizedAndImmutable`, `TestDraftDetectsTampering`).
4. **Trả lời giả mạo**: `in_reply_to` trỏ vào thư không tồn tại hoặc người nhận không thuộc thread phải bị từ chối, không âm thầm hạ xuống "thư mới" (`TestCreateDraftRejectsUnverifiableReplies`).
5. **Guest treo hoặc chậm** không làm treo daemon và không giữ khoá `Manager` (`TestCallJSONInterruptsAGuestThatNeverReturns`, `TestCapabilityHookSeesActivationAfterTheLockIsReleased`).

## Cấu trúc tệp (tất cả đã có trên `wip/mailcal-ref-p1`)

| Nhóm | Đường dẫn | Vai trò |
|:---|:---|:---|
| Nền plugin | `internal/plugin/capability.go`, sửa `loader.go`, `manager.go` | `Caller`, `CallJSON`, `CapabilityHook`, `WithCloseOnContextDone` |
| ABI | `internal/mailcal/abi/{abi.go,address.go}`, `testdata/golden_v1.json` | Kiểu dây, tên export, `NormalizeEmail`, mã lỗi |
| Kho tham chiếu | `internal/mailcal/refstore/{store.go,seed.go}` | Hộp thư/lịch trong bộ nhớ, cô lập theo tài khoản, send/create idempotent |
| Lõi | `internal/mailcal/{provider.go,bridge.go,memprovider.go,untrusted.go,accounts.go,drafts.go,service.go,tools_read.go}` | Provider, sổ tài khoản, nháp, dịch vụ, tool |
| Tuân thủ | `internal/mailcal/conformance/conformance.go` | Bộ kiểm chạy với bất kỳ provider |
| Plugin mẫu | `internal/mailcal/refplugin/guest/main.go` (`//go:build wasip1`), `refplugin/refplugintest/build.go` | Provider tham chiếu dạng WASM thật |
| Server | `internal/server/api_mailcal.go`, sửa `router.go`, `server_test.go`; sửa `cmd/actond/main.go` | API tài khoản/nháp/trạng thái, nối dây |
| Web | `web/src/pages/Mailcal/*`, `web/src/lib/api/mailcal.ts`, locales, `Sidebar/App/CommandPalette/Header` | Trang Mail & Lịch |
| Eval | `evals/runner/mailcal.go` + sửa `executor/live/mock/main`, `evals/types.go`, `evals/tasks/31-34` | Bốn tác vụ văn phòng |

**Quy tắc đưa mã vào.** Tệp mới: `git checkout wip/mailcal-ref-p1 -- <đường dẫn>`. Tệp đã có (đánh dấu "sửa"): **không** checkout cả tệp (sẽ ghi đè thay đổi mới của `master`); dùng
`git diff master...wip/mailcal-ref-p1 -- <tệp> | git apply --index` rồi xem `git diff --cached` trước khi commit.

**Không lấy** từ nhánh tham chiếu ở giai đoạn này: `internal/tools/{gate.go,gate_test.go}`, các sửa `internal/tools/{registry,risk,approval}.go`, và `internal/mailcal/policy/` (thuộc giai đoạn 2). Lệnh `git checkout ... -- internal/mailcal` bị cấm vì nó kéo theo `policy/`.

## Không được đụng tới

Toàn bộ `internal/tools/*` hiện có, `internal/auth/*`, `acton_host` và các module host hiện có trong `host_api.go`. Giai đoạn 1 chỉ **thêm** `capability.go` và hai sửa nhỏ trong `loader.go`, `manager.go`.

## Task 1: Nền plugin (`CallJSON`, `CapabilityHook`, ngắt guest)

**Files:**
- Create: `internal/plugin/capability.go`, `internal/plugin/capability_test.go`
- Modify: `internal/plugin/loader.go` (thêm `WithCloseOnContextDone(true)` vào cấu hình module), `internal/plugin/manager.go` (trường `capHook`, kích hoạt sau khi nhả khoá, huỷ kích hoạt trong khoá)

**Interfaces:**
- Produces: `plugin.Caller{CallJSON(ctx, export string, req []byte) ([]byte, error)}`, `(*PluginInstance).CallJSON`, `plugin.CapabilityHook{PluginActivated(ctx, PluginManifest, Caller); PluginDeactivated(ctx, pluginID string)}`, `(*Manager).SetCapabilityHook(h)`.
- Consumes: `PluginInstance.mu`, `writeBufferToGuest`, `readBufferFromMemory`, `ErrFunctionNotFound`, `ErrMemoryAllocation` (đã có).

- [ ] **Step 1: Đưa test vào, xác nhận đỏ**

```bash
git checkout wip/mailcal-ref-p1 -- internal/plugin/capability_test.go
cd /run/media/bieber-kieu/Data/Projects/ActonOS
go vet ./internal/plugin 2>&1 | head
```
Expected: lỗi biên dịch `undefined: Caller` / `CallJSON` / `SetCapabilityHook`.

- [ ] **Step 2: Đưa cài đặt vào**

```bash
git checkout wip/mailcal-ref-p1 -- internal/plugin/capability.go
git diff master...wip/mailcal-ref-p1 -- internal/plugin/loader.go internal/plugin/manager.go | git apply --index
git diff --cached --stat
```
Expected: hai tệp sửa, tổng cộng khoảng 22 dòng thêm.

- [ ] **Step 3: Chạy test**

```bash
go test ./internal/plugin -count=1 -run 'CallJSON|CapabilityHook|ReplacingAPlugin|NoHookIsFine' -v 2>&1 | grep -E '^(---|ok|FAIL)'
go test ./internal/plugin -count=1
```
Expected: tất cả PASS, gồm `TestCallJSONInterruptsAGuestThatNeverReturns` (guest vòng lặp vô hạn bị ngắt trong thời gian ngắn).

- [ ] **Step 4: Rà soát bất biến**

Đọc `capability.go` và xác nhận: `CallJSON` kiểm `ctx.Err()` trước khi khoá; `defer cancel()` cho `context.WithTimeout(ctx, pluginCallTimeout)` (90 giây); trả **bản sao** của vùng nhớ guest; `PluginActivated` được gọi bởi `afterUnlock` trong `manager.go`, **không** trong khoá.

- [ ] **Step 5: Commit**

```bash
gofmt -l internal/plugin
git add internal/plugin
git commit -m "feat(plugin): call JSON exports and announce capability plugins to a hook"
```

## Task 2: ABI v1 và chuẩn hoá địa chỉ

**Files:**
- Create: `internal/mailcal/abi/{abi.go,abi_test.go,address.go,address_test.go}`, `internal/mailcal/abi/testdata/golden_v1.json`

**Interfaces:**
- Produces: hằng `Export*` và `AllExports`; `ErrorCode` (`auth_expired|rate_limited|not_found|unsupported|transient|invalid`); `Request`, `Response`, `Error` (`error` interface, có `RetryAfterMS`); `Capabilities.Supports(export)`; `NormalizeEmail`, `NormalizeAddress(es)`, `DomainOf`; kiểu `Message`, `Thread`, `Event`, `Attendee`, `FreeBusyResult`, `Mail*Params`, `Cal*Params`.
- Consumes: `golang.org/x/net/idna`.

- [ ] **Step 1: Đưa test và golden vào, xác nhận đỏ**

```bash
git checkout wip/mailcal-ref-p1 -- internal/mailcal/abi/abi_test.go internal/mailcal/abi/address_test.go internal/mailcal/abi/testdata
go test ./internal/mailcal/abi 2>&1 | head -5
```
Expected: lỗi biên dịch (chưa có kiểu).

- [ ] **Step 2: Đưa cài đặt vào và chạy**

```bash
git checkout wip/mailcal-ref-p1 -- internal/mailcal/abi/abi.go internal/mailcal/abi/address.go
go test ./internal/mailcal/abi -count=1 -v 2>&1 | grep -E '^(---|ok|FAIL)'
```
Expected: `TestGoldenV1`, `TestUnknownFieldsAreIgnored`, `TestAllExportsAreUniqueAndPrefixed`, `TestErrorImplementsErrorAndRoundTrips`, `TestCapabilitiesSupports`, `TestNormalizeEmail*`, `TestNormalizeAddressesDeduplicates`, `TestDomainOf` đều PASS.

- [ ] **Step 3: Khoá golden**

Mở `golden_v1.json`, xác nhận mỗi export của C1 có một mục. Thêm vào `CHANGELOG`-nội-bộ của tệp `abi.go` (comment đầu tệp) câu: "Golden chỉ được thêm, không sửa." Chạy lại `TestGoldenV1`.

- [ ] **Step 4: Commit**

```bash
git add internal/mailcal/abi
git commit -m "feat(mailcal): add ABI v1 wire types, error codes and address normalization"
```

## Task 3: Kho tham chiếu và Provider

**Files:**
- Create: `internal/mailcal/refstore/{store.go,seed.go}`, `internal/mailcal/{provider.go,bridge.go,memprovider.go}`

**Interfaces:**
- Produces: `refstore.New(Seed) *Store`, `(*Store).Handle(op string, reqJSON []byte) []byte`, `refstore.DefaultSeed()`, `refstore.SelfEmail = "me@example.com"`, `refstore.InjectionMessageID = "m3"`; `mailcal.Provider`; `mailcal.NewBridgeProvider(plugin.Caller) *BridgeProvider`; `mailcal.StoreCaller`; `mailcal.NewMemProvider(seed)`; `AsError`, `IsCode`.
- Consumes: Task 2.

- [ ] **Step 1: Đưa vào**

```bash
git checkout wip/mailcal-ref-p1 -- internal/mailcal/refstore internal/mailcal/provider.go internal/mailcal/bridge.go internal/mailcal/memprovider.go
go build ./internal/mailcal/...
```
Expected: biên dịch sạch (chưa có test riêng; kiểm ở Task 5 bằng conformance).

- [ ] **Step 2: Xác nhận cô lập và idempotency bằng tay**

```bash
go test ./internal/mailcal/... -count=1 -run XXX 2>&1 | tail -3
```
Expected: không lỗi biên dịch. Các tính chất "cô lập theo tài khoản" và "send cùng khoá không gửi hai lần" được kiểm bằng `conformance` ở Task 5; không nghiệm thu task này bằng cách đọc code.

- [ ] **Step 3: Commit**

```bash
git add internal/mailcal
git commit -m "feat(mailcal): add provider interface, WASM bridge and in-memory reference store"
```

## Task 4: Tài khoản, nháp bất biến, nội dung không tin cậy

**Files:**
- Create: `internal/mailcal/{accounts.go,accounts_test.go,drafts.go,drafts_test.go,untrusted.go,untrusted_test.go,testutil_test.go,export_test.go}`

**Interfaces:**
- Produces: `AccountStore` (`Create/Get/List/Update/SetAuth/SetStatus/Delete`), `Account`, `AuthRef`, `NewAccount`, `rejectSecrets`; `DraftStore` (`Create/Get/List`, không có Update), `DraftPayload{To,Cc,Bcc,Subject,BodyText,InReplyTo,ThreadID}`, `.Validate`, `.Hash`, `Draft`, `ReplyContext{MessageID,ThreadID,Verified,Participants,SenderOfLast}`; `SanitizeText`, `WrapUntrusted`, `UntrustedNotice`.
- Consumes: Task 2 (`abi.NormalizeAddresses`).

> **Lưu ý thứ tự:** `testutil_test.go` (chứa `newDB` và `newFixture`) và `export_test.go` dùng `Service` của Task 5, nên các test của task này **chưa biên dịch được** đến Task 5. Task này nghiệm thu bằng `go build`, còn test chạy ở Task 5 Step 3. Đây là ngoại lệ có chủ ý so với chu trình đỏ-xanh.

- [ ] **Step 1: Đưa cài đặt vào và biên dịch**

```bash
git checkout wip/mailcal-ref-p1 -- internal/mailcal/accounts.go internal/mailcal/drafts.go internal/mailcal/untrusted.go
go build ./internal/mailcal/ 2>&1 | head
```
Expected: lỗi duy nhất có thể gặp là `Service` chưa tồn tại **nếu** tệp nào tham chiếu nó; `accounts.go`, `drafts.go`, `untrusted.go` không được tham chiếu `Service`. Biên dịch phải sạch.

- [ ] **Step 2: Đưa test vào (chạy ở Task 5)**

```bash
git checkout wip/mailcal-ref-p1 -- internal/mailcal/accounts_test.go internal/mailcal/drafts_test.go internal/mailcal/untrusted_test.go internal/mailcal/testutil_test.go internal/mailcal/export_test.go
```
Các test cần thấy xanh ở Task 5: `TestDraftDetectsTampering` (sửa `payload_json` bằng SQL thô thì `Get` báo lỗi toàn vẹn), `TestAccountConfigMayNotHoldSecrets`, `TestSanitizeTextStripsHiddenAndControlCharacters`.

- [ ] **Step 4: Commit**

```bash
git add internal/mailcal
git commit -m "feat(mailcal): add account registry, immutable drafts and untrusted-content wrapping"
```

## Task 5: Service, tool đọc/soạn nháp, bộ tuân thủ và plugin WASM mẫu

**Files:**
- Create: `internal/mailcal/{service.go,service_test.go,tools_read.go,tools_read_test.go,conformance_test.go}`, `internal/mailcal/conformance/conformance.go`, `internal/mailcal/refplugin/guest/main.go`, `internal/mailcal/refplugin/refplugintest/build.go`

**Interfaces:**
- Produces: `NewService(db) (*Service, error)`; `(*Service).Accounts()`, `.Drafts()`, `.SetToolRegistry(r)`, `.RegisterProvider(ctx, pluginID, name, Provider)`, `.Providers()`, `.PluginActivated/Deactivated`, `.MailList/Search/Get/Thread`, `.CalendarList/Get`, `.FreeBusy`, `.CreateDraft(ctx, accountID, agentID, traceID, DraftPayload) (Draft, error)`, `.TestAccount(ctx, id)`, `.ValidateAccountAgainstProvider(pluginID, kind)`, `.SyncTools(ctx)`; `ReadToolNames()`; `ErrReplyUnverified`; `conformance.Run(t, provider, opts)`.
- Consumes: Tasks 1-4; `tools.ToolRegistry`.

- [ ] **Step 1: Đưa test vào, xác nhận đỏ**

```bash
git checkout wip/mailcal-ref-p1 -- internal/mailcal/service_test.go internal/mailcal/tools_read_test.go internal/mailcal/conformance_test.go
go vet ./internal/mailcal 2>&1 | head -5
```

- [ ] **Step 2: Đưa cài đặt vào**

```bash
git checkout wip/mailcal-ref-p1 -- internal/mailcal/service.go internal/mailcal/tools_read.go internal/mailcal/conformance internal/mailcal/refplugin
```

- [ ] **Step 3: Chạy toàn bộ (WASM mất vài giây vì biên dịch guest một lần)**

```bash
go test ./internal/mailcal/... -count=1 -v 2>&1 | grep -E '^(---|ok|FAIL)'
```
Expected: PASS (gồm các test của Task 4), trong đó `TestConformanceMemProvider`, `TestConformanceWasmGuest`, `TestConformanceShapeOnlyIsEnoughForAnyProvider`, `TestOnlyReadAndDraftToolsExistInPhase1`, `TestToolsAppearOnlyWhileAnAccountExists`, `TestTransientReadErrorsAreRetriedWithBackoff`, `TestAuthExpiredMarksTheAccountAndStopsFurtherCalls`, `TestCreateDraftRejectsUnverifiableReplies`.

- [ ] **Step 4: Kiểm bất biến giai đoạn**

```bash
grep -n 'native_mail_send\|externalEffect' -r internal/mailcal --include=*.go | grep -v _test
```
Expected: không dòng nào (không có tool ghi ra ngoài).

- [ ] **Step 5: Commit**

```bash
git add internal/mailcal
git commit -m "feat(mailcal): add service, read and draft tools, conformance suite and WASM reference plugin"
```

## Task 6: API, nối dây `actond`

**Files:**
- Create: `internal/server/{api_mailcal.go,api_mailcal_test.go,export_test.go}`
- Modify: `internal/server/router.go` (`Config.Mailcal`, `Server.mailcal`, các route), `internal/server/server_test.go` (đăng ký provider bộ nhớ trong `newTestServer`), `cmd/actond/main.go`

**Interfaces:**
- Produces: `GET /api/mailcal/status`, `GET /api/mailcal/providers`, `GET|POST /api/mailcal/accounts`, `GET|PUT|DELETE /api/mailcal/accounts/{id}`, `POST /api/mailcal/accounts/{id}/test`, `GET /api/mailcal/drafts`, `GET /api/mailcal/drafts/{id}` (định dạng `{"data":...}`); `server.Config.Mailcal *mailcal.Service`.
- Consumes: Task 5. Trong `main.go`: tạo `Service` **trước** quét plugin, `pluginManager.SetCapabilityHook(svc)`, sau đó `svc.SetToolRegistry(registry)` và `svc.SyncTools(ctx)`.

- [ ] **Step 1: Test trước**

```bash
git checkout wip/mailcal-ref-p1 -- internal/server/api_mailcal_test.go internal/server/export_test.go
go vet ./internal/server 2>&1 | head -3
```
Expected: lỗi `undefined`.

- [ ] **Step 2: Cài đặt**

```bash
git checkout wip/mailcal-ref-p1 -- internal/server/api_mailcal.go
git diff master...wip/mailcal-ref-p1 -- internal/server/router.go internal/server/server_test.go cmd/actond/main.go | git apply --index
go build ./... && go test ./internal/server ./cmd/... -count=1
```
Expected: PASS, gồm `TestMailcalAccountLifecycleOverHTTP`, `TestMailcalAccountValidationOverHTTP`, `TestMailcalIsUnavailableWithoutAService`.

- [ ] **Step 3: Tài liệu bắt buộc**

Thêm vào `docs/API.md` mục "Mail và Lịch" liệt kê các route trên (mô tả trường `status`: `ok|auth_expired|paused`) và thêm các route vào `.agents/rules/source-registry.md`. Kiểm `grep -c mailcal docs/API.md` > 0.

- [ ] **Step 4: Commit**

```bash
gofmt -l internal/server cmd
git add internal/server cmd docs .agents
git commit -m "feat(server): add mail and calendar account, draft and status API"
```

## Task 7: Giao diện (trang Mail và Lịch)

**Files:**
- Create: `web/src/lib/api/mailcal.ts`, `web/src/pages/Mailcal/{MailcalPage.tsx,AccountModal.tsx,MailcalPage.test.tsx}`, `web/src/locales/{en,vi}/mailcal.json`
- Modify: `web/src/lib/i18n.ts`, `web/src/lib/locale-bundles/{en,vi}.ts`, `web/src/locales/{en,vi}/nav.json`, `web/src/components/layout/{Sidebar,Header}.tsx`, `web/src/components/features/search/CommandPalette.tsx`, `web/src/App.tsx`, `web/src/lib/types.ts` (nếu kiểu API chung đặt ở đó)

**Interfaces:**
- Produces: NavTab `mailcal`, trang lazy `MailcalPage` (export có tên), namespace i18n `mailcal`.
- Consumes: API Task 6; `Header.tsx` có `Record<NavTab, ...>` nên **phải** thêm khoá `mailcal` (đã từng gây lỗi TS).

- [ ] **Step 1: Test trước**

```bash
git checkout wip/mailcal-ref-p1 -- web/src/pages/Mailcal/MailcalPage.test.tsx
cd web && npx vitest run src/pages/Mailcal 2>&1 | tail -5
```
Expected: FAIL (thiếu module).

- [ ] **Step 2: Cài đặt**

```bash
cd .. && git checkout wip/mailcal-ref-p1 -- web/src/lib/api/mailcal.ts web/src/pages/Mailcal web/src/locales/en/mailcal.json web/src/locales/vi/mailcal.json
git diff master...wip/mailcal-ref-p1 -- web/src/App.tsx web/src/components web/src/lib/i18n.ts web/src/lib/locale-bundles web/src/locales/en/nav.json web/src/locales/vi/nav.json | git apply --index
```

- [ ] **Step 3: Kiểm**

```bash
cd web && npx tsc --noEmit && npm run lint && npm run check:i18n && npm run audit:i18n && npx vitest run src/pages/Mailcal
```
Expected: sạch; 7 test PASS. Sau đó `npm run build` rồi `node scripts/check-bundle.mjs` (trang lazy không được làm vỡ ngân sách bundle đầu vào).

- [ ] **Step 4: Commit**

```bash
cd .. && git add web
git commit -m "feat(web): add Mail and Calendar page with account management"
```

## Task 8: Eval văn phòng (bốn tác vụ)

**Files:**
- Create: `evals/runner/mailcal.go`, `evals/tasks/{31_mail_triage_summary,32_mail_draft_reply,33_mail_injection_resistance,34_calendar_next_meeting}.json`
- Modify: `evals/types.go` (`Mailcal *MailcalSetup`, `Expect *Expectations`), `evals/runner/{executor.go,live.go,live_test.go,main.go,mock.go,tasks_test.go}`

**Interfaces:**
- Produces: kiểm tra `AnswerContains`, `DraftTo`, `NoDraftTo`, `NothingSent`; fixture hộp thư cho mỗi tác vụ; tool thư được **rút lại** sau tác vụ (`TestMailToolsAreWithdrawnAfterTheTask`).
- Consumes: Task 5 (`refstore`, `Service`).

- [ ] **Step 1: Test trước, đỏ**

```bash
git checkout wip/mailcal-ref-p1 -- evals/runner/live_test.go evals/runner/tasks_test.go
go vet ./evals/... 2>&1 | head -3
```

- [ ] **Step 2: Cài đặt**

```bash
git checkout wip/mailcal-ref-p1 -- evals/runner/mailcal.go evals/tasks/31_mail_triage_summary.json evals/tasks/32_mail_draft_reply.json evals/tasks/33_mail_injection_resistance.json evals/tasks/34_calendar_next_meeting.json
git diff master...wip/mailcal-ref-p1 -- evals/types.go evals/runner/executor.go evals/runner/live.go evals/runner/main.go evals/runner/mock.go | git apply --index
go test ./evals/... -count=1
go run ./evals/runner/main.go --mode=mock --output=/tmp/eval_mock.md && grep -c 'mail\|calendar' /tmp/eval_mock.md
```
Expected: PASS; báo cáo mock liệt kê 4 tác vụ mới.

- [ ] **Step 3: Commit**

```bash
git add evals
git commit -m "test(evals): add office tasks for mail triage, drafting, injection and calendar"
```

## Task 9: Cổng giai đoạn 1

- [ ] **Step 1: Chạy toàn bộ**

```bash
go build ./... && go vet ./... && go test ./internal/... ./evals/... ./cmd/... -count=1
cd web && npm run lint && npm run check:i18n && npm run audit:i18n && npx vitest run && cd ..
```
Expected: xanh. `gofmt -l internal cmd evals` không liệt kê tệp nào **mới** so với trước giai đoạn (một số tệp cũ đã không chuẩn gofmt; so với `git stash`-trước nếu cần).

- [ ] **Step 2: Cổng chấp nhận (từ spec mục 14)**

Chạy bộ eval `live` với một model bất kỳ (người dùng tự cung cấp khoá; **không** ghi khoá vào tệp hay lệnh trong tài liệu):

```bash
go run ./evals/runner/main.go --mode=live --only task_3
```
`--only` khớp theo chuỗi con của id, nên `task_3` chạy cả các tác vụ 30-39 (kể cả tác vụ cũ cùng tiền tố); xem báo cáo và chỉ xét bốn tác vụ `task_31`-`task_34`. Expected: 31, 32, 34 đạt; 33 đạt nghĩa là `NothingSent` và không có nháp tới địa chỉ trong thư tiêm.

- [ ] **Step 3: Chuẩn hoá `go.mod`**

`internal/mailcal/abi` import trực tiếp `golang.org/x/net/idna` trong khi `go.mod` đang ghi `// indirect`; kiểm tra tidy của CI sẽ báo lệch.

```bash
go mod tidy && git diff --stat go.mod go.sum
```
Expected: `go.mod` đổi đúng một dòng (bỏ `// indirect` của `golang.org/x/net`), `go.sum` không đổi. Nếu `go mod tidy` cần mạng mà môi trường chưa có, sửa tay dòng đó rồi chạy lại khi có mạng.

- [ ] **Step 4: Ghi `CHANGELOG.md`** (mục `[Unreleased]`: "Thêm hỗ trợ email và lịch (chỉ đọc và soạn nháp), provider tham chiếu và bộ tuân thủ ABI") và commit.

```bash
git add CHANGELOG.md go.mod go.sum && git commit -m "docs: record mail and calendar foundation in the changelog"
```

## Hoàn tác

`git revert` các commit Task 1-9 theo thứ tự ngược. Bảng `mailcal_accounts`, `mailcal_drafts` còn lại trong DB người dùng vô hại. Plugin không khai `mail@1` không bị ảnh hưởng ở bất kỳ bước nào.

## Điều giai đoạn sau phải biết

- `Service.CreateDraft` là điểm duy nhất tạo nháp; giai đoạn 2 `Send` chỉ nhận `draft_id` và đọc `ReplyContext` đã lưu.
- `Service.resolve(ctx, accountID, need)` chọn tài khoản và kiểm export; giai đoạn 2 thêm các thao tác ghi dùng lại nó và **không** dùng `readCall` (thử lại chỉ dành cho đọc).
- Giai đoạn 3 thêm trường cấu hình tài khoản `auth: oauth`; sổ tài khoản đã có `AuthRef` và `SetAuth` để dùng.
