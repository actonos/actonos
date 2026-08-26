import { describe, expect, it } from 'vitest';
import { parseMarkdownToHTML, sanitizeMarkdownHTML, unwrapStandaloneImages } from './MarkdownContent';

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

  it('does not wrap a standalone markdown image in a paragraph', () => {
    const html = parseMarkdownToHTML('![diagram](https://x.test/a.png)');
    expect(html).toContain('src="https://x.test/a.png"');
    expect(html).not.toMatch(/<p>\s*<img/i);
    expect(html).not.toMatch(/<p>\s*(<br\s*\/?>)?\s*<\/p>/i);
  });

  it('keeps text paragraphs around a following image', () => {
    const html = parseMarkdownToHTML('hello\n\n![diagram](https://x.test/a.png)\n\nworld');
    expect(html).toContain('<p>hello</p>');
    expect(html).toContain('<p>world</p>');
    expect(html).toContain('src="https://x.test/a.png"');
    expect(html).not.toMatch(/<p>\s*<img/i);
  });

  it('lifts multiple images out of one marked paragraph', () => {
    const html = unwrapStandaloneImages(
      '<p><img src="https://x.test/a.png" alt="a"><br><img src="https://x.test/b.png" alt="b"></p>',
    );
    expect(html).toBe(
      '<img src="https://x.test/a.png" alt="a"><img src="https://x.test/b.png" alt="b">',
    );
  });
});
