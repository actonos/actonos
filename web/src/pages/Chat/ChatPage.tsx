import { useState, useEffect, useRef, type FormEvent } from 'react';
import { getErrorMessage } from '@/lib/errors';
import { useTranslation } from 'react-i18next';
import { PageContainer } from '@/components/layout/PageContainer';
import { ChatHeader } from '@/components/features/chat/ChatHeader';
import {
  ChatComposer,
  type AttachedFile,
  isSupportedTextOrCodeFile,
  MAX_ATTACHMENT_SIZE_BYTES,
  MAX_TEXT_INJECTION_CHARS,
} from '@/components/features/chat/ChatComposer';
import { MessageTimeline } from '@/components/features/chat/MessageTimeline';
import { ChatSessionsTable } from '@/components/features/chat/ChatSessionsTable';
import { RenameSessionModal } from '@/components/features/chat/RenameSessionModal';
import { BlobBackdrop } from '@/components/ui/BlobBackdrop';
import { Card } from '@/components/ui/Card';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { ConfirmModal } from '@/components/ui/ConfirmModal';
import { useToast } from '@/components/ui/Toast';
import { useRealtime } from '@/components/providers/RealtimeProvider';
import { readHashParams, setHashParam } from '@/lib/url-state';
import {
  Bot,
  Sparkles,
  Trash2,
  Plus,
  ArrowLeft,
  ChevronDown,
  Pin,
  Edit3,
  Copy,
  Check,
} from 'lucide-react';
import { api } from '@/lib/api';
import type { AgentManifest, ConversationItem, ToolInfo } from '@/lib/types';
import type { NavTab } from '@/components/layout/Sidebar';
import type { AutocompleteFileItem } from '@/components/features/chat/ChatAutocompletePopover';
import {
  type ChatMessage,
  type ToolCallTrace,
} from './chatTypes';
import {
  isPersistedChatSession,
  NEW_CHAT_SESSION_PARAM,
  parseChatSessionRoute,
  shouldHydrateChatSession,
} from './chatSession';
import {
  applyChatStreamEvent,
  attachPendingApprovalToMessages,
  upsertStreamingAssistant,
} from './chatStream';

export interface ChatPageProps {
  selectedAgentID?: string;
  onSelectAgentID?: (id: string) => void;
  onNavigateTab?: (tab: NavTab) => void;
}

