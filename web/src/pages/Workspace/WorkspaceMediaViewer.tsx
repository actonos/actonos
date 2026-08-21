import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Download,
  ZoomIn,
  ZoomOut,
  RotateCcw,
  FileArchive,
  FileQuestion,
  FileSpreadsheet,
} from 'lucide-react';
import { api } from '@/lib/api';

interface WorkspaceMediaViewerProps {
  path: string;
  kind: string;
  rawUrl: string;
  size?: number;
}

export function WorkspaceMediaViewer({ path, kind, rawUrl, size = 0 }: WorkspaceMediaViewerProps) {
  const { t } = useTranslation('workspace');
  const [zoom, setZoom] = useState(1);
  const [imgError, setImgError] = useState(false);

  const mediaUrl = rawUrl || api.getWorkspaceRawUrl(path);

  if (kind === 'image') {
    return (
      <div className="h-full flex flex-col bg-canvas text-deep-ink">
        {/* Image Control Bar */}
        <div className="flex items-center justify-between p-3 border-b border-deep-ink/10 bg-soft-meadow/30">
          <div className="flex items-center gap-2">
            <button
              onClick={() => setZoom((z) => Math.max(0.2, z - 0.2))}
              className="p-1.5 rounded-full border border-deep-ink/10 hover:bg-soft-meadow transition-colors"
              title={t('preview.zoomOut')}
            >
              <ZoomOut className="w-4 h-4 text-slate" />
            </button>
            <span className="text-caption font-mono text-slate w-12 text-center">
              {Math.round(zoom * 100)}%
            </span>
            <button
              onClick={() => setZoom((z) => Math.min(3, z + 0.2))}
              className="p-1.5 rounded-full border border-deep-ink/10 hover:bg-soft-meadow transition-colors"
              title={t('preview.zoomIn')}
            >
              <ZoomIn className="w-4 h-4 text-slate" />
            </button>
            <button
              onClick={() => setZoom(1)}
              className="p-1.5 rounded-full border border-deep-ink/10 hover:bg-soft-meadow transition-colors"
              title={t('preview.resetZoom')}
            >
              <RotateCcw className="w-4 h-4 text-slate" />
            </button>
          </div>

          <a
            href={mediaUrl}
            download={path.split('/').pop()}
            className="flex items-center gap-1.5 px-3 py-1.5 rounded-full border border-deep-ink/10 bg-canvas hover:bg-soft-meadow text-body-sm font-medium transition-colors"
          >
            <Download className="w-4 h-4 text-slate" />
            <span>{t('actions.download')}</span>
          </a>
        </div>

        {/* Image container */}
        <div className="flex-1 flex items-center justify-center overflow-auto p-6 bg-soft-meadow/20">
          {!imgError ? (
            <img
              src={mediaUrl}
              alt={path}
              onError={() => setImgError(true)}
              style={{ transform: `scale(${zoom})`, transformOrigin: 'center center' }}
              className="max-h-full max-w-full object-contain rounded-xl border border-deep-ink/10 transition-transform duration-100"
            />
          ) : (
            <div className="text-center p-8 text-slate">
              <FileQuestion className="w-12 h-12 mx-auto mb-2 opacity-40" />
              <p>{t('preview.image')}</p>
            </div>
          )}
        </div>
      </div>
    );
  }

  if (kind === 'pdf') {
    return (
      <div className="h-full flex flex-col bg-canvas text-deep-ink">
        <div className="flex items-center justify-between p-3 border-b border-deep-ink/10 bg-soft-meadow/30">
          <span className="text-body-sm font-semibold">{t('preview.pdf')}</span>
          <a
            href={mediaUrl}
            download={path.split('/').pop()}
            className="flex items-center gap-1.5 px-3 py-1.5 rounded-full border border-deep-ink/10 bg-canvas hover:bg-soft-meadow text-body-sm font-medium transition-colors"
          >
            <Download className="w-4 h-4 text-slate" />
            <span>{t('actions.download')}</span>
          </a>
        </div>
        <div className="flex-1 w-full bg-slate/10 p-2">
          <iframe
            src={mediaUrl}
            title={path}
            className="w-full h-full rounded-xl border border-deep-ink/10 bg-white"
          />
        </div>
      </div>
    );
  }

  if (kind === 'audio') {
    return (
      <div className="h-full flex flex-col items-center justify-center p-8 bg-canvas text-deep-ink">
        <div className="max-w-md w-full p-6 rounded-[24px] border border-deep-ink/10 bg-soft-meadow/40 text-center flex flex-col gap-4">
          <div className="text-subheading font-bold truncate">{path.split('/').pop()}</div>
          <audio controls src={mediaUrl} className="w-full" />
          <a
            href={mediaUrl}
            download={path.split('/').pop()}
            className="mx-auto flex items-center gap-1.5 px-4 py-2 rounded-full bg-deep-ink text-canvas hover:bg-deep-ink/90 text-body-sm font-semibold transition-colors"
          >
            <Download className="w-4 h-4" />
            <span>{t('actions.download')}</span>
          </a>
        </div>
      </div>
    );
  }

  if (kind === 'video') {
    return (
      <div className="h-full flex flex-col bg-canvas text-deep-ink">
        <div className="flex items-center justify-between p-3 border-b border-deep-ink/10 bg-soft-meadow/30">
          <span className="text-body-sm font-semibold">{t('preview.video')}</span>
          <a
            href={mediaUrl}
            download={path.split('/').pop()}
            className="flex items-center gap-1.5 px-3 py-1.5 rounded-full border border-deep-ink/10 bg-canvas hover:bg-soft-meadow text-body-sm font-medium transition-colors"
          >
            <Download className="w-4 h-4 text-slate" />
            <span>{t('actions.download')}</span>
          </a>
        </div>
        <div className="flex-1 flex items-center justify-center p-4 bg-soft-meadow/20">
          <video controls src={mediaUrl} className="max-h-full max-w-full rounded-xl border border-deep-ink/10" />
        </div>
      </div>
    );
  }

  // Binary / Archive / Document fallback
  return (
    <div className="h-full flex flex-col items-center justify-center p-8 bg-canvas text-deep-ink text-center">
      <div className="max-w-md p-8 rounded-[24px] border border-deep-ink/10 bg-soft-meadow/40 flex flex-col items-center gap-4">
        <div className="w-16 h-16 rounded-full bg-hi-yellow flex items-center justify-center">
          {kind === 'archive' ? (
            <FileArchive className="w-8 h-8 text-deep-ink" />
          ) : kind === 'document' ? (
            <FileSpreadsheet className="w-8 h-8 text-deep-ink" />
          ) : (
            <FileQuestion className="w-8 h-8 text-deep-ink" />
          )}
        </div>

        <div>
          <h3 className="text-subheading font-bold mb-1 truncate max-w-xs">{path.split('/').pop()}</h3>
          {size > 0 && (
            <p className="text-caption font-mono text-slate mb-1">
              {size < 1024 ? `${size} B` : size < 1048576 ? `${(size / 1024).toFixed(1)} KB` : `${(size / 1048576).toFixed(1)} MB`}
            </p>
          )}
          <p className="text-body-sm text-slate">{t('preview.binary')}</p>
        </div>

        <a
          href={mediaUrl}
          download={path.split('/').pop()}
          className="flex items-center gap-2 px-5 py-2.5 rounded-full bg-deep-ink text-canvas hover:bg-deep-ink/90 font-semibold text-body-sm transition-colors"
        >
          <Download className="w-4 h-4" />
          <span>{t('preview.downloadBinary')}</span>
        </a>
      </div>
    </div>
  );
}
