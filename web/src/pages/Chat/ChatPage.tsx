import { useState, useEffect, useRef, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { PageContainer } from '@/components/layout/PageContainer';
import { BlobBackdrop } from '@/components/ui/BlobBackdrop';
import { Button } from '@/components/ui/Button';
import { Card } from '@/components/ui/Card';
import { Send, Bot, User } from 'lucide-react';
import { api } from '@/lib/api';
import type { AgentManifest } from '@/lib/types';

export interface ChatPageProps {
  selectedAgentID?: string;
  onSelectAgentID?: (id: string) => void;
}

interface Message {
  role: 'user' | 'assistant';
  content: string;
  timestamp: string;
}

export function ChatPage({ selectedAgentID, onSelectAgentID }: ChatPageProps) {
  const { t } = useTranslation('chat');
  const [agents, setAgents] = useState<AgentManifest[]>([]);
  const [activeAgentID, setActiveAgentID] = useState<string>(selectedAgentID || '');
  const [messages, setMessages] = useState<Message[]>([]);
  const [input, setInput] = useState('');
  const [loading, setLoading] = useState(false);
  const messagesEndRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    api.listAgents().then((res) => {
      setAgents(res.agents || []);
      if (!activeAgentID && res.agents?.length > 0) {
        setActiveAgentID(res.agents[0].agent_id);
      }
    });
  }, []);

  useEffect(() => {
    if (selectedAgentID) {
      setActiveAgentID(selectedAgentID);
    }
  }, [selectedAgentID]);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages]);

  const handleSend = async (e: FormEvent) => {
    e.preventDefault();
    if (!input.trim() || !activeAgentID || loading) return;

    const userMsg = input.trim();
    setInput('');
    const now = new Date().toLocaleTimeString();

    setMessages((prev) => [
      ...prev,
      { role: 'user', content: userMsg, timestamp: now },
    ]);
    setLoading(true);

    try {
      // POST to /api/agents/{id}/chat with streaming or non-streaming fallback
      const res = await fetch(`/api/agents/${activeAgentID}/chat`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ message: userMsg, stream: false }),
      });
      const data = await res.json();
      const assistantMsg = data.data?.content || data.content || 'Response received.';

      setMessages((prev) => [
        ...prev,
        { role: 'assistant', content: assistantMsg, timestamp: new Date().toLocaleTimeString() },
      ]);
    } catch (err: any) {
      setMessages((prev) => [
        ...prev,
        { role: 'assistant', content: `Error: ${err.message}`, timestamp: new Date().toLocaleTimeString() },
      ]);
    } finally {
      setLoading(false);
    }
  };

  const activeAgent = agents.find((a) => a.agent_id === activeAgentID);

  return (
    <div className="relative min-h-[calc(100vh-72px)] flex flex-col">
      <BlobBackdrop />

      <PageContainer className="flex-1 flex flex-col">
        {/* Agent selector header */}
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6 pb-4 border-b border-soft-meadow">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded-full bg-canvas flex items-center justify-center text-deep-ink border border-onyx">
              <Bot className="w-5 h-5" />
            </div>
            <div>
              <h2 className="font-serif text-heading-sm text-deep-ink">
                {activeAgent ? activeAgent.name : t('title')}
              </h2>
              <span className="text-caption uppercase text-slate font-medium">
                {activeAgent ? `${activeAgent.model_config.primary_model} • ${activeAgent.status}` : t('selectAgent')}
              </span>
            </div>
          </div>

          <div className="flex items-center gap-2">
            <select
              value={activeAgentID}
              onChange={(e) => {
                setActiveAgentID(e.target.value);
                if (onSelectAgentID) onSelectAgentID(e.target.value);
              }}
              className="bg-soft-meadow text-deep-ink font-sans text-body-sm px-4 py-2 rounded-full border border-onyx/20 focus:outline-none"
            >
              {agents.map((a) => (
                <option key={a.agent_id} value={a.agent_id}>
                  {a.name}
                </option>
              ))}
            </select>
          </div>
        </div>

        {/* Message history container */}
        <Card className="flex-1 flex flex-col justify-between overflow-hidden p-6 border border-onyx/10 min-h-[450px]">
          <div className="flex-1 overflow-y-auto pr-2 flex flex-col gap-4 max-h-[500px]">
            {messages.length === 0 ? (
              <div className="text-center my-auto py-12 text-slate font-sans">
                <Bot className="w-10 h-10 mx-auto mb-3 text-slate opacity-40" />
                <p className="text-body-sm">{t('selectAgent')}</p>
              </div>
            ) : (
              messages.map((m, idx) => (
                <div
                  key={idx}
                  className={`flex gap-3 max-w-[80%] ${
                    m.role === 'user' ? 'self-end flex-row-reverse' : 'self-start'
                  }`}
                >
                  <div
                    className={`w-8 h-8 rounded-full flex items-center justify-center shrink-0 ${
                      m.role === 'user' ? 'bg-deep-ink text-white' : 'bg-hi-yellow text-deep-ink'
                    }`}
                  >
                    {m.role === 'user' ? <User className="w-4 h-4" /> : <Bot className="w-4 h-4" />}
                  </div>
                  <div
                    className={`rounded-[20px] px-5 py-3 text-body-sm font-sans whitespace-pre-wrap ${
                      m.role === 'user'
                        ? 'bg-deep-ink text-white'
                        : 'bg-canvas text-deep-ink border border-onyx/10'
                    }`}
                  >
                    {m.content}
                    <span className="block text-[10px] opacity-60 mt-1 text-right">
                      {m.timestamp}
                    </span>
                  </div>
                </div>
              ))
            )}
            {loading && (
              <div className="flex gap-3 self-start max-w-[80%]">
                <div className="w-8 h-8 rounded-full bg-hi-yellow text-deep-ink flex items-center justify-center">
                  <Bot className="w-4 h-4 animate-spin" />
                </div>
                <div className="bg-canvas rounded-[20px] px-5 py-3 text-body-sm text-slate italic border border-onyx/10">
                  {t('streaming')}
                </div>
              </div>
            )}
            <div ref={messagesEndRef} />
          </div>

          {/* Chat input bar */}
          <form onSubmit={handleSend} className="pt-4 border-t border-canvas flex gap-3">
            <input
              type="text"
              value={input}
              onChange={(e) => setInput(e.target.value)}
              placeholder={t('placeholder')}
              className="flex-1 bg-canvas text-deep-ink font-sans text-body px-5 py-3 rounded-full border border-onyx focus:outline-none focus:ring-2 focus:ring-deep-ink"
              disabled={loading || !activeAgentID}
            />
            <Button
              type="submit"
              variant="primary"
              disabled={loading || !input.trim() || !activeAgentID}
              icon={<Send className="w-4 h-4" />}
            >
              {t('send')}
            </Button>
          </form>
        </Card>
      </PageContainer>
    </div>
  );
}
