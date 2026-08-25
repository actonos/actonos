import { describe, expect, it } from 'vitest';
import { parseMarkdownToHTML, sanitizeMarkdownHTML } from './MarkdownContent';

describe('parseMarkdownToHTML', () => {
  it('neutralizes javascript: links produced from markdown', () => {
    const html = parseMarkdownToHTML('[pwn](javascript:alert(1))');
    expect(html.toLowerCase()).not.toContain('javascript:');
    expect(html).toContain('href="#"');
  });

  it('strips inline event handlers from rendered HTML', () => {
    const html = sanitizeMarkdownHTML('<a href="https://example.com" onclick="alert(1)">x</a>');
    expect(html.toLowerCase()).not.toContain('onclick');
    expect(html).toContain('https://example.com');
  });
});
