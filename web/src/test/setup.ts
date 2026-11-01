import '@testing-library/jest-dom/vitest';
import { beforeAll } from 'vitest';
import { i18nReady } from '@/lib/i18n';

beforeAll(async () => {
  await i18nReady;
});

if (!Element.prototype.scrollIntoView) {
  Element.prototype.scrollIntoView = function scrollIntoView() {
    return undefined;
  };
}
