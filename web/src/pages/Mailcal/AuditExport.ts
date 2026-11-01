import type { MailcalDecision } from '@/lib/api/mailcal';

const HEADER = ['at', 'phase', 'account_id', 'tool', 'verdict', 'rule_id', 'approved', 'outcome', 'draft_id', 'payload_hash'] as const;

/** A cell that starts with one of these is read as a formula by spreadsheet programs. */
const FORMULA_START = /^[=+\-@\t\r\n]/;

function cell(value: unknown): string {
  let s = value === undefined || value === null ? '' : String(value);
  if (FORMULA_START.test(s)) s = `'${s}`;
  return `"${s.replace(/"/g, '""')}"`;
}

/** Audit lines as CSV. It carries ids, hashes and verdicts only, never message text. */
export function toCSV(rows: MailcalDecision[]): string {
  const lines = [HEADER.join(',')];
  for (const r of rows) {
    lines.push(
      [r.at, r.phase, r.account_id, r.tool, r.verdict, r.rule_id, r.approved ? 'yes' : 'no', r.outcome, r.draft_id, r.payload_hash]
        .map(cell)
        .join(',')
    );
  }
  return lines.join('\r\n') + '\r\n';
}
