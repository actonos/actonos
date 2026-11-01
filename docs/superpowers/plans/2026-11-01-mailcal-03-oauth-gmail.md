# Giai đoạn 3: `host_get_token`, đăng nhập OAuth do lõi giữ, plugin Gmail và Google Calendar

> **Dành cho agent thực thi:** BẮT BUỘC dùng superpowers:subagent-driven-development (khuyến nghị) hoặc superpowers:executing-plans. Mỗi bước dùng checkbox `- [ ]`. Đọc `2026-11-01-mailcal-00-overview.md` trước. **Điều kiện bắt đầu:** giai đoạn 2 đã gộp vào `master` và cổng của nó xanh.

**Mục tiêu:** Người dùng kết nối một tài khoản Google thật (OAuth client do chính họ mang tới), và agent đọc, soạn, gửi thư và thao tác lịch qua plugin WebAssembly, với mọi quy tắc giai đoạn 2 giữ nguyên.

**Kiến trúc:** Lõi chạy luồng OAuth (PKCE + `state` dùng một lần + cookie nonce), giữ refresh token trong kho `oauth_tokens` được mã hoá bằng vault dưới khoá `mailcal:<account_id>`, và chỉ trao **access token ngắn hạn** cho đúng plugin sở hữu tài khoản qua module host **mới** `acton_oauth.get_token`. Plugin được viết bằng bộ công cụ chung `provkit` (typed `Backend` → JSON dispatch, `netio`, `mimekit`, `wasmguest`) nên Giai đoạn 4 và 5 dùng lại.

**Tech stack:** Go (wazero, `GOOS=wasip1 -buildmode=c-shared`), Gmail REST và Google Calendar REST, React 19.

**Spec:** `docs/superpowers/specs/2026-11-01-mail-calendar-plugins-design.md` (mục 4, 9, 10, 14).

## Global Constraints

- Hợp đồng khoá C1-C8 (tệp tổng quan). Giai đoạn này **chỉ thêm**: trường manifest `oauth`, quyền `permissions.oauth`, module host `acton_oauth`. Không sửa `acton_host`, `acton_net`, `acton_sys`, `acton_vault`.
- Refresh token **không bao giờ** vào bộ nhớ WASM, log, tin nhắn lỗi, URL hay JSON trả về từ API.
- `state` OAuth dùng một lần; callback là route **công khai** vì cookie phiên là `SameSite=Strict` (trình duyệt không gửi nó khi từ miền khác quay về); bảo vệ bằng `state` + PKCE S256 + cookie nonce `SameSite=Lax` gắn với đúng trình duyệt đã bắt đầu.
- `token_url`, `auth_url` do manifest plugin khai: bắt buộc `https`, không có thông tin đăng nhập nhúng, và `token_url` phải qua `security.ValidateOutboundURL` (lõi tự gọi nó, nên manifest độc hại không được trỏ vào mạng nội bộ).
- Plugin Gmail không xin quyền `secrets`, `storage`, `workspace`, `bus_events`, không dùng `*` trong `net_outbound`, không xin phạm vi `https://mail.google.com/` (xoá vĩnh viễn).
- Client id/secret của người dùng nằm trong vault dưới `mailcal.oauth.<provider>.client_id|client_secret`; API **không bao giờ** trả lại giá trị, chỉ trả `configured: true|false`.
- Giao diện: khoá `en` và `vi`; không chuỗi cứng; không emoji.
- Không `Co-Authored-By`.

## Review Focus

1. **Plugin xin token của tài khoản người khác**: `TokenBroker` trả `ErrTokenForbidden` cho plugin không sở hữu tài khoản và cho tài khoản không tồn tại (cùng một kết quả, không lộ sự tồn tại) (`TestBrokerNeverServesAnotherPluginsAccount`, `TestOAuthGetTokenRefusals`).
2. **Plugin không khai `permissions.oauth`** không bao giờ đánh thức broker (`oauthGetToken` từ chối trước khi gọi).
3. **Login CSRF qua callback công khai** (kẻ tấn công gửi nạn nhân URL callback với `code` và `state` của kẻ tấn công): thiếu cookie nonce thì thất bại, và `state` đã bị tiêu thụ nên không dùng lại được; không có yêu cầu đổi token nào được gửi (`TestSignInRejectsAForeignBrowserAndCannotBeReplayed`).
4. **Manifest độc hại trỏ `token_url` vào LAN** hoặc dùng `http://` (`TestTokenEndpointsMustBeHTTPSAndPublic`).
5. **Hai luồng cùng làm mới token** (xoay refresh token): chỉ một yêu cầu tới nhà cung cấp (`TestRefreshNowRefreshesOnceForConcurrentCallers`, chạy với `-race`).
6. **Gmail không có khoá idempotency**: gửi lại sau khi mất phản hồi không được tạo thư thứ hai (`TestALostAnswerAfterSendingIsFoundAgainByTheRetry`); **tìm kiếm không bị mở rộng bằng toán tử** (`in:anywhere`, `label:`) (`TestSearchCannotBeWidenedByOperators`); **HTML ẩn lệnh** bị bỏ (`TestGetUsesPlainText...`); **tiêm header** qua tiêu đề (`TestSendRefusesWithoutAKeyAndNeverInjectsHeaders`, `TestBuildRejectsHeaderInjection`).
7. **Tài khoản bị thu hồi quyền**: `auth_expired` được ghi trên tài khoản và các lần gọi sau không dồn dập gọi nhà cung cấp (`TestBrokerRenewsNearExpiryAndMarksRevokedLogins`).