export function ChatPage({ selectedAgentID, onSelectAgentID }: ChatPageProps) {
  const { t } = useTranslation('chat');
  const { t: tCommon } = useTranslation('common');
  const { success, error } = useToast();

  // Mode: 'sessions' = Sessions Table Hub, 'chat' = Active Conversation Canvas
  const [viewMode, setViewMode] = useState<'sessions' | 'chat'>('sessions');

  const [agents, setAgents] = useState<AgentManifest[]>([]);
  const [activeAgentID, setActiveAgentID] = useState<string>(selectedAgentID || 'agent_system_core');

  // Conversations & History
  const [conversations, setConversations] = useState<ConversationItem[]>([]);
  const [activeConvID, setActiveConvID] = useState<string | null>(null);
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [input, setInput] = useState('');
  const [loading, setLoading] = useState(false);
  const [copiedIdx, setCopiedIdx] = useState<number | null>(null);
  const [copiedConvId, setCopiedConvId] = useState(false);
  const [activeTab, setActiveTab] = useState<Record<string, 'traces' | 'audit'>>({});
  const [expandedTrace, setExpandedTrace] = useState<Record<string, boolean>>({});

  // Skills & Files for Autocomplete
  const [skills, setSkills] = useState<ToolInfo[]>([]);
  const [workspaceFiles, setWorkspaceFiles] = useState<AutocompleteFileItem[]>([]);

  // Table Hub Filters & Search
  const [tableSearch, setTableSearch] = useState('');
  const [tableAgentID, setTableAgentID] = useState('all');
  const [tableChannel, setTableChannel] = useState('all');
  const [tablePinnedOnly, setTablePinnedOnly] = useState(false);

  // Modals & Renaming
  const [deletingConvId, setDeletingConvId] = useState<string | null>(null);
  const [editingConv, setEditingConv] = useState<ConversationItem | null>(null);
  const [pendingAttachments, setPendingAttachments] = useState<AttachedFile[]>([]);

  const messagesEndRef = useRef<HTMLDivElement>(null);
  const messagesContainerRef = useRef<HTMLDivElement>(null);
  const isAutoScrollEnabled = useRef(true);
  const [showScrollBottom, setShowScrollBottom] = useState(false);
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const activeStreamAbortController = useRef<AbortController | null>(null);
  const waitingForResumeRef = useRef(false);
  const loadingRef = useRef(false);
  loadingRef.current = loading;
  const activeConvIDRef = useRef<string | null>(activeConvID);
  activeConvIDRef.current = activeConvID;
  const sessionHydrateGenRef = useRef(0);

  // Load agents and conversations
  const loadAgents = async () => {
    try {
      const res = await api.listAgents();
      setAgents(res.agents || []);
      if (!activeAgentID && res.agents?.length > 0) {
        setActiveAgentID(res.agents[0].agent_id);
      }
    } catch (err) {
      error('Failed to load agents', getErrorMessage(err));
    }
  };

  const loadConversations = async () => {
    try {
      const res = await api.listConversations();
      const list = res.conversations || [];
      setConversations(list);
    } catch (err) {
      error('Failed to load conversations', getErrorMessage(err));
    }
  };

  const loadSkillsAndFiles = async () => {
    try {
      const [skillsRes, filesRes] = await Promise.allSettled([
        api.listTools(),
        api.listWorkspaceFiles(),
      ]);

      if (skillsRes.status === 'fulfilled' && skillsRes.value?.tools) {
        setSkills(skillsRes.value.tools);
      }

      if (filesRes.status === 'fulfilled' && filesRes.value?.files) {
        const mapped: AutocompleteFileItem[] = filesRes.value.files.map((f) => ({
          id: f.id,
          name: f.name,
          path: f.virtual_path || f.path || `/${f.name}`,
          is_dir: f.is_dir,
          size: f.size,
        }));
        setWorkspaceFiles(mapped);
      }
    } catch {}
  };

  const selectConversation = async (convID: string) => {
    if (!isPersistedChatSession(convID)) {
      activeConvIDRef.current = null;
      setActiveConvID(null);
      setMessages([]);
      return;
    }
    if (loadingRef.current && convID === activeConvIDRef.current) {
      return;
    }
    const generation = ++sessionHydrateGenRef.current;
    activeConvIDRef.current = convID;
    setActiveConvID(convID);
    localStorage.setItem('actonos_active_conv_id', convID);
    try {
      const res = await api.getConversation(convID);
      if (generation !== sessionHydrateGenRef.current) {
        return;
      }
      if (loadingRef.current && convID === activeConvIDRef.current) {
        return;
      }
      if (res.conversation?.agent_id) {
        setActiveAgentID(res.conversation.agent_id);
      }
      if (res.messages) {
        const mapped: ChatMessage[] = res.messages.map((m) => {
            let toolCalls: ToolCallTrace[] = [];
            if (m.tool_calls_json && m.tool_calls_json !== 'null' && m.tool_calls_json !== '[]') {
              try {
                const parsed = JSON.parse(m.tool_calls_json);
                if (Array.isArray(parsed)) {
                  toolCalls = parsed.map((rawCall: unknown) => {
                    const tc =
                      typeof rawCall === 'object' && rawCall !== null
                        ? (rawCall as Record<string, unknown>)
                        : {};
                    const fn =
                      typeof tc.function === 'object' && tc.function !== null
                        ? (tc.function as Record<string, unknown>)
                        : {};
                    return {
                      tool:
                        typeof fn.name === 'string'
                          ? fn.name
                          : typeof tc.name === 'string'
                            ? tc.name
                            : 'native_tool',
                      args: fn.arguments,
                      status: 'success',
                    };
                  });
                }
              } catch {}
            }
            return {
              id: m.id,
              role: m.role === 'user' || m.role === 'assistant' ? m.role : 'system',
              content: m.content,
              timestamp: new Date(m.created_at).toLocaleTimeString([], {
                hour: '2-digit',
                minute: '2-digit',
              }),
              toolCalls: toolCalls.length > 0 ? toolCalls : undefined,
            };
          });
        const agentID = res.conversation?.agent_id || activeAgentID;
        let hydrated = mapped;
        try {
          const pending = await api.listApprovals('pending');
          hydrated = attachPendingApprovalToMessages(mapped, pending.approvals || [], agentID);
        } catch {
          hydrated = mapped;
        }
        if (hydrated.some((message) => message.pendingApproval)) {
          waitingForResumeRef.current = true;
          setLoading(true);
        }
        setMessages(hydrated);
      } else {
        setMessages([]);
      }
    } catch (err) {
      error('Failed to load messages', getErrorMessage(err));
    }
  };

  // URL session_id and file_id Synchronization
  const syncSessionFromUrl = async () => {
    const params = readHashParams();
    const urlSessionId = params.get('session_id');
    const urlFileId = params.get('file_id');
    const urlFilePath = params.get('file_path');

    if (urlFileId) {
      // Clear file query params from URL so it doesn't loop
      setHashParam('file_id', undefined);
      setHashParam('file_path', undefined);

      try {
        const fileDetail = await api.getWorkspaceFile(urlFileId);
        if (fileDetail) {
          const rawPath =
            urlFilePath ||
            (fileDetail as { virtual_path?: string }).virtual_path ||
            (fileDetail as { name?: string }).name ||
            'document';
          const fileName = rawPath.split('/').pop() || rawPath;

          if (!isSupportedTextOrCodeFile(fileName, fileDetail.mime)) {
            error(t('attachments.unsupportedType', 'Only code, text, and document files are supported.'));
          } else if ((fileDetail.size || 0) > MAX_ATTACHMENT_SIZE_BYTES) {
            error(t('attachments.fileTooLarge', 'File size exceeds 5MB limit for code and document analysis.'));
          } else {
            let textContent = fileDetail.content || '';
            if (textContent.length > MAX_TEXT_INJECTION_CHARS) {
              const originalKb = ((fileDetail.size || 0) / 1024).toFixed(1);
              textContent =
                textContent.slice(0, MAX_TEXT_INJECTION_CHARS) +
                `\n\n... [Content truncated: displaying first ${(MAX_TEXT_INJECTION_CHARS / 1024).toFixed(1)} KB of ${originalKb} KB] ...`;
            }

            const attachedWsFile: AttachedFile = {
              id: `att_ws_${Date.now()}_${Math.random().toString(36).slice(2, 6)}`,
              name: fileName,
              size: fileDetail.size || 0,
              type: fileDetail.mime || 'text/plain',
              textContent,
              file_id: urlFileId,
              path: rawPath,
              isWorkspace: true,
            };
            setPendingAttachments((prev) => [...prev, attachedWsFile]);
          }
        }
      } catch (err) {
        console.warn('Failed to load workspace file for chat:', err);
      }

      if (!isPersistedChatSession(activeConvIDRef.current)) {
        setHashParam('session_id', NEW_CHAT_SESSION_PARAM);
        activeConvIDRef.current = null;
        setActiveConvID(null);
        localStorage.removeItem('actonos_active_conv_id');
        setMessages([]);
      }
      setViewMode('chat');
      setTimeout(() => {
        inputRef.current?.focus();
      }, 150);
    } else {
      const route = parseChatSessionRoute(urlSessionId);
      if (route.mode === 'draft') {
        if (!loadingRef.current) {
          activeConvIDRef.current = null;
          setActiveConvID(null);
          setMessages([]);
        }
        setViewMode('chat');
      } else if (route.mode === 'load') {
        if (
          shouldHydrateChatSession(route, activeConvIDRef.current, {
            streaming: loadingRef.current,
          })
        ) {
          void selectConversation(route.sessionId);
        } else if (route.sessionId !== activeConvIDRef.current) {
          activeConvIDRef.current = route.sessionId;
          setActiveConvID(route.sessionId);
          localStorage.setItem('actonos_active_conv_id', route.sessionId);
        }
        setViewMode('chat');
      } else {
        setViewMode('sessions');
        activeConvIDRef.current = null;
        setActiveConvID(null);
      }
    }
  };

  useEffect(() => {
    loadAgents();
    loadConversations();
    loadSkillsAndFiles();
    syncSessionFromUrl();

    const handleHashChange = () => {
      syncSessionFromUrl();
    };
    window.addEventListener('hashchange', handleHashChange);
    return () => window.removeEventListener('hashchange', handleHashChange);
  }, []);

  const handleCancelStreaming = () => {
    if (activeStreamAbortController.current) {
      activeStreamAbortController.current.abort();
      activeStreamAbortController.current = null;
    }
    waitingForResumeRef.current = false;
    loadingRef.current = false;
    setLoading(false);
  };

  const handleViewSession = (convID: string) => {
    handleCancelStreaming();
    sessionHydrateGenRef.current += 1;
    setHashParam('session_id', convID);
    selectConversation(convID);
    setViewMode('chat');
  };

  const handleNewChat = () => {
    handleCancelStreaming();
    sessionHydrateGenRef.current += 1;
    setHashParam('session_id', NEW_CHAT_SESSION_PARAM);
    activeConvIDRef.current = null;
    setActiveConvID(null);
    localStorage.removeItem('actonos_active_conv_id');
    setMessages([]);
    setViewMode('chat');
    setTimeout(() => {
      inputRef.current?.focus();
    }, 100);
  };

  const handleBackToSessions = () => {
    handleCancelStreaming();
    sessionHydrateGenRef.current += 1;
    setHashParam('session_id', undefined);
    activeConvIDRef.current = null;
    setActiveConvID(null);
    setViewMode('sessions');
    loadConversations();
  };

  const handleTogglePin = async (convID: string, currentPinned: boolean) => {
    const nextPinned = !currentPinned;
    // Optimistically update list
    setConversations((prev) => {
      const updated = prev.map((c) => (c.id === convID ? { ...c, is_pinned: nextPinned } : c));
      return updated.sort((a, b) => {
        if (!!a.is_pinned !== !!b.is_pinned) {
          return a.is_pinned ? -1 : 1;
        }
        return (
          new Date(b.updated_at || b.created_at).getTime() -
          new Date(a.updated_at || a.created_at).getTime()
        );
      });
    });

    try {
      await api.togglePinConversation(convID, nextPinned);
      success(
        nextPinned ? t('actions.pin') : t('actions.unpin'),
        nextPinned ? 'Session pinned to top.' : 'Session unpinned.'
      );
    } catch (err) {
      loadConversations();
      error('Failed to update pin', getErrorMessage(err));
    }
  };

  const handleConfirmDeleteConv = async () => {
    if (!deletingConvId) return;
    try {
      await api.deleteConversation(deletingConvId);
      const remaining = conversations.filter((c) => c.id !== deletingConvId);
      setConversations(remaining);
      success(t('deleteSession', 'Session Deleted'), 'Conversation history cleared.');
      if (activeConvID === deletingConvId) {
        handleBackToSessions();
      }
      setDeletingConvId(null);
    } catch (err) {
      error('Failed to delete session', getErrorMessage(err));
    }
  };

  const handleSaveRename = async (newTitle: string) => {
    if (!editingConv) return;
    try {
      await api.updateConversationTitle(editingConv.id, newTitle);
      setConversations((prev) =>
        prev.map((c) => (c.id === editingConv.id ? { ...c, title: newTitle } : c))
      );
      success('Title Updated', 'Session renamed.');
    } catch (err) {
      error('Failed to update title', getErrorMessage(err));
    } finally {
      setEditingConv(null);
    }
  };

  const { snapshot } = useRealtime();

  useEffect(() => {
    const onDecided = async (event: Event) => {
      if (!waitingForResumeRef.current) return;
      const convID = activeConvIDRef.current;
      waitingForResumeRef.current = false;
      if (!isPersistedChatSession(convID)) {
        setLoading(false);
        return;
      }
      const approved = Boolean((event as CustomEvent<{ approved?: boolean }>).detail?.approved);
      for (let attempt = 0; attempt < 12; attempt += 1) {
        await new Promise((resolve) => setTimeout(resolve, 400));
        try {
          const res = await api.getConversation(convID);
          if (!res.messages?.length) continue;
          const last = res.messages[res.messages.length - 1];
          if (approved && last.role === 'assistant' && last.content && !last.content.startsWith('Execution error:')) {
            await selectConversation(convID);
            break;
          }
          if (!approved) {
            setMessages((prev) =>
              prev.map((message) =>
                message.pendingApproval
                  ? {
                      ...message,
                      pendingApproval: undefined,
                      thought: undefined,
                      toolCalls: (message.toolCalls || []).map((call) =>
                        call.status === 'awaiting_approval' ? { ...call, status: 'rejected' } : call
                      ),
                    }
                  : message
              )
            );
            break;
          }
        } catch {
          break;
        }
      }
      setLoading(false);
    };
    window.addEventListener('actonos:approval-decided', onDecided);
    return () => window.removeEventListener('actonos:approval-decided', onDecided);
  }, []);

  useEffect(() => {
    if (selectedAgentID) {
      setActiveAgentID(selectedAgentID);
    }
  }, [selectedAgentID]);

  // Real-time synchronization: sync when snapshot timestamp updates from WebSocket
  useEffect(() => {
    if (!snapshot?.timestamp) return;
    let cancelled = false;

    const syncOnSocketEvent = async () => {
      if (loadingRef.current) return;
      const currentID = activeConvIDRef.current;

      try {
        if (isPersistedChatSession(currentID)) {
          const res = await api.getConversation(currentID);
          if (cancelled || !res.messages) return;

          const newFormatted: ChatMessage[] = res.messages.map((m) => {
            let toolCalls: ToolCallTrace[] = [];
            if (m.tool_calls_json && m.tool_calls_json !== 'null' && m.tool_calls_json !== '[]') {
              try {
                const parsed = JSON.parse(m.tool_calls_json);
                if (Array.isArray(parsed)) {
                  toolCalls = parsed.map((rawCall: unknown) => {
                    const tc =
                      typeof rawCall === 'object' && rawCall !== null
                        ? (rawCall as Record<string, unknown>)
                        : {};
                    const fn =
                      typeof tc.function === 'object' && tc.function !== null
                        ? (tc.function as Record<string, unknown>)
                        : {};
                    return {
                      tool:
                        typeof fn.name === 'string'
                          ? fn.name
                          : typeof tc.name === 'string'
                            ? tc.name
                            : 'native_tool',
                      args: fn.arguments,
                      status: 'success',
                    };
                  });
                }
              } catch {}
            }
            return {
              id: m.id,
              role: m.role === 'user' || m.role === 'assistant' ? m.role : 'system',
              content: m.content,
              timestamp: new Date(m.created_at).toLocaleTimeString([], {
                hour: '2-digit',
                minute: '2-digit',
              }),
              toolCalls: toolCalls.length > 0 ? toolCalls : undefined,
            };
          });

          setMessages((prev) => {
            if (newFormatted.length < prev.length) {
              return prev;
            }
            if (
              prev.length !== newFormatted.length ||
              (prev.length > 0 &&
                prev[prev.length - 1]?.content !== newFormatted[newFormatted.length - 1]?.content)
            ) {
              return newFormatted;
            }
            return prev;
          });
        }

        const convsRes = await api.listConversations();
        if (!cancelled && convsRes.conversations) {
          setConversations(convsRes.conversations);
        }
      } catch {}
    };

    syncOnSocketEvent();

    return () => {
      cancelled = true;
    };
  }, [snapshot?.timestamp]);

  const handleScroll = () => {
    if (!messagesContainerRef.current) return;
    const { scrollTop, scrollHeight, clientHeight } = messagesContainerRef.current;
    const distanceFromBottom = scrollHeight - scrollTop - clientHeight;
    const atBottom = distanceFromBottom < 80;
    isAutoScrollEnabled.current = atBottom;
    setShowScrollBottom(!atBottom);
  };

  const scrollToBottom = (behavior: ScrollBehavior = 'auto') => {
    requestAnimationFrame(() => {
      if (messagesContainerRef.current) {
        messagesContainerRef.current.scrollTo({
          top: messagesContainerRef.current.scrollHeight,
          behavior,
        });
      } else {
        messagesEndRef.current?.scrollIntoView({ behavior, block: 'end' });
      }
    });
  };

  const handleScrollToBottomClick = () => {
    isAutoScrollEnabled.current = true;
    setShowScrollBottom(false);
    scrollToBottom('smooth');
  };

  // Scroll to bottom when conversation is selected or messages are loaded
  const activeConvRef = useRef<string | null>(null);
  useEffect(() => {
    if (viewMode === 'chat' && (activeConvID !== activeConvRef.current || messages.length > 0)) {
      activeConvRef.current = activeConvID;
      isAutoScrollEnabled.current = true;
      setShowScrollBottom(false);
      scrollToBottom('auto');
      const timer = setTimeout(() => scrollToBottom('auto'), 60);
      const timer2 = setTimeout(() => scrollToBottom('auto'), 200);
      return () => {
        clearTimeout(timer);
        clearTimeout(timer2);
      };
    }
  }, [viewMode, activeConvID, messages.length]);

  const wasLoadingRef = useRef(false);
  useEffect(() => {
    if (viewMode !== 'chat') return;
    const lastMsg = messages[messages.length - 1];
    const justSent = lastMsg?.role === 'user';
    const justFinished = wasLoadingRef.current && !loading;
    wasLoadingRef.current = loading;

    if (justSent) {
      isAutoScrollEnabled.current = true;
      setShowScrollBottom(false);
      scrollToBottom('smooth');
      return;
    }

    if (justFinished) {
      if (isAutoScrollEnabled.current) {
        scrollToBottom('smooth');
      }
      return;
    }

    if (loading && isAutoScrollEnabled.current) {
      scrollToBottom('auto');
    }
  }, [viewMode, messages, loading]);

  const handleSend = async (e?: FormEvent, attachedFiles: AttachedFile[] = []) => {
    if (e) e.preventDefault();
    if ((!input.trim() && attachedFiles.length === 0) || !activeAgentID || loading) return;

    const rawUserInput = input.trim();
    setInput('');

    // If there are attached text/workspace files, incorporate contents into the context for AI
    let userMsgText = rawUserInput;
    if (attachedFiles.length > 0) {
      const fileContextParts: string[] = [];
      attachedFiles.forEach((file) => {
        if (file.isImage && file.previewUrl) {
          const dims = file.width && file.height ? ` (${file.width}x${file.height})` : '';
          fileContextParts.push(
            `\n\n---\n[Attached Image: "${file.name}"${dims} (${(file.size / 1024).toFixed(1)} KB)]\n![${file.name}](${file.previewUrl})`
          );
        } else if (file.isWorkspace && file.file_id) {
          fileContextParts.push(
            `\n\n---\n[Attached User Workspace Document: "${file.name}" (File ID: ${file.file_id}, Path: ${file.path || file.name}, Size: ${(file.size / 1024).toFixed(1)} KB)]\n\`\`\`\n${file.textContent || '(content empty or binary)'}\n\`\`\``
          );
        } else if (file.textContent) {
          fileContextParts.push(
            `\n\n---\n[Attached File: "${file.name}" (${(file.size / 1024).toFixed(1)} KB)]\n\`\`\`\n${file.textContent}\n\`\`\``
          );
        } else {
          fileContextParts.push(
            `\n\n---\n[Attached File: "${file.name}" (${(file.size / 1024).toFixed(1)} KB, type: ${file.type})]`
          );
        }
      });

      userMsgText = rawUserInput
        ? `${fileContextParts.join('')}\n\n[User Request]:\n${rawUserInput}`
        : `${fileContextParts.join('')}\n\n[User Request]:\nVui lòng đọc, phân tích và giải đáp theo các tệp và hình ảnh đính kèm trên.`;
    }

    const now = new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });

    const userMsgObj: ChatMessage = {
      id: 'msg_' + Date.now(),
      role: 'user',
      content: userMsgText,
      displayContent: rawUserInput || (attachedFiles.length > 0 ? `Đã gửi ${attachedFiles.length} tệp đính kèm` : ''),
      attachments: attachedFiles.map((att) => ({
        name: att.name,
        size: att.size,
        type: att.type,
        isWorkspace: att.isWorkspace,
        file_id: att.file_id,
        path: att.path,
        previewUrl: att.previewUrl,
        thumbnailUrl: att.thumbnailUrl,
        isImage: att.isImage,
        width: att.width,
        height: att.height,
      })),
      timestamp: now,
    };

    const assistantMsgId = 'msg_' + (Date.now() + 1);
    let currentAssistantMsg: ChatMessage = {
      id: assistantMsgId,
      role: 'assistant',
      content: '',
      timestamp: now,
      thought: `Thinking with ${activeAgent?.name || 'Assistant'}...`,
      segments: [],
      toolCalls: [],
      auditLogs: [],
      finalized: false,
    };

    sessionHydrateGenRef.current += 1;
    setMessages((prev) => [...prev, userMsgObj, currentAssistantMsg]);
    loadingRef.current = true;
    setLoading(true);

    const abortController = new AbortController();
    activeStreamAbortController.current = abortController;

    try {
      const response = await api.streamChat(
        activeAgentID,
        {
          conversation_id: isPersistedChatSession(activeConvID) ? activeConvID : undefined,
          message: userMsgText,
        },
        abortController.signal
      );

      if (!response.ok) {
        if (response.status === 401) {
          throw new Error('Authentication required (401). Please unlock ActonOS.');
        }
        throw new Error(`Server returned HTTP ${response.status}`);
      }

      if (!response.body) {
        throw new Error('ReadableStream not supported');
      }

      const reader = response.body.getReader();
      const decoder = new TextDecoder('utf-8');
      let buffer = '';
      let currentEvent = 'token';

      const yieldToRenderer = () =>
        new Promise<void>((resolve) => {
          requestAnimationFrame(() => resolve());
        });

      while (true) {
        const { value, done } = await reader.read();
        if (done) break;

        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split('\n');
        buffer = lines.pop() || '';

        for (const line of lines) {
          const trimmed = line.trim();
          if (!trimmed) continue;

          if (trimmed.startsWith('event:')) {
            currentEvent = trimmed.slice(6).trim();
            continue;
          }

          if (trimmed.startsWith('data:')) {
            const dataStr = trimmed.slice(5).trim();
            try {
              const parsed = JSON.parse(dataStr);

              if (typeof parsed.conversation_id === 'string' && parsed.conversation_id) {
                const nextConvID = parsed.conversation_id;
                if (activeConvIDRef.current !== nextConvID) {
                  activeConvIDRef.current = nextConvID;
                  setActiveConvID(nextConvID);
                  setHashParam('session_id', nextConvID);
                  localStorage.setItem('actonos_active_conv_id', nextConvID);
                }
                setConversations((prev) => {
                  const exists = prev.some((c) => c.id === parsed.conversation_id);
                  if (exists) {
                    return prev.map((c) =>
                      c.id === parsed.conversation_id && parsed.title
                        ? {
                            ...c,
                            title: parsed.title,
                            last_message: userMsgText,
                            updated_at: new Date().toISOString(),
                            message_count: (c.message_count || 0) + 2,
                          }
                        : c
                    );
                  } else {
                    return [
                      {
                        id: parsed.conversation_id,
                        agent_id: activeAgentID,
                        title: parsed.title || userMsgText.slice(0, 35) + '...',
                        channel: 'web',
                        is_pinned: false,
                        message_count: 2,
                        last_message: userMsgText,
                        created_at: new Date().toISOString(),
                        updated_at: new Date().toISOString(),
                      },
                      ...prev,
                    ];
                  }
                });
              }

              currentAssistantMsg = applyChatStreamEvent(
                currentAssistantMsg,
                currentEvent,
                parsed as Record<string, unknown>
              );
              if (currentEvent === 'approval_required') {
                const toolName =
                  (typeof parsed.tool === 'string' && parsed.tool) ||
                  currentAssistantMsg.pendingApproval?.tool_name ||
                  '';
                currentAssistantMsg = {
                  ...currentAssistantMsg,
                  thought: t('waitingApproval', { tool: toolName }),
                };
                waitingForResumeRef.current = false;
              }

              setMessages((prev) =>
                upsertStreamingAssistant(prev, assistantMsgId, currentAssistantMsg)
              );

              if (currentEvent === 'token') {
                await yieldToRenderer();
              }
            } catch (jsonErr) {
              console.error('Error parsing SSE data line:', jsonErr);
            }
          }
        }
      }
    } catch (err: unknown) {
      if ((err as { name?: string })?.name === 'AbortError') {
        currentAssistantMsg = {
          ...currentAssistantMsg,
          thought: currentAssistantMsg.pendingApproval ? currentAssistantMsg.thought : undefined,
          finalized: !currentAssistantMsg.pendingApproval,
        };
        setMessages((prev) =>
          prev.map((m) => (m.id === assistantMsgId ? { ...currentAssistantMsg } : m))
        );
      } else {
        setMessages((prev) =>
          prev.map((m) =>
            m.id === assistantMsgId
              ? {
                  ...m,
                  content: m.content + `\n\nExecution error: ${getErrorMessage(err)}`,
                  thought: undefined,
                }
              : m
          )
        );
      }
    } finally {
      activeStreamAbortController.current = null;
      if (currentAssistantMsg.pendingApproval) {
        waitingForResumeRef.current = true;
      } else {
        waitingForResumeRef.current = false;
        loadingRef.current = false;
        setLoading(false);
      }
    }
  };

  const handlePromptChip = (chipText: string) => {
    setInput(chipText);
    inputRef.current?.focus();
  };

  const handleCopy = (text: string, idx: number) => {
    navigator.clipboard.writeText(text);
    setCopiedIdx(idx);
    success(tCommon('copied', 'Copied to Clipboard'), 'Response text copied.');
    setTimeout(() => setCopiedIdx(null), 2000);
  };

  const handleCopyConvId = () => {
    if (!activeConvID) return;
    navigator.clipboard.writeText(activeConvID);
    setCopiedConvId(true);
    success(t('actions.copyId'), activeConvID);
    setTimeout(() => setCopiedConvId(false), 2000);
  };

  const toggleTrace = (msgId: string) => {
    setExpandedTrace((prev) => ({ ...prev, [msgId]: !prev[msgId] }));
  };

  const activeAgent = agents.find((a) => a.agent_id === activeAgentID) || agents[0];
  const activeConv = conversations.find((c) => c.id === activeConvID);

  const promptChips = [
    t('prompts.diagnostics'),
    t('prompts.workspace'),
    t('prompts.decompose'),
    t('prompts.architecture'),
  ];

  return (
    <div className="relative flex flex-col h-[calc(100vh-5rem)] overflow-hidden">
      <BlobBackdrop />

      <PageContainer maxWidth="wide" className="flex-1 flex flex-col py-2 min-h-0 overflow-hidden">
        <ChatHeader
          agent={activeAgent}
          viewMode={viewMode}
          onBackToSessions={handleBackToSessions}
          onNewSession={handleNewChat}
        />

        {/* Mode 1: Sessions Hub Table View (Default when accessing Chat) */}
        {viewMode === 'sessions' ? (
          <div className="flex-1 overflow-y-auto min-h-0 mt-2 pr-1">
            <ChatSessionsTable
              conversations={conversations}
              agents={agents}
              search={tableSearch}
              onSearchChange={setTableSearch}
              selectedAgentID={tableAgentID}
              onSelectAgentID={setTableAgentID}
              selectedChannel={tableChannel}
              onSelectChannel={setTableChannel}
              pinnedOnly={tablePinnedOnly}
              onTogglePinnedOnly={setTablePinnedOnly}
              onViewSession={handleViewSession}
              onRenameSession={(conv) => setEditingConv(conv)}
              onTogglePin={handleTogglePin}
              onDeleteSession={(convID) => setDeletingConvId(convID)}
              onNewSession={handleNewChat}
            />
          </div>
        ) : (
          /* Mode 2: Active Chat Conversation Canvas (Full width, no sidebar) */
          <div className="flex-1 flex flex-col max-w-4xl w-full mx-auto mt-1 min-h-0 overflow-hidden">
            <Card className="flex-1 flex flex-col p-4 sm:p-5 border border-onyx/10 h-full bg-canvas/95 min-h-0 shadow-sm overflow-hidden rounded-[24px]">
              {/* Top Navigation Bar inside Chat Feed */}
              <div className="flex items-center justify-between pb-3 border-b border-onyx/10 mb-2 shrink-0">
                <div className="flex items-center gap-3 min-w-0 flex-1">
                  <button
                    type="button"
                    onClick={handleBackToSessions}
                    className="p-2 rounded-full hover:bg-soft-meadow text-slate hover:text-deep-ink transition-colors cursor-pointer shrink-0"
                    title={t('backToSessions')}
                    aria-label={t('backToSessions')}
                  >
                    <ArrowLeft className="w-5 h-5" />
                  </button>

                  <div className="w-9 h-9 rounded-full bg-deep-ink text-hi-yellow flex items-center justify-center border border-deep-ink shadow-xs shrink-0">
                    {activeAgent?.avatar_icon === 'sparkles' || activeAgent?.is_system ? (
                      <Sparkles className="w-4 h-4" />
                    ) : (
                      <Bot className="w-4 h-4" />
                    )}
                  </div>

                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      {/* Agent Selector Dropdown */}
                      <select
                        value={activeAgentID}
                        onChange={(e) => {
                          setActiveAgentID(e.target.value);
                          onSelectAgentID?.(e.target.value);
                        }}
                        className="font-serif text-heading-sm font-semibold text-deep-ink bg-transparent hover:bg-soft-meadow/80 px-2 py-0.5 rounded-lg border-0 cursor-pointer focus:outline-none focus:ring-1 focus:ring-deep-ink truncate"
                      >
                        {agents.map((a) => (
                          <option key={a.agent_id} value={a.agent_id}>
                            {a.name} {a.is_system ? '(System)' : ''}
                          </option>
                        ))}
                      </select>
                      {activeAgent?.is_system && <Badge variant="accent">{t('root')}</Badge>}
                    </div>

                    <div className="flex items-center gap-2 text-caption text-slate font-mono text-[11px] mt-0.5">
                      {activeConv ? (
                        <div className="flex items-center gap-1.5 truncate">
                          <span className="font-sans font-medium text-deep-ink truncate max-w-[200px] sm:max-w-[280px]">
                            {activeConv.title}
                          </span>
                          <button
                            type="button"
                            onClick={() => setEditingConv(activeConv)}
                            className="p-1 rounded-full text-slate hover:text-deep-ink hover:bg-soft-meadow transition-colors"
                            title={t('renameSession')}
                          >
                            <Edit3 className="w-3 h-3" />
                          </button>
                        </div>
                      ) : null}

                      {activeConvID && (
                        <>
                          <span>•</span>
                          <button
                            type="button"
                            onClick={handleCopyConvId}
                            className="flex items-center gap-1 text-[10px] text-slate hover:text-deep-ink bg-onyx/5 px-2 py-0.5 rounded-full transition-colors font-mono"
                            title={t('actions.copyId')}
                          >
                            {copiedConvId ? (
                              <Check className="w-3 h-3 text-emerald-600" />
                            ) : (
                              <Copy className="w-3 h-3" />
                            )}
                            <span className="truncate max-w-[90px]">{activeConvID}</span>
                          </button>
                        </>
                      )}
                    </div>
                  </div>
                </div>

                <div className="flex items-center gap-1.5 shrink-0">
                  {activeConv && (
                    <button
                      type="button"
                      onClick={() => handleTogglePin(activeConv.id, !!activeConv.is_pinned)}
                      className={`p-2 rounded-full transition-colors ${
                        activeConv.is_pinned
                          ? 'text-deep-ink bg-hi-yellow hover:bg-hi-yellow/80'
                          : 'text-slate hover:text-deep-ink hover:bg-soft-meadow'
                      }`}
                      title={activeConv.is_pinned ? t('actions.unpin') : t('actions.pin')}
                    >
                      <Pin className={`w-4 h-4 ${activeConv.is_pinned ? 'fill-current' : ''}`} />
                    </button>
                  )}

                  {activeConvID && (
                    <button
                      type="button"
                      onClick={() => setDeletingConvId(activeConvID)}
                      className="p-2 rounded-full text-slate hover:text-red-600 hover:bg-red-50 transition-colors"
                      title={t('deleteSession')}
                    >
                      <Trash2 className="w-4 h-4" />
                    </button>
                  )}

                  <Button
                    variant="ghost"
                    size="sm"
                    icon={<Plus className="w-3.5 h-3.5" />}
                    onClick={handleNewChat}
                    className="font-medium ml-1"
                  >
                    {t('newShort')}
                  </Button>
                </div>
              </div>

              {/* Message Timeline */}
              <div className="flex-1 min-h-0 overflow-hidden flex flex-col">
                <MessageTimeline
                  messages={messages}
                  loading={loading}
                  agentName={activeAgent?.name || t('defaultAgent')}
                  prompts={promptChips}
                  copiedIndex={copiedIdx}
                  expandedTraces={expandedTrace}
                  traceTabs={activeTab}
                  endRef={messagesEndRef}
                  containerRef={messagesContainerRef}
                  onScroll={handleScroll}
                  onPrompt={handlePromptChip}
                  onCopy={handleCopy}
                  onToggleTrace={toggleTrace}
                  onTraceTabChange={(messageID, tab) =>
                    setActiveTab((previous) => ({ ...previous, [messageID]: tab }))
                  }
                />
              </div>

              {/* Input Area with Autocomplete, Attachments, and Voice */}
              <div className="relative mt-2 shrink-0">
                {showScrollBottom && (
                  <button
                    type="button"
                    onClick={handleScrollToBottomClick}
                    className="absolute -top-11 right-4 z-20 flex items-center gap-1.5 px-3.5 py-1.5 rounded-full bg-soft-meadow dark:bg-charcoal hover:bg-canvas dark:hover:bg-charcoal/80 text-deep-ink dark:text-white text-caption shadow-md border border-onyx/15 dark:border-white/15 backdrop-blur-md transition-all animate-in fade-in slide-in-from-bottom-2 duration-200 cursor-pointer"
                    title={t('scrollToBottom', 'Scroll to bottom')}
                  >
                    <ChevronDown className="w-3.5 h-3.5 text-slate dark:text-hi-yellow" />
                    <span className="text-[11px] font-semibold">
                      {t('scrollToBottom', 'Scroll to bottom')}
                    </span>
                  </button>
                )}

                <ChatComposer
                  value={input}
                  loading={loading}
                  inputRef={inputRef}
                  skills={skills}
                  workspaceFiles={workspaceFiles}
                  pendingAttachments={pendingAttachments}
                  onClearPendingAttachments={() => setPendingAttachments([])}
                  onChange={setInput}
                  onSubmit={handleSend}
                  onCancelLoading={handleCancelStreaming}
                />
              </div>
            </Card>
          </div>
        )}
      </PageContainer>

      {/* Rename Conversation Modal */}
      <RenameSessionModal
        isOpen={!!editingConv}
        onClose={() => setEditingConv(null)}
        initialTitle={editingConv?.title || ''}
        onSave={handleSaveRename}
      />

      {/* Delete Conversation Confirmation Modal */}
      <ConfirmModal
        isOpen={!!deletingConvId}
        onClose={() => setDeletingConvId(null)}
        onConfirm={handleConfirmDeleteConv}
        title={t('deleteSession', 'Delete Conversation Session')}
        description={t(
          'deleteConfirm',
          'Are you sure you want to permanently clear this conversation session history?'
        )}
        confirmLabel={t('deleteSession')}
        variant="danger"
      />
    </div>
  );
}
