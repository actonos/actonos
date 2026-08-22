import { useState, useEffect, useRef, type FormEvent } from 'react';
import { getErrorMessage } from '@/lib/errors';
import { useTranslation } from 'react-i18next';
import { PageContainer } from '@/components/layout/PageContainer';
import { ChatHeader } from '@/components/features/chat/ChatHeader';
import { ChatComposer } from '@/components/features/chat/ChatComposer';
import { MessageTimeline } from '@/components/features/chat/MessageTimeline';
import { ChatSessionRail } from '@/components/features/chat/ChatSessionRail';
import { ChatSessionsTable } from '@/components/features/chat/ChatSessionsTable';
import { RenameSessionModal } from '@/components/features/chat/RenameSessionModal';
import { BlobBackdrop } from '@/components/ui/BlobBackdrop';
import { Card } from '@/components/ui/Card';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { ConfirmModal } from '@/components/ui/ConfirmModal';
import { useToast } from '@/components/ui/Toast';
import { useRealtime } from '@/components/providers/RealtimeProvider';
import {
  Bot,
  Sparkles,
  Trash2,
  Plus,
  ArrowLeft,
  ChevronDown,
} from 'lucide-react';
import { api } from '@/lib/api';
import type { AgentManifest, ConversationItem } from '@/lib/types';
import type { NavTab } from '@/components/layout/Sidebar';
import {
  type ChatMessage,
  type ToolCallTrace,
} from './chatTypes';

export interface ChatPageProps {
  selectedAgentID?: string;
  onSelectAgentID?: (id: string) => void;
  onNavigateTab?: (tab: NavTab) => void;
}