## Mã tham chiếu đã kiểm chứng

Nhánh `wip/mailcal-ref-p3` chứa mọi mã Go của giai đoạn này đã chạy xanh (kể cả `-race` cho `internal/auth`), gồm cả plugin Gmail biên dịch ra WebAssembly (~6 MB) và nạp thật qua `plugin.WasmLoader`. Cách đưa vào: tệp mới `git checkout wip/mailcal-ref-p3 -- <tệp>`; tệp đã có `git diff wip/mailcal-ref-p2 wip/mailcal-ref-p3 -- <tệp> | git apply --index`. **Chưa có** mã tham chiếu cho: nối dây `cmd/actond/main.go`, web (trang đăng nhập OAuth), e2e, và kiểm thử trên Google thật; kế hoạch dưới đây chứa mã đầy đủ cho các phần đó.

## Không được đụng tới

- ABI v1, golden, `Provider`, `Caller`, `CapabilityHook`.
- `internal/tools/gate.go`, `policy/`, `writeops.go`: Gmail **không** có đường gửi nào ngoài `Service.Send`. Plugin không biết gì về luật.
- Các module host `acton_host`, `acton_net`, `acton_sys`, `acton_vault`, `acton_storage`, `acton_bus`, `acton_ws`, `acton_workspace`.
- `StateStore.Consume` (một lần dùng) và `OAuthEngine.ExchangeCode`: chỉ **thêm** `NewOAuthEngineWithClient`.

## Cấu trúc tệp

| Việc | Đường dẫn |
|:---|:---|
| Manifest + quyền + host API | sửa `internal/plugin/{types.go,security_gate.go,host_api.go,manager.go}`; tạo `host_api_oauth.go` (+test) |
| Làm mới token tức thì | sửa `internal/auth/{token_refresher.go,oauth2.go}`; tạo `refresh_now_test.go` |
| Broker, luồng OAuth, kho client | `internal/mailcal/{tokens.go,oauth.go,clients.go,oauth_test.go}`; sửa `service.go` (`OAuthSpec`) |
| API | `internal/server/api_mailcal_oauth.go` (+test); sửa `router.go` |
| Bộ công cụ nhà cung cấp | `internal/mailcal/provkit/{provkit.go,netio/,mimekit/,wasmguest/}` |
| Plugin mẫu dò mạng | `internal/mailcal/refplugin/netguest/main.go`; sửa `refplugintest/build.go` (`BuildGuest`, `InstanceOf`) |
| Plugin Gmail | `internal/mailcal/plugins/gmail/{gmail.go,calendar.go,manifest.json,wasm/main.go}` + test |
| Nối dây | sửa `cmd/actond/main.go` |
| Web | `web/src/pages/Mailcal/{OAuthClientPanel,ConnectButton}.tsx`, sửa `AccountModal.tsx`, `MailcalPage.tsx`, `lib/api/mailcal.ts`, locales |
| Gói/cài đặt | `Makefile` (`make plugin-gmail`), `docs/PLUGINS-MAILCAL.md` |

## Task 1: Quyền `oauth` và module host `acton_oauth.get_token`

**Files:**
- Create: `internal/plugin/host_api_oauth.go`, `internal/plugin/host_api_oauth_test.go`
- Modify: `internal/plugin/types.go` (`PluginPermissions.OAuth`, `PluginManifest.OAuth *PluginOAuth`, kiểu `PluginOAuth`), `internal/plugin/security_gate.go` (`ErrOAuthUnauthorized`, `CheckOAuthAccess`), `internal/plugin/host_api.go` (`HostContext.Tokens`, đăng ký module `acton_oauth` **trước** `acton_host`), `internal/plugin/manager.go` (`tokens`, `HostContext.Tokens`)

**Interfaces:**
- Produces: `plugin.TokenBroker{AccessToken(ctx, pluginID, accountID string) (TokenGrant, error)}`, `TokenGrant{Provider, AccessToken, ExpiresAt}`, `ErrTokenForbidden`, `ErrTokenAuthExpired`, `(*Manager).SetTokenBroker(b)`, hằng `OAuthErrNoHost=-1`, `OAuthErrDenied=-2`, `OAuthErrAuthExpired=-3`, `OAuthErrUnavailable=-4`; host import WASM `acton_oauth.get_token(ptr,len) -> i32` (độ dài JSON `{"access_token","expires_at"}` rồi đọc bằng `acton_sys.read_response`).
- Quy tắc: id plugin lấy từ `HostContext.PluginID`, **không** từ guest; `accountID` tối đa 256 byte; token được `rememberSecret` để bị che khỏi log của plugin; lỗi từ chối trả mã âm và để `LastResponse` rỗng.

- [ ] **Step 1: Test trước (đỏ)**

```bash
git checkout wip/mailcal-ref-p3 -- internal/plugin/host_api_oauth_test.go
go vet ./internal/plugin 2>&1 | head -3
```
Expected: `undefined: TokenBroker` v.v.

- [ ] **Step 2: Cài đặt**

