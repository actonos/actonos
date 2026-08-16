import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { PageContainer } from '@/components/layout/PageContainer';
import { BlobBackdrop } from '@/components/ui/BlobBackdrop';
import { Card } from '@/components/ui/Card';
import { Button } from '@/components/ui/Button';
import { Folder, FileText, Plus, RefreshCw, Trash2, Save } from 'lucide-react';

interface WorkspaceFile {
  name: string;
  path: string;
  is_dir: boolean;
  size: number;
  mod_time: string;
}

export function WorkspacePage() {
  const { t } = useTranslation('workspace');
  const [files, setFiles] = useState<WorkspaceFile[]>([]);
  const [selectedFile, setSelectedFile] = useState<string | null>(null);
  const [fileContent, setFileContent] = useState('');
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);

  const loadFiles = async () => {
    try {
      setLoading(true);
      const res = await fetch('/api/workspace/files');
      const data = await res.json();
      setFiles(data.data?.files || []);
    } catch (err) {
      console.error('Failed to load workspace files:', err);
    } finally {
      setLoading(false);
    }
  };

  const openFile = async (filePath: string) => {
    try {
      setSelectedFile(filePath);
      const res = await fetch(`/api/workspace/file?path=${encodeURIComponent(filePath)}`);
      const data = await res.json();
      setFileContent(data.data?.content || '');
    } catch (err) {
      console.error('Failed to read file:', err);
    }
  };

  const saveFile = async () => {
    if (!selectedFile) return;
    try {
      setSaving(true);
      await fetch('/api/workspace/file', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ path: selectedFile, content: fileContent }),
      });
      loadFiles();
    } catch (err) {
      console.error('Failed to save file:', err);
    } finally {
      setSaving(false);
    }
  };

  const deleteFile = async (filePath: string) => {
    if (window.confirm(t('actions.deleteConfirm'))) {
      await fetch(`/api/workspace/file?path=${encodeURIComponent(filePath)}`, { method: 'DELETE' });
      if (selectedFile === filePath) {
        setSelectedFile(null);
        setFileContent('');
      }
      loadFiles();
    }
  };

  const handleNewFile = () => {
    const filename = prompt('Enter new file name (e.g. script.py, data.json):');
    if (filename) {
      setSelectedFile(filename);
      setFileContent('');
    }
  };

  useEffect(() => {
    loadFiles();
  }, []);

  return (
    <div className="relative min-h-[calc(100vh-72px)]">
      <BlobBackdrop />

      <PageContainer>
        {/* Header */}
        <div className="flex flex-col md:flex-row md:items-end justify-between gap-4 mb-8">
          <div>
            <span className="text-caption uppercase tracking-wider text-slate font-semibold block mb-1">
              {t('eyebrow')}
            </span>
            <h1 className="font-serif text-heading-lg text-deep-ink tracking-tight">
              {t('title')}
            </h1>
            <p className="font-sans text-body text-slate mt-2 max-w-2xl">
              {t('subtitle')}
            </p>
          </div>

          <div className="flex items-center gap-3">
            <Button
              variant="ghost"
              size="sm"
              icon={<RefreshCw className="w-4 h-4" />}
              onClick={loadFiles}
            >
              {t('actions.refresh')}
            </Button>
            <Button
              variant="primary"
              size="sm"
              icon={<Plus className="w-4 h-4" />}
              onClick={handleNewFile}
            >
              {t('actions.newFile')}
            </Button>
          </div>
        </div>

        {/* 2-Column Explorer: Left Tree + Right Editor */}
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
          {/* File list */}
          <Card className="p-4 border border-onyx/10 flex flex-col gap-2 max-h-[600px] overflow-y-auto">
            <h3 className="font-serif text-heading-sm text-deep-ink px-2 mb-2">Files (/workspace)</h3>
            {loading ? (
              <div className="p-8 text-center text-slate text-body-sm">Loading workspace files...</div>
            ) : files.length === 0 ? (
              <div className="p-8 text-center text-slate text-body-sm">{t('table.empty')}</div>
            ) : (
              files.map((file) => (
                <div
                  key={file.path}
                  className={`flex items-center justify-between p-3 rounded-full transition-all cursor-pointer select-none ${
                    selectedFile === file.path
                      ? 'bg-deep-ink text-white font-medium'
                      : 'bg-canvas text-deep-ink hover:bg-white border border-onyx/5'
                  }`}
                  onClick={() => openFile(file.path)}
                >
                  <div className="flex items-center gap-2 truncate">
                    {file.is_dir ? <Folder className="w-4 h-4 text-hi-yellow shrink-0" /> : <FileText className="w-4 h-4 shrink-0" />}
                    <span className="truncate text-body-sm">{file.name}</span>
                  </div>

                  <div className="flex items-center gap-2 shrink-0">
                    <span className="text-caption opacity-70">{file.size} B</span>
                    <button
                      onClick={(e) => {
                        e.stopPropagation();
                        deleteFile(file.path);
                      }}
                      className="p-1 hover:text-red-500 rounded-full"
                    >
                      <Trash2 className="w-3.5 h-3.5" />
                    </button>
                  </div>
                </div>
              ))
            )}
          </Card>

          {/* File content editor */}
          <Card className="lg:col-span-2 p-6 border border-onyx/10 flex flex-col justify-between min-h-[500px]">
            {selectedFile ? (
              <div className="flex flex-col h-full gap-4">
                <div className="flex items-center justify-between pb-3 border-b border-canvas">
                  <span className="font-mono text-body-sm font-semibold text-deep-ink truncate">
                    {t('editor.title', { path: selectedFile })}
                  </span>
                  <Button
                    variant="primary"
                    size="sm"
                    icon={<Save className="w-3.5 h-3.5" />}
                    onClick={saveFile}
                    disabled={saving}
                  >
                    {saving ? '...' : t('actions.save')}
                  </Button>
                </div>

                <textarea
                  value={fileContent}
                  onChange={(e) => setFileContent(e.target.value)}
                  placeholder={t('editor.placeholder')}
                  className="flex-1 w-full bg-canvas text-deep-ink font-mono text-body-sm p-4 rounded-[16px] border border-onyx focus:outline-none focus:ring-2 focus:ring-deep-ink min-h-[380px]"
                />
              </div>
            ) : (
              <div className="my-auto text-center py-20 text-slate">
                <FileText className="w-12 h-12 mx-auto mb-3 opacity-30 text-deep-ink" />
                <p className="text-body font-sans">Select a file from the left to view or edit.</p>
              </div>
            )}
          </Card>
        </div>
      </PageContainer>
    </div>
  );
}
