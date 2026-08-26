import { useEffect, useMemo } from 'react';
import { useEditor, EditorContent } from '@tiptap/react';
import StarterKit from '@tiptap/starter-kit';
import Typography from '@tiptap/extension-typography';
import Highlight from '@tiptap/extension-highlight';
import { TableKit } from "@tiptap/extension-table";
import { Node } from '@tiptap/core';

import { marked } from 'marked';

export const ImageExtension = Node.create({
  name: 'image',
  group: 'block',
  selectable: true,
  draggable: true,
  atom: true,

  addAttributes() {
    return {
      src: { default: null },
      alt: { default: null },
      title: { default: null },
    };
  },

  parseHTML() {
    return [
      {
        tag: 'img[src]',
      },
    ];
  },

  renderHTML({ HTMLAttributes }) {
    return [
      'img',
      {
        ...HTMLAttributes,
        class: 'rounded-xl max-w-full h-auto my-3 border border-border-subtle shadow-sm object-contain max-h-[512px] bg-subtle-box inline-block',
        loading: 'lazy',
      },
    ];
  },
});

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
  return unwrapStandaloneImages(sanitizeMarkdownHTML(html));
}

/**
 * marked wraps images in <p><img></p>. TipTap's image node is a block atom, so
 * that paragraph is split into an empty <p><br></p> sitting above the image.
 * Lift image-only paragraphs (and leftover empty ones) before setContent.
 */
export function unwrapStandaloneImages(html: string): string {
  if (!html) return '';
  return html
    .replace(
      /<p>(?:\s|<br\s*\/?>)*((?:<img\b[^>]*>\s*(?:<br\s*\/?>\s*)?)+)<\/p>/gi,
      (_match, images: string) => images.replace(/<br\s*\/?>/gi, '').trim(),
    )
    .replace(/<p>(?:\s|&nbsp;|<br\s*\/?>)*<\/p>/gi, '');
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
      Typography,
      Highlight,
      TableKit,
      ImageExtension,
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
