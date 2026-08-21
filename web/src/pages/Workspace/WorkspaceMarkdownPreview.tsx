import { useMemo } from 'react';

interface WorkspaceMarkdownPreviewProps {
  content: string;
}

// Lightweight, safe markdown to HTML parser for high performance without external heavy dependencies
function parseMarkdown(md: string): string {
  if (!md) return '';

  const html = md
    // Escape HTML special characters
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')

    // Headers
    .replace(/^### (.*$)/gim, '<h3 class="text-heading-sm font-bold text-deep-ink mt-4 mb-2">$1</h3>')
    .replace(/^## (.*$)/gim, '<h2 class="text-subheading font-bold text-deep-ink mt-6 mb-3 pb-1 border-b border-deep-ink/10">$1</h2>')
    .replace(/^# (.*$)/gim, '<h1 class="text-heading font-extrabold text-deep-ink mt-6 mb-4 pb-2 border-b border-deep-ink/10">$1</h1>')

    // Blockquotes
    .replace(/^> (.*$)/gim, '<blockquote class="border-l-4 border-hi-yellow bg-soft-meadow/50 pl-4 py-2 my-3 rounded-r-xl italic text-slate">$1</blockquote>')

    // Bold & Italic
    .replace(/\*\*\*(.*?)\*\*\*/gim, '<strong><em>$1</em></strong>')
    .replace(/\*\*(.*?)\*\*/gim, '<strong class="font-bold text-deep-ink">$1</strong>')
    .replace(/\*(.*?)\*/gim, '<em class="italic">$1</em>')

    // Fenced Code Blocks
    .replace(/```([a-zA-Z0-9_-]*)\n([\s\S]*?)```/gim, '<pre class="bg-deep-ink text-canvas p-4 rounded-2xl my-3 overflow-x-auto text-caption font-mono"><code>$2</code></pre>')

    // Inline Code
    .replace(/`([^`]+)`/gim, '<code class="bg-soft-meadow text-deep-ink px-1.5 py-0.5 rounded-lg text-caption font-mono border border-deep-ink/5">$1</code>')

    // Unordered Lists
    .replace(/^\s*-\s+(.*$)/gim, '<li class="ml-4 list-disc text-body-sm text-deep-ink my-1">$1</li>')
    .replace(/^\s*\*\s+(.*$)/gim, '<li class="ml-4 list-disc text-body-sm text-deep-ink my-1">$1</li>')

    // Links
    .replace(/\[([^\]]+)\]\(([^)]+)\)/gim, '<a href="$2" target="_blank" rel="noreferrer" class="text-deep-ink font-semibold underline decoration-hi-yellow decoration-2 hover:bg-hi-yellow/20 rounded px-0.5 transition-colors">$1</a>')

    // Horizontal Rule
    .replace(/^---$/gim, '<hr class="my-6 border-deep-ink/10" />')

    // Paragraphs
    .replace(/\n\n/gim, '</p><p class="my-2.5 text-body text-deep-ink leading-relaxed">');

  return `<p class="my-2.5 text-body text-deep-ink leading-relaxed">${html}</p>`;
}

export function WorkspaceMarkdownPreview({ content }: WorkspaceMarkdownPreviewProps) {
  const renderedHtml = useMemo(() => parseMarkdown(content), [content]);

  return (
    <div className="h-full w-full overflow-y-auto p-6 bg-canvas selection:bg-hi-yellow selection:text-deep-ink">
      <div
        className="max-w-3xl mx-auto prose prose-sm"
        dangerouslySetInnerHTML={{ __html: renderedHtml }}
      />
    </div>
  );
}
