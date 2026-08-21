import { useMemo } from 'react';
import CodeMirror from '@uiw/react-codemirror';
import { EditorView } from '@codemirror/view';
import { type Extension } from '@codemirror/state';
import { javascript } from '@codemirror/lang-javascript';
import { markdown } from '@codemirror/lang-markdown';
import { json } from '@codemirror/lang-json';
import { python } from '@codemirror/lang-python';
import { html } from '@codemirror/lang-html';
import { css } from '@codemirror/lang-css';
import { sql } from '@codemirror/lang-sql';
import { rust } from '@codemirror/lang-rust';
import { cpp } from '@codemirror/lang-cpp';
import { go } from '@codemirror/lang-go';
import { yaml } from '@codemirror/lang-yaml';
import { xml } from '@codemirror/lang-xml';
import { java } from '@codemirror/lang-java';
import { useTheme } from '@/components/providers/ThemeProvider';

export interface WorkspaceCodeMirrorProps {
  value: string;
  filename: string;
  kind?: string;
  placeholder?: string;
  readOnly?: boolean;
  onChange?: (value: string) => void;
  onSave?: () => void;
}

export function getLanguageExtension(filename: string, kind?: string): Extension | null {
  const lowerName = filename.toLowerCase();
  const ext = lowerName.split('.').pop() || '';

  if (kind === 'markdown' || ['md', 'markdown', 'mdown', 'mkd'].includes(ext)) {
    return markdown();
  }
  if (['js', 'mjs', 'cjs'].includes(ext)) {
    return javascript();
  }
  if (['jsx'].includes(ext)) {
    return javascript({ jsx: true });
  }
  if (['ts', 'mts', 'cts'].includes(ext)) {
    return javascript({ typescript: true });
  }
  if (['tsx'].includes(ext)) {
    return javascript({ jsx: true, typescript: true });
  }
  if (['json', 'json5', 'jsonc'].includes(ext) || kind === 'json') {
    return json();
  }
  if (['py', 'pyw', 'python'].includes(ext) || kind === 'python') {
    return python();
  }
  if (['go'].includes(ext) || kind === 'go') {
    return go();
  }
  if (['rs'].includes(ext) || kind === 'rust') {
    return rust();
  }
  if (['c', 'cpp', 'cc', 'cxx', 'h', 'hpp', 'hh', 'hxx'].includes(ext)) {
    return cpp();
  }
  if (['html', 'htm', 'xhtml'].includes(ext)) {
    return html();
  }
  if (['css', 'scss', 'sass', 'less'].includes(ext)) {
    return css();
  }
  if (['sql'].includes(ext)) {
    return sql();
  }
  if (['yaml', 'yml'].includes(ext)) {
    return yaml();
  }
  if (['xml', 'svg'].includes(ext)) {
    return xml();
  }
  if (['java'].includes(ext)) {
    return java();
  }

  return null;
}

const actonosLightTheme = EditorView.theme({
  '&': {
    backgroundColor: '#f9fbf2',
    color: '#130e30',
    fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace',
    fontSize: '13px',
    height: '100%',
  },
  '.cm-scroller': {
    overflow: 'auto',
    fontFamily: 'inherit',
    lineHeight: '1.6',
  },
  '.cm-content': {
    caretColor: '#130e30',
    padding: '16px 0',
  },
  '.cm-cursor, .cm-dropCursor': {
    borderLeftColor: '#130e30',
    borderLeftWidth: '2px',
  },
  '&.cm-focused .cm-selectionBackground, .cm-selectionBackground, .cm-content ::selection': {
    backgroundColor: 'rgba(255, 226, 40, 0.45) !important',
  },
  '.cm-panels': {
    backgroundColor: '#eff2e5',
    color: '#130e30',
  },
  '.cm-panels.cm-panels-top': {
    borderBottom: '1px solid rgba(19, 14, 48, 0.1)',
  },
  '.cm-panels.cm-panels-bottom': {
    borderTop: '1px solid rgba(19, 14, 48, 0.1)',
  },
  '.cm-searchMatch': {
    backgroundColor: 'rgba(255, 226, 40, 0.6)',
    outline: '1px solid #130e30',
  },
  '.cm-searchMatch.cm-searchMatch-selected': {
    backgroundColor: '#ffe228',
  },
  '.cm-activeLine': {
    backgroundColor: 'rgba(239, 242, 229, 0.5)',
  },
  '.cm-selectionMatch': {
    backgroundColor: 'rgba(255, 226, 40, 0.3)',
  },
  '&.cm-focused .cm-matchingBracket, &.cm-focused .cm-nonmatchingBracket': {
    backgroundColor: 'rgba(255, 226, 40, 0.5)',
    outline: '1px solid #130e30',
  },
  '.cm-gutters': {
    backgroundColor: '#eff2e5',
    color: '#5f5c6e',
    borderRight: '1px solid rgba(19, 14, 48, 0.08)',
    paddingLeft: '4px',
    userSelect: 'none',
  },
  '.cm-activeLineGutter': {
    backgroundColor: '#e2e7d5',
    color: '#130e30',
    fontWeight: '600',
  },
  '.cm-foldPlaceholder': {
    backgroundColor: '#eff2e5',
    border: '1px solid rgba(19, 14, 48, 0.2)',
    color: '#130e30',
    borderRadius: '4px',
    padding: '0 4px',
  },
});

