import { describe, expect, it } from 'vitest';
import { needsHttpsForSignIn } from './secureOrigin';

describe('needsHttpsForSignIn', () => {
  it.each([
    ['http:', '192.168.1.20', true],
    ['http:', 'acton-mini.local', true],
    ['http:', 'acton-mini.tail1234.ts.net', true],
    ['https:', 'acton-mini.tail1234.ts.net', false],
    ['http:', 'localhost', false],
    ['http:', '127.0.0.1', false],
    ['http:', '[::1]', false],
    ['https:', '192.168.1.20', false],
  ])('%s //%s -> %s', (protocol, host, expected) => {
    expect(needsHttpsForSignIn(protocol, host)).toBe(expected);
  });
});
