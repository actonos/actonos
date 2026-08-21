import { useState, useEffect, useRef, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Save,
  Columns2,
  X,
  FileCode,
  FileText,
  Layers,
} from 'lucide-react';
import { WorkspaceMarkdownPreview } from './WorkspaceMarkdownPreview';
import { WorkspaceTableViewer } from './WorkspaceTableViewer';
import { WorkspaceJsonViewer } from './WorkspaceJsonViewer';
import { WorkspaceMediaViewer } from './WorkspaceMediaViewer';

export interface OpenTab {
  id: string;
  parentId: string;
  path: string;
  name: string;
  content: string;
  originalContent: string;
  kind: string;
  mime: string;
  dataUrl?: string;
  size: number;
  version: number;
}

interface WorkspaceEditorProps {
  tabs: OpenTab[];
  activeTabPath: string | null;
  saving: boolean;
  onSelectTab: (path: string) => void;
  onCloseTab: (path: string) => void;
  onChangeContent: (path: string, content: string) => void;
  onSave: (path: string) => void;
  showInspector: boolean;
  onToggleInspector: () => void;
}

export function WorkspaceEditor({
  tabs,
  activeTabPath,
  saving,
  onSelectTab,
  onCloseTab,
  onChangeContent,
  onSave,
  showInspector,
  onToggleInspector,
}: WorkspaceEditorProps) {
  const { t } = useTranslation('workspace');
  const [markdownMode, setMarkdownMode] = useState<'code' | 'split' | 'preview'>('code');
  const [showDiff, setShowDiff] = useState(false);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const lineNumbersRef = useRef<HTMLDivElement>(null);

  const activeTab = useMemo(
    () => tabs.find((tab) => tab.id === activeTabPath) || null,
    [tabs, activeTabPath]
  );

  // Keyboard shortcut Ctrl+S
  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      if ((e.ctrlKey || e.metaKey) && e.key === 's') {
        e.preventDefault();
        if (activeTab) {
          onSave(activeTab.id);
        }
      }
    }
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [activeTab, onSave]);

  // Sync line numbers scroll with textarea
  const handleScroll = () => {
    if (textareaRef.current && lineNumbersRef.current) {
      lineNumbersRef.current.scrollTop = textareaRef.current.scrollTop;
    }
  };

  // Support Tab key in textarea
  const handleKeyDownTextarea = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Tab') {
      e.preventDefault();
      const target = e.currentTarget;
      const start = target.selectionStart;
      const end = target.selectionEnd;
      const val = target.value;
      const updated = val.substring(0, start) + '  ' + val.substring(end);
      if (activeTab) {
        onChangeContent(activeTab.id, updated);
      }
      setTimeout(() => {
        target.selectionStart = target.selectionEnd = start + 2;
      }, 0);
    }
  };

  if (!activeTab) {
    return (
      <div className="flex-1 flex flex-col items-center justify-center p-8 text-center text-slate bg-canvas">
        <div className="w-16 h-16 rounded-full bg-soft-meadow flex items-center justify-center mb-3">
          <FileCode className="w-8 h-8 text-deep-ink/40" />
        </div>
        <h3 className="text-subheading font-bold text-deep-ink mb-1">{t('editor.noSelection')}</h3>
        <p className="text-body-sm text-slate max-w-sm">{t('editor.noSelectionDescription')}</p>
      </div>
    );
  }

  const isDirty = activeTab.content !== activeTab.originalContent;
  const lines = activeTab.content.split('\n');
  const linesCount = lines.length;
  const charsCount = activeTab.content.length;
  const tokenEstimate = Math.max(1, Math.round(charsCount / 4));

  const isMarkdown = activeTab.kind === 'markdown';
  const isCsv = activeTab.kind === 'csv';
  const isJson = activeTab.kind === 'json';
  const isMediaOrBinary =
    activeTab.kind === 'image' ||
    activeTab.kind === 'pdf' ||
    activeTab.kind === 'audio' ||
    activeTab.kind === 'video' ||
    activeTab.kind === 'binary' ||
    activeTab.kind === 'archive' ||
    activeTab.kind === 'document';

  return (
    <div className="flex-1 flex flex-col h-full bg-canvas text-deep-ink overflow-hidden">
      {/* Top Tab Bar */}
      <div className="flex items-center justify-between border-b border-deep-ink/10 bg-soft-meadow/50 px-2 pt-2 gap-2 overflow-x-auto select-none">
        <div className="flex items-center gap-1 overflow-x-auto">
          {tabs.map((tab) => {
            const isActive = tab.id === activeTabPath;
            const dirty = tab.content !== tab.originalContent;
            return (
              <div
                key={tab.id}
                onClick={() => onSelectTab(tab.id)}
                className={`group flex items-center gap-2 px-3 py-1.5 rounded-t-xl text-body-sm font-medium cursor-pointer transition-all border-t border-x ${isActive
                  ? 'bg-canvas border-deep-ink/10 text-deep-ink font-semibold shadow-xs'
                  : 'bg-soft-meadow/40 border-transparent text-slate hover:text-deep-ink'
                  }`}
              >
                {/* Dirty dot */}
                {dirty ? (
                  <span className="w-2 h-2 rounded-full bg-hi-yellow shrink-0" title={t('editor.unsavedChanges')} />
                ) : (
                  <FileText className="w-3.5 h-3.5 text-slate shrink-0" />
                )}

                <span className="truncate max-w-[140px]">{tab.name}</span>

                <button
                  onClick={(e) => {
                    e.stopPropagation();
                    onCloseTab(tab.id);
                  }}
                  className="p-0.5 rounded-full hover:bg-deep-ink/10 text-slate hover:text-deep-ink transition-colors opacity-60 group-hover:opacity-100"
                >
                  <X className="w-3 h-3" />
                </button>
              </div>
            );
          })}
        </div>

        {/* Tab Bar Right Controls */}
        <div className="flex items-center gap-2 pb-2 shrink-0">
          {isMarkdown && (
            <div className="flex items-center p-0.5 rounded-full bg-canvas border border-deep-ink/10 text-caption font-semibold">
              <button
                onClick={() => setMarkdownMode('code')}
                className={`px-2.5 py-0.5 rounded-full transition-colors ${markdownMode === 'code' ? 'bg-deep-ink text-canvas' : 'text-slate hover:text-deep-ink'
                  }`}
              >
                {t('editor.modeCode')}
              </button>
              <button
                onClick={() => setMarkdownMode('split')}
                className={`px-2.5 py-0.5 rounded-full transition-colors ${markdownMode === 'split' ? 'bg-deep-ink text-canvas' : 'text-slate hover:text-deep-ink'
                  }`}
              >
                {t('editor.modeSplit')}
              </button>
              <button
                onClick={() => setMarkdownMode('preview')}
                className={`px-2.5 py-0.5 rounded-full transition-colors ${markdownMode === 'preview' ? 'bg-deep-ink text-canvas' : 'text-slate hover:text-deep-ink'
                  }`}
              >
                {t('editor.modePreview')}
              </button>
            </div>
          )}

          {!isMediaOrBinary && (
            <>
              <button
                onClick={() => setShowDiff(!showDiff)}
                className={`flex items-center gap-1.5 px-3 py-1 rounded-full border border-deep-ink/10 text-caption font-semibold transition-colors ${showDiff ? 'bg-hi-yellow text-deep-ink' : 'bg-canvas hover:bg-soft-meadow text-slate'
                  }`}
              >
                <Columns2 className="w-3.5 h-3.5" />
                <span>{showDiff ? t('diff.hideDiff') : t('diff.showDiff')}</span>
              </button>

              <button
                disabled={saving || !isDirty}
                onClick={() => onSave(activeTab.id)}
                className={`flex items-center gap-1.5 px-4 py-1 rounded-full text-caption font-semibold transition-all ${isDirty
                  ? 'bg-deep-ink text-canvas hover:bg-deep-ink/90 cursor-pointer shadow-xs'
                  : 'bg-soft-meadow text-slate/50 cursor-not-allowed'
                  }`}
              >
                <Save className={`w-3.5 h-3.5 ${saving ? 'animate-spin' : ''}`} />
                <span>{saving ? t('actions.saving') : t('actions.save')}</span>
              </button>
            </>
          )}

          {/* Inspector Toggle */}
          <button
            onClick={onToggleInspector}
            className={`p-1.5 rounded-full border border-deep-ink/10 transition-colors cursor-pointer ${showInspector ? 'bg-deep-ink text-canvas' : 'bg-canvas hover:bg-soft-meadow text-slate'
              }`}
            title={t('inspector.title')}
          >
            <Layers className="w-4 h-4" />
          </button>
        </div>
      </div>

      {/* Editor Main Content Body */}
      <div className="flex-1 overflow-hidden relative">
        {/* Media or Binary files */}
        {isMediaOrBinary ? (
          <WorkspaceMediaViewer
            path={activeTab.id}
            kind={activeTab.kind}
            dataUrl={activeTab.dataUrl}
            size={activeTab.size}
          />
        ) : isCsv ? (
          <WorkspaceTableViewer
            content={activeTab.content}
            isTsv={activeTab.name.toLowerCase().endsWith('.tsv')}
          />
        ) : isJson && !showDiff ? (
          <WorkspaceJsonViewer content={activeTab.content} />
        ) : showDiff ? (
          /* Side-by-side Diff View */
          <div className="h-full grid grid-cols-2 divide-x divide-deep-ink/10 overflow-hidden font-mono text-caption">
            <div className="flex flex-col h-full overflow-hidden bg-status-danger-soft/20">
              <div className="px-3 py-1.5 bg-status-danger-soft text-status-danger font-bold uppercase tracking-wider text-[11px] border-b border-deep-ink/5">
                {t('diff.before')}
              </div>
              <pre className="flex-1 p-4 overflow-auto whitespace-pre-wrap select-text">
                {activeTab.originalContent}
              </pre>
            </div>
            <div className="flex flex-col h-full overflow-hidden bg-status-success-soft/20">
              <div className="px-3 py-1.5 bg-status-success-soft text-status-success font-bold uppercase tracking-wider text-[11px] border-b border-deep-ink/5">
                {t('diff.after')}
              </div>
              <pre className="flex-1 p-4 overflow-auto whitespace-pre-wrap select-text">
                {activeTab.content}
              </pre>
            </div>
          </div>
        ) : isMarkdown ? (
          /* Markdown modes: Code / Split / Preview */
          <div className="h-full flex overflow-hidden">
            {(markdownMode === 'code' || markdownMode === 'split') && (
              <div className={`h-full flex overflow-hidden ${markdownMode === 'split' ? 'w-1/2 border-r border-deep-ink/10' : 'w-full'}`}>
                {/* Line numbers gutter */}
                <div
                  ref={lineNumbersRef}
                  className="w-12 py-4 bg-soft-meadow/40 text-slate/50 font-mono text-caption text-right select-none pr-3 overflow-hidden border-r border-deep-ink/5"
                >
                  {lines.map((_, i) => (
                    <div key={i} className="leading-6">
                      {i + 1}
                    </div>
                  ))}
                </div>

                {/* Textarea */}
                <textarea
                  ref={textareaRef}
                  value={activeTab.content}
                  onChange={(e) => onChangeContent(activeTab.id, e.target.value)}
                  onScroll={handleScroll}
                  onKeyDown={handleKeyDownTextarea}
                  placeholder={t('editor.placeholder')}
                  spellCheck={false}
                  className="flex-1 h-full p-4 bg-canvas text-deep-ink font-mono text-body-sm leading-6 resize-none focus:outline-none overflow-auto"
                />
              </div>
            )}

            {(markdownMode === 'preview' || markdownMode === 'split') && (
              <div className={`h-full overflow-hidden ${markdownMode === 'split' ? 'w-1/2' : 'w-full'}`}>
                <WorkspaceMarkdownPreview content={activeTab.content} />
              </div>
            )}
          </div>
        ) : (
          /* Standard Code & Text Editor with Line Numbers Gutter */
          <div className="h-full flex overflow-hidden">
            {/* Line Numbers */}
            <div
              ref={lineNumbersRef}
              className="w-12 py-4 bg-soft-meadow/40 text-slate/50 font-mono text-caption text-right select-none pr-3 overflow-hidden border-r border-deep-ink/5"
            >
              {lines.map((_, i) => (
                <div key={i} className="leading-6">
                  {i + 1}
                </div>
              ))}
            </div>

            {/* Textarea Code Input */}
            <textarea
              ref={textareaRef}
              value={activeTab.content}
              onChange={(e) => onChangeContent(activeTab.id, e.target.value)}
              onScroll={handleScroll}
              onKeyDown={handleKeyDownTextarea}
              placeholder={t('editor.placeholder')}
              spellCheck={false}
              className="flex-1 h-full p-4 bg-canvas text-deep-ink font-mono text-body-sm leading-6 resize-none focus:outline-none overflow-auto"
            />
          </div>
        )}
      </div>

      {/* Editor Status Bar */}
      <div className="flex items-center justify-between px-4 py-1.5 border-t border-deep-ink/10 bg-soft-meadow/30 text-caption font-mono text-slate select-none">
        <div className="flex items-center gap-3 truncate">
          <span className="truncate">{activeTab.path}</span>
          <span>•</span>
          <span>UTF-8</span>
          <span>•</span>
          <span>{activeTab.kind.toUpperCase()}</span>
        </div>

        <div className="flex items-center gap-3 shrink-0">
          <span>{t('editor.stats', { lines: linesCount, chars: charsCount, bytes: activeTab.size })}</span>
          <span>•</span>
          <span className="text-status-success font-semibold">{t('editor.tokens', { tokens: tokenEstimate })}</span>
          <span>•</span>
          <span>{t('editor.shortcut')}</span>
        </div>
      </div>
    </div>
  );
}
