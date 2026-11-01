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
3. Open ActonOS over HTTPS (for Tailscale: `https://<hostname>.<tailnet>.ts.net`, see `docs/DEPLOYMENT.md`); Google rejects `http://` redirect addresses except for `localhost`. Store your client: `PUT /api/mailcal/oauth/clients/google` with `{client_id, client_secret}`.
4. Create an account with `plugin_id: "mailcal-gmail"`, `kind: "both"`, `auth: {"type": "oauth"}`, then `POST /api/mailcal/accounts/{id}/oauth/start`. Add the returned `redirect_uri` to the client's authorized redirect URIs and open `auth_url`.

The plugin asks only for the Gmail "modify" scope (read, draft, send, label, trash; not permanent delete) and the Calendar events and free/busy scopes.

## Microsoft 365 and Outlook (Graph)

Register an app in Microsoft Entra (personal or work accounts), add the `redirect_uri` shown by `POST /api/mailcal/accounts/{id}/oauth/start`, and store the client with `PUT /api/mailcal/oauth/clients/microsoft`. Build with `make plugin-graph` and install `build/plugins/mailcal-graph`. Scopes: `offline_access Mail.ReadWrite Mail.Send Calendars.ReadWrite`. Search returns one page (a Graph limit); mail delete moves to Deleted Items.

## CalDAV (Fastmail, iCloud, Nextcloud, Radicale)

Use an app password. Build with `make plugin-caldav`. Account config: `calendar_url` (https URL of the calendar collection), `email` (your own address, needed to answer invitations), optional `username`; then `PUT /api/mailcal/accounts/{id}/password`. The shipped manifest allows `caldav.fastmail.com` and `caldav.icloud.com`; for another server copy the plugin folder under a new `id`, add your host to `permissions.net_outbound`, and install it unsigned. Limits: no free/busy for other people, single occurrences of recurring events cannot be changed, and the server (not the plugin) sends invitations, so events with attendees can only be created when notifying attendees is requested.

## IMAP and SMTP

Use an app password. Build with `make plugin-imap-smtp`. The shipped manifest has an **empty** `permissions.net_tcp`: copy the plugin folder under a new `id`, add your servers (for example `imap.example.com:993` and `smtp.example.com:587`), and install it unsigned. Account config: `imap_host`, `smtp_host`, `email` (required), `imap_mode`/`smtp_mode` (`tls` or `starttls`; defaults are IMAP `tls` on 993 and SMTP `starttls` on 587), optional ports, `username`, `sent_mailbox`, `trash_mailbox`; then `PUT /api/mailcal/accounts/{id}/password`. Sending needs a Sent folder.

**Sending is at most once, not exactly once** (SMTP has no idempotency key). If a send fails in a way that leaves it unknown whether the message was delivered, the plugin reports "may or may not have been delivered" and refuses to send again. Check the Sent folder; remove the leftover copy flagged as a draft (or delete it) before asking the agent to try again.

Known unverified assumptions (check on a test account): that Gmail keeps the `X-Acton-Idempotency-Key` header on sent mail (it is how a retry finds its own earlier send), and that the Gmail API removes a `Bcc` header before delivery.