```bash
git checkout wip/mailcal-ref-p3 -- internal/plugin/host_api_oauth.go
git diff wip/mailcal-ref-p2 wip/mailcal-ref-p3 -- internal/plugin/types.go internal/plugin/security_gate.go internal/plugin/host_api.go internal/plugin/manager.go | git apply --index
gofmt -l internal/plugin/host_api_oauth.go
go test ./internal/plugin -count=1
```
Expected: PASS, gồm `TestOAuthGetTokenHandsOutOnlyTheAccessToken`, `TestOAuthGetTokenRefusals` (6 ca), `TestOAuthGetTokenRejectsAbsurdAccountIDs`, và toàn bộ test plugin cũ (chứng minh plugin cũ không vỡ).

- [ ] **Step 3: Đột biến**

Trong `oauthGetToken` đổi `h.Tokens.AccessToken(ctx, h.PluginID, ...)` thành `h.Tokens.AccessToken(ctx, "gmail", ...)`: Expected test `TestOAuthGetTokenHandsOutOnlyTheAccessToken` đỏ (`broker saw plugin=...`). Hoàn nguyên. Xoá dòng `h.rememberSecret(...)`: Expected test log đỏ. Hoàn nguyên.

- [ ] **Step 4: Commit**

```bash
git add internal/plugin
git commit -m "feat(plugin): add oauth permission and the acton_oauth.get_token host call"
```

## Task 2: `TokenRefreshDaemon.RefreshNow`

**Files:**
- Modify: `internal/auth/token_refresher.go` (trường `refreshLocks sync.Map`, phương thức `RefreshNow`), `internal/auth/oauth2.go` (`NewOAuthEngineWithClient`)
- Create: `internal/auth/refresh_now_test.go`

**Interfaces:**
- Produces: `(*TokenRefreshDaemon).RefreshNow(ctx, provider string) (*StoredTokenData, error)`: khoá theo provider, đọc lại token sau khi có khoá (người đến sau dùng kết quả của người đến trước), lỗi bọc `ErrTokenRefresh` khi nhà cung cấp từ chối (ví dụ `invalid_grant`); `NewOAuthEngineWithClient(states, *http.Client)`.
- Consumes: `OAuthEngine.RefreshToken`, `SaveToken`, `GetToken`.

- [ ] **Step 1-3:**

```bash
git checkout wip/mailcal-ref-p3 -- internal/auth/refresh_now_test.go
go vet ./internal/auth 2>&1 | head -3        # đỏ: RefreshNow chưa có
git diff wip/mailcal-ref-p2 wip/mailcal-ref-p3 -- internal/auth/token_refresher.go internal/auth/oauth2.go | git apply --index
go test ./internal/auth -count=1 -race
```
Expected: PASS: `TestRefreshNowRefreshesOnceForConcurrentCallers` (6 goroutine, máy chủ giả đếm đúng 1 lần gọi), `TestRefreshNowSkipsAFreshToken`, `TestRefreshNowReportsARevokedGrant`.

- [ ] **Step 4: Đột biến:** bỏ `mu.Lock()` ở đầu `RefreshNow`: `TestRefreshNowRefreshesOnceForConcurrentCallers` phải đỏ (hoặc `-race` báo). Hoàn nguyên.
- [ ] **Step 5: Commit** `feat(auth): refresh a token on demand without spending a refresh token twice`.

## Task 3: Broker token, luồng OAuth, kho client (lõi mailcal)

**Files:**
- Create: `internal/mailcal/{tokens.go,oauth.go,clients.go,oauth_test.go}`
- Modify: `internal/mailcal/service.go` (`providerEntry.oauth`, ghi `manifest.OAuth` trong `PluginActivated`, `Service.OAuthSpec`), `internal/mailcal/export_test.go` (`SetURLCheckForTest`)

**Interfaces:**
- Produces: `ProviderInfo.OAuthProvider` (để UI biết plugin đăng nhập qua ActonOS), `auth.TokenRefreshDaemon.DeleteToken`; `ProviderKey(accountID) = "mailcal:"+accountID`; `TokenBroker` (`NewTokenBroker(accounts, src TokenSource, specs func(pluginID) (plugin.PluginOAuth, bool))`) implements `plugin.TokenBroker`: tài khoản của plugin khác hoặc không tồn tại ⇒ `ErrTokenForbidden`; tài khoản `auth_expired` ⇒ `ErrTokenAuthExpired` mà không gọi nhà cung cấp; token còn dưới 60 giây ⇒ `RefreshNow`; `ErrTokenRefresh` ⇒ đánh dấu `auth_expired`. `OAuthFlow` (`NewOAuthFlow(svc, engine, states, tokens, clients)`): `Start(ctx, accountID, redirectURI) (authURL, nonce, err)`, `Complete(ctx, state, code, nonce) (accountID, err)`, `Forget(ctx, accountID) error` (xoá token khi xoá tài khoản), `RegisterAll(ctx) int`; lỗi `ErrOAuthNotConfigured`, `ErrOAuthState`, `ErrNoRefreshToken`. `ClientSource`, `VaultClients{Vault}` (`Client`, `Set`, `Configured`).
- Consumes: Tasks 1-2, `auth.StateStore`, `auth.OAuthEngine`, `memory.Vault`, `security.ValidateOutboundURL`.

