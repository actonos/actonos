import { describe, it, expect } from 'vitest';
import { isImageFile, preprocessImage } from './imagePreprocessor';

describe('imagePreprocessor', () => {
  it('identifies image files correctly', () => {
    expect(isImageFile('photo.jpg')).toBe(true);
    expect(isImageFile('screenshot.PNG')).toBe(true);
    expect(isImageFile('diagram.webp')).toBe(true);
    expect(isImageFile('vector.svg')).toBe(true);
    expect(isImageFile('icon.gif')).toBe(true);
    expect(isImageFile('file.txt')).toBe(false);
    expect(isImageFile('code.ts')).toBe(false);
    expect(isImageFile('unknown', 'image/jpeg')).toBe(true);
  });

  it('preprocesses SVG images to data URLs', async () => {
    const svgContent = '<svg xmlns="http://www.w3.org/2000/svg" width="100" height="100"><circle cx="50" cy="50" r="40"/></svg>';
    const blob = new Blob([svgContent], { type: 'image/svg+xml' });

    const result = await preprocessImage(blob, 'circle.svg');
    expect(result.mimeType).toBe('image/svg+xml');
    expect(result.dataUrl).toContain('data:image/svg+xml;base64,');
    expect(result.name).toBe('circle.svg');
  });
});
