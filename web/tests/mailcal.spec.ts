import { expect, test, type Page } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';
import { mockAuthenticatedApi } from './helpers/mock-api';

const account = (over: Record<string, unknown> = {}) => ({
  id: 'acct_1', label: 'Work', plugin_id: 'mailcal-gmail', kind: 'both', email: 'me@example.com', auth: { type: 'oauth' },
  status: 'ok', created_at: '2026-03-01T00:00:00Z', updated_at: '2026-03-01T00:00:00Z', ...over,
});

const decision = {
  id: 'dec_1', at: '2026-03-02T10:00:00Z', phase: 'gate', account_id: 'acct_1', agent_id: 'a', tool: 'native_mail_send',
  verdict: 'needs_approval', reasons: ['no rule allows this action'], draft_id: 'draft_1', payload_hash: 'abc',
};

async function mockMail(page: Page, opts: { accounts?: unknown[]; enabled?: boolean } = {}) {
  let enabled = opts.enabled ?? false;
  await page.route('**/api/mailcal/**', async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname;
    const method = route.request().method();
    const json = (data: unknown) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ data }) });
    if (path.endsWith('/providers')) return json({ providers: [{ plugin_id: 'mailcal-gmail', name: 'Gmail', version: '1', abi: ['mail@1', 'calendar@1'], ops: [], limits: {}, oauth_provider: 'google', auth_kind: 'oauth' }] });
    if (path.endsWith('/accounts')) return json({ accounts: opts.accounts ?? [account()], count: (opts.accounts ?? [account()]).length });
    if (path.endsWith('/settings') && method === 'PUT') {
      enabled = JSON.parse(route.request().postData() ?? '{}').enabled;
      return json({ enabled });
    }
    if (path.endsWith('/settings')) return json({ enabled });
    if (path.endsWith('/rules')) return json({ rules: [], count: 0 });
    if (path.endsWith('/proposals')) return json({ proposals: [], count: 0 });
    if (path.endsWith('/decisions')) return json({ decisions: [decision], count: 1 });
    return json({});
  });
}

test.beforeEach(async ({ page }) => {
  await mockAuthenticatedApi(page);
});

test('mail and calendar page shows the master switch off by default and can turn it on', async ({ page }) => {
  await mockMail(page);
  await page.goto('/#/mailcal');
  await expect(page.locator('h1').first()).toBeVisible();
  await expect(page.getByText(/every send, delete and calendar change is refused|mọi thao tác gửi/i)).toBeVisible();
  await page.getByRole('switch').click();
  await expect(page.getByRole('switch')).toBeChecked();
  await expect(page.locator('body')).not.toHaveCSS('overflow-x', 'scroll');
});

test('tabs open the rules, proposals and audit views', async ({ page }) => {
  await mockMail(page);
  await page.goto('/#/mailcal');
  await page.getByRole('tab', { name: /Auto-send rules|Luật tự gửi/ }).click();
  await expect(page.getByText(/No rules yet|Chưa có luật/)).toBeVisible();
  await page.getByRole('tab', { name: /Rule proposals|Đề xuất luật/ }).click();
  await expect(page.getByText(/No proposals waiting|Không có đề xuất/)).toBeVisible();
  await page.getByRole('tab', { name: /Audit|Nhật ký/ }).click();
  await expect(page.getByText('native_mail_send')).toBeVisible();
});

test('the audit trail exports a CSV with the expected header and no message text', async ({ page }) => {
  await mockMail(page);
  await page.goto('/#/mailcal');
  await page.getByRole('tab', { name: /Audit|Nhật ký/ }).click();
  const [download] = await Promise.all([page.waitForEvent('download'), page.getByRole('button', { name: /Export CSV|Xuất CSV/ }).click()]);
  const path = await download.path();
  const text = (await import('node:fs')).readFileSync(path!, 'utf8');
  expect(text.split('\r\n')[0]).toBe('at,phase,account_id,tool,verdict,rule_id,approved,outcome,draft_id,payload_hash');
  expect(text).toContain('native_mail_send');
  expect(text).not.toMatch(/subject|body_text/i);
});

test('an account whose sign-in expired shows an alert with a way back in', async ({ page }) => {
  await mockMail(page, { accounts: [account({ status: 'auth_expired', status_detail: 'the sign-in was revoked' })] });
  await page.goto('/#/mailcal');
  const alert = page.getByRole('alert').filter({ hasText: 'the sign-in was revoked' });
  await expect(alert).toBeVisible();
  await expect(alert.getByRole('button', { name: /Sign in|Đăng nhập/ })).toBeVisible();
});

test('the page has no serious accessibility violations and does not scroll sideways', async ({ page }) => {
  await mockMail(page);
  await page.goto('/#/mailcal');
  await expect(page.locator('h1').first()).toBeVisible();
  await page.waitForLoadState('networkidle');
  const results = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa']).analyze();
  expect(
    results.violations.filter((v) => v.impact === 'critical' || v.impact === 'serious'),
    JSON.stringify(results.violations, null, 2),
  ).toEqual([]);
  await expect(page.locator('body')).not.toHaveCSS('overflow-x', 'scroll');
  const overflows = await page.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth);
  expect(overflows).toBe(false);
});