Quy tắc cần giữ (đã có test): `Complete` tiêu thụ `state` **trước** khi kiểm nonce; so nonce bằng `subtle.ConstantTimeCompare` trên SHA-256; từ chối khi không có refresh token (kèm hướng dẫn thu hồi quyền để nhận lại); `Start` đặt trạng thái tài khoản `auth_expired`/"waiting for sign-in" cho tới khi hoàn tất; mọi `*_url` phải là `https` thuần.

- [ ] **Step 1: Test trước (đỏ)**

```bash
git checkout wip/mailcal-ref-p3 -- internal/mailcal/oauth_test.go
git diff wip/mailcal-ref-p2 wip/mailcal-ref-p3 -- internal/mailcal/export_test.go | git apply --index
go vet ./internal/mailcal 2>&1 | head -3
```

- [ ] **Step 2: Cài đặt**

```bash
git checkout wip/mailcal-ref-p3 -- internal/mailcal/tokens.go internal/mailcal/oauth.go internal/mailcal/clients.go
git diff wip/mailcal-ref-p2 wip/mailcal-ref-p3 -- internal/mailcal/service.go | git apply --index
go test ./internal/mailcal -count=1 -run 'SignIn|Token|Broker' -v 2>&1 | grep -E '^(---|ok|FAIL)'
```
Expected: sáu test PASS (`TestSignInStoresTheTokenUnderThePerAccountKey`, `...RejectsAForeignBrowserAndCannotBeReplayed`, `...NeedsARefreshToken`, `TestTokenEndpointsMustBeHTTPSAndPublic`, `TestBrokerNeverServesAnotherPluginsAccount`, `TestBrokerRenewsNearExpiryAndMarksRevokedLogins`).

- [ ] **Step 3: Đột biến (3 lần):** (a) trong `Complete` đổi thứ tự để kiểm nonce **trước** `Consume`: test "single-use" đỏ. (b) bỏ `f.checkURL(...)` khỏi `config`: `TestTokenEndpointsMustBeHTTPSAndPublic` đỏ. (c) trong `AccessToken` bỏ kiểm `acct.PluginID != pluginID`: test sở hữu đỏ. Hoàn nguyên từng cái.
- [ ] **Step 4: Commit** `feat(mailcal): add per-account OAuth sign-in and the token broker`.

## Task 4: API đăng nhập và nối dây `actond`

**Files:**
- Create: `internal/server/api_mailcal_oauth.go`, `internal/server/api_mailcal_oauth_test.go`
- Modify: `internal/server/router.go` (trường `mailcalOAuth`, `mailcalClients`, `Config.MailcalOAuth`, `Config.MailcalClients`; route công khai `GET /api/mailcal/oauth/callback`; route có xác thực `POST /accounts/{id}/oauth/start`, `GET|PUT /oauth/clients/{provider}`), `internal/server/api_mailcal.go` (xoá tài khoản gọi `Forget`), `cmd/actond/main.go`, `docs/API.md`, `.agents/rules/source-registry.md`

**Interfaces (HTTP):**

| Route | Xác thực | Ý nghĩa |
|:---|:-:|:---|
| `POST /api/mailcal/accounts/{id}/oauth/start` | có | trả `{auth_url, redirect_uri}` và đặt cookie `acton_mailcal_oauth` (HttpOnly, `SameSite=Lax`, `Path=/api/mailcal/oauth`, 10 phút, `Secure` khi HTTPS) |
| `GET /api/mailcal/oauth/callback` | **không** | hoàn tất; luôn trả 302 về `/#/mailcal?oauth=ok|denied|state|no_refresh_token|failed`, xoá cookie, `Cache-Control: no-store`, `Referrer-Policy: no-referrer`; **không** đưa dữ liệu nhà cung cấp vào URL |
| `GET|PUT /api/mailcal/oauth/clients/{provider}` | có | PUT `{client_id, client_secret}`; GET chỉ `{configured}` |

- [ ] **Step 1-2: Test và handler**

```bash
git checkout wip/mailcal-ref-p3 -- internal/server/api_mailcal_oauth_test.go
go vet ./internal/server 2>&1 | head -3
git checkout wip/mailcal-ref-p3 -- internal/server/api_mailcal_oauth.go
git diff wip/mailcal-ref-p2 wip/mailcal-ref-p3 -- internal/server/router.go | git apply --index
go test ./internal/server -count=1 -run MailcalOAuth -v 2>&1 | grep -E '^(---|ok|FAIL)'
```
Expected: `TestMailcalOAuthCallbackFailsClosedAndLeaksNothing`, `TestMailcalOAuthStartNeedsAFlow` PASS.

- [ ] **Step 3: Nối dây `cmd/actond/main.go` (mã tham chiếu chưa có; làm theo đúng đoạn dưới)**

Hiện tại `stateStore`, `oauthEngine`, `tokenDaemon` được tạo ở khoảng dòng 450, **sau** `plugin.NewManager` (~390) và quét plugin. `HostContext.Tokens` được sao lúc nạp plugin, nên nếu giữ nguyên thứ tự thì plugin nạp lúc khởi động sẽ không có broker. **Chuyển ba dòng tạo** `stateStore`, `oauthEngine`, `tokenDaemon` lên ngay sau khi tạo `vault` và `eventBus` (trước `mailcal.NewService`), rồi thêm:

