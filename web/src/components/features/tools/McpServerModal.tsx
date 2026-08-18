import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { Modal } from '@/components/ui/Modal';
import { Input } from '@/components/ui/Input';
import { Button } from '@/components/ui/Button';
import { Sparkles, Terminal } from 'lucide-react';

export interface McpServerModalProps {
  isOpen: boolean;
  onClose: () => void;
  onConnect: (cfg: { id: string; transport: string; command?: string; args?: string[]; url?: string; env?: Record<string, string> }) => Promise<void>;
}

export function McpServerModal({ isOpen, onClose, onConnect }: McpServerModalProps) {
  const { t } = useTranslation('tools');
  const [id, setId] = useState('fetch_server');
  const [transport, setTransport] = useState('stdio');
  const [command, setCommand] = useState('npx');
  const [argsStr, setArgsStr] = useState('-y @modelcontextprotocol/server-fetch');
  const [url, setURL] = useState('');
  const [envText, setEnvText] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  const presets = [
    {
      name: '🌐 Web Fetch',
      id: 'fetch_mcp',
      command: 'npx',
      args: '-y @modelcontextprotocol/server-fetch',
      desc: 'Allows agents to fetch & extract text content from any public HTTP/HTTPS URL.',
    },
    {
      name: '🗄️ SQLite DB',
      id: 'sqlite_mcp',
      command: 'npx',
      args: '-y @modelcontextprotocol/server-sqlite --db ./data/storage/acton.db',
      desc: 'Enables querying schemas and inspecting relational records with read/write safety.',
    },
    {
      name: '📁 File System',
      id: 'filesystem_mcp',
      command: 'npx',
      args: '-y @modelcontextprotocol/server-filesystem ./data/workspace',
      desc: 'Provides stdio protocol directory traversal and sandboxed file manipulation.',
    },
    {
      name: '🔍 Brave Search',
      id: 'brave_search_mcp',
      command: 'npx',
      args: '-y @modelcontextprotocol/server-brave-search',
      desc: 'Real-time privacy-first web search API for fresh agent research.',
    },
  ];

  const handleApplyPreset = (p: typeof presets[0]) => {
    setId(p.id);
    setCommand(p.command);
    setArgsStr(p.args);
    setError('');
  };

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setLoading(true);
    setError('');

    try {
      const args = argsStr.trim() ? argsStr.split(' ') : [];
      const env = Object.fromEntries(
        envText.split('\n').map((line) => line.trim()).filter(Boolean).map((line) => {
          const separator = line.indexOf('=');
          return separator < 0 ? [line, ''] : [line.slice(0, separator), line.slice(separator + 1)];
        })
      );
      await onConnect({ id, transport, command: transport === 'stdio' ? command : undefined, args, url: transport === 'stdio' ? undefined : url, env });
      onClose();
    } catch (err: any) {
      setError(err.message || 'Failed to connect MCP server');
    } finally {
      setLoading(false);
    }
  };

  return (
    <Modal isOpen={isOpen} onClose={onClose} title={t('mcp.connectTitle')}>
      <form onSubmit={handleSubmit} className="flex flex-col gap-4">
        {error && (
          <div className="p-3 bg-red-100 text-red-700 rounded-[14px] text-body-sm font-medium">
            {error}
          </div>
        )}

        {/* 1-Click Presets */}
        <div>
          <label className="text-caption uppercase text-slate font-semibold block mb-2 flex items-center gap-1.5">
            <Sparkles className="w-3.5 h-3.5 text-hi-yellow" /> Recommended Presets
          </label>
          <div className="grid grid-cols-2 gap-2">
            {presets.map((p) => (
              <button
                key={p.id}
                type="button"
                onClick={() => handleApplyPreset(p)}
                className={`p-2.5 rounded-[14px] text-left border transition-all cursor-pointer ${
                  id === p.id
                    ? 'bg-deep-ink text-white border-deep-ink font-semibold'
                    : 'bg-canvas text-deep-ink border-onyx/10 hover:bg-soft-meadow'
                }`}
              >
                <div className="text-body-sm font-medium leading-none mb-1">{p.name}</div>
                <div className={`text-[11px] line-clamp-1 ${id === p.id ? 'text-white/70' : 'text-slate'}`}>
                  {p.desc}
                </div>
              </button>
            ))}
          </div>
        </div>

        <Input
          label="Server Identifier (Unique ID)"
          placeholder="e.g., fetch_mcp"
          value={id}
          onChange={(e) => setId(e.target.value)}
          required
        />

        <div>
          <label className="text-caption uppercase text-slate font-semibold block mb-1">{t('mcp.transport')}</label>
          <select value={transport} onChange={(event) => setTransport(event.target.value)} className="w-full bg-canvas text-deep-ink px-4 py-2.5 rounded-full border border-onyx/15 text-body-sm">
            <option value="stdio">stdio</option>
            <option value="http">Streamable HTTP</option>
            <option value="sse">SSE</option>
          </select>
        </div>

        {transport === 'stdio' ? (
          <Input label={t('mcp.command')} placeholder="npx" value={command} onChange={(e) => setCommand(e.target.value)} required />
        ) : (
          <Input label={t('mcp.url')} placeholder="https://mcp.example.com" value={url} onChange={(e) => setURL(e.target.value)} required />
        )}

        <div>
          <label className="text-caption uppercase text-slate font-semibold block mb-1">
            CLI Arguments (Space-separated)
          </label>
          <input
            type="text"
            placeholder="-y @modelcontextprotocol/server-fetch"
            value={argsStr}
            onChange={(e) => setArgsStr(e.target.value)}
            className="w-full bg-canvas text-deep-ink font-mono text-body-sm px-4 py-2.5 rounded-full border border-onyx/15 focus:outline-none focus:ring-2 focus:ring-deep-ink"
          />
        </div>

        <div>
          <label className="text-caption uppercase text-slate font-semibold block mb-1">{t('mcp.environment')}</label>
          <textarea
            value={envText}
            onChange={(event) => setEnvText(event.target.value)}
            placeholder="API_KEY=••••••"
            rows={3}
            className="w-full bg-canvas text-deep-ink font-mono text-body-sm px-4 py-3 rounded-[18px] border border-onyx/15 focus:outline-none focus:ring-2 focus:ring-deep-ink"
          />
          <p className="text-[11px] text-slate mt-1">{t('mcp.environmentHelp')}</p>
        </div>

        <div className="p-3 bg-canvas rounded-[14px] border border-onyx/5 text-caption text-slate space-y-1">
          <div className="font-mono flex items-center gap-1 text-deep-ink font-semibold">
            <Terminal className="w-3.5 h-3.5" /> Transport: stdio pipe with JSON-RPC 2.0
          </div>
          <p>ActonOS spawns the MCP process inside the isolated executor and automatically registers all exported tools.</p>
        </div>

        <Button
          type="submit"
          variant="primary"
          disabled={loading || !id.trim()}
          className="w-full justify-center mt-2"
        >
          {loading ? 'Connecting & Registering Tools...' : 'Connect MCP Server'}
        </Button>
      </form>
    </Modal>
  );
}
