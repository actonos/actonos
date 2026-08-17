import { useState, useEffect, useRef, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { PageContainer } from '@/components/layout/PageContainer';
import { BlobBackdrop } from '@/components/ui/BlobBackdrop';
import { Card } from '@/components/ui/Card';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { ConfirmModal } from '@/components/ui/Modal';
import { useToast } from '@/components/ui/Toast';
import {
  Bot,
  Send,
  Sparkles,
  Zap,
  Trash2,
  Copy,
  Check,
  Terminal,
  ChevronDown,
  ChevronRight,
  ShieldCheck,
  Plus,
  MessageSquare,
  Clock,
  Activity,
  Cpu,
  FileCode,
  Search,
  Edit3,
  X,
} from 'lucide-react';
import { api } from '@/lib/api';
import type { AgentManifest, ConversationItem } from '@/lib/types';
import type { NavTab } from '@/components/layout/Sidebar';
import { MarkdownContent } from '@/components/chat/MarkdownContent';

export interface ChatPageProps {
  selectedAgentID?: string;
  onSelectAgentID?: (id: string) => void;
  onNavigateTab?: (tab: NavTab) => void;
}

export interface AuditLogItem {
  timestamp: string;
  agent_id: string;
  action: string;
  tool_name?: string;
  parameters?: any;
  status: string;
  verification: string;
  duration_ms: number;
}

export interface ToolCallTrace {
  tool: string;
  args?: any;
  result?: string;
  status?: string;
  latency_ms?: number;
}

interface ChatMessage {
  id: string;
  role: 'user' | 'assistant' | 'system';
  content: string;
  timestamp: string;
  model?: string;
  tokens_used?: number;
  thought?: string;
  toolCalls?: ToolCallTrace[];
  auditLogs?: AuditLogItem[];
}

function formatRelativeTime(dateStr?: string): string {
  if (!dateStr) return '';
  const date = new Date(dateStr);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffSec = Math.floor(diffMs / 1000);
  const diffMin = Math.floor(diffSec / 60);
  const diffHour = Math.floor(diffMin / 60);
  const diffDay = Math.floor(diffHour / 24);

  if (diffSec < 60) return 'Just now';
  if (diffMin < 60) return `${diffMin}m ago`;
  if (diffHour < 24) return `${diffHour}h ago`;
  if (diffDay === 1) return 'Yesterday';
  if (diffDay < 7) return `${diffDay}d ago`;
  return date.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
}

export function ChatPage({ selectedAgentID, onSelectAgentID, onNavigateTab }: ChatPageProps) {
  const { t } = useTranslation('chat');
  const { t: tCommon } = useTranslation('common');
  const { success, error, info } = useToast();

  const [agents, setAgents] = useState<AgentManifest[]>([]);
  const [activeAgentID, setActiveAgentID] = useState<string>(selectedAgentID || 'agent_system_core');
  const [agentDropdownOpen, setAgentDropdownOpen] = useState(false);

  // Conversations & History
  const [conversations, setConversations] = useState<ConversationItem[]>([]);
  const [activeConvID, setActiveConvID] = useState<string | null>(null);
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [input, setInput] = useState('');
  const [loading, setLoading] = useState(false);
  const [copiedIdx, setCopiedIdx] = useState<number | null>(null);
  const [activeTab, setActiveTab] = useState<Record<string, 'traces' | 'audit'>>({});
  const [expandedTrace, setExpandedTrace] = useState<Record<string, boolean>>({});

  // Search & Filter
  const [sessionSearch, setSessionSearch] = useState('');
  const [sessionFilterScope, setSessionFilterScope] = useState<'all' | 'agent'>('all');

  // Modals & Renaming
  const [deletingConvId, setDeletingConvId] = useState<string | null>(null);
  const [editingConvId, setEditingConvId] = useState<string | null>(null);
  const [editingConvTitle, setEditingConvTitle] = useState('');

  const messagesEndRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const dropdownRef = useRef<HTMLDivElement>(null);

  // Close dropdown on outside click
  useEffect(() => {
    const handleClickOutside = (e: MouseEvent) => {
      if (dropdownRef.current && !dropdownRef.current.contains(e.target as Node)) {
        setAgentDropdownOpen(false);
      }
    };
    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, []);

  // Load agents and conversations
  const loadAgents = async () => {
    try {
      const res = await api.listAgents();
      setAgents(res.agents || []);
      if (!activeAgentID && res.agents?.length > 0) {
        setActiveAgentID(res.agents[0].agent_id);
      }
    } catch (err: any) {
      error('Failed to load agents', err.message);
    }
  };

  const loadConversations = async () => {
    try {
      const res = await api.listConversations();
      const list = res.conversations || [];
      setConversations(list);
      if (list.length > 0 && !activeConvID) {
        selectConversation(list[0].id);
      }
    } catch (err: any) {
      error('Failed to load conversations', err.message);
    }
  };

  const selectConversation = async (convID: string) => {
    setActiveConvID(convID);
    try {
      const res = await api.getConversation(convID);
      if (res.conversation?.agent_id) {
        setActiveAgentID(res.conversation.agent_id);
        onSelectAgentID?.(res.conversation.agent_id);
      }
      if (res.messages) {
        setMessages(
          res.messages.map((m: any) => {
            let toolCalls: ToolCallTrace[] = [];
            if (m.tool_calls_json && m.tool_calls_json !== 'null' && m.tool_calls_json !== '[]') {
              try {
                const parsed = JSON.parse(m.tool_calls_json);
                if (Array.isArray(parsed)) {
                  toolCalls = parsed.map((tc: any) => ({
                    tool: tc.function?.name || tc.name || 'native_tool',
                    args: tc.function?.arguments,
                    status: 'success',
                  }));
                }
              } catch (_) {}
            }
            return {
              id: m.id,
              role: m.role as any,
              content: m.content,
              timestamp: new Date(m.created_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }),
              toolCalls: toolCalls.length > 0 ? toolCalls : undefined,
            };
          })
        );
      } else {
        setMessages([]);
      }
    } catch (err: any) {
      error('Failed to load messages', err.message);
    }
  };

  const handleNewChat = () => {
    setActiveConvID(null);
    setMessages([]);
    info(t('newSession', 'New Chat Session'), 'Type a message to start a real-time streamed session.');
    inputRef.current?.focus();
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
        }
      }
      setDeletingConvId(null);
    } catch (err: any) {
      error('Failed to delete session', err.message);
    }
  };

  const handleSaveRename = async (convID: string) => {
    if (!editingConvTitle.trim()) {
      setEditingConvId(null);
      return;
    }
    try {
      await api.updateConversationTitle(convID, editingConvTitle.trim());
      setConversations((prev) =>
        prev.map((c) => (c.id === convID ? { ...c, title: editingConvTitle.trim() } : c))
      );
      success('Title Updated', 'Session renamed.');
    } catch (err: any) {
      error('Failed to update title', err.message);
    } finally {
      setEditingConvId(null);
    }
  };

  useEffect(() => {
    loadAgents();
    loadConversations();
  }, []);

  useEffect(() => {
    if (selectedAgentID) {
      setActiveAgentID(selectedAgentID);
    }
  }, [selectedAgentID]);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages, loading]);

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
      toolCalls: [],
      auditLogs: [],
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

      while (true) {
        const { value, done } = await reader.read();
        if (done) break;

        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split('\n');
        buffer = lines.pop() || '';

        let currentEvent = 'token';

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
                setConversations((prev) => {
                  const exists = prev.some((c) => c.id === parsed.conversation_id);
                  if (exists) {
                    return prev.map((c) =>
                      c.id === parsed.conversation_id && parsed.title
                        ? { ...c, title: parsed.title, updated_at: new Date().toISOString(), message_count: (c.message_count || 0) + 2 }
                        : c
                    );
                  } else {
                    return [
                      {
                        id: parsed.conversation_id,
                        agent_id: activeAgentID,
                        title: parsed.title || userMsg.slice(0, 35) + '...',
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
              } else if (currentEvent === 'token' && parsed.content) {
                currentAssistantMsg = {
                  ...currentAssistantMsg,
                  content: currentAssistantMsg.content + parsed.content,
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
                  content: parsed.content || currentAssistantMsg.content,
                  model: parsed.model,
                  tokens_used: parsed.tokens_used,
                  thought: undefined,
                };
              } else if (currentEvent === 'error') {
                currentAssistantMsg = {
                  ...currentAssistantMsg,
                  content: currentAssistantMsg.content + `\n\n⚠️ Error: ${parsed.error}`,
                  thought: undefined,
                };
              }

              setMessages((prev) =>
                prev.map((m) => (m.id === assistantMsgId ? { ...currentAssistantMsg } : m))
              );
            } catch (jsonErr) {
              console.error('Error parsing SSE data line:', jsonErr);
            }
          }
        }
      }
    } catch (err: any) {
      setMessages((prev) =>
        prev.map((m) =>
          m.id === assistantMsgId
            ? { ...m, content: m.content + `\n\nExecution error: ${err.message}`, thought: undefined }
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

  // Filter conversations
  const filteredConversations = conversations.filter((c) => {
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
    'System Diagnostics & Performance',
    'List files in workspace',
    'Decompose goal into subtasks',
    'Explain ActonOS architecture',
  ];

  return (
    <div className="relative min-h-[calc(100vh-64px)] flex flex-col">
      <BlobBackdrop />

      <PageContainer className="flex-1 flex flex-col py-4">
        {/* Main 2-Column Chat Area */}
        <div className="grid grid-cols-1 lg:grid-cols-12 gap-6 flex-1 max-h-[calc(100vh-100px)]">
          {/* Left Column: Sessions & Custom Agent Switcher (4 Cols) */}
          <Card className="hidden lg:flex lg:col-span-4 flex-col p-4 border border-onyx/10 justify-between h-full bg-canvas/95 shadow-xs overflow-hidden">
            <div className="space-y-3.5 flex flex-col h-full overflow-hidden">
              {/* 1. Custom Agent Switcher Header */}
              <div className="relative" ref={dropdownRef}>
                <div className="flex items-center justify-between mb-1.5 px-1">
                  <span className="text-[11px] uppercase tracking-wider text-slate font-semibold">
                    {t('selectAgent', 'Active Agent')}
                  </span>
                  {activeAgent?.is_system && (
                    <span className="text-[10px] bg-hi-yellow/40 text-deep-ink px-2 py-0.5 rounded-full font-semibold">
                      ⭐ Root
                    </span>
                  )}
                </div>

                {/* Custom Agent Dropdown Button */}
                <button
                  type="button"
                  onClick={() => setAgentDropdownOpen(!agentDropdownOpen)}
                  className="w-full flex items-center justify-between p-2.5 px-3 rounded-[18px] bg-soft-meadow hover:bg-white border border-onyx/15 transition-all text-left cursor-pointer group"
                >
                  <div className="flex items-center gap-2.5 truncate">
                    <div className="w-8 h-8 rounded-full bg-deep-ink text-hi-yellow flex items-center justify-center border border-deep-ink shrink-0 shadow-xs">
                      {activeAgent?.avatar_icon === 'sparkles' || activeAgent?.is_system ? (
                        <Sparkles className="w-4 h-4" />
                      ) : (
                        <Bot className="w-4 h-4" />
                      )}
                    </div>
                    <div className="truncate">
                      <div className="font-semibold text-deep-ink text-body-sm leading-tight truncate">
                        {activeAgent?.name || 'Nova (Root System)'}
                      </div>
                      <div className="text-[11px] font-mono text-slate truncate">
                        {activeAgent?.model_config.primary_model || 'claude-3-7-sonnet'}
                      </div>
                    </div>
                  </div>
                  <ChevronDown className={`w-4 h-4 text-slate transition-transform duration-200 ${agentDropdownOpen ? 'rotate-180 text-deep-ink' : ''}`} />
                </button>

                {/* Agent Dropdown Menu */}
                {agentDropdownOpen && (
                  <div className="absolute top-full left-0 right-0 mt-1.5 p-1.5 bg-canvas rounded-[20px] border border-onyx/15 shadow-md z-30 space-y-1 max-h-60 overflow-y-auto">
                    {agents.map((ag) => {
                      const isSelected = ag.agent_id === activeAgentID;
                      return (
                        <button
                          key={ag.agent_id}
                          type="button"
                          onClick={() => {
                            setActiveAgentID(ag.agent_id);
                            onSelectAgentID?.(ag.agent_id);
                            setAgentDropdownOpen(false);
                          }}
                          className={`w-full flex items-center justify-between p-2.5 rounded-[14px] text-left transition-colors cursor-pointer ${
                            isSelected
                              ? 'bg-deep-ink text-white font-medium shadow-xs'
                              : 'text-deep-ink hover:bg-soft-meadow'
                          }`}
                        >
                          <div className="flex items-center gap-2.5 truncate">
                            <div className={`w-7 h-7 rounded-full flex items-center justify-center shrink-0 ${
                              isSelected ? 'bg-hi-yellow text-deep-ink' : 'bg-canvas text-deep-ink border border-onyx/10'
                            }`}>
                              {ag.avatar_icon === 'sparkles' || ag.is_system ? (
                                <Sparkles className="w-3.5 h-3.5" />
                              ) : (
                                <Bot className="w-3.5 h-3.5" />
                              )}
                            </div>
                            <div className="truncate">
                              <div className="text-body-sm font-semibold truncate leading-tight">
                                {ag.name}
                              </div>
                              <div className={`text-[10px] font-mono truncate ${isSelected ? 'text-white/80' : 'text-slate'}`}>
                                {ag.model_config.primary_model}
                              </div>
                            </div>
                          </div>
                          {ag.is_system && (
                            <span className={`text-[10px] px-1.5 py-0.5 rounded font-semibold ${
                              isSelected ? 'bg-white/20 text-hi-yellow' : 'bg-soft-meadow text-slate'
                            }`}>
                              Root
                            </span>
                          )}
                        </button>
                      );
                    })}
                  </div>
                )}
              </div>

              {/* 2. New Chat Action */}
              <Button
                variant="primary"
                size="sm"
                icon={<Plus className="w-4 h-4" />}
                onClick={handleNewChat}
                className="w-full justify-center text-caption py-2.5 font-semibold"
              >
                {t('newSession', '+ New Chat Session')}
              </Button>

              {/* 3. Search & Filter Bar */}
              <div className="space-y-1.5">
                <div className="relative">
                  <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-slate" />
                  <input
                    type="text"
                    placeholder={t('searchSessions', 'Search sessions...')}
                    value={sessionSearch}
                    onChange={(e) => setSessionSearch(e.target.value)}
                    className="w-full bg-soft-meadow text-deep-ink pl-8 pr-7 py-1.5 rounded-full border border-onyx/10 text-[12px] font-sans placeholder:text-slate focus:outline-none focus:border-deep-ink shadow-xs"
                  />
                  {sessionSearch && (
                    <button
                      onClick={() => setSessionSearch('')}
                      className="absolute right-2.5 top-1/2 -translate-y-1/2 text-slate hover:text-deep-ink"
                    >
                      <X className="w-3.5 h-3.5" />
                    </button>
                  )}
                </div>

                <div className="flex items-center gap-1 bg-soft-meadow p-1 rounded-full border border-onyx/10 text-[11px] font-sans">
                  <button
                    onClick={() => setSessionFilterScope('all')}
                    className={`flex-1 py-1 rounded-full transition-all text-center font-medium cursor-pointer ${
                      sessionFilterScope === 'all'
                        ? 'bg-deep-ink text-white shadow-xs font-semibold'
                        : 'text-slate hover:text-deep-ink'
                    }`}
                  >
                    {t('allAgents', 'All Sessions')} ({conversations.length})
                  </button>
                  <button
                    onClick={() => setSessionFilterScope('agent')}
                    className={`flex-1 py-1 rounded-full transition-all text-center font-medium cursor-pointer ${
                      sessionFilterScope === 'agent'
                        ? 'bg-deep-ink text-white shadow-xs font-semibold'
                        : 'text-slate hover:text-deep-ink'
                    }`}
                  >
                    {activeAgent?.name || 'Current'} (
                    {conversations.filter((c) => c.agent_id === activeAgentID).length}
                    )
                  </button>
                </div>
              </div>

              {/* 4. Scrollable Sessions List */}
              <div className="flex-1 overflow-y-auto space-y-1.5 pr-1 min-h-0">
                {filteredConversations.length === 0 ? (
                  <div className="p-6 text-center text-caption text-slate bg-soft-meadow rounded-[20px] border border-onyx/5">
                    <MessageSquare className="w-6 h-6 mx-auto mb-2 opacity-40 text-deep-ink" />
                    <p className="font-semibold text-deep-ink mb-0.5">
                      {sessionSearch ? t('noFilterMatches', 'No sessions match your search') : t('noSessions', 'No saved sessions yet')}
                    </p>
                    <p className="text-[11px] text-slate">
                      {sessionSearch ? 'Try a different keyword or switch filter.' : t('noSessionsDesc', 'Type a message to start your first session.')}
                    </p>
                  </div>
                ) : (
                  filteredConversations.map((conv) => {
                    const isActive = activeConvID === conv.id;
                    const convAgent = agents.find((a) => a.agent_id === conv.agent_id);
                    const isEditing = editingConvId === conv.id;

                    return (
                      <div
                        key={conv.id}
                        onClick={() => !isEditing && selectConversation(conv.id)}
                        className={`p-2.5 px-3 rounded-[16px] transition-all cursor-pointer border group relative ${
                          isActive
                            ? 'bg-deep-ink text-white border-deep-ink shadow-xs'
                            : 'bg-soft-meadow/70 text-deep-ink hover:bg-soft-meadow border-onyx/5 hover:border-onyx/15'
                        }`}
                      >
                        {/* Row 1: Title or Inline Edit Input */}
                        <div className="flex items-center justify-between gap-1.5 mb-1">
                          {isEditing ? (
                            <input
                              type="text"
                              value={editingConvTitle}
                              onChange={(e) => setEditingConvTitle(e.target.value)}
                              onBlur={() => handleSaveRename(conv.id)}
                              onKeyDown={(e) => {
                                if (e.key === 'Enter') handleSaveRename(conv.id);
                                if (e.key === 'Escape') setEditingConvId(null);
                              }}
                              autoFocus
                              className="w-full text-caption bg-canvas text-deep-ink px-2 py-0.5 rounded-lg border border-onyx/20 focus:outline-none"
                            />
                          ) : (
                            <div className="flex items-center gap-1.5 truncate">
                              <MessageSquare
                                className={`w-3.5 h-3.5 shrink-0 ${isActive ? 'text-hi-yellow' : 'text-slate'}`}
                              />
                              <span className="font-medium text-caption truncate max-w-[170px]">
                                {conv.title}
                              </span>
                            </div>
                          )}

                          {/* Hover action icons */}
                          {!isEditing && (
                            <div className="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                              <button
                                onClick={(e) => {
                                  e.stopPropagation();
                                  setEditingConvId(conv.id);
                                  setEditingConvTitle(conv.title);
                                }}
                                className={`p-1 rounded-full ${isActive ? 'text-white/70 hover:text-white' : 'text-slate hover:text-deep-ink'}`}
                                title={t('renameSession', 'Rename')}
                              >
                                <Edit3 className="w-3 h-3" />
                              </button>
                              <button
                                onClick={(e) => {
                                  e.stopPropagation();
                                  setDeletingConvId(conv.id);
                                }}
                                className={`p-1 rounded-full ${isActive ? 'text-white/70 hover:text-red-300' : 'text-slate hover:text-red-600'}`}
                                title={t('deleteSession', 'Delete')}
                              >
                                <Trash2 className="w-3 h-3" />
                              </button>
                            </div>
                          )}
                        </div>

                        {/* Row 2: Metadata (Agent badge, Msg count, Timestamp) */}
                        <div className={`flex items-center justify-between text-[10px] font-sans ${isActive ? 'text-white/70' : 'text-slate'}`}>
                          <div className="flex items-center gap-1.5 truncate max-w-[140px]">
                            <span className={`px-1.5 py-0.2 rounded font-semibold truncate ${
                              isActive ? 'bg-white/20 text-hi-yellow' : 'bg-canvas text-deep-ink border border-onyx/5'
                            }`}>
                              {convAgent?.name || conv.agent_id}
                            </span>
                            {conv.message_count !== undefined && conv.message_count > 0 && (
                              <span>💬 {conv.message_count}</span>
                            )}
                          </div>
                          <span className="font-mono text-[10px] shrink-0">
                            {formatRelativeTime(conv.updated_at || conv.created_at)}
                          </span>
                        </div>
                      </div>
                    );
                  })
                )}
              </div>

              {/* 5. Active Agent Footer Cockpit */}
              {activeAgent && (
                <div className="pt-2 border-t border-onyx/10 flex items-center justify-between text-caption shrink-0">
                  <div className="flex items-center gap-2 truncate">
                    <div className="w-6 h-6 rounded-full bg-deep-ink text-hi-yellow flex items-center justify-center shrink-0">
                      <Sparkles className="w-3 h-3" />
                    </div>
                    <div className="truncate">
                      <span className="font-semibold text-deep-ink truncate block text-[11px]">
                        {activeAgent.name}
                      </span>
                      <span className="text-[10px] text-slate font-mono block truncate">
                        {activeAgent.authorized_tools?.length || 0} tools bound
                      </span>
                    </div>
                  </div>

                  <div className="flex items-center gap-1.5 shrink-0">
                    {onNavigateTab && (
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => onNavigateTab('agents')}
                        className="text-[11px] px-2.5 py-1"
                        title="Manage Agents"
                      >
                        Agents
                      </Button>
                    )}
                  </div>
                </div>
              )}
            </div>
          </Card>

          {/* Right Column: Chat History & Input Canvas (8 Cols) */}
          <Card className="lg:col-span-8 flex flex-col justify-between p-4 sm:p-6 border border-onyx/10 h-full bg-canvas/80 min-h-[580px] shadow-xs overflow-hidden">
            {/* Top Bar inside Chat Feed */}
            <div className="flex items-center justify-between pb-3 border-b border-soft-meadow">
              <div className="flex items-center gap-3 truncate">
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
                    {activeAgent?.is_system && <Badge variant="accent">Root</Badge>}
                  </h3>
                  <div className="flex items-center gap-2 text-caption text-slate font-mono text-[11px] truncate">
                    <span className="truncate">Model: {activeAgent?.model_config.primary_model || 'claude-3-7-sonnet'}</span>
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
                    title="Clear Conversation"
                  />
                )}
                <Button
                  variant="primary"
                  size="sm"
                  icon={<Plus className="w-3.5 h-3.5" />}
                  onClick={handleNewChat}
                  className="lg:hidden text-caption px-3"
                >
                  + New
                </Button>
              </div>
            </div>

            {/* Messages Scroll Area */}
            <div className="flex-1 overflow-y-auto my-4 space-y-4 pr-2 max-h-[480px]">
              {messages.length === 0 ? (
                <div className="py-16 text-center text-slate">
                  <div className="w-14 h-14 rounded-full bg-soft-meadow flex items-center justify-center mx-auto mb-3 border border-onyx/10">
                    <Sparkles className="w-7 h-7 text-hi-yellow" />
                  </div>
                  <h4 className="font-serif text-heading-sm text-deep-ink mb-1">
                    Start a real-time conversation with {activeAgent?.name || 'Nova'}
                  </h4>
                  <p className="font-sans text-body-sm text-slate max-w-md mx-auto mb-6">
                    Real-time SSE token streaming, live tool execution traces, and immutable security audit logs.
                  </p>

                  {/* Prompt Suggestion Chips */}
                  <div className="flex flex-wrap justify-center gap-2 max-w-lg mx-auto">
                    {promptChips.map((chip, idx) => (
                      <button
                        key={idx}
                        onClick={() => handlePromptChip(chip)}
                        className="px-3.5 py-1.5 rounded-full bg-soft-meadow hover:bg-white text-caption font-medium text-deep-ink border border-onyx/10 transition-colors shadow-xs cursor-pointer flex items-center gap-1.5"
                      >
                        <Zap className="w-3 h-3 text-hi-yellow" />
                        <span>{chip}</span>
                      </button>
                    ))}
                  </div>
                </div>
              ) : (
                messages.map((msg, idx) => {
                  const hasTraces = (msg.toolCalls && msg.toolCalls.length > 0) || !!msg.thought;
                  const hasAudits = msg.auditLogs && msg.auditLogs.length > 0;
                  const currentMsgTab = activeTab[msg.id] || 'traces';

                  return (
                    <div
                      key={msg.id || idx}
                      className={`flex flex-col ${msg.role === 'user' ? 'items-end' : 'items-start'}`}
                    >
                      <div
                        className={`max-w-[90%] sm:max-w-[80%] p-4 rounded-[20px] transition-all shadow-xs relative group ${
                          msg.role === 'user'
                            ? 'bg-deep-ink text-white rounded-br-none'
                            : 'bg-soft-meadow text-deep-ink border border-onyx/10 rounded-bl-none'
                        }`}
                      >
                        {/* Live Thought Banner */}
                        {msg.thought && (
                          <div className="mb-3 p-2.5 bg-canvas/80 backdrop-blur-xs rounded-xl border border-onyx/10 text-caption font-mono flex items-center gap-2 text-deep-ink shadow-xs">
                            <Activity className="w-4 h-4 text-hi-yellow animate-spin shrink-0" />
                            <span className="truncate">{msg.thought}</span>
                          </div>
                        )}

                        {/* Message Content */}
                        <div className="font-sans text-body-sm leading-relaxed">
                          {msg.content ? (
                            <MarkdownContent content={msg.content} isUser={msg.role === 'user'} />
                          ) : msg.thought ? (
                            ''
                          ) : (
                            '...'
                          )}
                        </div>

                        {/* ReAct Execution Trace & Security Audit Log Accordion */}
                        {(hasTraces || hasAudits) && (
                          <div className="mt-3 pt-2.5 border-t border-onyx/10 text-caption">
                            <div className="flex items-center justify-between">
                              <button
                                onClick={() => toggleTrace(msg.id)}
                                className="flex items-center gap-1.5 text-slate hover:text-deep-ink font-semibold cursor-pointer font-mono text-[11px]"
                              >
                                <Terminal className="w-3.5 h-3.5 text-deep-ink" />
                                <span>
                                  {msg.toolCalls?.length ? `${msg.toolCalls.length} Tool Execution(s)` : 'Execution Details'}
                                  {hasAudits ? ` • ${msg.auditLogs?.length} Audit Logs` : ''}
                                </span>
                                {expandedTrace[msg.id] ? (
                                  <ChevronDown className="w-3.5 h-3.5" />
                                ) : (
                                  <ChevronRight className="w-3.5 h-3.5" />
                                )}
                              </button>

                              {expandedTrace[msg.id] && hasAudits && (
                                <div className="flex items-center gap-1 bg-canvas p-0.5 rounded-lg border border-onyx/5">
                                  <button
                                    onClick={() => setActiveTab((prev) => ({ ...prev, [msg.id]: 'traces' }))}
                                    className={`px-2 py-0.5 rounded text-[10px] font-semibold cursor-pointer transition-colors ${
                                      currentMsgTab === 'traces'
                                        ? 'bg-deep-ink text-white'
                                        : 'text-slate hover:text-deep-ink'
                                    }`}
                                  >
                                    Traces
                                  </button>
                                  <button
                                    onClick={() => setActiveTab((prev) => ({ ...prev, [msg.id]: 'audit' }))}
                                    className={`px-2 py-0.5 rounded text-[10px] font-semibold cursor-pointer transition-colors ${
                                      currentMsgTab === 'audit'
                                        ? 'bg-deep-ink text-white'
                                        : 'text-slate hover:text-deep-ink'
                                    }`}
                                  >
                                    Audit Logs
                                  </button>
                                </div>
                              )}
                            </div>

                            {expandedTrace[msg.id] && (
                              <div className="mt-2 p-3 bg-canvas rounded-[14px] border border-onyx/10 space-y-2 text-slate text-caption font-mono">
                                {currentMsgTab === 'traces' ? (
                                  <div className="space-y-2">
                                    {msg.toolCalls && msg.toolCalls.length > 0 ? (
                                      msg.toolCalls.map((tc, tcIdx) => (
                                        <div
                                          key={tcIdx}
                                          className="p-2 bg-soft-meadow rounded-xl border border-onyx/5 space-y-1 text-[11px]"
                                        >
                                          <div className="flex items-center justify-between text-deep-ink font-semibold">
                                            <div className="flex items-center gap-1.5">
                                              <FileCode className="w-3.5 h-3.5 text-hi-yellow shrink-0" />
                                              <span>{tc.tool}</span>
                                            </div>
                                            {tc.latency_ms !== undefined && (
                                              <span className="text-[10px] bg-canvas px-1.5 py-0.5 rounded text-slate">
                                                {tc.latency_ms} ms
                                              </span>
                                            )}
                                          </div>

                                          {tc.args && (
                                            <div className="text-slate text-[10px] truncate">
                                              <strong>Args:</strong> {typeof tc.args === 'string' ? tc.args : JSON.stringify(tc.args)}
                                            </div>
                                          )}

                                          {tc.result && (
                                            <div className="text-deep-ink text-[10px] bg-canvas p-1.5 rounded max-h-24 overflow-y-auto whitespace-pre-wrap">
                                              {tc.result}
                                            </div>
                                          )}
                                        </div>
                                      ))
                                    ) : (
                                      <div className="text-[11px] text-slate italic">
                                        No explicit tool calls executed in this iteration.
                                      </div>
                                    )}
                                  </div>
                                ) : (
                                  <div className="space-y-1.5 max-h-40 overflow-y-auto">
                                    {msg.auditLogs?.map((al, alIdx) => (
                                      <div
                                        key={alIdx}
                                        className="p-2 bg-soft-meadow rounded-xl border border-onyx/5 text-[10px] space-y-0.5"
                                      >
                                        <div className="flex items-center justify-between font-semibold text-deep-ink">
                                          <div className="flex items-center gap-1">
                                            <ShieldCheck className="w-3 h-3 text-emerald-600" />
                                            <span>{al.action}</span>
                                            {al.tool_name && <code className="text-[9px] bg-canvas px-1 rounded">{al.tool_name}</code>}
                                          </div>
                                          <span className="text-emerald-700 font-sans">{al.status}</span>
                                        </div>
                                        <div className="text-slate flex items-center justify-between">
                                          <span>{al.verification}</span>
                                          <span>{al.duration_ms} ms</span>
                                        </div>
                                      </div>
                                    ))}
                                  </div>
                                )}
                              </div>
                            )}
                          </div>
                        )}

                        {/* Bottom bar: Timestamp, Model & Copy Button */}
                        <div className="flex items-center justify-between mt-2 pt-1 text-[11px] opacity-70 gap-3">
                          <div className="flex items-center gap-2">
                            <span className="flex items-center gap-1">
                              <Clock className="w-3 h-3" />
                              <span>{msg.timestamp}</span>
                            </span>
                            {msg.model && (
                              <span className="flex items-center gap-1 font-mono text-[10px]">
                                <Cpu className="w-3 h-3" />
                                <span>{msg.model}</span>
                              </span>
                            )}
                          </div>

                          <button
                            onClick={() => handleCopy(msg.content, idx)}
                            className="hover:opacity-100 flex items-center gap-1 cursor-pointer p-0.5"
                            title="Copy response"
                          >
                            {copiedIdx === idx ? (
                              <Check className="w-3 h-3 text-emerald-500" />
                            ) : (
                              <Copy className="w-3 h-3" />
                            )}
                          </button>
                        </div>
                      </div>
                    </div>
                  );
                })
              )}

              {loading && !messages.some((m) => m.role === 'assistant' && m.thought) && (
                <div className="flex items-center gap-2 text-body-sm text-slate animate-pulse p-2">
                  <Bot className="w-4 h-4 text-hi-yellow" />
                  <span>Connecting to model cascade...</span>
                </div>
              )}
              <div ref={messagesEndRef} />
            </div>

            {/* Input Form */}
            <form onSubmit={handleSend} className="pt-2 border-t border-soft-meadow">
              <div className="flex items-center gap-2 bg-white rounded-full p-1.5 border border-onyx/15 shadow-sm focus-within:ring-2 focus-within:ring-deep-ink">
                <input
                  ref={inputRef}
                  type="text"
                  placeholder={t('placeholder', 'Ask a question or assign an autonomous task...')}
                  value={input}
                  onChange={(e) => setInput(e.target.value)}
                  className="flex-1 bg-transparent px-4 py-2 text-body-sm text-deep-ink focus:outline-none"
                  disabled={loading}
                />

                <Button
                  type="submit"
                  variant="primary"
                  size="sm"
                  disabled={!input.trim() || loading}
                  icon={<Send className="w-3.5 h-3.5" />}
                  className="shrink-0 px-5 py-2 font-semibold"
                >
                  {t('send', 'Send')}
                </Button>
              </div>
            </form>
          </Card>
        </div>
      </PageContainer>

      {/* Delete Conversation Confirmation Modal */}
      <ConfirmModal
        isOpen={!!deletingConvId}
        onClose={() => setDeletingConvId(null)}
        onConfirm={handleConfirmDeleteConv}
        title={t('deleteSession', 'Delete Conversation Session')}
        description={t('deleteConfirm', 'Are you sure you want to permanently clear this conversation session history?')}
        confirmLabel="Delete Session"
        variant="danger"
      />
    </div>
  );
}
