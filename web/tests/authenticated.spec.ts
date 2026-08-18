import { expect, test } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';
import { mockAuthenticatedApi } from './helpers/mock-api';

test.beforeEach(async ({ page }) => {
  await mockAuthenticatedApi(page);
});

test('authenticated dashboard is responsive and accessible', async ({ page }) => {
  await page.goto('/#/dashboard');
  await expect(page.getByRole('heading', { name: /Dashboard|Bảng điều khiển/ })).toBeVisible();
  await expect(page.locator('body')).not.toHaveCSS('overflow-x', 'scroll');

  const results = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'])
    .analyze();
  expect(
    results.violations.filter((violation) => violation.impact === 'critical' || violation.impact === 'serious'),
    JSON.stringify(results.violations, null, 2)
  ).toEqual([]);
  await expect(page).toHaveScreenshot('dashboard.png', { fullPage: true, maxDiffPixels: 1000 });
});

test('command palette navigates and density persists', async ({ page }) => {
  await page.goto('/#/dashboard');
  await page.keyboard.press('Control+K');
  await expect(page.getByRole('dialog', { name: /Quick search|Tìm nhanh/ })).toBeVisible();
  await page.getByPlaceholder(/Search pages|Tìm trang/).fill('operations');
  await page.keyboard.press('Enter');
  await expect(page).toHaveURL(/#\/operations$/);

  const densityButton = page.getByRole('button', { name: /compact density|mật độ gọn/i });
  await densityButton.click();
  await expect.poll(() => page.evaluate(() => document.documentElement.dataset.density)).toBe('compact');
  await page.reload();
  await expect.poll(() => page.evaluate(() => document.documentElement.dataset.density)).toBe('compact');
});

test('operations view has a stable visual baseline', async ({ page }) => {
  await page.goto('/#/operations');
  await expect(page.getByRole('heading', { name: /Live Operations|Vận hành/ })).toBeVisible();
  await expect(page).toHaveScreenshot('operations.png', { fullPage: true, maxDiffPixels: 1000 });
});
