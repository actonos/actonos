import { useEffect, useRef, useState } from 'react';
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
  Maximize2,
  Minimize2,
  ZoomIn,
  ZoomOut,
  ChevronDown,
  Sparkles,
  Server,
} from 'lucide-react';
import { Button } from '@/components/ui/Button';
import { Badge } from '@/components/ui/Badge';
import { api } from '@/lib/api';

interface TerminalShellOption {
  id: string;
  name: string;
  available: boolean;
}

interface TerminalInfo {
  os: string;
  default_shell: string;
  available_shells: TerminalShellOption[];
}

export function TerminalPage() {
  const { t } = useTranslation('terminal');
  const terminalNodeRef = useRef<HTMLDivElement>(null);
  const terminalRef = useRef<Terminal | null>(null);
  const fitAddonRef = useRef<FitAddon | null>(null);
  const socketRef = useRef<WebSocket | null>(null);

  const [status, setStatus] = useState<'connecting' | 'connected' | 'disconnected' | 'error'>('connecting');
  const [terminalInfo, setTerminalInfo] = useState<TerminalInfo | null>(null);
  const [selectedShell, setSelectedShell] = useState<string>('bash');
  const [fontSize, setFontSize] = useState<number>(13);
  const [isFullscreen, setIsFullscreen] = useState(false);

  // 1. Fetch Terminal & Host OS Info from Backend
  useEffect(() => {
    let isMounted = true;
    api.getTerminalInfo()
      .then((info) => {
        if (!isMounted) return;
        setTerminalInfo(info);
        if (info.default_shell) {
          setSelectedShell(info.default_shell);
        }
      })
      .catch((err: unknown) => {
        console.warn('Failed to fetch terminal info, using fallback defaults:', err);
      });

    return () => {
      isMounted = false;
    };
  }, []);

  // 2. Initialize and connect WebSocket to backend PTY
  const startSession = (shell: string) => {
    if (socketRef.current) {
      socketRef.current.close();
      socketRef.current = null;
    }

    const term = terminalRef.current;
    if (!term) return;

    setStatus('connecting');
    term.clear();

    const cols = term.cols || 120;
    const rows = term.rows || 30;

    const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${wsProtocol}//${window.location.host}/api/terminal/ws?shell=${encodeURIComponent(shell)}&cols=${cols}&rows=${rows}`;

    try {
      const socket = new WebSocket(wsUrl);
      socketRef.current = socket;

      socket.onopen = () => {
        setStatus('connected');
        term.focus();
        if (socket.readyState === WebSocket.OPEN) {
          socket.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }));
        }
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
        term.writeln('\r\n\x1b[33m⚡ Terminal session ended.\x1b[0m\r\n');
      };

      socket.onerror = () => {
        setStatus('error');
        term.writeln('\r\n\x1b[31m✖ WebSocket connection error.\x1b[0m\r\n');
      };
    } catch (err) {
      setStatus('error');
      term.writeln(`\r\n\x1b[31m✖ Failed to connect: ${err}\x1b[0m\r\n`);
    }
  };

  // 3. Mount Xterm and bind keyboard I/O
  useEffect(() => {
    if (!terminalNodeRef.current) return;

    const term = new Terminal({
      cursorBlink: true,
      cursorStyle: 'bar',
      cursorWidth: 2,
      fontFamily: '"Cascadia Code", "Fira Code", "JetBrains Mono", Consolas, monospace',
      fontSize,
      lineHeight: 1.2,
      letterSpacing: 0,
      scrollback: 5000,
      allowTransparency: true,
      theme: {
        background: '#090814',
        foreground: '#f8fafc',
        cursor: '#ffe228',
        cursorAccent: '#090814',
        selectionBackground: 'rgba(255, 226, 40, 0.25)',
        black: '#17142b',
        red: '#f87171',
        green: '#34d399',
        yellow: '#fbbf24',
        blue: '#60a5fa',
        magenta: '#e879f9',
        cyan: '#38bdf8',
        white: '#f1f5f9',
        brightBlack: '#64748b',
        brightRed: '#ef4444',
        brightGreen: '#10b981',
        brightYellow: '#f59e0b',
        brightBlue: '#3b82f6',
        brightMagenta: '#d946ef',
        brightCyan: '#06b6d4',
        brightWhite: '#ffffff',
      },
    });

    const fitAddon = new FitAddon();
    term.loadAddon(fitAddon);
    term.open(terminalNodeRef.current);

    terminalRef.current = term;
    fitAddonRef.current = fitAddon;

    // Send keystrokes directly to WebSocket backend
    const onDataDisposable = term.onData((data) => {
      if (socketRef.current && socketRef.current.readyState === WebSocket.OPEN) {
        socketRef.current.send(data);
      }
    });

    // Auto-fit on resize
    const handleResize = () => {
      try {
        fitAddon.fit();
        if (socketRef.current && socketRef.current.readyState === WebSocket.OPEN) {
          socketRef.current.send(
            JSON.stringify({
              type: 'resize',
              cols: term.cols,
              rows: term.rows,
            })
          );
        }
      } catch {
        // ignore resize layout errors before container mounted
      }
    };

    window.addEventListener('resize', handleResize);
    setTimeout(handleResize, 100);

    // Start initial session
    startSession(selectedShell);

    return () => {
      window.removeEventListener('resize', handleResize);
      onDataDisposable.dispose();
      term.dispose();
      terminalRef.current = null;
      if (socketRef.current) {
        socketRef.current.close();
      }
    };
  }, []);

  // Update Font size in real-time
  useEffect(() => {
    if (terminalRef.current && fitAddonRef.current) {
      terminalRef.current.options.fontSize = fontSize;
      fitAddonRef.current.fit();
      if (socketRef.current && socketRef.current.readyState === WebSocket.OPEN) {
        socketRef.current.send(
          JSON.stringify({
            type: 'resize',
            cols: terminalRef.current.cols,
            rows: terminalRef.current.rows,
          })
        );
      }
    }
  }, [fontSize]);

  const handleShellChange = (shell: string) => {
    setSelectedShell(shell);
    startSession(shell);
  };

  const handleClear = () => {
    if (terminalRef.current) {
      terminalRef.current.clear();
      terminalRef.current.focus();
    }
  };

  const isWindows = terminalInfo ? terminalInfo.os === 'windows' : false;

  return (
    <div className={`w-full mx-auto px-4 md:px-8 py-8 md:py-12 ${isFullscreen ? 'max-w-none fixed inset-0 z-50 bg-[#090814] p-4 m-0 flex flex-col' : 'max-w-[1200px]'}`}>
      {/* Page Header */}
      {!isFullscreen && (
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
          <div className='mb-6'>
            <div className="flex items-center gap-2">
              <h1 className="font-serif text-heading text-deep-ink sm:text-heading-lg">
                {t('title', 'Web Terminal')}
              </h1>
              <Badge variant="neutral" className="gap-1 font-mono text-[11px] uppercase">
                <Server className="w-3 h-3" />
                {terminalInfo?.os || 'System'} PTY
              </Badge>
            </div>
            <p className="mt-1 max-w-3xl text-body-sm text-slate sm:text-body">
              {t('subtitle', 'Interactive pseudo-terminal connection directly into the ActonOS runtime kernel.')}
            </p>
          </div>
        </div>
      )}

      {/* Terminal Main Container */}
      <div className={`flex flex-col rounded-2xl border border-[#2d274f] bg-[#090814] shadow-2xl overflow-hidden transition-all ${isFullscreen ? 'flex-1 min-h-0' : 'h-[680px]'}`}>
        {/* Top Terminal Bar */}
        <div className="flex items-center justify-between px-4 py-3 bg-[#130e30] border-b border-[#251f47] select-none">
          {/* Left: Window Dots & Title */}
          <div className="flex items-center gap-3">
            <div className="flex items-center gap-1.5">
              <div className="w-3 h-3 rounded-full bg-rose-500/80 border border-rose-600/50" />
              <div className="w-3 h-3 rounded-full bg-amber-500/80 border border-amber-600/50" />
              <div className="w-3 h-3 rounded-full bg-emerald-500/80 border border-emerald-600/50" />
            </div>
            <div className="flex items-center gap-2 pl-2 border-l border-white/10">
              <TerminalIcon className="w-4 h-4 text-amber-400 shrink-0" />
              <span className="text-xs font-mono font-medium text-white/90 hidden sm:inline">
                ActonOS Pseudo-Terminal ({isWindows ? 'ConPTY' : 'POSIX PTY'})
              </span>
            </div>

            {/* Status Badge */}
            <Badge
              variant={
                status === 'connected' ? 'success' :
                  status === 'connecting' ? 'neutral' :
                    'stopped'
              }
              className="ml-2 hidden sm:inline-flex py-0.5 text-[11px]"
            >
              {status === 'connected' && <CheckCircle2 className="w-3 h-3 mr-1 text-emerald-400" />}
              {status === 'connecting' && <Clock className="w-3 h-3 mr-1 animate-spin text-amber-400" />}
              {(status === 'disconnected' || status === 'error') && <AlertCircle className="w-3 h-3 mr-1 text-rose-400" />}
              {t(`status.${status}`, status)}
            </Badge>
          </div>

          {/* Right: Controls & Actions */}
          <div className="flex items-center gap-2 shrink-0">
            {/* Shell Switcher */}
            <div className="relative inline-flex items-center">
              <select
                value={selectedShell}
                onChange={(e) => handleShellChange(e.target.value)}
                className="appearance-none bg-[#1d1738] hover:bg-[#251f47] text-white/90 font-mono text-[12px] pl-3 pr-7 py-1.5 rounded-lg border border-white/15 focus:outline-none focus:border-amber-400/50 cursor-pointer transition-colors"
                title="Select shell environment"
              >
                {terminalInfo?.available_shells && terminalInfo.available_shells.length > 0 ? (
                  terminalInfo.available_shells.map((sh) => (
                    <option key={sh.id} value={sh.id}>
                      {sh.name}
                    </option>
                  ))
                ) : isWindows ? (
                  <>
                    <option value="powershell">PowerShell (ConPTY)</option>
                    <option value="cmd">Command Prompt (CMD)</option>
                    <option value="bash">WSL / Linux</option>
                  </>
                ) : (
                  <>
                    <option value="bash">Bash (/bin/bash)</option>
                    <option value="sh">POSIX Shell (/bin/sh)</option>
                  </>
                )}
              </select>
              <ChevronDown className="w-3.5 h-3.5 text-white/50 absolute right-2 pointer-events-none" />
            </div>

            {/* Font Zoom Controls */}
            <div className="hidden md:flex items-center gap-1 bg-[#1d1738] rounded-lg p-0.5 border border-white/15">
              <button
                type="button"
                onClick={() => setFontSize((s) => Math.max(10, s - 1))}
                className="p-1 rounded text-white/70 hover:text-white hover:bg-white/10 transition-colors"
                title="Zoom Out"
              >
                <ZoomOut className="w-3.5 h-3.5" />
              </button>
              <span className="text-[11px] font-mono text-white/60 px-1">{fontSize}px</span>
              <button
                type="button"
                onClick={() => setFontSize((s) => Math.min(24, s + 1))}
                className="p-1 rounded text-white/70 hover:text-white hover:bg-white/10 transition-colors"
                title="Zoom In"
              >
                <ZoomIn className="w-3.5 h-3.5" />
              </button>
            </div>

            {/* Clear Button */}
            <Button
              variant="ghost"
              size="sm"
              onClick={handleClear}
              className="text-white/70 hover:text-white hover:bg-white/10 h-8 px-2"
              title="Clear terminal screen"
              icon={<Trash2 className="w-3.5 h-3.5" />}
            >
              <span className="hidden lg:inline text-xs ml-1.5">{t('actions.clear', 'Clear')}</span>
            </Button>

            {/* Reconnect Button */}
            <Button
              variant="ghost"
              size="sm"
              onClick={() => startSession(selectedShell)}
              className="text-white/70 hover:text-white hover:bg-white/10 h-8 px-2"
              title="Reconnect terminal session"
              icon={<RefreshCw className="w-3.5 h-3.5" />}
            >
              <span className="hidden lg:inline text-xs ml-1.5">{t('actions.reconnect', 'Reconnect')}</span>
            </Button>

            {/* Fullscreen Toggle */}
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                setIsFullscreen(!isFullscreen);
                setTimeout(() => {
                  if (fitAddonRef.current && terminalRef.current) {
                    fitAddonRef.current.fit();
                    if (socketRef.current && socketRef.current.readyState === WebSocket.OPEN) {
                      socketRef.current.send(
                        JSON.stringify({
                          type: 'resize',
                          cols: terminalRef.current.cols,
                          rows: terminalRef.current.rows,
                        })
                      );
                    }
                  }
                }, 100);
              }}
              className="text-white/70 hover:text-white hover:bg-white/10 h-8 px-2"
              title={isFullscreen ? 'Exit Fullscreen' : 'Fullscreen'}
              icon={isFullscreen ? <Minimize2 className="w-3.5 h-3.5" /> : <Maximize2 className="w-3.5 h-3.5" />}
            >
            </Button>
          </div>
        </div>

        {/* Xterm DOM Mount Container */}
        <div
          ref={terminalNodeRef}
          className="flex-1 w-full p-3 overflow-hidden bg-[#090814] cursor-text"
          onClick={() => terminalRef.current?.focus()}
        />

        {/* Bottom Status Bar */}
        <div className="flex items-center justify-between px-4 py-1.5 bg-[#0e0a24] border-t border-[#1d1738] text-[11px] font-mono text-white/50 select-none">
          <div className="flex items-center gap-3">
            <span className="flex items-center gap-1">
              <Sparkles className="w-3 h-3 text-amber-400" />
              ActonOS Sandbox Environment
            </span>
            <span className="hidden sm:inline">•</span>
            <span className="hidden sm:inline">UTF-8 / xterm-256color</span>
          </div>
          <div className="flex items-center gap-3">
            <span>{isWindows ? 'Windows ConPTY' : 'Linux POSIX PTY (/dev/pts)'}</span>
            <span>•</span>
            <span className="text-white/70">{selectedShell}</span>
          </div>
        </div>
      </div>
    </div>
  );
}