```go
	var mailcalOAuth *mailcal.OAuthFlow
	var mailcalClients *mailcal.VaultClients
	if mailcalSvc != nil {
		mailcalClients = &mailcal.VaultClients{Vault: vault}
		mailcalOAuth = mailcal.NewOAuthFlow(mailcalSvc, oauthEngine, stateStore, tokenDaemon, mailcalClients)
	}
	// ... sau khi pluginMgr được tạo và TRƯỚC khi quét/nạp plugin:
	if pluginMgr != nil && mailcalSvc != nil {
		pluginMgr.SetTokenBroker(mailcal.NewTokenBroker(mailcalSvc.Accounts(), tokenDaemon, mailcalSvc.OAuthSpec))
	}
	// ... sau khi plugin đã nạp (để OAuthSpec có dữ liệu) và vault đã mở:
	if mailcalOAuth != nil {
		slog.Info("mailcal: oauth accounts registered with the refresh daemon", "count", mailcalOAuth.RegisterAll(ctx))
	}
```
và trong `server.Config{...}` thêm `MailcalOAuth: mailcalOAuth, MailcalClients: mailcalClients`.

Vì `RegisterAll` cần `OAuthSpec` (có sau khi plugin kích hoạt), nếu plugin được nạp bất đồng bộ thì gọi lại `RegisterAll` trong `PluginActivated` của Service qua một callback `Service.OnProviderReady func()` (thêm trường này nếu thật sự cần; kiểm bằng test: khởi động có tài khoản OAuth + plugin, sau đó `RefreshNow` không báo "no provider config registered").

- [ ] **Step 4: Kiểm nối dây**

```bash
go build ./... && go test ./cmd/... ./internal/server -count=1
```
Thêm test `TestMainWiresTheTokenBrokerBeforePluginsLoad` trong `cmd/actond` nếu có cách dựng nhẹ `run()`; nếu không, kiểm bằng đọc: `grep -n "SetTokenBroker\|ScanAndLoadAll" cmd/actond/main.go` phải cho `SetTokenBroker` đứng **trước** `ScanAndLoadAll`.

- [ ] **Step 5: Tài liệu** `docs/API.md` (bảng route trên, kể cả lý do callback công khai), `.agents/rules/source-registry.md`.
- [ ] **Step 6: Commit** `feat(server): add the OAuth sign-in endpoints and wire the token broker into actond`.

## Task 5: Bộ công cụ nhà cung cấp (`provkit`)

**Files:**
- Create: `internal/mailcal/provkit/{provkit.go,provkit_test.go}`, `provkit/netio/{netio.go,stdlib.go}`, `provkit/mimekit/{mimekit.go,mimekit_test.go}`, `provkit/wasmguest/wasmguest.go`, `refplugin/netguest/main.go`; sửa `refplugin/refplugintest/build.go`; test `internal/mailcal/wasmnet_test.go`

**Interfaces:**
- Produces:
  - `provkit.Backend` (cùng bộ phương thức với `mailcal.Provider`), `provkit.Base` (mọi thao tác trả `unsupported`), `provkit.Dispatch(ctx, Backend, op, reqJSON) []byte` (không bao giờ panic: panic ⇒ lỗi `transient` có mã).
  - `netio.Request/Response/Doer/Tokens`, `netio.StatusError(resp, what) *abi.Error` (401 → `auth_expired`; 404/410 → `not_found`; 429 và 403 "ratelimit" → `rate_limited` kèm `Retry-After`; 403 khác → `invalid`; 408/5xx → `transient`; còn lại → `invalid`), `netio.Stdlib{Client}` và `netio.StaticToken` (không build trên `wasip1`).
  - `mimekit.Build(Message)` (từ chối tiêm header; Bcc chỉ ghi khi `KeepBcc`), `mimekit.Parse(raw)`, `mimekit.HTMLToText` (bỏ `script/style/head/title`, chú thích, nội dung ẩn bằng `display:none`/`visibility:hidden`/`font-size:0`), `mimekit.Base64URL`.
  - `wasmguest.Alloc/Free/Serve`, `wasmguest.HTTP`, `wasmguest.Tokens` (chỉ `wasip1`; map mã `-3` ⇒ `auth_expired`, `-2` ⇒ `invalid`, khác ⇒ `transient`; lỗi từ chính host: 403 ⇒ `invalid` "blocked by the plugin's network permissions").
  - `refplugintest.BuildGuest(tb, pkg)` và `InstanceOf(tb, wasm, manifest, host)` (biên dịch một lần mỗi tiến trình, mỗi `manifest.ID` một loader dùng chung).
- Ràng buộc kiến trúc: `provkit`, `netio`, `mimekit` **không** phụ thuộc `internal/plugin`, `internal/tools`, wazero (kiểm bằng `go list -deps ./internal/mailcal/provkit | grep -c 'wazero\|internal/plugin\|internal/tools'` ra `0`) và phải biên dịch với `GOOS=wasip1`.

- [ ] **Step 1: Test trước**

```bash
git checkout wip/mailcal-ref-p3 -- internal/mailcal/provkit/provkit_test.go internal/mailcal/provkit/mimekit/mimekit_test.go internal/mailcal/wasmnet_test.go
go vet ./internal/mailcal/provkit/... 2>&1 | head -3
```

- [ ] **Step 2: Cài đặt**

