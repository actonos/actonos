import { describe, it, expect } from 'vitest';
import { extractTextFromDocument } from './documentParser';
import * as XLSX from 'xlsx';

describe('documentParser', () => {
  it('extracts plain text file correctly', async () => {
    const textContent = 'Hello World!\nconst a = 42;';
    const blob = new Blob([textContent], { type: 'text/plain' });
    const file = new File([blob], 'test.ts', { type: 'text/plain' });

    const result = await extractTextFromDocument(file);
    expect(result.text).toContain('Hello World!');
    expect(result.text).toContain('const a = 42;');
    expect(result.truncated).toBe(false);
  });

  it('extracts spreadsheet CSV tables from xlsx workbook', async () => {
    const wb = XLSX.utils.book_new();
    const ws = XLSX.utils.aoa_to_sheet([
      ['Name', 'Role', 'Status'],
      ['Nova', 'Orchestrator', 'Active'],
      ['Nexus', 'Coder', 'Idle'],
    ]);
    XLSX.utils.book_append_sheet(wb, ws, 'Employees');

    const wbArray = XLSX.write(wb, { type: 'array', bookType: 'xlsx' });
    const blob = new Blob([wbArray], {
      type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
    });
    const file = new File([blob], 'employees.xlsx', {
      type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
    });

    const result = await extractTextFromDocument(file);
    expect(result.text).toContain('### Sheet: Employees');
    expect(result.text).toContain('Name,Role,Status');
    expect(result.text).toContain('Nova,Orchestrator,Active');
    expect(result.sheetCount).toBe(1);
  });

  it('truncates very large text when maxChars is exceeded', async () => {
    const largeText = 'A'.repeat(5000);
    const blob = new Blob([largeText], { type: 'text/plain' });
    const file = new File([blob], 'large.txt', { type: 'text/plain' });

    const result = await extractTextFromDocument(file, 1000);
    expect(result.text.length).toBeLessThan(1200);
    expect(result.text).toContain('[Content truncated:');
    expect(result.truncated).toBe(true);
  });
});
