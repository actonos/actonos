import { expect, test } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

test('ActonOS shell renders', async ({ page }) => {
  await page.goto('/');
  await expect(page.locator('#root')).toBeVisible();
  await expect(page).toHaveTitle(/ActonOS/);
});

test('ActonOS shell has no serious accessibility violations', async ({ page }) => {
  await page.goto('/');
  await expect(page.locator('#root')).toBeVisible();
  const results = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'])
    .analyze();
  const violations = results.violations.filter(
    (violation) => violation.impact === 'critical' || violation.impact === 'serious'
  );
  expect(violations, JSON.stringify(violations, null, 2)).toEqual([]);
});