```bash
git checkout wip/mailcal-ref-p3 -- internal/mailcal/provkit internal/mailcal/refplugin/netguest internal/mailcal/refplugin/refplugintest
go test ./internal/mailcal/provkit/... ./internal/mailcal -count=1 -run 'Kit|Base|Bcc|Header|HTML|Parse|Build|Reply|AWasm|AnExpired' -v 2>&1 | grep -E '^(---|ok|FAIL)'
go list -deps ./internal/mailcal/provkit | grep -c 'wazero\|internal/plugin\|internal/tools'
GOOS=wasip1 GOARCH=wasm go build ./internal/mailcal/provkit/... && echo wasm-ok
```
Expected: PASS (WASM mất vài giây lần đầu); lệnh đếm in `0`; `wasm-ok`. `TestKitRoundTripPassesConformance` chứng minh `Dispatch` và `BridgeProvider` là hai chiều ngược nhau (nó chạy toàn bộ `RunSeeded`).

- [ ] **Step 3: Đột biến:** trong `netio.StatusError` đổi `r.Status == 401` thành `r.Status == 403`: test `TestAnExpiredLoginSurfacesAsAuthExpired...` đỏ. Trong `mimekit.HTMLToText` bỏ `hiddenStyle`: `TestHTMLOnlyMailLosesHiddenInstructions` đỏ. Hoàn nguyên.
- [ ] **Step 4: Commit** `feat(mailcal): add the provider kit with HTTP, MIME and WebAssembly guest runtime`.

## Task 6: Plugin Gmail và Google Calendar

**Files:**
- Create: `internal/mailcal/plugins/gmail/{gmail.go,calendar.go,manifest.json,fake_test.go,gmail_test.go,calendar_test.go,wasm/main.go}`, `internal/mailcal/gmailwasm_test.go`
- Modify: `Makefile` (target `plugin-gmail`)

**Interfaces:**
- Produces: `gmail.Backend{HTTP netio.Doer, Tokens netio.Tokens, APIBase, CalAPIBase string}` implements `provkit.Backend`; `gmail.IdempotencyHeader = "X-Acton-Idempotency-Key"`.

Bảng ánh xạ (đã cài và kiểm với máy chủ giả):

| ABI | Gmail / Calendar REST | Ghi chú |
|:---|:---|:---|
| `mail_list` | `GET /messages?labelIds=&maxResults=` + `GET /messages/{id}?format=metadata` mỗi thư | thư mục hệ thống ánh xạ nhãn; nhãn lạ ⇒ `q=label:"..."` |
| `mail_search` | `GET /messages?q=` | **chỉ giữ từ thường**: bỏ toán tử, dấu `-`, ngoặc, `OR/AND` |
| `mail_get`, `get_thread` | `format=full`, `GET /threads/{id}` | ưu tiên `text/plain`; HTML qua `HTMLToText`; tệp đính kèm chỉ là metadata; `in_reply_to` trong thread được đổi sang id Gmail của thư cha |
| `mail_send` | `GET /messages` (SENT, 10 thư gần nhất) + metadata để tìm header khoá, rồi `POST /messages/send` `{raw, threadId}` | **bắt buộc có khoá**; header `X-Acton-Idempotency-Key`; `From` lấy từ `/profile`; Bcc ghi qua `KeepBcc` |
| `mail_mark`, `move` | `POST /messages/batchModify` | chỉ chuyển tới INBOX/TRASH/SPAM |
| `mail_delete` | `POST /messages/{id}/trash` | **vào Thùng rác**, không xoá vĩnh viễn |
| `calendar_list` | `GET /calendars/primary/events?singleEvents=true&orderBy=startTime` | sự kiện lặp đã khai triển, bỏ `cancelled` |
| `calendar_create` | `POST .../events?sendUpdates=` | `id = "acton"+sha256hex(khoá)`; 409 ⇒ lấy sự kiện đã có (idempotent) |
| `calendar_update`, `delete` | `PUT`, `DELETE` | |
| `calendar_respond` | `GET` rồi `PATCH` `attendees` (chỉ người có `self:true`) | không phải khách mời ⇒ `invalid` |
| `calendar_free_busy` | `POST /freeBusy` | email chuẩn hoá |

- [ ] **Step 1: Test trước**

```bash
git checkout wip/mailcal-ref-p3 -- internal/mailcal/plugins/gmail/fake_test.go internal/mailcal/plugins/gmail/gmail_test.go internal/mailcal/plugins/gmail/calendar_test.go internal/mailcal/gmailwasm_test.go internal/mailcal/plugins/gmail/manifest.json
go vet ./internal/mailcal/plugins/gmail 2>&1 | head -3
```
Expected: lỗi `undefined: gmail.Backend`.

- [ ] **Step 2: Cài đặt và chạy**

```bash
git checkout wip/mailcal-ref-p3 -- internal/mailcal/plugins/gmail/gmail.go internal/mailcal/plugins/gmail/calendar.go internal/mailcal/plugins/gmail/wasm
go test ./internal/mailcal/plugins/gmail -count=1 -v 2>&1 | grep -E '^(---|ok|FAIL)'
go test ./internal/mailcal -count=1 -run Gmail -v 2>&1 | grep -E '^(---|ok|FAIL)'
```
Expected: PASS, gồm `TestSendingTheSameKeyTwiceSendsOnce`, `TestALostAnswerAfterSendingIsFoundAgainByTheRetry`, `TestSearchCannotBeWidenedByOperators`, `TestCreatingTheSameEventTwiceWithOneKeyMakesOneEvent`, `TestRespondChangesOnlyTheCallersOwnAnswer`, `TestGmailPassesTheShapeConformanceSuite`, `TestGmailManifestAsksForOnlyWhatItNeeds`, `TestGmailWasmDeclaresItsCapabilities`.