const actonosDarkTheme = EditorView.theme({
  '&': {
    backgroundColor: '#0f0c1b',
    color: '#f9fbf2',
    fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace',
    fontSize: '13px',
    height: '100%',
  },
  '.cm-scroller': {
    overflow: 'auto',
    fontFamily: 'inherit',
    lineHeight: '1.6',
  },
  '.cm-content': {
    caretColor: '#ffe228',
    padding: '16px 0',
  },
  '.cm-cursor, .cm-dropCursor': {
    borderLeftColor: '#ffe228',
    borderLeftWidth: '2px',
  },
  '&.cm-focused .cm-selectionBackground, .cm-selectionBackground, .cm-content ::selection': {
    backgroundColor: 'rgba(255, 226, 40, 0.3) !important',
  },
  '.cm-panels': {
    backgroundColor: '#19152b',
    color: '#f9fbf2',
  },
  '.cm-panels.cm-panels-top': {
    borderBottom: '1px solid rgba(255, 255, 255, 0.1)',
  },
  '.cm-panels.cm-panels-bottom': {
    borderTop: '1px solid rgba(255, 255, 255, 0.1)',
  },
  '.cm-searchMatch': {
    backgroundColor: 'rgba(255, 226, 40, 0.4)',
    outline: '1px solid #ffe228',
  },
  '.cm-searchMatch.cm-searchMatch-selected': {
    backgroundColor: '#ffe228',
    color: '#130e30',
  },
  '.cm-activeLine': {
    backgroundColor: 'rgba(255, 255, 255, 0.04)',
  },
  '.cm-selectionMatch': {
    backgroundColor: 'rgba(255, 226, 40, 0.2)',
  },
  '&.cm-focused .cm-matchingBracket, &.cm-focused .cm-nonmatchingBracket': {
    backgroundColor: 'rgba(255, 226, 40, 0.4)',
    outline: '1px solid #ffe228',
  },
  '.cm-gutters': {
    backgroundColor: '#141024',
    color: '#827f94',
    borderRight: '1px solid rgba(255, 255, 255, 0.08)',
    paddingLeft: '4px',
    userSelect: 'none',
  },
  '.cm-activeLineGutter': {
    backgroundColor: '#1e1936',
    color: '#ffe228',
    fontWeight: '600',
  },
  '.cm-foldPlaceholder': {
    backgroundColor: '#19152b',
    border: '1px solid rgba(255, 255, 255, 0.2)',
    color: '#f9fbf2',
    borderRadius: '4px',
    padding: '0 4px',
  },
});

export function WorkspaceCodeMirror({
  value,
  filename,
  kind,
  placeholder,
  readOnly = false,
  onChange,
}: WorkspaceCodeMirrorProps) {
  const { resolvedTheme } = useTheme();

  const extensions = useMemo(() => {
    const list: Extension[] = [
      EditorView.lineWrapping,
      resolvedTheme === 'dark' ? actonosDarkTheme : actonosLightTheme,
    ];

    const langExt = getLanguageExtension(filename, kind);
    if (langExt) {
      list.push(langExt);
    }

    return list;
  }, [filename, kind, resolvedTheme]);

  return (
    <div className="w-full h-full overflow-hidden flex flex-col">
      <CodeMirror
        value={value}
        height="100%"
        className="h-full w-full flex-1 overflow-hidden"
        theme={resolvedTheme === 'dark' ? 'dark' : 'light'}
        extensions={extensions}
        editable={!readOnly}
        readOnly={readOnly}
        placeholder={placeholder}
        onChange={onChange}
        basicSetup={{
          lineNumbers: true,
          highlightActiveLineGutter: true,
          highlightSpecialChars: true,
          history: true,
          foldGutter: true,
          drawSelection: true,
          dropCursor: true,
          allowMultipleSelections: true,
          indentOnInput: true,
          syntaxHighlighting: true,
          bracketMatching: true,
          closeBrackets: true,
          autocompletion: true,
          rectangularSelection: true,
          crosshairCursor: true,
          highlightActiveLine: true,
          highlightSelectionMatches: true,
          closeBracketsKeymap: true,
          defaultKeymap: true,
          searchKeymap: true,
          historyKeymap: true,
          foldKeymap: true,
          completionKeymap: true,
          lintKeymap: true,
        }}
      />
    </div>
  );
}
