import { useState, useEffect, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { PageContainer } from '@/components/layout/PageContainer';
import { PageHeader } from '@/components/ui/PageHeader';
import { BlobBackdrop } from '@/components/ui/BlobBackdrop';
import { Card } from '@/components/ui/Card';
import { ConfirmModal } from '@/components/ui/ConfirmModal';
import { PromptModal } from '@/components/ui/PromptModal';
import { useToast } from '@/components/ui/Toast';
import { API_BASE, api } from '@/lib/api';
import { getErrorMessage } from '@/lib/errors';
import { isApprovalRequired, type WorkspaceBreadcrumb, type WorkspaceFile, type WorkspaceStatsResponse } from '@/lib/types';
import { WorkspaceStatsCard } from './WorkspaceStatsCard';
import { WorkspaceExplorer } from './WorkspaceExplorer';
import { WorkspaceEditor, type OpenTab } from './WorkspaceEditor';
import { WorkspaceInspector } from './WorkspaceInspector';
import { WorkspaceContextMenu } from './WorkspaceContextMenu';

export function WorkspacePage() {
  const { t } = useTranslation('workspace');
  const { success, error: toastError } = useToast();

  // Navigation & File selection state
  const [currentDir, setCurrentDir] = useState('');
	const [breadcrumbs, setBreadcrumbs] = useState<WorkspaceBreadcrumb[]>([]);
  const [files, setFiles] = useState<WorkspaceFile[]>([]);
  const [selectedPaths, setSelectedPaths] = useState<Set<string>>(new Set());
  const [loadingFiles, setLoadingFiles] = useState(true);

  // Editor Tabs state
  const [tabs, setTabs] = useState<OpenTab[]>([]);
  const [activeTabPath, setActiveTabPath] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  // Inspector & Stats state
  const [showInspector, setShowInspector] = useState(false);
  const [stats, setStats] = useState<WorkspaceStatsResponse | null>(null);
  const [loadingStats, setLoadingStats] = useState(false);

  // Context Menu state
  const [contextMenu, setContextMenu] = useState<{
    x: number;
    y: number;
    file: WorkspaceFile;
  } | null>(null);

  // Modals state
  const [newFileModalOpen, setNewFileModalOpen] = useState(false);
  const [newFolderModalOpen, setNewFolderModalOpen] = useState(false);
  const [renameModalFile, setRenameModalFile] = useState<WorkspaceFile | null>(null);
  const [deleteModalTarget, setDeleteModalTarget] = useState<{
    file?: WorkspaceFile;
    isBatch?: boolean;
  } | null>(null);

  // Load files in directory
  const loadFiles = useCallback(async (dir: string = currentDir) => {
    setLoadingFiles(true);
    try {
      const resp = await api.listWorkspaceFiles(dir);
      setFiles(resp.files || []);
		setCurrentDir(resp.parent_id || '');
		setBreadcrumbs(resp.breadcrumbs || []);
    } catch (err: unknown) {
      toastError(getErrorMessage(err, 'Failed to load files'));
    } finally {
      setLoadingFiles(false);
    }
  }, [currentDir, toastError]);

  // Load workspace storage stats
  const loadStats = useCallback(async () => {
    setLoadingStats(true);
    try {
      const data = await api.getWorkspaceStats();
      setStats(data);
    } catch {
      // stats error fallback silently
    } finally {
      setLoadingStats(false);
    }
  }, []);

  useEffect(() => {
    loadFiles('');
    loadStats();
  }, []);

  // Open file in Editor
  const handleOpenFile = async (file: WorkspaceFile) => {
    if (file.is_dir) {
		loadFiles(file.id);
      return;
    }

    // Check if tab is already open
		const existing = tabs.find((tab) => tab.id === file.id);
    if (existing) {
		setActiveTabPath(file.id);
      return;
    }

    try {
		const detail = await api.getWorkspaceFile(file.id);
      const newTab: OpenTab = {
			id: file.id,
			parentId: file.parent_id,
			path: file.virtual_path || file.path,
        name: file.name,
        content: detail.content || '',
        originalContent: detail.content || '',
        kind: detail.kind || 'text',
        mime: detail.mime || '',
        rawUrl: detail.raw_url,
        size: detail.size || file.size,
			version: detail.version,
      };
      setTabs((prev) => [...prev, newTab]);
		setActiveTabPath(file.id);
    } catch (err: unknown) {
      toastError(getErrorMessage(err, 'Failed to open file'));
    }
  };

  // Close tab
	const handleCloseTab = (id: string) => {
		const tabToClose = tabs.find((tab) => tab.id === id);
    if (tabToClose && tabToClose.content !== tabToClose.originalContent) {
      if (!window.confirm(t('editor.discardPrompt'))) {
        return;
      }
    }

		const nextTabs = tabs.filter((tab) => tab.id !== id);
    setTabs(nextTabs);
		if (activeTabPath === id) {
			setActiveTabPath(nextTabs.length > 0 ? nextTabs[nextTabs.length - 1].id : null);
    }
  };

  // Change active tab content
	const handleChangeContent = (id: string, content: string) => {
    setTabs((prev) =>
			prev.map((tab) => (tab.id === id ? { ...tab, content } : tab))
    );
  };

  // Save file content
	const handleSaveFile = async (id: string) => {
		const targetTab = tabs.find((tab) => tab.id === id);
    if (!targetTab) return;

    setSaving(true);
    try {
			const result = await api.saveWorkspaceFile({ fileId: id, content: targetTab.content, expectedVersion: targetTab.version });
			if (isApprovalRequired(result)) {
				success(t('toasts.approvalQueued'));
				return;
			}
      setTabs((prev) =>
        prev.map((t) =>
				t.id === id ? { ...t, originalContent: t.content, size: t.content.length, version: result.version || t.version } : t
        )
      );
      success(t('toasts.saveSuccess', { name: targetTab.name }));
      loadFiles(currentDir);
      loadStats();
    } catch (err: unknown) {
      toastError(t('toasts.saveFailed', { error: getErrorMessage(err) }));
    } finally {
      setSaving(false);
    }
  };

  // Toggle multi-select path
  const handleToggleSelectPath = (path: string) => {
    setSelectedPaths((prev) => {
      const next = new Set(prev);
      if (next.has(path)) next.delete(path);
      else next.add(path);
      return next;
    });
  };

  // Create new file
  const handleCreateFile = async (fileName: string) => {
    if (!fileName.trim()) return;
    try {
			const result = await api.saveWorkspaceFile({ parentId: currentDir, name: fileName.trim(), content: '' });
      setNewFileModalOpen(false);
			if (isApprovalRequired(result)) {
				success(t('toasts.approvalQueued'));
				return;
			}
			success(t('toasts.saveSuccess', { name: fileName }));
			await loadFiles(currentDir);
			loadStats();
    } catch (err: unknown) {
      toastError(getErrorMessage(err, 'Failed to create file'));
    }
  };

  // Create new directory
  const handleCreateFolder = async (folderName: string) => {
    if (!folderName.trim()) return;
    try {
			const result = await api.mkdirWorkspace(currentDir, folderName.trim());
      setNewFolderModalOpen(false);
			if (isApprovalRequired(result)) {
				success(t('toasts.approvalQueued'));
				return;
			}
      success(t('toasts.mkdirSuccess', { path: folderName }));
      loadFiles(currentDir);
      loadStats();
    } catch (err: unknown) {
      toastError(t('toasts.mkdirFailed', { error: getErrorMessage(err) }));
    }
  };

  // Rename file / folder
  const handleRename = async (newName: string) => {
    if (!renameModalFile || !newName.trim()) return;

    try {
			const result = await api.renameWorkspaceFile(renameModalFile.id, renameModalFile.parent_id, newName.trim(), renameModalFile.version);
      setRenameModalFile(null);
			if (isApprovalRequired(result)) {
				success(t('toasts.approvalQueued'));
				return;
			}
			success(t('toasts.renameSuccess', { path: newName.trim() }));
      loadFiles(currentDir);
      loadStats();
    } catch (err: unknown) {
      toastError(t('toasts.renameFailed', { error: getErrorMessage(err) }));
    }
  };

  // Duplicate file
  const handleDuplicate = async (file: WorkspaceFile) => {
    try {
			const resp = await api.duplicateWorkspaceFile(file.id, file.parent_id);
			if (isApprovalRequired(resp)) {
				success(t('toasts.approvalQueued'));
				return;
			}
			success(t('toasts.duplicateSuccess', { path: resp.virtual_path || resp.name }));
      loadFiles(currentDir);
      loadStats();
    } catch (err: unknown) {
      toastError(t('toasts.duplicateFailed', { error: getErrorMessage(err) }));
    }
  };

  // Copy relative path to clipboard
  const handleCopyPath = async (file: WorkspaceFile) => {
    try {
      await navigator.clipboard.writeText(file.path);
      success(t('toasts.copiedPath'));
    } catch { }
  };

  // Download single file or folder ZIP
  const handleDownload = (file: WorkspaceFile) => {
    if (file.is_dir) {
      window.open(api.getWorkspaceZipUrl(file.id), '_blank', 'noopener,noreferrer');
    } else {
      window.open(api.getWorkspaceRawUrl(file.id, true), '_blank', 'noopener,noreferrer');
    }
  };

  // Delete file or folder
  const handleDeleteConfirm = async () => {
    if (!deleteModalTarget) return;

    if (deleteModalTarget.isBatch) {
		for (const id of selectedPaths) {
        try {
					const selected = files.find((file) => file.id === id);
					await api.deleteWorkspaceFile(id, selected?.version, selected?.is_dir || false);
					handleCloseTab(id);
        } catch { }
      }
      success(t('toasts.deleteSuccess', { path: `${selectedPaths.size} items` }));
      setSelectedPaths(new Set());
    } else if (deleteModalTarget.file) {
		const target = deleteModalTarget.file;
      try {
				const result = await api.deleteWorkspaceFile(target.id, target.version, target.is_dir);
				if (isApprovalRequired(result)) {
					success(t('toasts.approvalQueued'));
				} else {
					handleCloseTab(target.id);
					success(t('toasts.deleteSuccess', { path: target.path }));
				}
      } catch (err: unknown) {
        toastError(t('toasts.deleteFailed', { error: getErrorMessage(err) }));
      }
    }

    setDeleteModalTarget(null);
    loadFiles(currentDir);
    loadStats();
  };

  // Reindex file for AI
  const handleReindex = async (file: WorkspaceFile) => {
    try {
			await api.reindexWorkspaceFile(file.id);
      success(t('toasts.reindexSuccess', { path: file.path }));
      loadFiles(currentDir);
    } catch (err: unknown) {
      toastError(t('toasts.reindexFailed', { error: getErrorMessage(err) }));
    }
  };

  // Upload file list
  const handleUploadFiles = async (fileList: FileList) => {
    let successCount = 0;
	let approvalCount = 0;
    for (let i = 0; i < fileList.length; i++) {
      const item = fileList[i];
      const formData = new FormData();
      formData.append('file', item);
			formData.append('parent_id', currentDir);

      try {
			const res = await fetch(`${API_BASE}/workspace/upload`, {
          method: 'POST',
          body: formData,
        });
			if (res.ok) {
				const envelope = await res.json();
				const data = envelope.data ?? envelope;
				if (data?.status === 'approval_required' && data.approval) {
					approvalCount++;
					window.dispatchEvent(new CustomEvent('actonos:approval-required', { detail: data.approval }));
				} else {
					successCount++;
				}
			}
      } catch { }
    }

		if (approvalCount > 0) {
			success(t('toasts.approvalQueuedCount', { count: approvalCount }));
		}
		if (successCount > 0) {
      success(t('toasts.uploadSuccess', { count: successCount }));
      loadFiles(currentDir);
      loadStats();
		} else if (approvalCount === 0) {
      toastError(t('toasts.uploadFailed', { error: 'Upload failed' }));
    }
  };

	const activeTab = tabs.find((tab) => tab.id === activeTabPath) || null;

  return (
    <PageContainer maxWidth='wide'>
      <BlobBackdrop />

      {/* Page Header */}
      <div className="flex flex-col md:flex-row md:items-center md:justify-between gap-4 mb-4">
        <PageHeader
          eyebrow={t('eyebrow')}
          title={t('title')}
          description={t('subtitle')}
        />
        <div className="w-full md:w-100 shrink-0">
          <WorkspaceStatsCard stats={stats} loading={loadingStats} />
        </div>
      </div>

      {/* Main Workspace 3-Pane Studio Container */}
      <Card className="p-0 overflow-hidden border border-deep-ink/10 shadow-xs h-[calc(100vh-250px)] min-h-[550px] flex">
        {/* Left Pane: Explorer */}
        <WorkspaceExplorer
          files={files}
					breadcrumbs={breadcrumbs}
          selectedPath={activeTabPath}
          selectedPaths={selectedPaths}
          loading={loadingFiles}
          onSelectFile={handleOpenFile}
          onToggleSelectPath={handleToggleSelectPath}
          onNavigateDir={(dir) => {
            loadFiles(dir);
          }}
          onUpload={handleUploadFiles}
          onNewFile={() => setNewFileModalOpen(true)}
          onNewFolder={() => setNewFolderModalOpen(true)}
          onRefresh={() => {
            loadFiles(currentDir);
            loadStats();
          }}
          onBatchDelete={() => setDeleteModalTarget({ isBatch: true })}
          onBatchDownload={() => {
            window.open(api.getWorkspaceZipUrl(undefined, Array.from(selectedPaths)), '_blank', 'noopener,noreferrer');
          }}
          onContextMenu={(file, e) => {
            setContextMenu({ x: e.clientX, y: e.clientY, file });
          }}
        />

        {/* Center Pane: Multi-Tab Rich Editor */}
        <WorkspaceEditor
          tabs={tabs}
          activeTabPath={activeTabPath}
          saving={saving}
          onSelectTab={setActiveTabPath}
          onCloseTab={handleCloseTab}
          onChangeContent={handleChangeContent}
          onSave={handleSaveFile}
          showInspector={showInspector}
          onToggleInspector={() => setShowInspector(!showInspector)}
        />

        {/* Right Pane: Inspector Drawer */}
        {showInspector && (
          <WorkspaceInspector
						fileId={activeTab?.id || null}
            filePath={activeTab?.path || null}
            fileKind={activeTab?.kind || 'text'}
            fileSize={activeTab?.size || 0}
            mimeType={activeTab?.mime || ''}
            content={activeTab?.content || ''}
            onRename={() => {
              if (activeTab) {
                setRenameModalFile({
								id: activeTab.id,
								parent_id: activeTab.parentId,
                  name: activeTab.name,
                  path: activeTab.path,
								virtual_path: activeTab.path,
                  is_dir: false,
                  size: activeTab.size,
                  mod_time: '',
								version: activeTab.version,
								kind: activeTab.kind,
								mime_type: activeTab.mime,
                });
              }
            }}
            onDelete={() => {
              if (activeTab) {
                setDeleteModalTarget({
                  file: {
									id: activeTab.id,
									parent_id: activeTab.parentId,
                    name: activeTab.name,
                    path: activeTab.path,
									virtual_path: activeTab.path,
                    is_dir: false,
                    size: activeTab.size,
                    mod_time: '',
									version: activeTab.version,
									kind: activeTab.kind,
									mime_type: activeTab.mime,
                  },
                });
              }
            }}
            onChatWithFile={() => {
              if (activeTab) {
								window.location.hash = `#/chat?file_id=${encodeURIComponent(activeTab.id)}`;
              }
            }}
          />
        )}
      </Card>

      {/* Right Click Context Menu */}
      {contextMenu && (
        <WorkspaceContextMenu
          x={contextMenu.x}
          y={contextMenu.y}
          file={contextMenu.file}
          onClose={() => setContextMenu(null)}
          onOpen={handleOpenFile}
          onRename={(file) => setRenameModalFile(file)}
          onDuplicate={handleDuplicate}
          onCopyPath={handleCopyPath}
          onDownload={handleDownload}
          onReindex={handleReindex}
          onChatWithFile={(file) => {
			window.location.hash = `#/chat?file_id=${encodeURIComponent(file.id)}`;
          }}
          onDelete={(file) => setDeleteModalTarget({ file })}
        />
      )}

      {/* Create New File Modal */}
      <PromptModal
        isOpen={newFileModalOpen}
        onClose={() => setNewFileModalOpen(false)}
        onSubmit={handleCreateFile}
        title={t('createFileModal.title')}
        label={t('createFileModal.label')}
        placeholder={t('createFileModal.placeholder')}
        confirmLabel={t('createFileModal.confirm')}
      />

      {/* Create New Folder Modal */}
      <PromptModal
        isOpen={newFolderModalOpen}
        onClose={() => setNewFolderModalOpen(false)}
        onSubmit={handleCreateFolder}
        title={t('createFolderModal.title')}
        label={t('createFolderModal.label')}
        placeholder={t('createFolderModal.placeholder')}
        confirmLabel={t('createFolderModal.confirm')}
      />

      {/* Rename File / Folder Modal */}
      <PromptModal
        isOpen={Boolean(renameModalFile)}
        onClose={() => setRenameModalFile(null)}
        onSubmit={handleRename}
        defaultValue={renameModalFile?.name || ''}
        title={t('renameModal.title')}
        label={t('renameModal.label')}
        placeholder={t('renameModal.placeholder')}
        confirmLabel={t('renameModal.confirm')}
      />

      {/* Delete Confirmation Modal */}
      <ConfirmModal
        isOpen={Boolean(deleteModalTarget)}
        onClose={() => setDeleteModalTarget(null)}
        onConfirm={handleDeleteConfirm}
        title={
          deleteModalTarget?.isBatch
            ? t('deleteModal.batchTitle')
            : t('deleteModal.title')
        }
        description={
          deleteModalTarget?.isBatch
            ? t('deleteModal.batchDescription', { count: selectedPaths.size })
            : t('deleteModal.description', { path: deleteModalTarget?.file?.name || '' })
        }
      />
    </PageContainer>
  );
}