- [ ] **Step 3: Đột biến** (ba, đã thực hiện khi dựng thử và đều bị bắt): đổi khoá tìm lại thành `key+"x"` ⇒ hai test idempotency đỏ; bỏ lọc toán tử ⇒ test tìm kiếm đỏ; (mới) đổi `eventIDFor` về dạng ngắn hơn khoá ⇒ test tạo sự kiện hai lần đỏ. Hoàn nguyên.

- [ ] **Step 4: Target Makefile**

```make
plugin-gmail:
	GOOS=wasip1 GOARCH=wasm CGO_ENABLED=0 go build -buildmode=c-shared -o build/plugins/mailcal-gmail/plugin.wasm ./internal/mailcal/plugins/gmail/wasm
	cp internal/mailcal/plugins/gmail/manifest.json build/plugins/mailcal-gmail/manifest.json
```
Chạy `make plugin-gmail && ls -la build/plugins/mailcal-gmail`. Expected: hai tệp; `plugin.wasm` khoảng 6 MB. Gói được ký/được phép chạy chưa ký theo cơ chế `internal/plugin/signature.go` hiện có (`ACTONOS_ALLOW_UNSIGNED_PLUGINS=1` cho phát triển, hoặc ký bằng khoá của dự án); **không** thay đổi cơ chế đó.

- [ ] **Step 5: Commit** `feat(mailcal): add the Gmail and Google Calendar provider plugin`.

## Task 7: Giao diện kết nối Google

**Files:**
- Create: `web/src/pages/Mailcal/OAuthClientPanel.tsx` (+test), `web/src/pages/Mailcal/ConnectButton.tsx` (+test)
- Modify: `web/src/lib/api/mailcal.ts`, `web/src/pages/Mailcal/AccountModal.tsx`, `web/src/pages/Mailcal/MailcalPage.tsx`, `web/src/locales/{en,vi}/mailcal.json`

**Interfaces (TS):**

```ts
// thêm vào mailcal.ts
export interface MailcalOAuthStart { auth_url: string; redirect_uri: string }

// trong mailcalApi:
  startOAuth: (id: string) =>
    fetchJSON<MailcalOAuthStart>(`/mailcal/accounts/${encodeURIComponent(id)}/oauth/start`, { method: 'POST' }),
  getOAuthClient: (provider: string) =>
    fetchJSON<{ configured: boolean }>(`/mailcal/oauth/clients/${encodeURIComponent(provider)}`),
  putOAuthClient: (provider: string, clientId: string, clientSecret: string) =>
    fetchJSON<{ configured: boolean }>(`/mailcal/oauth/clients/${encodeURIComponent(provider)}`, {
      method: 'PUT',
      body: JSON.stringify({ client_id: clientId, client_secret: clientSecret }),
    }),
```

Hành vi:
- `AccountModal`: khi plugin được chọn có `oauth` (trường `oauth_provider?: string` của `MailcalProvider`, lấy từ `ProviderInfo.OAuthProvider` của API `GET /api/mailcal/providers`; thêm vào kiểu TS), hiển thị `OAuthClientPanel` (hai ô nhập `client_id`, `client_secret` kiểu `password`, nút Lưu; sau khi lưu chỉ hiển thị "Đã cấu hình", **không** hiển thị lại giá trị) và dòng hướng dẫn kèm `redirect_uri` để người dùng đăng ký ở Google Cloud Console (lấy từ `startOAuth`, hoặc dựng `window.location.origin + '/api/mailcal/oauth/callback'`).
- Tạo tài khoản với `auth: {type: 'oauth'}` rồi hiển thị `ConnectButton`: gọi `startOAuth(id)` rồi `window.location.assign(auth_url)` (điều hướng cùng tab để cookie nonce đặt ở phản hồi `start` còn hiệu lực).
- Khi trang tải với `?oauth=ok|denied|state|no_refresh_token|failed`, hiển thị toast tương ứng bằng khoá locale `oauth.result.<code>` (mã lạ ⇒ `failed`), rồi `history.replaceState` xoá tham số. Trạng thái `auth_expired` hiển thị nút "Kết nối lại".

- [ ] **Step 1: Test trước** (`ConnectButton.test.tsx`): mock `mailcalApi.startOAuth` trả `auth_url`, mock `window.location.assign`; bấm nút gọi `assign` đúng URL; lỗi API hiện thông báo và không điều hướng. (`OAuthClientPanel.test.tsx`): lưu gọi `putOAuthClient` với giá trị đã nhập; sau khi lưu ô `client_secret` bị xoá khỏi DOM; `configured: true` hiển thị nhãn "đã cấu hình" mà không có giá trị nào. (`MailcalPage.test.tsx`): `?oauth=no_refresh_token` hiện đúng thông báo hướng dẫn thu hồi quyền; `?oauth=<mã lạ>` rơi về `failed`.
- [ ] **Step 2: Cài đặt** theo mô tả; khoá locale `oauth.*` đầy đủ ở `en` và `vi` (ví dụ `oauth.result.no_refresh_token`: "Google không cấp quyền truy cập lâu dài. Hãy gỡ ứng dụng khỏi trang Bảo mật tài khoản Google rồi kết nối lại.").
- [ ] **Step 3: Kiểm**

