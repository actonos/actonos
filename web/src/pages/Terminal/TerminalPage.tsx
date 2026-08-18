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
} from 'lucide-react';
import { Button } from '@/components/ui/Button';
import { Badge } from '@/components/ui/Badge';

type ShellType = 'powershell' | 'cmd' | 'bash';

export function TerminalPage() {
  const { t } = useTranslation('terminal');
  const terminalNodeRef = useRef<HTMLDivElement>(null);
  const terminalRef = useRef<Terminal | null>(null);
  const fitAddonRef = useRef<FitAddon | null>(null);
  const socketRef = useRef<WebSocket | null>(null);

  const [status, setStatus] = useState<'connecting' | 'connected' | 'disconnected' | 'error'>('connecting');
  const [selectedShell, setSelectedShell] = useState<ShellType>('powershell');
  const [fontSize, setFontSize] = useState<number>(13);
  const [isFullscreen, setIsFullscreen] = useState(false);

  // Initialize and connect WebSocket to backend PTY
  const startSession = (shell: ShellType) => {
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
    const wsUrl = `${wsProtocol}//${window.location.host}/api/terminal/ws?shell=${shell}&cols=${cols}&rows=${rows}`;

    try {
      const socket = new WebSocket(wsUrl);
      socketRef.current = socket;

      socket.onopen = () => {
        setStatus('connected');
        term.focus();
        // Send initial exact dimensions to PTY
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

    setTimeout(() => {
      fitAddon.fit();
    }, 100);

    // Forward raw user keystrokes into WebSocket
    const onDataDisposable = term.onData((data) => {
      if (socketRef.current && socketRef.current.readyState === WebSocket.OPEN) {
        socketRef.current.send(data);
      }
    });

    // Resize handler sends resize message to PTY
    const handleResize = () => {
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
    };
    window.addEventListener('resize', handleResize);

    startSession(selectedShell);

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
  }, []);

  // Update font size dynamically
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

  const handleShellChange = (newShell: ShellType) => {
    setSelectedShell(newShell);
    startSession(newShell);
  };

  const handleClear = () => {
    if (terminalRef.current) {
      terminalRef.current.clear();
      terminalRef.current.focus();
    }
  };

  const handleReconnect = () => {
    startSession(selectedShell);
  };

  const isWindows = typeof navigator !== 'undefined' && navigator.userAgent.includes('Windows');

  return (
    <div
      className={`flex flex-col w-full transition-all duration-200 ${
        isFullscreen
          ? 'fixed inset-0 z-50 bg-[#090814] p-0'
          : 'h-[calc(100vh-4rem)] p-4 sm:p-6 bg-canvas'
      }`}
    >
      {/* Container Card */}
      <div
        className={`flex flex-col flex-1 w-full bg-[#090814] border border-onyx/15 shadow-2xl overflow-hidden ${
          isFullscreen ? 'rounded-none' : 'rounded-2xl'
        }`}
      >
        {/* Top Professional Terminal Bar */}
        <div className="h-12 px-4 bg-[#130f26] border-b border-white/10 flex items-center justify-between gap-3 select-none">
          {/* Left: Window Dots & Title */}
          <div className="flex items-center gap-3 min-w-0">
            <div className="flex items-center gap-1.5 shrink-0">
              <span className="w-3 h-3 rounded-full bg-rose-500/90 shadow-xs inline-block" />
              <span className="w-3 h-3 rounded-full bg-amber-500/90 shadow-xs inline-block" />
              <span className="w-3 h-3 rounded-full bg-emerald-500/90 shadow-xs inline-block" />
            </div>

            <div className="h-4 w-px bg-white/15 mx-1 shrink-0" />

            <div className="flex items-center gap-2 min-w-0">
              <TerminalIcon className="w-4 h-4 text-amber-400 shrink-0" />
              <span className="font-mono text-body-sm font-semibold text-white truncate">
                ActonOS Pseudo-Terminal (PTY)
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
                onChange={(e) => handleShellChange(e.target.value as ShellType)}
                className="appearance-none bg-[#1d1738] hover:bg-[#251f47] text-white/90 font-mono text-[12px] pl-3 pr-7 py-1.5 rounded-lg border border-white/15 focus:outline-none focus:border-amber-400/50 cursor-pointer transition-colors"
                title="Select shell environment"
              >
                {isWindows ? (
                  <>
                    <option value="powershell">PowerShell (ConPTY)</option>
                    <option value="cmd">Command Prompt (CMD)</option>
                    <option value="bash">WSL / Linux</option>
                  </>
                ) : (
                  <>
                    <option value="bash">Bash</option>
                    <option value="sh">POSIX Shell</option>
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
              icon={<Trash2 className="w-3.5 h-3.5" />}
              onClick={handleClear}
              className="text-white/80 hover:text-white hover:bg-white/10"
              title="Clear terminal screen"
            >
              <span className="hidden sm:inline">{t('actions.clear', 'Clear')}</span>
            </Button>

            {/* Reconnect Button */}
            <Button
              variant="secondary"
              size="sm"
              icon={<RefreshCw className={`w-3.5 h-3.5 ${status === 'connecting' ? 'animate-spin' : ''}`} />}
              onClick={handleReconnect}
              className="bg-amber-400/15 text-amber-300 hover:bg-amber-400/25 border-amber-400/30"
              title="Reconnect terminal session"
            >
              <span className="hidden sm:inline">{t('actions.reconnect', 'Reconnect')}</span>
            </Button>

            {/* Fullscreen Toggle */}
            <button
              type="button"
              onClick={() => {
                setIsFullscreen((f) => !f);
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
                }, 120);
              }}
              className="p-2 rounded-lg bg-[#1d1738] hover:bg-[#251f47] text-white/70 hover:text-white border border-white/15 transition-colors cursor-pointer"
              title={isFullscreen ? 'Exit Fullscreen' : 'Fullscreen'}
            >
              {isFullscreen ? <Minimize2 className="w-3.5 h-3.5" /> : <Maximize2 className="w-3.5 h-3.5" />}
            </button>
          </div>
        </div>

        {/* XTerm Screen Container */}
        <div
          ref={terminalNodeRef}
          className="flex-1 w-full p-4 overflow-hidden cursor-text bg-[#090814]"
          onClick={() => terminalRef.current?.focus()}
        />

        {/* Bottom Status Bar */}
        <div className="h-7 px-4 bg-[#0d0a1d] border-t border-white/5 flex items-center justify-between text-[11px] font-mono text-white/50 select-none">
          <div className="flex items-center gap-3">
            <span className="flex items-center gap-1.5">
              <span className={`w-2 h-2 rounded-full ${status === 'connected' ? 'bg-emerald-400 animate-pulse' : 'bg-rose-400'}`} />
              {status === 'connected' ? 'LIVE PTY' : 'OFFLINE'}
            </span>
            <span>UTF-8</span>
            <span>ConPTY Virtual Terminal</span>
          </div>

          <div className="flex items-center gap-3">
            <span className="hidden sm:inline flex items-center gap-1 text-white/40">
              <Sparkles className="w-3 h-3 text-amber-400/60" /> ActonOS Kernel Direct I/O
            </span>
          </div>
        </div>
      </div>
    </div>
  );
}
