import { useState, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Search, ChevronLeft, ChevronRight, ArrowUpDown } from 'lucide-react';

interface WorkspaceTableViewerProps {
  content: string;
  isTsv?: boolean;
}

function parseDelimited(text: string, delimiter: string): string[][] {
  const lines = text.trim().split(/\r?\n/);
  return lines.map((line) => {
    // Simple CSV parser supporting quotes
    const values: string[] = [];
    let current = '';
    let inQuotes = false;
    for (let i = 0; i < line.length; i++) {
      const char = line[i];
      if (char === '"' || char === "'") {
        inQuotes = !inQuotes;
      } else if (char === delimiter && !inQuotes) {
        values.push(current.trim());
        current = '';
      } else {
        current += char;
      }
    }
    values.push(current.trim());
    return values;
  });
}

export function WorkspaceTableViewer({ content, isTsv = false }: WorkspaceTableViewerProps) {
  const { t } = useTranslation('workspace');
  const [filterQuery, setFilterQuery] = useState('');
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(25);
  const [sortCol, setSortCol] = useState<number | null>(null);
  const [sortAsc, setSortAsc] = useState(true);

  const delimiter = isTsv ? '\t' : ',';
  const rawRows = useMemo(() => parseDelimited(content, delimiter), [content, delimiter]);

  const headers = rawRows.length > 0 ? rawRows[0] : [];
  const dataRows = rawRows.slice(1);

  const filteredRows = useMemo(() => {
    if (!filterQuery.trim()) return dataRows;
    const q = filterQuery.toLowerCase();
    return dataRows.filter((row) => row.some((cell) => cell.toLowerCase().includes(q)));
  }, [dataRows, filterQuery]);

  const sortedRows = useMemo(() => {
    if (sortCol === null) return filteredRows;
    return [...filteredRows].sort((a, b) => {
      const valA = a[sortCol] || '';
      const valB = b[sortCol] || '';
      const numA = Number(valA);
      const numB = Number(valB);
      if (!isNaN(numA) && !isNaN(numB)) {
        return sortAsc ? numA - numB : numB - numA;
      }
      return sortAsc ? valA.localeCompare(valB) : valB.localeCompare(valA);
    });
  }, [filteredRows, sortCol, sortAsc]);

  const totalPages = Math.max(1, Math.ceil(sortedRows.length / pageSize));
  const currentPageRows = useMemo(() => {
    const start = (page - 1) * pageSize;
    return sortedRows.slice(start, start + pageSize);
  }, [sortedRows, page, pageSize]);

  const handleHeaderClick = (colIndex: number) => {
    if (sortCol === colIndex) {
      setSortAsc(!sortAsc);
    } else {
      setSortCol(colIndex);
      setSortAsc(true);
    }
  };

  return (
    <div className="h-full flex flex-col bg-canvas text-deep-ink">
      {/* Control bar */}
      <div className="flex flex-wrap items-center justify-between gap-3 p-3 border-b border-deep-ink/10 bg-soft-meadow/30">
        <div className="flex items-center gap-2">
          <div className="relative">
            <Search className="w-4 h-4 text-slate absolute left-3 top-1/2 -translate-y-1/2" />
            <input
              type="text"
              value={filterQuery}
              onChange={(e) => {
                setFilterQuery(e.target.value);
                setPage(1);
              }}
              placeholder={t('tableViewer.search')}
              className="pl-9 pr-3 py-1.5 rounded-full border border-deep-ink/10 bg-canvas text-body-sm focus:outline-none focus:border-deep-ink w-64"
            />
          </div>
          <span className="text-caption text-slate font-medium">
            {t('tableViewer.rows', { count: filteredRows.length })} • {t('tableViewer.cols', { count: headers.length })}
          </span>
        </div>

        {/* Pagination controls */}
        <div className="flex items-center gap-2">
          <select
            value={pageSize}
            onChange={(e) => {
              setPageSize(Number(e.target.value));
              setPage(1);
            }}
            className="px-2 py-1 rounded-full border border-deep-ink/10 bg-canvas text-caption focus:outline-none"
          >
            <option value={25}>{t('tableViewer.perPage', { count: 25 })}</option>
            <option value={50}>{t('tableViewer.perPage', { count: 50 })}</option>
            <option value={100}>{t('tableViewer.perPage', { count: 100 })}</option>
          </select>
          <span className="text-caption text-slate">
            {t('tableViewer.page', { current: page, total: totalPages })}
          </span>
          <button
            disabled={page <= 1}
            onClick={() => setPage((p) => Math.max(1, p - 1))}
            className="p-1 rounded-full border border-deep-ink/10 hover:bg-soft-meadow disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
          >
            <ChevronLeft className="w-4 h-4" />
          </button>
          <button
            disabled={page >= totalPages}
            onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
            className="p-1 rounded-full border border-deep-ink/10 hover:bg-soft-meadow disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
          >
            <ChevronRight className="w-4 h-4" />
          </button>
        </div>
      </div>

      {/* Table viewport */}
      <div className="flex-1 overflow-auto p-3">
        <div className="rounded-2xl border border-deep-ink/10 overflow-hidden inline-block min-w-full">
          <table className="w-full text-left border-collapse text-body-sm">
            <thead className="bg-soft-meadow font-semibold text-deep-ink sticky top-0 z-10 border-b border-deep-ink/10">
              <tr>
                <th className="p-2.5 text-caption font-mono text-slate border-r border-deep-ink/5 w-12 text-center">#</th>
                {headers.map((header, idx) => (
                  <th
                    key={idx}
                    onClick={() => handleHeaderClick(idx)}
                    className="p-2.5 cursor-pointer hover:bg-deep-ink/5 transition-colors border-r border-deep-ink/5 select-none"
                  >
                    <div className="flex items-center justify-between gap-2">
                      <span className="truncate">{header || `Col ${idx + 1}`}</span>
                      <ArrowUpDown className={`w-3.5 h-3.5 ${sortCol === idx ? 'text-deep-ink' : 'text-slate/40'}`} />
                    </div>
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {currentPageRows.map((row, rowIdx) => {
                const globalRowIndex = (page - 1) * pageSize + rowIdx + 1;
                return (
                  <tr
                    key={rowIdx}
                    className={`border-b border-deep-ink/5 hover:bg-soft-meadow/50 transition-colors ${
                      rowIdx % 2 === 0 ? 'bg-canvas' : 'bg-soft-meadow/20'
                    }`}
                  >
                    <td className="p-2 text-caption font-mono text-slate/70 text-center border-r border-deep-ink/5">
                      {globalRowIndex}
                    </td>
                    {headers.map((_, colIdx) => (
                      <td key={colIdx} className="p-2 border-r border-deep-ink/5 truncate max-w-xs" title={row[colIdx]}>
                        {row[colIdx] || ''}
                      </td>
                    ))}
                  </tr>
                );
              })}
              {currentPageRows.length === 0 && (
                <tr>
                  <td colSpan={headers.length + 1} className="p-8 text-center text-slate">
                    {t('table.empty')}
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}
