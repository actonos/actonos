export interface ProcessedImageResult {
  dataUrl: string; // compressed base64 data URL for AI vision payload
  thumbnailUrl: string; // small thumbnail for fast UI rendering
  width: number;
  height: number;
  size: number; // processed size in bytes
  originalSize: number;
  mimeType: string;
  name: string;
}

export interface ImagePreprocessOptions {
  maxDimension?: number; // max width or height (default 1600px, optimal for LLM vision)
  quality?: number; // 0.1 to 1.0 (default 0.85)
  thumbDimension?: number; // default 240px
}

/**
 * Checks if a filename or MIME type is a supported image format.
 */
export function isImageFile(name: string, mimeType?: string): boolean {
  const cleanName = name.trim().toLowerCase();
  const ext = cleanName.split('.').pop() || '';
  if (['png', 'jpg', 'jpeg', 'webp', 'gif', 'bmp', 'svg'].includes(ext)) {
    return true;
  }
  if (mimeType && mimeType.startsWith('image/')) {
    return true;
  }
  return false;
}

/**
 * Preprocesses an image file in the browser:
 * - Downscales large images to maxDimension (e.g. 1600px) preserving aspect ratio
 * - Compresses to lightweight JPEG/WebP (typically reducing 5-15MB phone photos to ~150-300KB)
 * - Retains high clarity for OCR, text, diagrams, and AI vision analysis
 * - Generates UI preview thumbnails
 */
export async function preprocessImage(
  file: File | Blob,
  fileName = 'image.jpg',
  options: ImagePreprocessOptions = {}
): Promise<ProcessedImageResult> {
  const { maxDimension = 1600, quality = 0.85, thumbDimension = 240 } = options;
  const originalSize = file.size;

  // Handle SVG directly
  if (
    file.type === 'image/svg+xml' ||
    fileName.toLowerCase().endsWith('.svg')
  ) {
    const text = await new Promise<string>((resolve, reject) => {
      const reader = new FileReader();
      reader.onload = () => resolve((reader.result as string) || '');
      reader.onerror = () => reject(new Error('Failed to read SVG file'));
      reader.readAsText(file);
    });

    const dataUrl = `data:image/svg+xml;base64,${btoa(unescape(encodeURIComponent(text)))}`;
    return {
      dataUrl,
      thumbnailUrl: dataUrl,
      width: 800,
      height: 600,
      size: dataUrl.length,
      originalSize,
      mimeType: 'image/svg+xml',
      name: fileName,
    };
  }

  // Create an image element from file blob
  const imgUrl = URL.createObjectURL(file);
  try {
    const img = await new Promise<HTMLImageElement>((resolve, reject) => {
      const image = new Image();
      image.onload = () => resolve(image);
      image.onerror = () => reject(new Error('Failed to decode image file'));
      image.src = imgUrl;
    });

    const origWidth = img.naturalWidth || img.width;
    const origHeight = img.naturalHeight || img.height;

    if (!origWidth || !origHeight) {
      throw new Error('Image has zero dimensions or is corrupted');
    }

    // 1. Calculate scaled dimensions for AI Vision (maxDimension)
    let targetWidth = origWidth;
    let targetHeight = origHeight;

    if (origWidth > maxDimension || origHeight > maxDimension) {
      if (origWidth >= origHeight) {
        targetWidth = maxDimension;
        targetHeight = Math.round((origHeight * maxDimension) / origWidth);
      } else {
        targetHeight = maxDimension;
        targetWidth = Math.round((origWidth * maxDimension) / origHeight);
      }
    }

    // Render scaled image to Canvas
    const canvas = document.createElement('canvas');
    canvas.width = targetWidth;
    canvas.height = targetHeight;
    const ctx = canvas.getContext('2d');

    if (!ctx) {
      throw new Error('Could not initialize canvas 2D rendering context');
    }

    // Set white background for transparent images converted to JPEG
    ctx.fillStyle = '#FFFFFF';
    ctx.fillRect(0, 0, targetWidth, targetHeight);

    ctx.imageSmoothingEnabled = true;
    ctx.imageSmoothingQuality = 'high';
    ctx.drawImage(img, 0, 0, targetWidth, targetHeight);

    // Export compressed JPEG
    const dataUrl = canvas.toDataURL('image/jpeg', quality);
    const sizeInBytes = Math.round(
      (dataUrl.length - dataUrl.indexOf(',') - 1) * 0.75
    );

    // 2. Generate small UI Thumbnail
    let thumbWidth = origWidth;
    let thumbHeight = origHeight;
    if (origWidth > thumbDimension || origHeight > thumbDimension) {
      if (origWidth >= origHeight) {
        thumbWidth = thumbDimension;
        thumbHeight = Math.round((origHeight * thumbDimension) / origWidth);
      } else {
        thumbHeight = thumbDimension;
        thumbWidth = Math.round((origWidth * thumbDimension) / origHeight);
      }
    }

    const thumbCanvas = document.createElement('canvas');
    thumbCanvas.width = thumbWidth;
    thumbCanvas.height = thumbHeight;
    const thumbCtx = thumbCanvas.getContext('2d');
    let thumbnailUrl = dataUrl;

    if (thumbCtx) {
      thumbCtx.fillStyle = '#FFFFFF';
      thumbCtx.fillRect(0, 0, thumbWidth, thumbHeight);
      thumbCtx.imageSmoothingEnabled = true;
      thumbCtx.imageSmoothingQuality = 'medium';
      thumbCtx.drawImage(canvas, 0, 0, thumbWidth, thumbHeight);
      thumbnailUrl = thumbCanvas.toDataURL('image/jpeg', 0.75);
    }

    return {
      dataUrl,
      thumbnailUrl,
      width: targetWidth,
      height: targetHeight,
      size: sizeInBytes,
      originalSize,
      mimeType: 'image/jpeg',
      name: fileName,
    };
  } finally {
    URL.revokeObjectURL(imgUrl);
  }
}
