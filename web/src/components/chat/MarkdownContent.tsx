import { useEffect, useMemo } from 'react';
import { useEditor, EditorContent } from '@tiptap/react';
import StarterKit from '@tiptap/starter-kit';
import Link from '@tiptap/extension-link';
import Typography from '@tiptap/extension-typography';
import Highlight from '@tiptap/extension-highlight';
import CodeBlock from '@tiptap/extension-code-block';
import { TableKit } from "@tiptap/extension-table";
import HorizontalRule from '@tiptap/extension-horizontal-rule'

import { marked } from 'marked';

export interface MarkdownContentProps {
  content: string;
  className?: string;
  isUser?: boolean;
}

/**
 * Converts Markdown formatted text into TipTap-compatible semantic HTML
 * using marked for full CommonMark and GitHub Flavored Markdown (GFM) support.
 */
export function parseMarkdownToHTML(markdown: string): string {
  if (!markdown) return '';

  const cleanMarkdown = markdown
    .replace(/<[|｜]{1,2}DSML[|｜]{1,2}[\s\S]*?<\/[|｜]{1,2}DSML[|｜]{1,2}tool_calls>/g, '')
    .replace(/<[|｜]{1,2}[\s\S]*?>/g, '')
    .replace(/<\/?(?:tool_call|function_call|invoke|parameter)[^>]*>/g, '')
    .trim();

  if (!cleanMarkdown) return '';

  const html = marked.parse(cleanMarkdown, {
    gfm: true,
    breaks: true,
  }) as string;
  return sanitizeMarkdownHTML(html);
}

/** Strips javascript: URLs and inline event handlers from rendered markdown HTML. */
export function sanitizeMarkdownHTML(html: string): string {
  if (!html) return '';
  return html
    .replace(/\s+on[a-z]+\s*=\s*("[^"]*"|'[^']*'|[^\s>]+)/gi, '')
    .replace(/href\s*=\s*(["'])\s*javascript:[^"']*\1/gi, 'href="#"')
    .replace(/href\s*=\s*javascript:[^\s>]*/gi, 'href="#"')
    .replace(/src\s*=\s*(["'])\s*javascript:[^"']*\1/gi, '')
    .replace(/src\s*=\s*javascript:[^\s>]*/gi, '');
}

export function MarkdownContent({ content, className = '', isUser = false }: MarkdownContentProps) {
  const htmlContent = useMemo(() => parseMarkdownToHTML(content), [content]);

  const editor = useEditor({
    extensions: [
      StarterKit.configure({
        heading: { levels: [1, 2, 3] }
      }),
      Link.configure({
        openOnClick: false,
        protocols: ['http', 'https'],
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
      CodeBlock,
      TableKit,
      HorizontalRule
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
