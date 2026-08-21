import { useState, useEffect, useRef, type FormEvent } from 'react';
import { getErrorMessage } from '@/lib/errors';
import { useTranslation } from 'react-i18next';
import { PageContainer } from '@/components/layout/PageContainer';
import { ChatHeader } from '@/components/features/chat/ChatHeader';
import { ChatComposer } from '@/components/features/chat/ChatComposer';
import { MessageTimeline } from '@/components/features/chat/MessageTimeline';
import { ChatSessionRail } from '@/components/features/chat/ChatSessionRail';
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
  ChevronDown,
  Plus,
  MessageSquare,
  Search,
  Edit3,
  X,
} from 'lucide-react';
import { api } from '@/lib/api';
import type { AgentManifest, ConversationItem } from '@/lib/types';
import type { NavTab } from '@/components/layout/Sidebar';
import {
  formatRelativeTime,
  type ChatMessage,
  type ToolCallTrace,
} from './chatTypes';

export interface ChatPageProps {
  selectedAgentID?: string;
  onSelectAgentID?: (id: string) => void;
  onNavigateTab?: (tab: NavTab) => void;
}

export function ChatPage({ selectedAgentID, onSelectAgentID, onNavigateTab }: ChatPageProps) {
  const { t, i18n } = useTranslation('chat');
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
  const [sessionsOpen, setSessionsOpen] = useState(false);

  // Modals & Renaming
  const [deletingConvId, setDeletingConvId] = useState<string | null>(null);
  const [editingConvId, setEditingConvId] = useState<string | null>(null);
  const [editingConvTitle, setEditingConvTitle] = useState('');

  const messagesEndRef = useRef<HTMLDivElement>(null);
  const messagesContainerRef = useRef<HTMLDivElement>(null);
  const isAutoScrollEnabled = useRef(true);
  const [showScrollBottom, setShowScrollBottom] = useState(false);
  const inputRef = useRef<HTMLTextAreaElement>(null);
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
    } catch (err) {
      error('Failed to load agents', getErrorMessage(err));
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
    } catch (err) {
      error('Failed to load conversations', getErrorMessage(err));
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
    } catch (err) {
      error('Failed to delete session', getErrorMessage(err));
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
    } catch (err) {
      error('Failed to update title', getErrorMessage(err));
    } finally {
      setEditingConvId(null);
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

  // Real-time synchronization: pull incoming messages from channels/cron/heartbeat without flickering
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
        setConversations((prev) => {
          if (prev.length !== res.conversations.length) {
            return res.conversations;
          }
          return prev;
        });
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
    if (activeConvID !== activeConvRef.current || messages.length > 0) {
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
  }, [activeConvID, messages.length]);

  const wasLoadingRef = useRef(false);
  useEffect(() => {
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

    // While streaming tokens / reasoning, ONLY autoscroll if user has NOT scrolled up!
    if (loading && isAutoScrollEnabled.current) {
      scrollToBottom('auto');
    }
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
      // SSE state that must survive across reader.read() boundaries: a single
      // event's `event:` and `data:` lines can land in different chunks.
      let currentEvent = 'token';

      // One network read can carry many token events. Every setMessages inside the
      // synchronous parse loop below is batched by React into a single render, so a
      // chunk holding 40 tokens painted once instead of 40 times — the "jumping
      // text" effect. Yielding to the event loop after each parsed event lets React
      // commit one frame per token.
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

        // currentEvent must persist across read() chunks. Resetting it per chunk
        // meant a `data:` line that arrived in a later TCP read than its `event:`
        // line was mis-attributed to the default 'token' type.
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
                // Operational status update: update to the latest step in real-time
                currentAssistantMsg = {
                  ...currentAssistantMsg,
                  thought: parsed.thought,
                };
              } else if (currentEvent === 'reasoning' && parsed.reasoning) {
                // Streaming CoT reasoning tokens into separate segments
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
                // Streaming tokens into separate segments
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
                  // A token arriving means the answer has started: clear transient status
                  thought: undefined,
                  segments: segs,
                };
              } else if (currentEvent === 'token_reset') {
                // If this turn called tools, shift any streamed preamble into reasoning (stripping any raw DSML/XML markup)
                const rawPreamble = currentAssistantMsg.content;
                const cleanPreamble = rawPreamble
                  .replace(/<[|｜]{1,2}DSML[|｜]{1,2}[\s\S]*?<\/[|｜]{1,2}DSML[|｜]{1,2}tool_calls>/g, '')
                  .replace(/<[|｜]{1,2}[\s\S]*?>/g, '')
                  .replace(/<\/?(?:tool_call|function_call|invoke|parameter)[^>]*>/g, '')
                  .trim();
                const segs = (currentAssistantMsg.segments || []).filter(s => s.type === 'reasoning');
                if (cleanPreamble) {
                  segs.push({ type: 'reasoning', text: cleanPreamble });
                }
                currentAssistantMsg = {
                  ...currentAssistantMsg,
                  reasoning: cleanPreamble
                    ? (currentAssistantMsg.reasoning ? currentAssistantMsg.reasoning + '\n\n' + cleanPreamble : cleanPreamble)
                    : currentAssistantMsg.reasoning,
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
                // Keep the incrementally streamed text as-is. Overwriting it with
                // the full payload made the message visibly re-render at the end;
                // the payload is only a fallback when nothing was streamed.
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

              // Let React paint this token before parsing the next one.
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
    t('prompts.diagnostics'),
    t('prompts.workspace'),
    t('prompts.decompose'),
    t('prompts.architecture'),
  ];

  return (
    <div className="relative flex flex-col" style={{ height: 'calc(100dvh - 64px)', overflow: 'hidden' }}>
      <BlobBackdrop />

      <PageContainer maxWidth="wide" className="flex-1 flex flex-col py-4 min-h-0 overflow-hidden">
        <ChatHeader agent={activeAgent} onOpenSessions={() => setSessionsOpen(true)} />
        {/* Main 2-Column Chat Area */}
        <div className="grid min-h-0 flex-1 grid-cols-1 gap-6 lg:grid-cols-12">
          <ChatSessionRail
            agents={agents}
            conversations={filteredConversations}
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
          {/* Left Column: Sessions & Custom Agent Switcher (4 Cols) */}
          <Card className="hidden flex-col p-4 border border-onyx/10 justify-between h-full bg-canvas/95 shadow-xs overflow-hidden" aria-hidden="true">
            <div className="space-y-3.5 flex flex-col h-full overflow-hidden">
              {/* 1. Custom Agent Switcher Header */}
              <div className="relative" ref={dropdownRef}>
                <div className="flex items-center justify-between mb-1.5 px-1">
                  <span className="text-[11px] uppercase tracking-wider text-slate font-semibold">
                    {t('selectAgent', 'Active Agent')}
                  </span>
                  {activeAgent?.is_system && (
                    <span className="text-[10px] bg-hi-yellow/40 text-deep-ink px-2 py-0.5 rounded-full font-semibold">
                      {t('rootBadge')}
                    </span>
                  )}
                </div>

                {/* Custom Agent Dropdown Button */}
                <button
                  type="button"
                  onClick={() => setAgentDropdownOpen(!agentDropdownOpen)}
                  className="w-full flex items-center justify-between p-2.5 px-3 rounded-[18px] bg-soft-meadow hover:bg-canvas border border-onyx/15 transition-all text-left cursor-pointer group"
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
                        {activeAgent?.model_config.primary_model || 'claude-sonnet-4-6'}
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
                          className={`w-full flex items-center justify-between p-2.5 rounded-[14px] text-left transition-colors cursor-pointer ${isSelected
                            ? 'bg-deep-ink text-white font-medium shadow-xs'
                            : 'text-deep-ink hover:bg-soft-meadow'
                            }`}
                        >
                          <div className="flex items-center gap-2.5 truncate">
                            <div className={`w-7 h-7 rounded-full flex items-center justify-center shrink-0 ${isSelected ? 'bg-hi-yellow text-deep-ink' : 'bg-canvas text-deep-ink border border-onyx/10'
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
                            <span className={`text-[10px] px-1.5 py-0.5 rounded font-semibold ${isSelected ? 'bg-white/20 text-hi-yellow' : 'bg-soft-meadow text-slate'
                              }`}>
                              {t('root')}
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
                    className={`flex-1 py-1 rounded-full transition-all text-center font-medium cursor-pointer ${sessionFilterScope === 'all'
                      ? 'bg-deep-ink text-white shadow-xs font-semibold'
                      : 'text-slate hover:text-deep-ink'
                      }`}
                  >
                    {t('allAgents', 'All Sessions')} ({conversations.length})
                  </button>
                  <button
                    onClick={() => setSessionFilterScope('agent')}
                    className={`flex-1 py-1 rounded-full transition-all text-center font-medium cursor-pointer ${sessionFilterScope === 'agent'
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
                        className={`p-2.5 px-3 rounded-[16px] transition-all cursor-pointer border group relative ${isActive
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
                            <span className={`px-1.5 py-0.2 rounded font-semibold truncate ${isActive ? 'bg-white/20 text-hi-yellow' : 'bg-canvas text-deep-ink border border-onyx/5'
                              }`}>
                              {convAgent?.name || conv.agent_id}
                            </span>
                            {conv.message_count !== undefined && conv.message_count > 0 && (
                              <span>{conv.message_count}</span>
                            )}
                          </div>
                          <span className="font-mono text-[10px] shrink-0">
                            {formatRelativeTime(conv.updated_at || conv.created_at, i18n.resolvedLanguage || i18n.language)}
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
                        {t('toolsBound', { count: activeAgent.authorized_tools?.length || 0 })}
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
                        title={t('manageAgents')}
                      >
                        {t('agents')}
                      </Button>
                    )}
                  </div>
                </div>
              )}
            </div>
          </Card>

          {/* Right Column: Chat History & Input Canvas (8/9 Cols) */}
          <Card className="lg:col-span-8 xl:col-span-9 flex flex-col justify-between p-4 sm:p-6 border border-onyx/10 h-full bg-canvas/80 min-h-0 shadow-xs overflow-hidden">
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
      </PageContainer>

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
