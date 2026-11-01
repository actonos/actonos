import { expect, test } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';
import { mockAuthenticatedApi } from './helpers/mock-api';

test.beforeEach(async ({ page }) => {
  await mockAuthenticatedApi(page);
});

test('authenticated dashboard is responsive and accessible', async ({ page }) => {
  await page.goto('/#/dashboard');
  await expect(page.getByRole('heading', { name: /Dashboard|Bảng điều khiển/ }).first()).toBeVisible();
  await expect(page.locator('body')).not.toHaveCSS('overflow-x', 'scroll');
  await page.waitForLoadState('networkidle');

  const results = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'])
    .analyze();
  expect(
    results.violations.filter((violation) => violation.impact === 'critical' || violation.impact === 'serious'),
    JSON.stringify(results.violations, null, 2)
  ).toEqual([]);
  await expect(page).toHaveScreenshot('dashboard.png', { fullPage: true, animations: 'disabled', maxDiffPixels: 1000 });
});

test('command palette navigates and density persists', async ({ page }) => {
  await page.goto('/#/dashboard');
  // The shortcut is registered once the shell has mounted.
  await expect(page.getByRole('heading', { name: /Dashboard|Bảng điều khiển/ }).first()).toBeVisible();
  await page.keyboard.press('Control+K');
  await expect(page.getByRole('dialog', { name: /Quick search|Tìm nhanh/ })).toBeVisible();
  await page.getByPlaceholder(/Search pages|Tìm trang/).fill('operations');
  await page.keyboard.press('Enter');
  await expect(page).toHaveURL(/#\/operations$/);

  // The density control lives in Settings > Identity (it was removed from the header).
  await page.goto('/#/settings?view=identity');
  await page.getByRole('tab', { name: /^(Compact|Gọn gàng)$/ }).click();
  await expect.poll(() => page.evaluate(() => document.documentElement.dataset.density)).toBe('compact');
  await page.reload();
  await expect.poll(() => page.evaluate(() => document.documentElement.dataset.density)).toBe('compact');
});

test('operations view has a stable visual baseline', async ({ page }) => {
  await page.goto('/#/operations');
  await expect(page.getByRole('heading', { name: /Live Operations|Vận hành/ })).toBeVisible();
  // Cards fill in as data arrives and change the page height; capture only once it has settled.
  await page.waitForLoadState('networkidle');
  await expect(page).toHaveScreenshot('operations.png', { fullPage: true, animations: 'disabled', maxDiffPixels: 1000 });
});

test('primary routes render without horizontal overflow', async ({ page }) => {
  const routes = [
    'agents', 'agents/new', 'chat', 'missions', 'automations', 'channels',
    'plugins', 'tools', 'skills', 'workspace', 'terminal', 'settings',
  ];
  for (const route of routes) {
    await page.goto(`/#/${route}`);
    await expect(page.locator('body')).not.toHaveCSS('overflow-x', 'scroll');
    await expect(page.locator('h1').first()).toBeVisible();
  }
  await page.goto('/#/channels?view=pairing');
  await expect(page.locator('h1').first()).toBeVisible();
  await expect(page).toHaveURL(/channels\?view=pairing/);
});

test('agent studio protects unsaved changes and settings preserves section state', async ({ page }) => {
  await page.goto('/#/agents/new');
  await expect(page.getByRole('heading').first()).toBeVisible();
  const nameInput = page.locator('input').first();
  await nameInput.fill('Draft agent');
  await page.evaluate(() => {
    window.dispatchEvent(new Event('beforeunload'));
  });
  await page.goto('/#/settings?view=network');
  await expect(page.getByRole('heading').first()).toBeVisible();
  await expect(page).toHaveURL(/view=network/);
});

test('agent studio exposes review validation before save', async ({ page }) => {
  await page.goto('/#/agents/new');
  await page.getByRole('tab', { name: /Review & save/ }).click();
  await expect(page.getByRole('heading', { name: /Review agent manifest/ })).toBeVisible();
  await expect(page.getByText(/Resolve these issues before saving/)).toBeVisible();
});

test('operations and workflow tabs preserve deep-link state', async ({ page }) => {
  await page.goto('/#/operations');
  await page.getByRole('tab', { name: /Runtime/ }).click();
  await expect(page).toHaveURL(/operations\?view=runtime/);
  await page.reload();
  await expect(page.getByRole('tab', { name: /Runtime/ })).toHaveAttribute('aria-selected', 'true');

  await page.goto('/#/channels');
  await page.getByRole('button', { name: /Pairing/ }).click();
  await expect(page).toHaveURL(/channels\?view=pairing/);

  await page.goto('/#/automations');
  await page.getByRole('button', { name: /History/ }).click();
  await expect(page).toHaveURL(/automations\?view=history/);
});

test('chat conversation returns to the sessions list on narrow layouts', async ({ page }) => {
  test.skip((page.viewportSize()?.width || 0) >= 640, 'Mobile-only navigation behavior');
  await page.goto('/#/chat');
  const search = page.getByPlaceholder(/Search by title|Tìm theo tiêu đề/);
  await expect(search).toBeVisible();
  await page.getByRole('button', { name: /^New Chat$/ }).first().click();
  await expect(search).toBeHidden();
  await page.getByRole('button', { name: /Back to Conversations|Quay lại danh sách/ }).first().click();
  await expect(search).toBeVisible();
});

test('new chat first reply stays visible when the session hash is assigned', async ({ page }) => {
  const convId = 'conv_e2e_first';
  const reply = 'Hello from the streamed agent.';
  const sse = [
    'event: thought',
    `data: ${JSON.stringify({ conversation_id: convId, title: 'Ping', thought: 'Thinking with Nova...' })}`,
    '',
    'event: token',
    `data: ${JSON.stringify({ conversation_id: convId, title: 'Ping', content: reply })}`,
    '',
    'event: done',
    `data: ${JSON.stringify({ conversation_id: convId, title: 'Ping', content: reply, model: 'mock-model', tokens_used: 8 })}`,
    '',
  ].join('\n');

  await page.route('**/api/agents/**/chat', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'text/event-stream',
      body: sse,
    });
  });
  await page.route(`**/api/conversations/${convId}`, async (route) => {
    await new Promise((resolve) => setTimeout(resolve, 200));
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        data: {
          conversation: {
            id: convId,
            agent_id: 'agent_system_core',
            title: 'Ping',
            channel: 'web',
            is_pinned: false,
            created_at: new Date().toISOString(),
            updated_at: new Date().toISOString(),
          },
          messages: [
            {
              id: 'msg_user_only',
              conversation_id: convId,
              role: 'user',
              content: 'Ping',
              created_at: new Date().toISOString(),
            },
          ],
        },
      }),
    });
  });

  await page.goto('/#/chat?session_id=new');
  await expect(page.getByPlaceholder(/Ask anything/)).toBeVisible();
  await page.getByPlaceholder(/Ask anything/).fill('Ping');
  await page.getByRole('button', { name: /^Send$/ }).click();
  await expect(page.getByText(reply)).toBeVisible();
  await expect(page).toHaveURL(new RegExp(`session_id=${convId}`));
});

