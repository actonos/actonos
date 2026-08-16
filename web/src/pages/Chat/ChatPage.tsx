import { useState, useEffect, useRef, type FormEvent } from 'react';
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
} from 'lucide-react';
import { api } from '@/lib/api';
import type { AgentManifest, ConversationItem } from '@/lib/types';
import { MarkdownContent } from '@/components/chat/MarkdownContent';

export interface ChatPageProps {
  selectedAgentID?: string;
  onSelectAgentID?: (id: string) => void;
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

export function ChatPage({ selectedAgentID, onSelectAgentID }: ChatPageProps) {
  const { success, error, info } = useToast();
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
  const [deletingConvId, setDeletingConvId] = useState<string | null>(null);

  const messagesEndRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

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

  const loadConversations = async (agentID?: string) => {
    try {
      const res = await api.listConversations(agentID);
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
    info('New Chat Session', 'Type a message to start a real-time streamed session.');
    inputRef.current?.focus();
  };

  const handleConfirmDeleteConv = async () => {
    if (!deletingConvId) return;
    try {
      await api.deleteConversation(deletingConvId);
      const remaining = conversations.filter((c) => c.id !== deletingConvId);
      setConversations(remaining);
      success('Session Deleted', 'Conversation history cleared.');
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
      thought: 'Activating cognitive ReAct engine...',
      toolCalls: [],
      auditLogs: [],
    };

    setMessages((prev) => [...prev, userMsgObj, currentAssistantMsg]);
    setLoading(true);

    try {
      const response = await fetch(`/api/agents/${activeAgentID}/chat`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ conversation_id: activeConvID, message: userMsg, stream: true }),
      });

      if (!response.ok) {
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
                        ? { ...c, title: parsed.title, updated_at: new Date().toISOString() }
                        : c
                    );
                  } else {
                    return [
                      {
                        id: parsed.conversation_id,
                        agent_id: activeAgentID,
                        title: parsed.title || userMsg.slice(0, 35) + '...',
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
    success('Copied to Clipboard', 'Response text copied.');
    setTimeout(() => setCopiedIdx(null), 2000);
  };

  const toggleTrace = (msgId: string) => {
    setExpandedTrace((prev) => ({ ...prev, [msgId]: !prev[msgId] }));
  };

  const activeAgent = agents.find((a) => a.agent_id === activeAgentID);
  const activeConv = conversations.find((c) => c.id === activeConvID);

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
        <div className="grid grid-cols-1 lg:grid-cols-4 gap-6 flex-1 max-h-[calc(100vh-100px)]">
          {/* Left Column: Sessions & Agent Selector */}
          <Card className="hidden lg:flex flex-col p-4 border border-onyx/10 justify-between h-full bg-canvas/90 shadow-xs">
            <div className="space-y-4">
              {/* Agent Picker */}
              <div>
                <label className="text-caption uppercase text-slate font-semibold block mb-1.5">
                  Select Active Agent
                </label>
                <select
                  value={activeAgentID}
                  onChange={(e) => {
                    setActiveAgentID(e.target.value);
                    onSelectAgentID?.(e.target.value);
                  }}
                  className="w-full bg-soft-meadow text-deep-ink font-medium p-2.5 rounded-full border border-onyx/10 text-body-sm font-sans focus:outline-none focus:ring-2 focus:ring-deep-ink"
                >
                  {agents.map((ag) => (
                    <option key={ag.agent_id} value={ag.agent_id}>
                      {ag.name} {ag.is_system ? '⭐ (Root)' : ''}
                    </option>
                  ))}
                </select>
              </div>

              {/* New Chat Button */}
              <Button
                variant="primary"
                size="sm"
                icon={<Plus className="w-4 h-4" />}
                onClick={handleNewChat}
                className="w-full justify-center text-caption py-2.5"
              >
                + New Chat Session
              </Button>

              {/* Sessions List */}
              <div>
                <div className="flex items-center justify-between mb-2 px-1">
                  <span className="text-caption uppercase text-slate font-semibold">
                    History Sessions
                  </span>
                  <span className="text-[11px] font-mono text-slate">
                    {conversations.length} total
                  </span>
                </div>

                <div className="space-y-1.5 max-h-[380px] overflow-y-auto pr-1">
                  {conversations.length === 0 ? (
                    <div className="p-4 text-center text-caption text-slate bg-soft-meadow rounded-2xl border border-onyx/5">
                      <MessageSquare className="w-5 h-5 mx-auto mb-1 opacity-40" />
                      <span>No saved sessions yet. Send a message to start one.</span>
                    </div>
                  ) : (
                    conversations.map((conv) => {
                      const isActive = activeConvID === conv.id;
                      return (
                        <div
                          key={conv.id}
                          onClick={() => selectConversation(conv.id)}
                          className={`flex items-center justify-between p-2.5 px-3.5 rounded-2xl transition-all cursor-pointer text-body-sm group ${
                            isActive
                              ? 'bg-deep-ink text-white font-medium shadow-xs'
                              : 'text-deep-ink hover:bg-soft-meadow border border-transparent hover:border-onyx/5'
                          }`}
                        >
                          <div className="flex items-center gap-2 truncate">
                            <MessageSquare className={`w-3.5 h-3.5 shrink-0 ${isActive ? 'text-hi-yellow' : 'text-slate'}`} />
                            <span className="truncate max-w-[155px] text-caption">{conv.title}</span>
                          </div>
                          <button
                            onClick={(e) => {
                              e.stopPropagation();
                              setDeletingConvId(conv.id);
                            }}
                            className={`p-1 rounded-full opacity-0 group-hover:opacity-100 transition-opacity ${
                              isActive ? 'hover:text-red-300 text-white/70' : 'hover:text-red-500 text-slate'
                            }`}
                            title="Delete session"
                          >
                            <Trash2 className="w-3.5 h-3.5" />
                          </button>
                        </div>
                      );
                    })
                  )}
                </div>
              </div>
            </div>

            {/* Bottom active agent mini card */}
            {activeAgent && (
              <div className="p-3 bg-soft-meadow rounded-[16px] border border-onyx/5 text-caption space-y-1">
                <div className="flex items-center gap-1.5 font-semibold text-deep-ink">
                  <Sparkles className="w-3.5 h-3.5 text-hi-yellow shrink-0" />
                  <span className="truncate">{activeAgent.name}</span>
                </div>
                <p className="text-slate font-mono text-[11px] truncate">
                  {activeAgent.model_config.primary_model}
                </p>
              </div>
            )}
          </Card>

          {/* Right Column: Chat History & Input Canvas */}
          <Card className="lg:col-span-3 flex flex-col justify-between p-4 sm:p-6 border border-onyx/10 h-full bg-canvas/70 min-h-[560px] shadow-xs">
            {/* Top Bar inside Chat */}
            <div className="flex items-center justify-between pb-3 border-b border-soft-meadow">
              <div className="flex items-center gap-3">
                <div className="w-10 h-10 rounded-full bg-deep-ink text-hi-yellow flex items-center justify-center border border-deep-ink shadow-xs shrink-0">
                  <Bot className="w-5 h-5" />
                </div>
                <div>
                  <h3 className="font-serif text-heading-sm text-deep-ink flex items-center gap-2">
                    <span>{activeAgent?.name || 'Acton Assistant'}</span>
                    {activeAgent?.is_system && <Badge variant="accent">Root</Badge>}
                  </h3>
                  <div className="flex items-center gap-2 text-caption text-slate font-mono text-[11px]">
                    <span>Model: {activeAgent?.model_config.primary_model || 'claude-3-7-sonnet'}</span>
                    {activeConv && (
                      <>
                        <span>•</span>
                        <span className="truncate max-w-[200px] text-deep-ink font-sans">
                          {activeConv.title}
                        </span>
                      </>
                    )}
                  </div>
                </div>
              </div>

              <div className="flex items-center gap-2">
                <Button variant="ghost" size="sm" onClick={handleNewChat} className="lg:hidden">
                  + New
                </Button>
              </div>
            </div>

            {/* Messages Scroll Area */}
            <div className="flex-1 overflow-y-auto my-4 space-y-4 pr-2 max-h-[460px]">
              {messages.length === 0 ? (
                <div className="py-16 text-center text-slate">
                  <Sparkles className="w-12 h-12 text-hi-yellow mx-auto mb-3 opacity-60" />
                  <h4 className="font-serif text-heading-sm text-deep-ink mb-1">
                    Start a real-time conversation with {activeAgent?.name || 'Acton Assistant'}
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
                        {/* Live Thought Shimmer Banner (When Agent is thinking) */}
                        {msg.thought && (
                          <div className="mb-3 p-2.5 bg-canvas/80 backdrop-blur-xs rounded-xl border border-onyx/10 text-caption font-mono flex items-center gap-2 text-deep-ink shadow-xs">
                            <Activity className="w-4 h-4 text-hi-yellow animate-spin shrink-0" />
                            <span className="truncate">{msg.thought}</span>
                          </div>
                        )}

                        {/* Message Content (Formatted with TipTap Markdown) */}
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
                  placeholder="Ask a question or assign an autonomous task..."
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
                  className="shrink-0 px-4 py-2"
                >
                  Send
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
        title="Delete Conversation Session"
        description="Are you sure you want to permanently clear this conversation session history?"
        confirmLabel="Delete Session"
        variant="danger"
      />
    </div>
  );
}
