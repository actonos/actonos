import { useMemo } from 'react';
import { marked } from 'marked';

interface WorkspaceMarkdownPreviewProps {
  content: string;
}

export function WorkspaceMarkdownPreview({ content }: WorkspaceMarkdownPreviewProps) {
  const renderedHtml = useMemo(() => {
    if (!content || !content.trim()) return '';
    try {
      return marked.parse(content, {
        gfm: true,
        breaks: true,
      }) as string;
    } catch {
      return content;
    }
  }, [content]);

  return (
    <div className="h-full w-full overflow-y-auto p-6 md:p-8 bg-canvas selection:bg-hi-yellow selection:text-deep-ink">
      <div
        className="max-w-4xl mx-auto workspace-markdown-preview text-deep-ink font-sans text-body-sm leading-relaxed"
        dangerouslySetInnerHTML={{ __html: renderedHtml }}
      />
    </div>
  );
}
