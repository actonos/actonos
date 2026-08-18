import { useEffect, useRef, useState, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import '@xterm/xterm/css/xterm.css';
import {
  Terminal as TerminalIcon,
  RefreshCw,
  Trash2,
  CheckCircle2,
  AlertCircle,
  Clock,
  Laptop,
} from 'lucide-react';
import { PageContainer } from '@/components/layout/PageContainer';
import { PageHeader } from '@/components/ui/PageHeader';
import { Card } from '@/components/ui/Card';
import { Button } from '@/components/ui/Button';
import { Badge } from '@/components/ui/Badge';

export function TerminalPage() {
  const { t } = useTranslation('terminal');
  const terminalNodeRef = useRef<HTMLDivElement>(null);
  const terminalRef = useRef<Terminal | null>(null);
  const fitAddonRef = useRef<FitAddon | null>(null);
  const socketRef = useRef<WebSocket | null>(null);

  const [status, setStatus] = useState<'connecting' | 'connected' | 'disconnected' | 'error'>('connecting');
  const [shellInfo, setShellInfo] = useState<string>('');

  const connectWebSocket = useCallback(() => {
    // Clean up previous connection if any
    if (socketRef.current) {
      socketRef.current.close();
      socketRef.current = null;
    }

    const term = terminalRef.current;
    if (!term) return;

    setStatus('connecting');
    term.writeln('\x1b[90mConnecting to ActonOS Shell...\x1b[0m');

    const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${wsProtocol}//${window.location.host}/api/terminal/ws`;

    try {
      const socket = new WebSocket(wsUrl);
      socketRef.current = socket;

      socket.onopen = () => {
        setStatus('connected');
        term.writeln('\x1b[32m✔ ActonOS Host Interactive Terminal Session Established\x1b[0m\r\n');
        // Determine OS type hint
        if (navigator.userAgent.includes('Windows')) {
          setShellInfo('PowerShell / Host Shell');
        } else {
          setShellInfo('Bash / Sh');
        }
        term.focus();
      };

      socket.onmessage = (event) => {
        if (typeof event.data === 'string') {
          term.write(event.data);
        } else if (event.data instanceof Blob) {
          const reader = new FileReader();
          reader.onload = () => {
            if (typeof reader.result === 'string') {
              term.write(reader.result);
            }
          };
          reader.readAsText(event.data);
        }
      };

      socket.onclose = () => {
        setStatus('disconnected');
        term.writeln('\r\n\x1b[33m⚡ Shell process disconnected.\x1b[0m\r\n');
      };

      socket.onerror = () => {
        setStatus('error');
        term.writeln('\r\n\x1b[31m✖ WebSocket connection error.\x1b[0m\r\n');
      };
    } catch (err) {
      setStatus('error');
      term.writeln(`\r\n\x1b[31m✖ Failed to initialize connection: ${err}\x1b[0m\r\n`);
    }
  }, []);

  useEffect(() => {
    if (!terminalNodeRef.current) return;

    const term = new Terminal({
      convertEol: true,
      cursorBlink: true,
      cursorStyle: 'block',
      fontFamily: '"Cascadia Code", "SFMono-Regular", Consolas, "Courier New", monospace',
      fontSize: 13,
      lineHeight: 1.25,
      letterSpacing: 0,
      theme: {
        background: '#0e0b1f',
        foreground: '#f1f5f9',
        cursor: '#ffe228',
        cursorAccent: '#0e0b1f',
        selectionBackground: 'rgba(255, 226, 40, 0.3)',
        black: '#1e1a38',
        red: '#ef4444',
        green: '#10b981',
        yellow: '#f59e0b',
        blue: '#3b82f6',
        magenta: '#d946ef',
        cyan: '#06b6d4',
        white: '#f8fafc',
        brightBlack: '#64748b',
        brightRed: '#f87171',
        brightGreen: '#34d399',
        brightYellow: '#fbbf24',
        brightBlue: '#60a5fa',
        brightMagenta: '#e879f9',
        brightCyan: '#22d3ee',
        brightWhite: '#ffffff',
      },
    });

    const fitAddon = new FitAddon();
    term.loadAddon(fitAddon);
    term.open(terminalNodeRef.current);

    setTimeout(() => {
      fitAddon.fit();
    }, 100);

    terminalRef.current = term;
    fitAddonRef.current = fitAddon;

    // Send keystrokes to WebSocket
    const onDataDisposable = term.onData((data) => {
      if (socketRef.current && socketRef.current.readyState === WebSocket.OPEN) {
        socketRef.current.send(data);
      }
    });

    // Resize handler
    const handleResize = () => {
      if (fitAddonRef.current) {
        fitAddonRef.current.fit();
      }
    };
    window.addEventListener('resize', handleResize);

    // Initial connection
    connectWebSocket();

    return () => {
      window.removeEventListener('resize', handleResize);
      onDataDisposable.dispose();
      if (socketRef.current) {
        socketRef.current.close();
        socketRef.current = null;
      }
      term.dispose();
      terminalRef.current = null;
      fitAddonRef.current = null;
    };
  }, [connectWebSocket]);

  const handleClear = () => {
    if (terminalRef.current) {
      terminalRef.current.clear();
      terminalRef.current.focus();
    }
  };

  const handleReconnect = () => {
    if (terminalRef.current) {
      terminalRef.current.clear();
    }
    connectWebSocket();
  };

  return (
    <PageContainer>
      <PageHeader
        eyebrow={t('eyebrow', 'Host System Shell')}
        title={t('title', 'Web Terminal')}
        description={t('subtitle', 'Direct interactive terminal connection to host operating system environment without external SSH client.')}
        actions={
          <div className="flex items-center gap-2">
            <Badge
              variant={
                status === 'connected' ? 'success' :
                status === 'connecting' ? 'neutral' :
                'stopped'
              }
            >
              {status === 'connected' && <CheckCircle2 className="w-3.5 h-3.5 mr-1" />}
              {status === 'connecting' && <Clock className="w-3.5 h-3.5 mr-1 animate-spin" />}
              {(status === 'disconnected' || status === 'error') && <AlertCircle className="w-3.5 h-3.5 mr-1" />}
              {t(`status.${status}`, status)}
            </Badge>

            <Button
              variant="ghost"
              size="sm"
              icon={<Trash2 className="w-3.5 h-3.5" />}
              onClick={handleClear}
            >
              {t('actions.clear', 'Clear')}
            </Button>

            <Button
              variant="secondary"
              size="sm"
              icon={<RefreshCw className={`w-3.5 h-3.5 ${status === 'connecting' ? 'animate-spin' : ''}`} />}
              onClick={handleReconnect}
            >
              {t('actions.reconnect', 'Reconnect')}
            </Button>
          </div>
        }
      />

      <Card className="p-0 border border-onyx/10 overflow-hidden bg-[#0e0b1f] shadow-lg rounded-2xl flex flex-col min-h-[580px] h-[calc(100vh-210px)]">
        {/* Terminal Title Bar */}
        <div className="px-4 py-2.5 bg-[#171233] border-b border-white/10 flex items-center justify-between">
          <div className="flex items-center gap-2.5">
            <div className="flex gap-1.5">
              <div className="w-3 h-3 rounded-full bg-rose-500/80" />
              <div className="w-3 h-3 rounded-full bg-amber-500/80" />
              <div className="w-3 h-3 rounded-full bg-emerald-500/80" />
            </div>
            <div className="h-4 w-px bg-white/10 mx-1" />
            <div className="flex items-center gap-1.5 text-white/80 text-caption font-mono">
              <TerminalIcon className="w-3.5 h-3.5 text-amber-400" />
              <span>actonos@kernel:~</span>
            </div>
          </div>

          <div className="flex items-center gap-3 text-caption font-mono text-white/60">
            {shellInfo && (
              <span className="hidden sm:inline-flex items-center gap-1">
                <Laptop className="w-3.5 h-3.5 text-white/40" />
                {shellInfo}
              </span>
            )}
            <span className="text-[11px] px-2 py-0.5 rounded bg-white/10 text-white/80">
              UTF-8 · ANSI
            </span>
          </div>
        </div>

        {/* XTerm Container */}
        <div
          ref={terminalNodeRef}
          className="flex-1 w-full p-3 overflow-hidden"
          onClick={() => terminalRef.current?.focus()}
        />
      </Card>
    </PageContainer>
  );
}
