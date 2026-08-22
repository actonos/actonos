import * as XLSX from 'xlsx';
import mammoth from 'mammoth';

// Dynamic import or configured pdfjs-dist
let pdfjsPromise: Promise<typeof import('pdfjs-dist/legacy/build/pdf.mjs')> | null = null;

async function getPdfjs() {
  if (!pdfjsPromise) {
    pdfjsPromise = import('pdfjs-dist/legacy/build/pdf.mjs').then((pdfjs) => {
      // In browser environment, configure worker if needed
      try {
        if (pdfjs.GlobalWorkerOptions && !pdfjs.GlobalWorkerOptions.workerSrc) {
          pdfjs.GlobalWorkerOptions.workerSrc = new URL(
            'pdfjs-dist/legacy/build/pdf.worker.mjs',
            import.meta.url
          ).toString();
        }
      } catch {
        // Fallback to inline execution
      }
      return pdfjs;
    });
  }
  return pdfjsPromise;
}

async function getFileArrayBuffer(
  file: File | Blob | { arrayBuffer?: () => Promise<ArrayBuffer> }
): Promise<ArrayBuffer> {
  if (typeof file.arrayBuffer === 'function') {
    return file.arrayBuffer();
  }
  return new Promise<ArrayBuffer>((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result as ArrayBuffer);
    reader.onerror = () => reject(new Error('Failed to read file buffer'));
    reader.readAsArrayBuffer(file as Blob);
  });
}

export interface ExtractedDocumentResult {
  text: string;
  pageCount?: number;
  sheetCount?: number;
  truncated?: boolean;
}

/**
 * Extracts human-readable plain text / Markdown from PDF, DOCX, XLSX, XLS, CSV, or code/text files.
 */
export async function extractTextFromDocument(
  file: File | { name: string; type?: string; arrayBuffer?: () => Promise<ArrayBuffer> },
  maxChars = 120000
): Promise<ExtractedDocumentResult> {
  const name = file.name.trim();
  const ext = name.split('.').pop()?.toLowerCase() || '';

  const buffer = await getFileArrayBuffer(file);

  // 1. PDF Parser
  if (ext === 'pdf') {
    try {
      const pdfjs = await getPdfjs();
      const loadingTask = pdfjs.getDocument({
        data: new Uint8Array(buffer),
        useSystemFonts: true,
        disableFontFace: true,
      });
      const pdf = await loadingTask.promise;
      const pagesText: string[] = [];

      for (let i = 1; i <= pdf.numPages; i++) {
        const page = await pdf.getPage(i);
        const textContent = await page.getTextContent();
        const lineText = textContent.items
          .map((item: unknown) =>
            typeof item === 'object' && item && 'str' in item ? (item as { str: string }).str : ''
          )
          .join(' ')
          .replace(/\s+/g, ' ')
          .trim();

        if (lineText) {
          pagesText.push(`[Page ${i}]\n${lineText}`);
        }
      }

      let fullText = pagesText.join('\n\n');
      let truncated = false;
      if (fullText.length > maxChars) {
        fullText =
          fullText.slice(0, maxChars) +
          `\n\n... [PDF truncated: showing first ${(maxChars / 1024).toFixed(1)} KB of ${pdf.numPages} pages] ...`;
        truncated = true;
      }

      return {
        text: fullText || '(PDF contains no selectable text or scanned images only)',
        pageCount: pdf.numPages,
        truncated,
      };
    } catch (err: unknown) {
      throw new Error(`Failed to parse PDF document: ${err instanceof Error ? err.message : String(err)}`);
    }
  }

  // 2. DOCX Parser
  if (ext === 'docx') {
    try {
      const result = await mammoth.extractRawText({ arrayBuffer: buffer });
      let fullText = result.value.trim();
      let truncated = false;

      if (fullText.length > maxChars) {
        fullText =
          fullText.slice(0, maxChars) +
          `\n\n... [DOCX truncated: showing first ${(maxChars / 1024).toFixed(1)} KB] ...`;
        truncated = true;
      }

      return {
        text: fullText || '(DOCX document is empty)',
        truncated,
      };
    } catch (err: unknown) {
      throw new Error(`Failed to parse DOCX document: ${err instanceof Error ? err.message : String(err)}`);
    }
  }

  // 3. Spreadsheet Parser (XLSX, XLS, ODS, CSV, TSV)
  if (['xlsx', 'xls', 'ods', 'csv', 'tsv'].includes(ext)) {
    try {
      const workbook = XLSX.read(buffer, { type: 'array' });
      const sheetsText: string[] = [];

      for (const sheetName of workbook.SheetNames) {
        const sheet = workbook.Sheets[sheetName];
        if (!sheet) continue;
        const csv = XLSX.utils.sheet_to_csv(sheet);
        if (csv.trim()) {
          sheetsText.push(`### Sheet: ${sheetName}\n\`\`\`csv\n${csv.trim()}\n\`\`\``);
        }
      }

      let fullText = sheetsText.join('\n\n');
      let truncated = false;

      if (fullText.length > maxChars) {
        fullText =
          fullText.slice(0, maxChars) +
          `\n\n... [Spreadsheet truncated: showing first ${(maxChars / 1024).toFixed(1)} KB of ${workbook.SheetNames.length} sheets] ...`;
        truncated = true;
      }

      return {
        text: fullText || '(Spreadsheet contains no readable data)',
        sheetCount: workbook.SheetNames.length,
        truncated,
      };
    } catch (err: unknown) {
      throw new Error(`Failed to parse spreadsheet: ${err instanceof Error ? err.message : String(err)}`);
    }
  }

  // 4. Default Text / Code Parser (UTF-8)
  try {
    const decoder = new TextDecoder('utf-8', { fatal: false });
    let text = decoder.decode(buffer);
    let truncated = false;

    if (text.length > maxChars) {
      text =
        text.slice(0, maxChars) +
        `\n\n... [Content truncated: showing first ${(maxChars / 1024).toFixed(1)} KB] ...`;
      truncated = true;
    }

    return {
      text,
      truncated,
    };
  } catch (err: unknown) {
    throw new Error(`Failed to read text file: ${err instanceof Error ? err.message : String(err)}`);
  }
}