export function ChatPage({ selectedAgentID, onSelectAgentID }: ChatPageProps) {
  const { t } = useTranslation('chat');
  const { t: tCommon } = useTranslation('common');
  const { success, error, info } = useToast();

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
  const [activeTab, setActiveTab] = useState<Record<string, 'traces' | 'audit'>>({});
  const [expandedTrace, setExpandedTrace] = useState<Record<string, boolean>>({});

  // Table Hub Filters & Search
  const [tableSearch, setTableSearch] = useState('');
  const [tableAgentID, setTableAgentID] = useState('all');
  const [tableChannel, setTableChannel] = useState('all');
  const [tablePinnedOnly, setTablePinnedOnly] = useState(false);

  // Rail Search & Filter
  const [sessionSearch, setSessionSearch] = useState('');
  const [sessionFilterScope, setSessionFilterScope] = useState<'all' | 'agent'>('all');
  const [sessionsOpen, setSessionsOpen] = useState(false);

  // Modals & Renaming
  const [deletingConvId, setDeletingConvId] = useState<string | null>(null);
  const [editingConv, setEditingConv] = useState<ConversationItem | null>(null);

  const messagesEndRef = useRef<HTMLDivElement>(null);
  const messagesContainerRef = useRef<HTMLDivElement>(null);
  const isAutoScrollEnabled = useRef(true);
  const [showScrollBottom, setShowScrollBottom] = useState(false);
  const inputRef = useRef<HTMLTextAreaElement>(null);

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

  const selectConversation = async (convID: string) => {
    setActiveConvID(convID);
    localStorage.setItem('actonos_active_conv_id', convID);
    try {
      const res = await api.getConversation(convID);
      if (res.conversation?.agent_id) {
        setActiveAgentID(res.conversation.agent_id);
        onSelectAgentID?.(res.conversation.agent_id);
      }
      if (res.messages) {
        setMessages(
          res.messages.map((m) => {
            let toolCalls: ToolCallTrace[] = [];
            if (m.tool_calls_json && m.tool_calls_json !== 'null' && m.tool_calls_json !== '[]') {
              try {
                const parsed = JSON.parse(m.tool_calls_json);
                if (Array.isArray(parsed)) {
                  toolCalls = parsed.map((rawCall: unknown) => {
                    const tc = typeof rawCall === 'object' && rawCall !== null
                      ? rawCall as Record<string, unknown>
                      : {};
                    const fn = typeof tc.function === 'object' && tc.function !== null
                      ? tc.function as Record<string, unknown>
                      : {};
                    return {
                      tool: typeof fn.name === 'string'
                        ? fn.name
                        : typeof tc.name === 'string' ? tc.name : 'native_tool',
                      args: fn.arguments,
                      status: 'success',
                    };
                  });
                }
              } catch { }
            }
            return {
              id: m.id,
              role: m.role === 'user' || m.role === 'assistant' ? m.role : 'system',
              content: m.content,
              timestamp: new Date(m.created_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }),
              toolCalls: toolCalls.length > 0 ? toolCalls : undefined,
            };
          })
        );
      } else {
        setMessages([]);
      }
    } catch (err) {
      error('Failed to load messages', getErrorMessage(err));
    }
  };

  const handleViewSession = (convID: string) => {
    selectConversation(convID);
    setViewMode('chat');
  };

  const handleNewChat = () => {
    setActiveConvID(null);
    localStorage.removeItem('actonos_active_conv_id');
    setMessages([]);
    setViewMode('chat');
    info(t('newSession', 'New Chat Session'), 'Type a message to start a real-time streamed session.');
    setTimeout(() => {
      inputRef.current?.focus();
    }, 100);
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
        return new Date(b.updated_at || b.created_at).getTime() - new Date(a.updated_at || a.created_at).getTime();
      });
    });

    try {
      await api.togglePinConversation(convID, nextPinned);
      success(
        nextPinned ? t('actions.pin') : t('actions.unpin'),
        nextPinned ? 'Session pinned to top.' : 'Session unpinned.'
      );
    } catch (err) {
      // Revert if failed
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
        if (remaining.length > 0) {
          selectConversation(remaining[0].id);
        } else {
          setActiveConvID(null);
          setMessages([]);
          setViewMode('sessions');
        }
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
  const loadingRef = useRef(loading);
  loadingRef.current = loading;
  const activeConvIDRef = useRef(activeConvID);
  activeConvIDRef.current = activeConvID;

  useEffect(() => {
    loadAgents();
    loadConversations();
  }, []);

  useEffect(() => {
    if (selectedAgentID) {
      setActiveAgentID(selectedAgentID);
    }
  }, [selectedAgentID]);

  // Real-time synchronization: pull incoming messages and update conversations list
  useEffect(() => {
    let cancelled = false;

    const syncRecentMessages = async () => {
      if (loadingRef.current) return;
      const currentID = activeConvIDRef.current;
      if (!currentID) return;

      try {
        const res = await api.getConversation(currentID);
        if (cancelled || !res.messages) return;

        const newFormatted: ChatMessage[] = res.messages.map((m) => {
          let toolCalls: ToolCallTrace[] = [];
          if (m.tool_calls_json && m.tool_calls_json !== 'null' && m.tool_calls_json !== '[]') {
            try {
              const parsed = JSON.parse(m.tool_calls_json);
              if (Array.isArray(parsed)) {
                toolCalls = parsed.map((rawCall: unknown) => {
                  const tc = typeof rawCall === 'object' && rawCall !== null ? rawCall as Record<string, unknown> : {};
                  const fn = typeof tc.function === 'object' && tc.function !== null ? tc.function as Record<string, unknown> : {};
                  return {
                    tool: typeof fn.name === 'string' ? fn.name : typeof tc.name === 'string' ? tc.name : 'native_tool',
                    args: fn.arguments,
                    status: 'success',
                  };
                });
              }
            } catch { }
          }
          return {
            id: m.id,
            role: m.role === 'user' || m.role === 'assistant' ? m.role : 'system',
            content: m.content,
            timestamp: new Date(m.created_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }),
            toolCalls: toolCalls.length > 0 ? toolCalls : undefined,
          };
        });

        setMessages((prev) => {
          if (newFormatted.length < prev.length) {
            return prev;
          }
          if (prev.length !== newFormatted.length || (prev.length > 0 && prev[prev.length - 1]?.content !== newFormatted[newFormatted.length - 1]?.content)) {
            return newFormatted;
          }
          return prev;
        });
      } catch { }
    };

    const syncConversationsList = async () => {
      try {
        const res = await api.listConversations();
        if (cancelled || !res.conversations) return;
        setConversations(res.conversations);
      } catch { }
    };

    syncRecentMessages();
    syncConversationsList();

    const interval = setInterval(() => {
      syncRecentMessages();
      syncConversationsList();
    }, 2500);

    return () => {
      cancelled = true;
      clearInterval(interval);
    };
  }, [snapshot?.timestamp, activeConvID]);

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

  const handleSend = async (e?: FormEvent) => {
    if (e) e.preventDefault();
    if (!input.trim() || !activeAgentID || loading) return;

    const userMsg = input.trim();
    setInput('');
    const now = new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });

    const userMsgObj: ChatMessage = {
      id: 'msg_' + Date.now(),
      role: 'user',
      content: userMsg,
      timestamp: now,
    };

    const assistantMsgId = 'msg_' + (Date.now() + 1);
    let currentAssistantMsg: ChatMessage = {
      id: assistantMsgId,
      role: 'assistant',
      content: '',
      timestamp: now,
      thought: `Deliberating with ${activeAgent?.name || 'Agent'}...`,
      segments: [],
      toolCalls: [],
      auditLogs: [],
      finalized: false,
    };

    setMessages((prev) => [...prev, userMsgObj, currentAssistantMsg]);
    setLoading(true);

    try {
      const response = await api.streamChat(activeAgentID, {
        conversation_id: activeConvID,
        message: userMsg,
      });

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

              // Update session ID and title
              if (parsed.conversation_id) {
                setActiveConvID(parsed.conversation_id);
                localStorage.setItem('actonos_active_conv_id', parsed.conversation_id);
                setConversations((prev) => {
                  const exists = prev.some((c) => c.id === parsed.conversation_id);
                  if (exists) {
                    return prev.map((c) =>
                      c.id === parsed.conversation_id && parsed.title
                        ? {
                            ...c,
                            title: parsed.title,
                            last_message: userMsg,
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
                        title: parsed.title || userMsg.slice(0, 35) + '...',
                        channel: 'web',
                        is_pinned: false,
                        message_count: 2,
                        last_message: userMsg,
                        created_at: new Date().toISOString(),
                        updated_at: new Date().toISOString(),
                      },
                      ...prev,
                    ];
                  }
                });
              }

              if (currentEvent === 'thought' && parsed.thought) {
                currentAssistantMsg = {
                  ...currentAssistantMsg,
                  thought: parsed.thought,
                };
              } else if (currentEvent === 'reasoning' && parsed.reasoning) {
                const segs = [...(currentAssistantMsg.segments || [])];
                const last = segs[segs.length - 1];
                if (last && last.type === 'reasoning') {
                  segs[segs.length - 1] = { ...last, text: last.text + parsed.reasoning };
                } else {
                  segs.push({ type: 'reasoning', text: parsed.reasoning });
                }
                currentAssistantMsg = {
                  ...currentAssistantMsg,
                  reasoning: (currentAssistantMsg.reasoning ?? '') + parsed.reasoning,
                  segments: segs,
                };
              } else if (currentEvent === 'token' && parsed.content) {
                const segs = [...(currentAssistantMsg.segments || [])];
                const last = segs[segs.length - 1];
                if (last && last.type === 'content') {
                  segs[segs.length - 1] = { ...last, text: last.text + parsed.content };
                } else {
                  segs.push({ type: 'content', text: parsed.content });
                }
                currentAssistantMsg = {
                  ...currentAssistantMsg,
                  content: currentAssistantMsg.content + parsed.content,
                  thought: undefined,
                  segments: segs,
                };
              } else if (currentEvent === 'token_reset') {
                const segs = (currentAssistantMsg.segments || []).filter((s) => s.type === 'reasoning');
                currentAssistantMsg = {
                  ...currentAssistantMsg,
                  content: '',
                  segments: segs,
                };
              } else if (currentEvent === 'tool_call') {
                const newToolCall: ToolCallTrace = {
                  tool: parsed.tool,
                  args: parsed.args,
                  status: 'running',
                };
                currentAssistantMsg = {
                  ...currentAssistantMsg,
                  toolCalls: [...(currentAssistantMsg.toolCalls || []), newToolCall],
                };
              } else if (currentEvent === 'tool_result') {
                currentAssistantMsg = {
                  ...currentAssistantMsg,
                  toolCalls: (currentAssistantMsg.toolCalls || []).map((tc) =>
                    tc.tool === parsed.tool
                      ? { ...tc, result: parsed.result, status: parsed.status, latency_ms: parsed.latency_ms }
                      : tc
                  ),
                };
              } else if (currentEvent === 'audit' && parsed.audit_log) {
                currentAssistantMsg = {
                  ...currentAssistantMsg,
                  auditLogs: [...(currentAssistantMsg.auditLogs || []), parsed.audit_log],
                };
              } else if (currentEvent === 'done') {
                currentAssistantMsg = {
                  ...currentAssistantMsg,
                  content: currentAssistantMsg.content || parsed.content || (currentAssistantMsg.toolCalls?.length ? 'Completed operations successfully.' : ''),
                  model: parsed.model,
                  tokens_used: parsed.tokens_used,
                  thought: undefined,
                  finalized: true,
                };
              } else if (currentEvent === 'error') {
                currentAssistantMsg = {
                  ...currentAssistantMsg,
                  content: currentAssistantMsg.content + `\n\nError: ${parsed.error}`,
                  thought: undefined,
                  finalized: true,
                };
              }

              setMessages((prev) =>
                prev.map((m) => (m.id === assistantMsgId ? { ...currentAssistantMsg } : m))
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
    } catch (err) {
      setMessages((prev) =>
        prev.map((m) =>
          m.id === assistantMsgId
            ? { ...m, content: m.content + `\n\nExecution error: ${getErrorMessage(err)}`, thought: undefined }
            : m
        )
      );
    } finally {
      setLoading(false);
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

  const toggleTrace = (msgId: string) => {
    setExpandedTrace((prev) => ({ ...prev, [msgId]: !prev[msgId] }));
  };

  const activeAgent = agents.find((a) => a.agent_id === activeAgentID) || agents[0];
  const activeConv = conversations.find((c) => c.id === activeConvID);

  // Filter conversations for the side rail inside Chat view
  const railFilteredConversations = conversations.filter((c) => {
    if (sessionFilterScope === 'agent' && c.agent_id !== activeAgentID) {
      return false;
    }
    if (sessionSearch.trim()) {
      const q = sessionSearch.toLowerCase();
      return c.title.toLowerCase().includes(q) || (c.last_message && c.last_message.toLowerCase().includes(q));
    }
    return true;
  });

  const promptChips = [
    t('prompts.diagnostics'),
    t('prompts.workspace'),
    t('prompts.decompose'),
    t('prompts.architecture'),
  ];

  return (
    <div className="relative flex flex-col min-h-[calc(100dvh-64px)]">
      <BlobBackdrop />

      <PageContainer maxWidth="wide" className="flex-1 flex flex-col py-4 min-h-0">
        <ChatHeader
          agent={activeAgent}
          viewMode={viewMode}
          onOpenSessions={() => setSessionsOpen(true)}
          onBackToSessions={() => setViewMode('sessions')}
          onNewSession={handleNewChat}
        />

        {/* Mode 1: Sessions Hub Table View (Default when accessing Chat) */}
        {viewMode === 'sessions' ? (
          <div className="flex-1 mt-2">
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
          /* Mode 2: Active Chat Conversation Canvas */
          <div className="grid min-h-0 flex-1 grid-cols-1 gap-6 lg:grid-cols-12 mt-2">
            <ChatSessionRail
              agents={agents}
              conversations={railFilteredConversations}
              activeAgentID={activeAgentID}
              activeConversationID={activeConvID}
              search={sessionSearch}
              scope={sessionFilterScope}
              onAgentChange={(agentID) => {
                setActiveAgentID(agentID);
                onSelectAgentID?.(agentID);
              }}
              onConversationSelect={selectConversation}
              onConversationDelete={setDeletingConvId}
              onSearchChange={setSessionSearch}
              onScopeChange={setSessionFilterScope}
              onNew={handleNewChat}
              open={sessionsOpen}
              onClose={() => setSessionsOpen(false)}
            />

            {/* Right Column: Chat History & Input Canvas (8/9 Cols) */}
            <Card className="lg:col-span-8 xl:col-span-9 flex flex-col justify-between p-4 sm:p-6 border border-onyx/10 h-full bg-canvas/80 min-h-0 shadow-xs overflow-hidden">
              {/* Top Bar inside Chat Feed */}
              <div className="flex items-center justify-between pb-3 border-b border-soft-meadow">
                <div className="flex items-center gap-3 truncate">
                  <button
                    type="button"
                    onClick={() => setViewMode('sessions')}
                    className="p-2 rounded-full hover:bg-soft-meadow text-slate hover:text-deep-ink transition-colors cursor-pointer"
                    title={t('backToSessions')}
                  >
                    <ArrowLeft className="w-5 h-5" />
                  </button>

                  <div className="w-10 h-10 rounded-full bg-deep-ink text-hi-yellow flex items-center justify-center border border-deep-ink shadow-xs shrink-0">
                    {activeAgent?.avatar_icon === 'sparkles' || activeAgent?.is_system ? (
                      <Sparkles className="w-5 h-5" />
                    ) : (
                      <Bot className="w-5 h-5" />
                    )}
                  </div>
                  <div className="truncate">
                    <h3 className="font-serif text-heading-sm text-deep-ink flex items-center gap-2 truncate">
                      <span className="truncate">{activeAgent?.name || 'Nova (Root System)'}</span>
                      {activeAgent?.is_system && <Badge variant="accent">{t('root')}</Badge>}
                    </h3>
                    <div className="flex items-center gap-2 text-caption text-slate font-mono text-[11px] truncate">
                      <span className="truncate">{t('model', { name: activeAgent?.model_config.primary_model || 'claude-sonnet-4-6' })}</span>
                      {activeConv && (
                        <>
                          <span>•</span>
                          <span className="truncate max-w-[220px] text-deep-ink font-sans font-medium">
                            {activeConv.title}
                          </span>
                        </>
                      )}
                    </div>
                  </div>
                </div>

                <div className="flex items-center gap-2 shrink-0">
                  {activeConvID && (
                    <Button
                      variant="ghost"
                      size="sm"
                      icon={<Trash2 className="w-3.5 h-3.5 text-red-500" />}
                      onClick={() => setDeletingConvId(activeConvID)}
                      title={t('clearConversation')}
                    />
                  )}
                  <Button
                    variant="primary"
                    size="sm"
                    icon={<Plus className="w-3.5 h-3.5" />}
                    onClick={handleNewChat}
                    className="lg:hidden text-caption px-3"
                  >
                    {t('newShort')}
                  </Button>
                </div>
              </div>

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
                onTraceTabChange={(messageID, tab) => setActiveTab((previous) => ({ ...previous, [messageID]: tab }))}
              />

              <div className="relative">
                {showScrollBottom && (
                  <button
                    type="button"
                    onClick={handleScrollToBottomClick}
                    className="absolute -top-11 right-4 z-20 flex items-center gap-1.5 px-3.5 py-1.5 rounded-full bg-soft-meadow dark:bg-charcoal hover:bg-canvas dark:hover:bg-charcoal/80 text-deep-ink dark:text-white text-caption shadow-md border border-onyx/15 dark:border-white/15 backdrop-blur-md transition-all animate-in fade-in slide-in-from-bottom-2 duration-200 cursor-pointer"
                    title={t('scrollToBottom', 'Scroll to bottom')}
                  >
                    <ChevronDown className="w-3.5 h-3.5 text-slate dark:text-hi-yellow" />
                    <span className="text-[11px] font-semibold">{t('scrollToBottom', 'Scroll to bottom')}</span>
                  </button>
                )}

                <ChatComposer
                  value={input}
                  loading={loading}
                  inputRef={inputRef}
                  onChange={setInput}
                  onSubmit={handleSend}
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
        description={t('deleteConfirm', 'Are you sure you want to permanently clear this conversation session history?')}
        confirmLabel={t('deleteSession')}
        variant="danger"
      />
    </div>
  );
}

