import { useState, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Copy, Check, Code, ListTree, AlertCircle } from 'lucide-react';

interface WorkspaceJsonViewerProps {
  content: string;
}

interface TreeNodeProps {
  name?: string;
  value: unknown;
  level?: number;
}

function TreeNode({ name, value, level = 0 }: TreeNodeProps) {
  const [collapsed, setCollapsed] = useState(level > 1);

  if (value === null || value === undefined) {
    return (
      <div style={{ paddingLeft: `${level * 16}px` }} className="py-0.5 text-caption font-mono">
        {name && <span className="text-deep-ink font-semibold">{name}: </span>}
        <span className="text-slate italic">{'null'}</span>
      </div>
    );
  }

  if (typeof value === 'boolean') {
    return (
      <div style={{ paddingLeft: `${level * 16}px` }} className="py-0.5 text-caption font-mono">
        {name && <span className="text-deep-ink font-semibold">{name}: </span>}
        <span className="text-fuchsia font-semibold">{value ? 'true' : 'false'}</span>
      </div>
    );
  }

  if (typeof value === 'number') {
    return (
      <div style={{ paddingLeft: `${level * 16}px` }} className="py-0.5 text-caption font-mono">
        {name && <span className="text-deep-ink font-semibold">{name}: </span>}
        <span className="text-moss-green font-semibold">{value}</span>
      </div>
    );
  }

  if (typeof value === 'string') {
    return (
      <div style={{ paddingLeft: `${level * 16}px` }} className="py-0.5 text-caption font-mono">
        {name && <span className="text-deep-ink font-semibold">{name}: </span>}
        <span className="text-status-success">"{value}"</span>
      </div>
    );
  }

  const isArray = Array.isArray(value);
  const record = (typeof value === 'object' && value !== null ? value : {}) as Record<string, unknown>;
  const keys = Object.keys(record);
  const count = keys.length;

  return (
    <div className="select-text">
      <div
        onClick={() => setCollapsed(!collapsed)}
        style={{ paddingLeft: `${level * 16}px` }}
        className="flex items-center gap-1.5 py-0.5 text-caption font-mono cursor-pointer hover:bg-soft-meadow/50 rounded"
      >
        <span className="text-slate font-bold w-3 text-center">{collapsed ? '+' : '-'}</span>
        {name && <span className="text-deep-ink font-semibold">{name}: </span>}
        <span className="text-slate">
          {isArray ? `[${count}]` : `{${count}}`}
        </span>
      </div>

      {!collapsed && (
        <div>
          {keys.map((key) => (
            <TreeNode
              key={key}
              name={isArray ? undefined : key}
              value={record[key]}
              level={level + 1}
            />
          ))}
        </div>
      )}
    </div>
  );
}

export function WorkspaceJsonViewer({ content }: WorkspaceJsonViewerProps) {
  const { t } = useTranslation('workspace');
  const [viewMode, setViewMode] = useState<'tree' | 'formatted'>('tree');
  const [copied, setCopied] = useState(false);

  const { parsed, isValid, formatted } = useMemo(() => {
    try {
      const obj = JSON.parse(content);
      return {
        parsed: obj,
        isValid: true,
        formatted: JSON.stringify(obj, null, 2),
      };
    } catch {
      return {
        parsed: null,
        isValid: false,
        formatted: content,
      };
    }
  }, [content]);

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(formatted);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {}
  };

  return (
    <div className="h-full flex flex-col bg-canvas text-deep-ink">
      {/* Action Header */}
      <div className="flex items-center justify-between p-3 border-b border-deep-ink/10 bg-soft-meadow/30">
        <div className="flex items-center gap-2">
          <div className="flex items-center p-0.5 rounded-full bg-soft-meadow border border-deep-ink/10">
            <button
              onClick={() => setViewMode('tree')}
              className={`flex items-center gap-1.5 px-3 py-1 rounded-full text-caption font-semibold transition-colors ${
                viewMode === 'tree' ? 'bg-deep-ink text-canvas' : 'text-slate hover:text-deep-ink'
              }`}
            >
              <ListTree className="w-3.5 h-3.5" />
              <span>{t('jsonViewer.tree')}</span>
            </button>
            <button
              onClick={() => setViewMode('formatted')}
              className={`flex items-center gap-1.5 px-3 py-1 rounded-full text-caption font-semibold transition-colors ${
                viewMode === 'formatted' ? 'bg-deep-ink text-canvas' : 'text-slate hover:text-deep-ink'
              }`}
            >
              <Code className="w-3.5 h-3.5" />
              <span>{t('jsonViewer.raw')}</span>
            </button>
          </div>

          {!isValid && (
            <div className="flex items-center gap-1.5 px-2.5 py-1 rounded-full bg-status-danger-soft text-status-danger text-caption font-semibold">
              <AlertCircle className="w-3.5 h-3.5" />
              <span>{t('jsonViewer.invalidJson')}</span>
            </div>
          )}
        </div>

        <button
          onClick={handleCopy}
          className="flex items-center gap-1.5 px-3 py-1 rounded-full border border-deep-ink/10 bg-canvas hover:bg-soft-meadow text-body-sm font-medium transition-colors"
        >
          {copied ? <Check className="w-3.5 h-3.5 text-status-success" /> : <Copy className="w-3.5 h-3.5 text-slate" />}
          <span>{copied ? t('jsonViewer.copied') : t('jsonViewer.copy')}</span>
        </button>
      </div>

      {/* Content view */}
      <div className="flex-1 overflow-auto p-4 font-mono">
        {viewMode === 'tree' && isValid ? (
          <div className="p-2 rounded-2xl border border-deep-ink/10 bg-soft-meadow/20">
            <TreeNode value={parsed} level={0} />
          </div>
        ) : (
          <pre className="p-4 rounded-2xl bg-deep-ink text-canvas text-caption font-mono overflow-x-auto">
            <code>{formatted}</code>
          </pre>
        )}
      </div>
    </div>
  );
}