```bash
cd web && npx tsc --noEmit && npm run lint && npm run check:i18n && npm run audit:i18n && npx vitest run src/pages/Mailcal && npm run build && node scripts/check-bundle.mjs
```
- [ ] **Step 4: Commit** `feat(web): connect a Google account with the user's own OAuth client`.

## Task 8: Tài liệu cài plugin và cổng giai đoạn 3

- [ ] **Step 1: Tài liệu** `docs/PLUGINS-MAILCAL.md` (tạo): các bước tạo OAuth client trên Google Cloud Console (ứng dụng loại "Web", bật Gmail API và Google Calendar API, thêm `redirect_uri` hiển thị trong ứng dụng, thêm tài khoản của chính mình vào "người dùng thử" vì ứng dụng ở chế độ Testing), cách xây (`make plugin-gmail`), cách cài (`build/plugins/mailcal-gmail/` vào `<data>/plugins/`), giới hạn đã biết (refresh token của ứng dụng Testing hết hạn sau 7 ngày: kết nối lại; lý do ActonOS không phân phối client chung). Cập nhật `docs/SECURITY.md` (broker token, callback công khai và vì sao an toàn), `CHANGELOG.md`.
- [ ] **Step 2: Toàn bộ test**

```bash
go build ./... && go vet ./... && go test ./internal/... ./evals/... ./cmd/... -count=1
go test -race -count=1 ./internal/auth ./internal/mailcal/... ./internal/plugin
cd web && npm run lint && npm run check:i18n && npm run audit:i18n && npx vitest run && cd ..
```
- [ ] **Step 3: Nghiệm thu thủ công trên Google thật (người dùng làm, KHÔNG tự động hoá và KHÔNG đưa khoá vào tệp)**: dùng một tài khoản Google thử nghiệm. (1) Kết nối; trạng thái `ok`. (2) Chạy bộ `conformance.RunShape` với tài khoản đó (cần một test nhỏ `TestGmailLiveShape` gắn thẻ build `live` đọc id tài khoản và thông tin từ biến môi trường, mặc định bị bỏ qua). (3) Agent đọc và tóm tắt hộp thư. (4) Soạn nháp, **gửi cần duyệt**; sau khi duyệt, thư đến người nhận và **chỉ một lần** kể cả khi bấm duyệt hai lần. (5) Kiểm hai giả định chưa thể kiểm bằng máy chủ giả, ghi kết quả vào PR: **header `X-Acton-Idempotency-Key` có được Gmail giữ lại trong thư đã gửi không** (nếu không, `findEarlierSend` vô hiệu và phải đổi sang phương án dự phòng: tìm trong SENT theo người nhận + tiêu đề + dấu thời gian, so khớp phần đầu `snippet`), và **Gmail API có thật sự bỏ header `Bcc` khỏi thư giao tới người nhận không**. (6) Thu hồi quyền tại Google, gọi lại: tài khoản chuyển `auth_expired`, UI hiện "Kết nối lại".
- [ ] **Step 4: Commit** `docs: document connecting Google and the OAuth model`.

## Hoàn tác

`git revert` Task 8 → 1. Không có bảng mới ở giai đoạn này (`oauth_tokens` đã có). Gỡ plugin Gmail khỏi `<data>/plugins/` làm tài khoản Google báo "provider plugin is not loaded" mà không ảnh hưởng tài khoản khác. Xoá một tài khoản cũng xoá token của nó (`OAuthFlow.Forget`, gọi từ handler xoá tài khoản; `TestDeletingAnAccountDeletesItsToken`), nên không có thông tin đăng nhập nào sống lâu hơn tài khoản của nó.

## Điều giai đoạn sau phải biết

- **Dùng lại `provkit`.** Graph (GĐ4) chỉ cần `Backend` + `netio` + JSON; CalDAV cần thêm bí mật dạng mật khẩu (xem GĐ4 Task 1: `netio.Secrets` và `wasmguest.Secrets` qua `acton_vault.get_secret`); IMAP/SMTP (GĐ5) dùng `mimekit.Build/Parse` và `wasmguest` cho socket TLS.
- **Thêm host API mới = thêm module host mới.** Không bao giờ thêm hàm vào module đã phát hành.
- **Manifest mới chỉ thêm trường tuỳ chọn.** `PluginManifest` đã có `OAuth`; GĐ5 thêm `permissions.net_tcp` theo cùng cách.
- **Bộ `conformance.RunShape`** là cổng bắt buộc cho mọi plugin mới; thêm ca mới vào đó thì phải chạy lại cho Gmail.
- **Cửa sổ DNS-rebinding của đường HTTP plugin hiện có** (kiểm IP rồi `http.Client` phân giải lại) áp dụng cả cho Gmail. Rủi ro thấp vì host cố định `googleapis.com` do manifest khai, nhưng vá nó thuộc GĐ5 (cùng bộ dial an toàn).
