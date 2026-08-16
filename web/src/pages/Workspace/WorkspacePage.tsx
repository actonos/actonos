import { useState, useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { PageContainer } from '@/components/layout/PageContainer';
import { BlobBackdrop } from '@/components/ui/BlobBackdrop';
import { Card } from '@/components/ui/Card';
import { Button } from '@/components/ui/Button';
import {
  Folder,
  FileText,
  Plus,
  RefreshCw,
  Trash2,
  Save,
  Upload,
  Download,
  FolderPlus,
  ChevronRight,
  ArrowLeft,
  Sparkles,
} from 'lucide-react';
import { api } from '@/lib/api';

interface WorkspaceFile {
  name: string;
  path: string;
  is_dir: boolean;
  size: number;
  mod_time: string;
}

export function WorkspacePage() {
  const { t } = useTranslation('workspace');
  const [currentDir, setCurrentDir] = useState<string>('');
  const [files, setFiles] = useState<WorkspaceFile[]>([]);
  const [selectedFile, setSelectedFile] = useState<string | null>(null);
  const [fileContent, setFileContent] = useState('');
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const loadFiles = async (dir: string = currentDir) => {
    try {
      setLoading(true);
      const res = await api.listWorkspaceFiles(dir);
      setFiles(res.files || []);
      setCurrentDir(res.dir || '');
    } catch (err) {
      console.error('Failed to load workspace files:', err);
    } finally {
      setLoading(false);
    }
  };

  const openFile = async (filePath: string) => {
    try {
      setSelectedFile(filePath);
      const res = await api.getWorkspaceFile(filePath);
      setFileContent(res.content || '');
    } catch (err) {
      console.error('Failed to read file:', err);
    }
  };

  const saveFile = async () => {
    if (!selectedFile) return;
    try {
      setSaving(true);
      await api.saveWorkspaceFile(selectedFile, fileContent);
      loadFiles(currentDir);
    } catch (err) {
      console.error('Failed to save file:', err);
    } finally {
      setSaving(false);
    }
  };

  const deleteFile = async (filePath: string) => {
    if (window.confirm(t('actions.deleteConfirm', 'Are you sure you want to delete this item?'))) {
      await api.deleteWorkspaceFile(filePath);
      if (selectedFile === filePath) {
        setSelectedFile(null);
        setFileContent('');
      }
      loadFiles(currentDir);
    }
  };

  const handleNewFile = () => {
    const filename = prompt('Enter new file name (e.g. app.py, config.json):');
    if (filename) {
      const fullPath = currentDir ? `${currentDir}/${filename}` : filename;
      setSelectedFile(fullPath);
      setFileContent('');
    }
  };

  const handleNewFolder = async () => {
    const folderName = prompt('Enter new folder name:');
    if (folderName) {
      const fullPath = currentDir ? `${currentDir}/${folderName}` : folderName;
      await api.mkdirWorkspace(fullPath);
      loadFiles(currentDir);
    }
  };

  const handleSeedSamples = async () => {
    try {
      await api.saveWorkspaceFile(
        'agent_task.py',
        '# ActonOS Autonomous Agent Task\nimport json, sys\n\ndef main():\n    print("ActonOS sandbox task running successfully!")\n    data = {"status": "ok", "runtime": sys.version}\n    print(json.dumps(data, indent=2))\n\nif __name__ == "__main__":\n    main()\n'
      );
      await api.saveWorkspaceFile(
        'config.yaml',
        '# ActonOS Configuration File\napp:\n  name: ActonApp\n  version: 0.1.0\n  sandbox: true\n  memory_decay: ebbinghaus\n'
      );
      await api.saveWorkspaceFile(
        'README.md',
        '# Sandboxed Agent Workspace\n\nFiles placed here can be read, created, or edited by autonomous agents using `native_file_read` and `native_file_write` tools.\n'
      );
      loadFiles(currentDir);
      openFile('agent_task.py');
    } catch (err) {
      console.error('Failed to seed sample files:', err);
    }
  };

  const handleUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const uploaded = e.target.files?.[0];
    if (!uploaded) return;

    const formData = new FormData();
    formData.append('dir', currentDir);
    formData.append('file', uploaded);

    try {
      await fetch('/api/workspace/upload', {
        method: 'POST',
        body: formData,
      });
      loadFiles(currentDir);
    } catch (err) {
      console.error('Upload failed:', err);
    }
  };

  const handleDownload = () => {
    if (!selectedFile) return;
    const blob = new Blob([fileContent], { type: 'text/plain;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = selectedFile.split('/').pop() || 'download.txt';
    a.click();
    URL.revokeObjectURL(url);
  };

  const handleNavigateDir = (newDir: string) => {
    setSelectedFile(null);
    setFileContent('');
    loadFiles(newDir);
  };

  const handleGoUp = () => {
    if (!currentDir) return;
    const parts = currentDir.split('/');
    parts.pop();
    handleNavigateDir(parts.join('/'));
  };

  useEffect(() => {
    loadFiles('');
  }, []);

  const breadcrumbParts = currentDir ? currentDir.split('/') : [];
  const lineCount = fileContent ? fileContent.split('\n').length : 0;

  return (
    <div className="relative min-h-[calc(100vh-64px)]">
      <BlobBackdrop />

      <PageContainer>
        {/* Header with securely right-aligned action buttons */}
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6">
          <div className="flex-1">
            <span className="text-caption uppercase tracking-wider text-slate font-semibold block mb-1">
              {t('eyebrow', 'Sandboxed File System')}
            </span>
            <h1 className="font-serif text-heading-lg text-deep-ink tracking-tight">
              {t('title', 'Workspace Explorer')}
            </h1>
            <p className="font-sans text-body text-slate mt-1 max-w-2xl">
              {t('subtitle', 'Inspect, create, edit, and upload files used by autonomous agents inside the isolated /data/workspace/ sandbox.')}
            </p>
          </div>

          <div className="flex flex-wrap items-center gap-2.5 shrink-0 self-start sm:self-center">
            <Button
              variant="ghost"
              size="sm"
              icon={<RefreshCw className="w-3.5 h-3.5" />}
              onClick={() => loadFiles(currentDir)}
            >
              {t('actions.refresh', 'Refresh')}
            </Button>
            <Button
              variant="ghost"
              size="sm"
              icon={<Sparkles className="w-3.5 h-3.5 text-hi-yellow" />}
              onClick={handleSeedSamples}
              title="Seed sample Python & config files"
            >
              Seed Samples
            </Button>
            <Button
              variant="ghost"
              size="sm"
              icon={<FolderPlus className="w-3.5 h-3.5" />}
              onClick={handleNewFolder}
            >
              New Folder
            </Button>
            <Button
              variant="ghost"
              size="sm"
              icon={<Upload className="w-3.5 h-3.5" />}
              onClick={() => fileInputRef.current?.click()}
            >
              Upload
            </Button>
            <input
              type="file"
              ref={fileInputRef}
              onChange={handleUpload}
              className="hidden"
            />
            <Button
              variant="primary"
              size="sm"
              icon={<Plus className="w-3.5 h-3.5" />}
              onClick={handleNewFile}
            >
              {t('actions.newFile', 'New File')}
            </Button>
          </div>
        </div>

        {/* Breadcrumb Path Bar */}
        <div className="flex items-center gap-1.5 mb-4 p-2.5 bg-soft-meadow rounded-full border border-onyx/10 text-body-sm font-sans">
          <button
            onClick={() => handleNavigateDir('')}
            className="font-semibold text-deep-ink hover:text-slate px-2 py-0.5 rounded-full cursor-pointer"
          >
            /data/workspace
          </button>
          {breadcrumbParts.map((part, idx) => {
            const pathSoFar = breadcrumbParts.slice(0, idx + 1).join('/');
            return (
              <div key={idx} className="flex items-center gap-1">
                <ChevronRight className="w-3.5 h-3.5 text-slate" />
                <button
                  onClick={() => handleNavigateDir(pathSoFar)}
                  className="font-medium text-deep-ink hover:text-slate px-2 py-0.5 rounded-full cursor-pointer"
                >
                  {part}
                </button>
              </div>
            );
          })}
        </div>

        {/* 2-Column Explorer: Left Tree + Right Editor */}
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
          {/* File list */}
          <Card className="p-4 border border-onyx/10 flex flex-col gap-2 max-h-[600px] overflow-y-auto bg-canvas/80">
            <div className="flex items-center justify-between px-2 mb-1">
              <span className="font-serif text-body font-semibold text-deep-ink">Directory Files</span>
              {currentDir && (
                <button
                  onClick={handleGoUp}
                  className="flex items-center gap-1 text-caption text-slate hover:text-deep-ink font-medium cursor-pointer"
                >
                  <ArrowLeft className="w-3 h-3" /> Go Up
                </button>
              )}
            </div>

            {loading ? (
              <div className="p-8 text-center text-slate text-body-sm">Loading workspace files...</div>
            ) : files.length === 0 ? (
              <div className="p-8 text-center text-slate text-body-sm">
                <p className="mb-3">Directory is empty.</p>
                <Button variant="ghost" size="sm" onClick={handleSeedSamples}>
                  Seed Sample Files
                </Button>
              </div>
            ) : (
              files.map((file) => (
                <div
                  key={file.path}
                  className={`flex items-center justify-between p-3 rounded-full transition-all cursor-pointer select-none ${
                    selectedFile === file.path
                      ? 'bg-deep-ink text-white font-medium shadow-xs'
                      : 'bg-canvas text-deep-ink hover:bg-white border border-onyx/5'
                  }`}
                  onClick={() => {
                    if (file.is_dir) {
                      handleNavigateDir(file.path);
                    } else {
                      openFile(file.path);
                    }
                  }}
                >
                  <div className="flex items-center gap-2 truncate">
                    {file.is_dir ? (
                      <Folder className="w-4 h-4 text-hi-yellow shrink-0" />
                    ) : (
                      <FileText className="w-4 h-4 shrink-0" />
                    )}
                    <span className="truncate text-body-sm">{file.name}</span>
                  </div>

                  <div className="flex items-center gap-2 shrink-0">
                    {!file.is_dir && <span className="text-caption opacity-70">{file.size} B</span>}
                    <button
                      onClick={(e) => {
                        e.stopPropagation();
                        deleteFile(file.path);
                      }}
                      className="p-1 hover:text-red-500 rounded-full opacity-70 hover:opacity-100 cursor-pointer"
                      title="Delete item"
                    >
                      <Trash2 className="w-3.5 h-3.5" />
                    </button>
                  </div>
                </div>
              ))
            )}
          </Card>

          {/* File content editor */}
          <Card className="lg:col-span-2 p-6 border border-onyx/10 flex flex-col justify-between min-h-[520px] bg-canvas/90">
            {selectedFile ? (
              <div className="flex flex-col h-full gap-3">
                <div className="flex items-center justify-between pb-3 border-b border-soft-meadow">
                  <div>
                    <span className="font-mono text-body-sm font-semibold text-deep-ink truncate block">
                      {selectedFile}
                    </span>
                    <span className="text-caption text-slate font-mono">
                      {lineCount} lines • {fileContent.length} bytes
                    </span>
                  </div>

                  <div className="flex items-center gap-2">
                    <Button
                      variant="ghost"
                      size="sm"
                      icon={<Download className="w-3.5 h-3.5" />}
                      onClick={handleDownload}
                      title="Download file"
                    >
                      Download
                    </Button>
                    <Button
                      variant="primary"
                      size="sm"
                      icon={<Save className="w-3.5 h-3.5" />}
                      onClick={saveFile}
                      disabled={saving}
                    >
                      {saving ? '...' : t('actions.save', 'Save')}
                    </Button>
                  </div>
                </div>

                <textarea
                  value={fileContent}
                  onChange={(e) => setFileContent(e.target.value)}
                  placeholder="Enter file content..."
                  className="flex-1 w-full bg-white text-deep-ink font-mono text-body-sm p-4 rounded-[16px] border border-onyx/20 focus:outline-none focus:ring-2 focus:ring-deep-ink min-h-[400px]"
                />
              </div>
            ) : (
              <div className="my-auto text-center py-20 text-slate">
                <FileText className="w-12 h-12 mx-auto mb-3 opacity-30 text-deep-ink" />
                <p className="text-body font-sans mb-3">Select a file from the explorer or seed samples.</p>
                <Button variant="primary" size="sm" onClick={handleSeedSamples}>
                  Seed Sample Files
                </Button>
              </div>
            )}
          </Card>
        </div>
      </PageContainer>
    </div>
  );
}
