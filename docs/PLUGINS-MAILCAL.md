# Mail and calendar provider plugins

Mail and calendar support is served by WebAssembly provider plugins. The core decides what is allowed (approvals, rules, limits); a plugin only translates between the plugin ABI and one provider's API. See `docs/superpowers/specs/2026-11-01-mail-calendar-plugins-design.md`.

## Gmail and Google Calendar

You bring your own OAuth client, so ActonOS never goes through Google's app review.

1. In the Google Cloud Console create a project, enable the **Gmail API** and the **Google Calendar API**, and create an OAuth client of type **Web application**. While the app is in "Testing", add your own Google account as a test user. (Refresh tokens of apps in Testing expire after 7 days; reconnect when the account shows "sign-in expired".)
2. Build and install the plugin:
   ```bash
   make plugin-gmail
   cp -r build/plugins/mailcal-gmail <data dir>/plugins/
   ```
   Unsigned plugins need `ACTONOS_ALLOW_UNSIGNED_PLUGINS=1` unless you sign the package.
3. Store your client: `PUT /api/mailcal/oauth/clients/google` with `{client_id, client_secret}`.
4. Create an account with `plugin_id: "mailcal-gmail"`, `kind: "both"`, `auth: {"type": "oauth"}`, then `POST /api/mailcal/accounts/{id}/oauth/start`. Add the returned `redirect_uri` to the client's authorized redirect URIs and open `auth_url`.

The plugin asks only for the Gmail "modify" scope (read, draft, send, label, trash; not permanent delete) and the Calendar events and free/busy scopes.

Known unverified assumptions (check on a test account): that Gmail keeps the `X-Acton-Idempotency-Key` header on sent mail (it is how a retry finds its own earlier send), and that the Gmail API removes a `Bcc` header before delivery.
