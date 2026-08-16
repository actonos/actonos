import { useState, useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { PageContainer } from '@/components/layout/PageContainer';
import { BlobBackdrop } from '@/components/ui/BlobBackdrop';
import { Card } from '@/components/ui/Card';
import { Button } from '@/components/ui/Button';
import { ConfirmModal } from '@/components/ui/ConfirmModal';
import { PromptModal } from '@/components/ui/PromptModal';
import { useToast } from '@/components/ui/Toast';
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
  FileCode,
  FileSpreadsheet,
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
  const { success, error } = useToast();
  const [currentDir, setCurrentDir] = useState<string>('');
  const [files, setFiles] = useState<WorkspaceFile[]>([]);
  const [selectedFile, setSelectedFile] = useState<string | null>(null);
  const [fileContent, setFileContent] = useState('');
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [deletingPath, setDeletingPath] = useState<string | null>(null);
  const [isNewFileModalOpen, setIsNewFileModalOpen] = useState(false);
  const [isNewFolderModalOpen, setIsNewFolderModalOpen] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const loadFiles = async (dir: string = currentDir) => {
    try {
      setLoading(true);
      const res = await api.listWorkspaceFiles(dir);
      setFiles(res.files || []);
      setCurrentDir(res.dir || '');
    } catch (err: any) {
      error('Failed to load workspace files', err.message);
    } finally {
      setLoading(false);
    }
  };

  const openFile = async (filePath: string) => {
    try {
      setSelectedFile(filePath);
      const res = await api.getWorkspaceFile(filePath);
      setFileContent(res.content || '');
    } catch (err: any) {
      error('Failed to read file', err.message);
    }
  };

  const saveFile = async () => {
    if (!selectedFile) return;
    try {
      setSaving(true);
      await api.saveWorkspaceFile(selectedFile, fileContent);
      success('File Saved', `${selectedFile} saved to workspace.`);
      loadFiles(currentDir);
    } catch (err: any) {
      error('Failed to save file', err.message);
    } finally {
      setSaving(false);
    }
  };

  const handleConfirmDelete = async () => {
    if (!deletingPath) return;
    try {
      await api.deleteWorkspaceFile(deletingPath);
      success('File Deleted', `${deletingPath} removed from sandbox.`);
      if (selectedFile === deletingPath) {
        setSelectedFile(null);
        setFileContent('');
      }
      setDeletingPath(null);
      loadFiles(currentDir);
    } catch (err: any) {
      error('Failed to delete file', err.message);
    }
  };

  const handleCreateFile = (filename: string) => {
    const fullPath = currentDir ? `${currentDir}/${filename}` : filename;
    setSelectedFile(fullPath);
    setFileContent('');
    success('New File Created', `Editing "${fullPath}". Click Save to persist.`);
  };

  const handleCreateFolder = async (folderName: string) => {
    const fullPath = currentDir ? `${currentDir}/${folderName}` : folderName;
    try {
      await api.mkdirWorkspace(fullPath);
      success('Folder Created', `Directory "${fullPath}" created.`);
      loadFiles(currentDir);
    } catch (err: any) {
      error('Failed to create folder', err.message);
    }
  };



  const handleUploadFiles = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const fileList = e.target.files;
    if (!fileList || fileList.length === 0) return;

    try {
      for (let i = 0; i < fileList.length; i++) {
        const file = fileList[i];
        const content = await file.text();
        const fullPath = currentDir ? `${currentDir}/${file.name}` : file.name;
        await api.saveWorkspaceFile(fullPath, content);
      }
      success('Files Uploaded', `Uploaded ${fileList.length} file(s) into workspace.`);
      loadFiles(currentDir);
    } catch (err: any) {
      error('Upload failed', err.message);
    }
  };

  const navigateUp = () => {
    if (!currentDir) return;
    const parts = currentDir.split('/');
    parts.pop();
    const parent = parts.join('/');
    loadFiles(parent);
  };

  useEffect(() => {
    loadFiles();
  }, []);

  const getFileIcon = (fileName: string, isDir: boolean) => {
    if (isDir) return <Folder className="w-4 h-4 text-hi-yellow" />;
    if (fileName.endsWith('.py') || fileName.endsWith('.js') || fileName.endsWith('.go') || fileName.endsWith('.ts')) {
      return <FileCode className="w-4 h-4 text-emerald-600" />;
    }
    if (fileName.endsWith('.json') || fileName.endsWith('.yaml') || fileName.endsWith('.yml')) {
      return <FileSpreadsheet className="w-4 h-4 text-amber-600" />;
    }
    return <FileText className="w-4 h-4 text-slate" />;
  };

  const lineCount = fileContent ? fileContent.split('\n').length : 0;
  const byteSize = new Blob([fileContent]).size;

  return (
    <div className="relative min-h-[calc(100vh-64px)]">
      <BlobBackdrop />

      <PageContainer>
        {/* Header */}
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6">
          <div className="flex-1">
            <span className="text-caption uppercase tracking-wider text-slate font-semibold block mb-1">
              {t('eyebrow', 'Sandboxed Filesystem')}
            </span>
            <h1 className="font-serif text-heading-lg text-deep-ink tracking-tight">
              {t('title', 'Workspace Explorer')}
            </h1>
            <p className="font-sans text-body text-slate mt-1 max-w-2xl">
              {t(
                'subtitle',
                'Sandboxed environment for code execution, dataset storage, and autonomous file manipulation.'
              )}
            </p>
          </div>

          <div className="flex flex-wrap items-center gap-2.5 shrink-0 self-start sm:self-center">
            <Button
              variant="ghost"
              size="sm"
              icon={<RefreshCw className="w-3.5 h-3.5" />}
              onClick={() => loadFiles(currentDir)}
            >
              Refresh
            </Button>
            <input
              type="file"
              ref={fileInputRef}
              onChange={handleUploadFiles}
              multiple
              className="hidden"
            />
            <Button
              variant="ghost"
              size="sm"
              icon={<Upload className="w-3.5 h-3.5" />}
              onClick={() => fileInputRef.current?.click()}
            >
              Upload
            </Button>
            <Button
              variant="ghost"
              size="sm"
              icon={<FolderPlus className="w-3.5 h-3.5" />}
              onClick={() => setIsNewFolderModalOpen(true)}
            >
              New Folder
            </Button>
            <Button
              variant="primary"
              size="sm"
              icon={<Plus className="w-3.5 h-3.5" />}
              onClick={() => setIsNewFileModalOpen(true)}
            >
              New File
            </Button>
          </div>
        </div>

        {/* Main Explorer Workspace */}
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
          {/* File Tree / List Sidebar */}
          <Card className="p-4 border border-onyx/10 bg-canvas/90 flex flex-col h-[640px]">
            {/* Breadcrumb Path Bar */}
            <div className="flex items-center justify-between pb-3 mb-3 border-b border-soft-meadow">
              <div className="flex items-center gap-1.5 font-mono text-caption text-deep-ink overflow-x-auto">
                <button
                  onClick={() => loadFiles('')}
                  className="hover:underline font-bold text-deep-ink"
                >
                  root
                </button>
                {currentDir &&
                  currentDir.split('/').map((seg, idx, arr) => {
                    const subPath = arr.slice(0, idx + 1).join('/');
                    return (
                      <span key={subPath} className="flex items-center gap-1.5">
                        <ChevronRight className="w-3 h-3 text-slate" />
                        <button
                          onClick={() => loadFiles(subPath)}
                          className="hover:underline text-deep-ink"
                        >
                          {seg}
                        </button>
                      </span>
                    );
                  })}
              </div>

              {currentDir && (
                <Button
                  variant="ghost"
                  size="sm"
                  icon={<ArrowLeft className="w-3 h-3" />}
                  onClick={navigateUp}
                  className="px-2 py-1 h-auto text-caption"
                >
                  Up
                </Button>
              )}
            </div>

            {/* Files List */}
            <div className="flex-1 overflow-y-auto divide-y divide-onyx/5">
              {loading ? (
                <div className="py-16 text-center text-slate font-sans text-caption">Loading files...</div>
              ) : files.length === 0 ? (
                <div className="py-16 text-center text-slate font-sans text-caption">
                  Directory is empty.
                </div>
              ) : (
                files.map((file) => {
                  const isSelected = selectedFile === file.path;
                  return (
                    <div
                      key={file.path}
                      onClick={() => (file.is_dir ? loadFiles(file.path) : openFile(file.path))}
                      className={`py-2.5 px-3 flex items-center justify-between rounded-xl cursor-pointer transition-all ${
                        isSelected
                          ? 'bg-deep-ink text-white font-semibold'
                          : 'hover:bg-soft-meadow text-deep-ink'
                      }`}
                    >
                      <div className="flex items-center gap-2.5 truncate">
                        {getFileIcon(file.name, file.is_dir)}
                        <span className="font-mono text-body-sm truncate">{file.name}</span>
                      </div>

                      <div className="flex items-center gap-2 shrink-0">
                        {!file.is_dir && (
                          <span className={`text-[11px] font-mono ${isSelected ? 'text-white/70' : 'text-slate'}`}>
                            {file.size} B
                          </span>
                        )}
                        <button
                          onClick={(e) => {
                            e.stopPropagation();
                            setDeletingPath(file.path);
                          }}
                          className={`p-1 rounded-full hover:bg-black/10 transition-colors ${
                            isSelected ? 'text-white/80 hover:text-white' : 'text-slate hover:text-red-600'
                          }`}
                          title="Delete file"
                        >
                          <Trash2 className="w-3.5 h-3.5" />
                        </button>
                      </div>
                    </div>
                  );
                })
              )}
            </div>
          </Card>

          {/* Code Editor / Preview */}
          <Card className="lg:col-span-2 p-4 border border-onyx/10 bg-canvas/90 flex flex-col h-[640px]">
            {selectedFile ? (
              <div className="flex flex-col h-full">
                {/* Editor Header */}
                <div className="flex items-center justify-between pb-3 mb-3 border-b border-soft-meadow">
                  <div className="flex items-center gap-2 font-mono text-body-sm text-deep-ink font-semibold truncate">
                    <FileText className="w-4 h-4 text-deep-ink" />
                    <span className="truncate">{selectedFile}</span>
                  </div>

                  <div className="flex items-center gap-2 shrink-0">
                    <Button
                      variant="ghost"
                      size="sm"
                      icon={<Download className="w-3.5 h-3.5" />}
                      onClick={() => {
                        const blob = new Blob([fileContent], { type: 'text/plain;charset=utf-8' });
                        const url = URL.createObjectURL(blob);
                        const a = document.createElement('a');
                        a.href = url;
                        a.download = selectedFile.split('/').pop() || 'file.txt';
                        a.click();
                        URL.revokeObjectURL(url);
                        success('File Downloaded', `Downloaded ${a.download}`);
                      }}
                      title="Download file to local machine"
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
                      {saving ? 'Saving...' : 'Save File'}
                    </Button>
                  </div>
                </div>

                {/* Editor Textarea */}
                <div className="flex-1 relative">
                  <textarea
                    value={fileContent}
                    onChange={(e) => setFileContent(e.target.value)}
                    onKeyDown={(e) => {
                      if ((e.ctrlKey || e.metaKey) && e.key === 's') {
                        e.preventDefault();
                        saveFile();
                      }
                    }}
                    placeholder="File content..."
                    className="w-full h-full p-4 font-mono text-body-sm bg-soft-meadow rounded-[16px] border border-onyx/10 focus:outline-none focus:ring-2 focus:ring-deep-ink resize-none text-deep-ink leading-relaxed selection:bg-hi-yellow selection:text-deep-ink"
                    spellCheck={false}
                  />
                </div>

                {/* Status Bar */}
                <div className="pt-3 mt-2 border-t border-soft-meadow flex items-center justify-between text-caption font-mono text-slate">
                  <span>Lines: {lineCount} • Size: {byteSize} bytes</span>
                  <span>UTF-8 • Ctrl+S to save</span>
                </div>
              </div>
            ) : (
              <div className="flex flex-col items-center justify-center h-full text-center p-8">
                <FileCode className="w-12 h-12 text-slate opacity-40 mb-3" />
                <h3 className="font-serif text-heading-sm text-deep-ink mb-1">No File Selected</h3>
                <p className="font-sans text-body-sm text-slate max-w-sm mb-4">
                  Select a file from the explorer on the left, or create a new file to start editing.
                </p>
                <Button variant="primary" size="sm" onClick={() => setIsNewFileModalOpen(true)}>
                  Create New File
                </Button>
              </div>
            )}
          </Card>
        </div>
      </PageContainer>

      {/* Delete Confirmation Modal */}
      <ConfirmModal
        isOpen={!!deletingPath}
        onClose={() => setDeletingPath(null)}
        onConfirm={handleConfirmDelete}
        title="Delete Workspace File"
        description={`Are you sure you want to permanently delete "${deletingPath}" from your sandboxed workspace?`}
        confirmLabel="Delete"
        variant="danger"
      />

      {/* New File Modal */}
      <PromptModal
        isOpen={isNewFileModalOpen}
        onClose={() => setIsNewFileModalOpen(false)}
        onSubmit={handleCreateFile}
        title="Create New File"
        label="File Name & Extension"
        placeholder="e.g. main.py, data.json, script.sh"
        confirmLabel="Create & Edit"
      />

      {/* New Folder Modal */}
      <PromptModal
        isOpen={isNewFolderModalOpen}
        onClose={() => setIsNewFolderModalOpen(false)}
        onSubmit={handleCreateFolder}
        title="Create New Directory"
        label="Folder Name"
        placeholder="e.g. scripts, models, datasets"
        confirmLabel="Create Folder"
      />
    </div>
  );
}