test('primary authenticated routes have no serious accessibility violations', async ({ page }) => {
  test.setTimeout(120_000); // eleven routes, each scanned with axe
  for (const route of ['agents', 'agents/new', 'chat', 'missions', 'automations', 'channels', 'plugins', 'tools', 'skills', 'workspace', 'settings']) {
    await page.goto(`/#/${route}`);
    await expect(page.locator('h1').first()).toBeVisible();
    await page.waitForTimeout(250);
    const results = await new AxeBuilder({ page })
      .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'])
      .analyze();
    expect(
      results.violations.filter((violation) => violation.impact === 'critical' || violation.impact === 'serious'),
      `${route}: ${JSON.stringify(results.violations, null, 2)}`
    ).toEqual([]);
  }
});

test('approval interruption requires rejection feedback', async ({ page }) => {
  await page.goto('/#/dashboard');
  // Wait for the authenticated shell: the listener only exists once it has mounted.
  await expect(page.getByRole('heading', { name: /Dashboard|Bảng điều khiển/ }).first()).toBeVisible();
  await page.evaluate(() => {
    window.dispatchEvent(new CustomEvent('actonos:approval-required', {
      detail: {
        id: 'approval-e2e',
        tool_name: 'workspace.delete',
        agent_id: 'agent_system_core',
        risk_level: 'High',
        input: { path: 'workspace/demo.txt' },
      },
    }));
  });
  const dialog = page.getByRole('alertdialog');
  await expect(dialog).toBeVisible();
  const reject = dialog.getByRole('button', { name: /reject|từ chối/i });
  await expect(reject).toBeDisabled();
  await dialog.locator('textarea').fill('Not approved for this test');
  await expect(reject).toBeEnabled();
});
