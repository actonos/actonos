import { useEffect, useMemo } from 'react';
import { useEditor, EditorContent } from '@tiptap/react';
import StarterKit from '@tiptap/starter-kit';
import Link from '@tiptap/extension-link';
import Typography from '@tiptap/extension-typography';
import Highlight from '@tiptap/extension-highlight';

export interface MarkdownContentProps {
  content: string;
  className?: string;
  isUser?: boolean;
}

/**
 * Converts Markdown formatted text into TipTap-compatible semantic HTML
 * with exact line break, code block, list, and paragraph preservation.
 */
export function parseMarkdownToHTML(markdown: string): string {
  if (!markdown) return '';

  // 1. Extract and protect code blocks so formatting doesn't alter code block content
  const codeBlocks: string[] = [];
  let processed = markdown.replace(/```([a-zA-Z0-9_-]*)\r?\n([\s\S]*?)```/g, (_match, lang, code) => {
    const placeholder = `__ACTON_CODE_BLOCK_${codeBlocks.length}__`;
    const escapedCode = code
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;');
    codeBlocks.push(`<pre><code class="language-${lang || 'text'}">${escapedCode.trim()}</code></pre>`);
    return `\n\n${placeholder}\n\n`;
  });

  // 2. Escape HTML entities in text
  processed = processed
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;');

  // Inline Code: `code`
  processed = processed.replace(/`([^`]+)`/g, '<code>$1</code>');

  // Headings
  processed = processed.replace(/^### (.*$)/gim, '<h3>$1</h3>');
  processed = processed.replace(/^## (.*$)/gim, '<h2>$1</h2>');
  processed = processed.replace(/^# (.*$)/gim, '<h1>$1</h1>');

  // Bold & Italic
  processed = processed.replace(/\*\*\*([^*]+)\*\*\*/g, '<strong><em>$1</em></strong>');
  processed = processed.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
  processed = processed.replace(/\*([^*]+)\*/g, '<em>$1</em>');

  // Links: [title](url)
  processed = processed.replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" target="_blank" rel="noopener noreferrer">$1</a>');

  // 3. Line-by-line block scanner for lists, quotes, and paragraphs
  const lines = processed.split(/\r?\n/);
  const outBlocks: string[] = [];
  let currentList: string[] = [];
  let inList = false;
  let currentQuote: string[] = [];
  let inQuote = false;

  const flushList = () => {
    if (inList && currentList.length > 0) {
      outBlocks.push(`<ul>${currentList.join('')}</ul>`);
      currentList = [];
      inList = false;
    }
  };

  const flushQuote = () => {
    if (inQuote && currentQuote.length > 0) {
      outBlocks.push(`<blockquote>${currentQuote.join('<br />')}</blockquote>`);
      currentQuote = [];
      inQuote = false;
    }
  };

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    const trimmed = line.trim();

    // Check code block placeholder
    if (trimmed.startsWith('__ACTON_CODE_BLOCK_') && trimmed.endsWith('__')) {
      flushList();
      flushQuote();
      outBlocks.push(trimmed);
      continue;
    }

    // Check for empty line
    if (!trimmed) {
      flushList();
      flushQuote();
      continue;
    }

    // Check for Headings
    if (trimmed.startsWith('<h1') || trimmed.startsWith('<h2') || trimmed.startsWith('<h3')) {
      flushList();
      flushQuote();
      outBlocks.push(trimmed);
      continue;
    }

    // Check for Bullet list items (- item or * item)
    const listMatch = line.match(/^\s*[-*]\s+(.*)$/);
    if (listMatch) {
      flushQuote();
      inList = true;
      currentList.push(`<li>${listMatch[1]}</li>`);
      continue;
    }

    // Check for Numbered list items (1. item)
    const numListMatch = line.match(/^\s*\d+\.\s+(.*)$/);
    if (numListMatch) {
      flushQuote();
      inList = true;
      currentList.push(`<li>${numListMatch[1]}</li>`);
      continue;
    }

    // Check for Blockquotes (> Quote)
    const quoteMatch = line.match(/^\s*&gt;\s?(.*)$/);
    if (quoteMatch) {
      flushList();
      inQuote = true;
      currentQuote.push(quoteMatch[1]);
      continue;
    }

    // Regular text line / paragraph
    flushList();
    flushQuote();
    outBlocks.push(`<p>${trimmed}</p>`);
  }

  flushList();
  flushQuote();

  let finalHTML = outBlocks.join('');

  // 4. Restore code blocks
  codeBlocks.forEach((cb, idx) => {
    finalHTML = finalHTML.replace(`__ACTON_CODE_BLOCK_${idx}__`, cb);
    finalHTML = finalHTML.replace(`<p>__ACTON_CODE_BLOCK_${idx}__</p>`, cb);
  });

  return finalHTML;
}

export function MarkdownContent({ content, className = '', isUser = false }: MarkdownContentProps) {
  const htmlContent = useMemo(() => parseMarkdownToHTML(content), [content]);

  const editor = useEditor({
    extensions: [
      StarterKit.configure({
        heading: { levels: [1, 2, 3] },
        codeBlock: false,
      }),
      Link.configure({
        openOnClick: true,
        HTMLAttributes: {
          class: isUser
            ? 'text-hi-yellow underline font-semibold'
            : 'text-deep-ink underline font-semibold hover:opacity-80 transition-opacity',
          target: '_blank',
          rel: 'noopener noreferrer',
        },
      }),
      Typography,
      Highlight,
    ],
    content: htmlContent,
    editable: false,
  });

  useEffect(() => {
    if (editor && htmlContent !== editor.getHTML()) {
      editor.commands.setContent(htmlContent);
    }
  }, [htmlContent, editor]);

  if (!editor) {
    return <div className={`font-sans text-body-sm whitespace-pre-wrap ${className}`}>{content}</div>;
  }

  return (
    <div className={`tiptap-renderer ${isUser ? 'tiptap-user' : ''} ${className}`}>
      <EditorContent editor={editor} />
    </div>
  );
}
