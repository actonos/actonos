import {
  useState,
  useEffect,
  useRef,
  useMemo,
  type FormEvent,
  type KeyboardEvent,
  type ClipboardEvent,
  type DragEvent,
  type RefObject,
} from 'react';
import {
  Send,
  Paperclip,
  Mic,
  Square,
  X,
  FileCode,
  FileText,
  FileSpreadsheet,
  Image as ImageIcon,
  Folder,
  Check,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/Button';
import { useToast } from '@/components/ui/Toast';
import type { ToolInfo } from '@/lib/types';
import {
  ChatAutocompletePopover,
  getFilteredSkills,
  getFilteredFiles,
  type AutocompleteSkillItem,
  type AutocompleteFileItem,
} from './ChatAutocompletePopover';
import { extractTextFromDocument } from '@/lib/documentParser';
import { isImageFile, preprocessImage } from '@/lib/imagePreprocessor';

export interface AttachedFile {
  id: string;
  file?: File;
  name: string;
  size: number;
  type: string;
  previewUrl?: string;
  thumbnailUrl?: string;
  textContent?: string;
  file_id?: string;
  path?: string;
  isWorkspace?: boolean;
  isImage?: boolean;
  width?: number;
  height?: number;
}

export interface ChatComposerProps {
  value: string;
  loading: boolean;
  inputRef?: RefObject<HTMLTextAreaElement | null>;
  skills?: ToolInfo[];
  workspaceFiles?: AutocompleteFileItem[];
  pendingAttachments?: AttachedFile[];
  onClearPendingAttachments?: () => void;
  onChange: (value: string) => void;
  onSubmit: (event: FormEvent, attachments?: AttachedFile[]) => void;
  onCancelLoading?: () => void;
}

// Max file size for text/code/doc analysis: 15MB
export const MAX_ATTACHMENT_SIZE_BYTES = 15 * 1024 * 1024;
// Max characters injected per file before truncating (safeguards LLM context): 120,000 chars (~30k tokens)
export const MAX_TEXT_INJECTION_CHARS = 120000;

export const CODE_EXTENSIONS = new Set([
  'js', 'jsx', 'ts', 'tsx', 'mjs', 'cjs', 'vue', 'svelte', 'astro', 'html', 'htm', 'css', 'scss', 'sass', 'less',
  'go', 'py', 'pyw', 'rs', 'c', 'cpp', 'cc', 'cxx', 'h', 'hpp', 'hxx', 'java', 'kt', 'kts', 'cs', 'fs', 'php', 'rb', 'pl', 'lua', 'r', 'swift', 'dart', 'scala', 'zig', 'ex', 'exs', 'erl', 'clj', 'hs', 'nim', 'v', 'jl',
  'sh', 'bash', 'zsh', 'fish', 'ps1', 'bat', 'cmd',
  'json', 'json5', 'jsonc', 'yaml', 'yml', 'toml', 'ini', 'conf', 'config', 'xml', 'sql', 'prisma', 'graphql', 'gql', 'proto', 'env', 'properties',
  'dockerfile', 'makefile', 'cmake', 'gradle', 'tf', 'hcl'
]);

export const DOC_TEXT_EXTENSIONS = new Set([
  'txt', 'md', 'markdown', 'mdx', 'rst', 'tex', 'log', 'csv', 'tsv', 'diff', 'patch', 'asciidoc', 'adoc', 'org', 'rtf',
  'pdf', 'docx', 'xlsx', 'xls', 'ods'
]);

export const SPECIAL_TEXT_FILES = new Set([
  'dockerfile', 'makefile', 'cmakelists.txt', 'gemfile', 'procfile', 'rakefile', 'vagrantfile',
  'license', 'licence', 'readme', 'changelog', 'authors', 'owners', 'contributing',
  '.env', '.env.local', '.env.development', '.env.production', '.env.test', '.env.example',
  '.gitignore', '.dockerignore', '.npmignore', '.eslintignore', '.prettierignore',
  '.editorconfig', '.eslintrc', '.prettierrc', '.babelrc'
]);

export function isSupportedTextOrCodeFile(name: string, mimeType?: string): boolean {
  if (isImageFile(name, mimeType)) return true;

  const cleanName = name.trim();
  const lowerName = cleanName.toLowerCase();

  if (SPECIAL_TEXT_FILES.has(lowerName)) return true;
  if (
    lowerName.startsWith('.env') ||
    lowerName.startsWith('.git') ||
    lowerName.startsWith('.docker') ||
    lowerName.startsWith('.eslint') ||
    lowerName.startsWith('.prettier')
  ) {
    return true;
  }

  const parts = cleanName.split('.');
  const ext = parts.length > 1 ? parts.pop()?.toLowerCase() || '' : '';

  if (ext && (CODE_EXTENSIONS.has(ext) || DOC_TEXT_EXTENSIONS.has(ext))) {
    return true;
  }

  if (
    mimeType &&
    (mimeType.startsWith('text/') ||
      mimeType.startsWith('image/') ||
      mimeType === 'application/json' ||
      mimeType === 'application/xml' ||
      mimeType === 'application/javascript' ||
      mimeType === 'application/typescript' ||
      mimeType === 'application/x-yaml' ||
      mimeType === 'application/sql' ||
      mimeType === 'application/x-sh' ||
      mimeType === 'application/pdf' ||
      mimeType === 'application/vnd.openxmlformats-officedocument.wordprocessingml.document' ||
      mimeType === 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' ||
      mimeType === 'application/vnd.ms-excel' ||
      mimeType === 'application/msword' ||
      mimeType === 'application/vnd.oasis.opendocument.spreadsheet')
  ) {
    return true;
  }

  return false;
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

function getFileIcon(filename: string) {
  const ext = filename.split('.').pop()?.toLowerCase() || '';
  if (isImageFile(filename)) return ImageIcon;
  if (CODE_EXTENSIONS.has(ext)) return FileCode;
  if (['csv', 'tsv', 'xlsx', 'xls', 'ods'].includes(ext)) return FileSpreadsheet;
  return FileText;
}

interface SpeechRecognitionEventLike {
  resultIndex: number;
  results: {
    length: number;
    [index: number]: {
      isFinal?: boolean;
      [index: number]: {
        transcript: string;
      };
    };
  };
}

interface SpeechRecognitionErrorEventLike {
  error: string;
  message?: string;
}

interface SpeechRecognitionInstance {
  lang: string;
  continuous: boolean;
  interimResults: boolean;
  maxAlternatives?: number;
  onresult: ((event: SpeechRecognitionEventLike) => void) | null;
  onerror: ((event: SpeechRecognitionErrorEventLike) => void) | null;
  onend: (() => void) | null;
  onstart?: (() => void) | null;
  start: () => void;
  stop: () => void;
  abort?: () => void;
}

export function ChatComposer({
  value,
  loading,
  inputRef,
  skills = [],
  workspaceFiles = [],
  pendingAttachments = [],
  onClearPendingAttachments,
  onChange,
  onSubmit,
  onCancelLoading,
}: ChatComposerProps) {
  const { t, i18n } = useTranslation('chat');
  const { error: toastError } = useToast();
  const internalRef = useRef<HTMLTextAreaElement | null>(null);
  const textareaRef = inputRef || internalRef;
  const fileInputRef = useRef<HTMLInputElement>(null);

  // Attachments State
  const [attachments, setAttachments] = useState<AttachedFile[]>([]);
  const [isDragging, setIsDragging] = useState(false);

  // Sync pending attachments (e.g. from Workspace page or external actions)
  useEffect(() => {
    if (pendingAttachments && pendingAttachments.length > 0) {
      setAttachments((prev) => {
        const existingIds = new Set(prev.map((a) => a.id));
        const newOnes = pendingAttachments.filter((a) => !existingIds.has(a.id));
        return [...prev, ...newOnes];
      });
      onClearPendingAttachments?.();
    }
  }, [pendingAttachments, onClearPendingAttachments]);

  // Voice Recording State
  const [isRecording, setIsRecording] = useState(false);
  const [recordingSeconds, setRecordingSeconds] = useState(0);
  const isRecordingRef = useRef(false);
  const baseTextRef = useRef('');
  const recognitionRef = useRef<SpeechRecognitionInstance | null>(null);
  const mediaRecorderRef = useRef<MediaRecorder | null>(null);
  const audioStreamRef = useRef<MediaStream | null>(null);
  const timerRef = useRef<number | null>(null);

  // Autocomplete Popover State
  const [popoverType, setPopoverType] = useState<'slash' | 'mention' | null>(null);
  const [popoverQuery, setPopoverQuery] = useState('');
  const [popoverStartIndex, setPopoverStartIndex] = useState(0);
  const [selectedIndex, setSelectedIndex] = useState(0);

  // Auto-resize textarea
  useEffect(() => {
    if (textareaRef.current) {
      textareaRef.current.style.height = 'auto';
      textareaRef.current.style.height = `${Math.min(textareaRef.current.scrollHeight, 220)}px`;
    }
  }, [value, textareaRef]);

  // Autocomplete trigger check
  const checkAutocomplete = (text: string, cursorPosition: number) => {
    const textBefore = text.slice(0, cursorPosition);
    const lastSlash = textBefore.lastIndexOf('/');
    const lastMention = textBefore.lastIndexOf('@');

    let triggered: 'slash' | 'mention' | null = null;
    let query = '';
    let startIdx = 0;

    if (lastSlash > lastMention && lastSlash !== -1) {
      const charBefore = lastSlash === 0 ? ' ' : textBefore[lastSlash - 1];
      if (charBefore === ' ' || charBefore === '\n' || lastSlash === 0) {
        const potentialQuery = textBefore.slice(lastSlash + 1);
        if (!potentialQuery.includes(' ')) {
          triggered = 'slash';
          startIdx = lastSlash;
          query = potentialQuery;
        }
      }
    } else if (lastMention > lastSlash && lastMention !== -1) {
      const charBefore = lastMention === 0 ? ' ' : textBefore[lastMention - 1];
      if (charBefore === ' ' || charBefore === '\n' || lastMention === 0) {
        const potentialQuery = textBefore.slice(lastMention + 1);
        if (!potentialQuery.includes(' ')) {
          triggered = 'mention';
          startIdx = lastMention;
          query = potentialQuery;
        }
      }
    }

    setPopoverType(triggered);
    setPopoverQuery(query);
    setPopoverStartIndex(startIdx);
    setSelectedIndex(0);
  };

  const handleTextChange = (newVal: string) => {
    onChange(newVal);
    const cursor = (textareaRef.current?.selectionStart || 0) + (newVal.length - value.length);
    checkAutocomplete(newVal, Math.max(0, cursor));
  };

  // Add Files to Attachments (Strict validation for code, text, and documents)
  const handleAddFiles = async (files: FileList | File[]) => {
    const fileList = Array.from(files);
    if (fileList.length === 0) return;

    const existingKeys = new Set(attachments.map((a) => `${a.name}_${a.size}`));
    let hasUnsupported = false;
    let hasTooLarge = false;
    let hasEmpty = false;
    let hasDuplicate = false;

    const rawItems = await Promise.all(
      fileList.map(async (file): Promise<AttachedFile | null> => {
        const cleanName = file.name.trim();

        // 1. Empty check
        if (file.size === 0) {
          hasEmpty = true;
          return null;
        }

        // 2. Strict type check (code, text, and document formats only)
        if (!isSupportedTextOrCodeFile(cleanName, file.type)) {
          hasUnsupported = true;
          return null;
        }

        // 3. Max size check (5MB)
        if (file.size > MAX_ATTACHMENT_SIZE_BYTES) {
          hasTooLarge = true;
          return null;
        }

        // 4. Duplicate check
        const fileKey = `${cleanName}_${file.size}`;
        if (existingKeys.has(fileKey)) {
          hasDuplicate = true;
          return null;
        }
        existingKeys.add(fileKey);

        // 5. If image: preprocess and compress for AI vision analysis
        if (isImageFile(cleanName, file.type)) {
          try {
            const processed = await preprocessImage(file, cleanName, {
              maxDimension: 1600,
              quality: 0.85,
            });

            const item: AttachedFile = {
              id: `att_${Date.now()}_${Math.random().toString(36).slice(2, 6)}`,
              file,
              name: cleanName,
              size: processed.size,
              type: processed.mimeType,
              previewUrl: processed.dataUrl,
              thumbnailUrl: processed.thumbnailUrl,
              isImage: true,
              width: processed.width,
              height: processed.height,
            };
            return item;
          } catch (err: unknown) {
            console.warn('Failed to preprocess image:', err);
            toastError(
              t('attachments.parseFailed', 'Failed to process image {{name}}: {{error}}', {
                name: cleanName,
                error: err instanceof Error ? err.message : String(err),
              })
            );
            return null;
          }
        }

        // 6. If document / code / text: extract text / Markdown content
        let textContent = '';
        try {
          const parsed = await extractTextFromDocument(file, MAX_TEXT_INJECTION_CHARS);
          textContent = parsed.text;
        } catch (err: unknown) {
          console.warn('Failed to extract text from document:', err);
          toastError(
            t('attachments.parseFailed', 'Failed to extract text from {{name}}: {{error}}', {
              name: cleanName,
              error: err instanceof Error ? err.message : String(err),
            })
          );
          return null;
        }

        const item: AttachedFile = {
          id: `att_${Date.now()}_${Math.random().toString(36).slice(2, 6)}`,
          file,
          name: cleanName,
          size: file.size,
          type: file.type || 'text/plain',
          textContent,
        };
        return item;
      })
    );

    if (hasUnsupported) {
      toastError(t('attachments.unsupportedType'));
    }
    if (hasTooLarge) {
      toastError(t('attachments.fileTooLarge'));
    }
    if (hasEmpty) {
      toastError(t('attachments.emptyFile'));
    }
    if (hasDuplicate && rawItems.every((item) => item === null)) {
      toastError(t('attachments.duplicateFile'));
    }

    const valid = rawItems.filter((item): item is AttachedFile => item !== null);
    if (valid.length > 0) {
      setAttachments((prev) => [...prev, ...valid]);
    }
  };

  const handleRemoveAttachment = (id: string) => {
    setAttachments((prev) => {
      const removed = prev.find((a) => a.id === id);
      if (removed?.previewUrl) {
        URL.revokeObjectURL(removed.previewUrl);
      }
      return prev.filter((a) => a.id !== id);
    });
  };

  // Drag and Drop
  const handleDragOver = (e: DragEvent<HTMLElement>) => {
    e.preventDefault();
    e.stopPropagation();
    setIsDragging(true);
  };

  const handleDragLeave = (e: DragEvent<HTMLElement>) => {
    e.preventDefault();
    e.stopPropagation();
    setIsDragging(false);
  };

  const handleDrop = (e: DragEvent<HTMLElement>) => {
    e.preventDefault();
    e.stopPropagation();
    setIsDragging(false);
    if (e.dataTransfer.files && e.dataTransfer.files.length > 0) {
      handleAddFiles(e.dataTransfer.files);
    }
  };

  // Clipboard Paste Support
  const handlePaste = (e: ClipboardEvent<HTMLTextAreaElement>) => {
    if (e.clipboardData.files && e.clipboardData.files.length > 0) {
      handleAddFiles(e.clipboardData.files);
    }
  };

  // Start Voice Recording (Speech-to-Text + MediaRecorder)
  const startRecording = async () => {
    const windowWithSpeech = window as unknown as {
      SpeechRecognition?: new () => SpeechRecognitionInstance;
      webkitSpeechRecognition?: new () => SpeechRecognitionInstance;
    };
    const SpeechRecognitionConstructor =
      windowWithSpeech.SpeechRecognition || windowWithSpeech.webkitSpeechRecognition;

    const hasMedia = Boolean(navigator.mediaDevices && navigator.mediaDevices.getUserMedia);

    if (!SpeechRecognitionConstructor && !hasMedia) {
      toastError(t('voice.notSupported', 'Voice input is not supported in this browser or requires HTTPS / localhost'));
      return;
    }

    try {
      baseTextRef.current = value;
      isRecordingRef.current = true;
      setIsRecording(true);
      setRecordingSeconds(0);

      // 1. Timer
      if (timerRef.current) clearInterval(timerRef.current);
      timerRef.current = window.setInterval(() => {
        setRecordingSeconds((prev) => prev + 1);
      }, 1000);

      // 2. Setup Web Speech Recognition
      if (SpeechRecognitionConstructor) {
        const recognition = new SpeechRecognitionConstructor();
        recognition.lang = i18n.language === 'vi' ? 'vi-VN' : 'en-US';
        recognition.continuous = true;
        recognition.interimResults = true;
        recognition.maxAlternatives = 1;

        recognition.onresult = (event: SpeechRecognitionEventLike) => {
          let final = '';
          let interim = '';
          for (let i = 0; i < event.results.length; i++) {
            const res = event.results[i];
            if (res.isFinal) {
              final += res[0].transcript;
            } else {
              interim += res[0].transcript;
            }
          }
          const speechText = (final + (interim ? ' ' + interim : '')).trim();
          const base = baseTextRef.current.trim();
          const nextVal = base ? `${base} ${speechText}` : speechText;
          onChange(nextVal);
        };

        recognition.onerror = (event: SpeechRecognitionErrorEventLike) => {
          if (event.error === 'no-speech' || event.error === 'aborted') {
            // Keep listening
            return;
          }
          if (event.error === 'not-allowed' || event.error === 'service-not-allowed') {
            toastError(t('voice.permissionDenied', 'Microphone access was denied. Please allow microphone permissions in your browser.'));
            stopRecording();
            return;
          }
        };

        recognition.onend = () => {
          if (isRecordingRef.current) {
            try {
              recognition.start();
            } catch {
              // Ignore restart error
            }
          }
        };

        recognitionRef.current = recognition;
        try {
          recognition.start();
        } catch {
          // Fallback if recognition fails
        }
      }

      // 3. Audio capture for pulse visualizer
      if (hasMedia) {
        try {
          const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
          audioStreamRef.current = stream;
          if (typeof MediaRecorder !== 'undefined') {
            const mediaRecorder = new MediaRecorder(stream);
            mediaRecorderRef.current = mediaRecorder;
            mediaRecorder.start();
          }
        } catch (streamErr: unknown) {
          const errName = (streamErr as { name?: string })?.name;
          if (errName === 'NotAllowedError' || errName === 'PermissionDeniedError') {
            toastError(t('voice.permissionDenied', 'Microphone access was denied. Please allow microphone permissions in your browser.'));
            stopRecording();
          }
        }
      }
    } catch {
      toastError(t('voice.notSupported', 'Failed to start voice input.'));
      stopRecording();
    }
  };

  // Stop Recording
  const stopRecording = () => {
    isRecordingRef.current = false;
    setIsRecording(false);
    if (timerRef.current) {
      clearInterval(timerRef.current);
      timerRef.current = null;
    }
    if (recognitionRef.current) {
      try {
        recognitionRef.current.stop();
      } catch {
        // Ignore
      }
      recognitionRef.current = null;
    }
    if (mediaRecorderRef.current && mediaRecorderRef.current.state !== 'inactive') {
      try {
        mediaRecorderRef.current.stop();
      } catch {
        // Ignore
      }
      mediaRecorderRef.current = null;
    }
    if (audioStreamRef.current) {
      audioStreamRef.current.getTracks().forEach((track) => track.stop());
      audioStreamRef.current = null;
    }
  };

  const handleCancelRecording = () => {
    stopRecording();
  };

  // Autocomplete Item Selection
  const handleSelectSkill = (item: AutocompleteSkillItem) => {
    const before = value.slice(0, popoverStartIndex);
    const after = value.slice(popoverStartIndex + 1 + popoverQuery.length);
    const nextVal = `${before}${item.command} ${after}`;
    onChange(nextVal);
    setPopoverType(null);
    window.setTimeout(() => {
      if (textareaRef.current) {
        textareaRef.current.focus();
        const nextPos = before.length + item.command.length + 1;
        textareaRef.current.setSelectionRange(nextPos, nextPos);
      }
    });
  };

  const handleSelectFile = (file: AutocompleteFileItem) => {
    const before = value.slice(0, popoverStartIndex);
    const after = value.slice(popoverStartIndex + 1 + popoverQuery.length);
    const token = `@[${file.path || file.name}] `;
    const nextVal = `${before}${token}${after}`;
    onChange(nextVal);
    setPopoverType(null);
    window.setTimeout(() => {
      if (textareaRef.current) {
        textareaRef.current.focus();
        const nextPos = before.length + token.length;
        textareaRef.current.setSelectionRange(nextPos, nextPos);
      }
    });
  };

  const filteredSkills = useMemo(
    () => getFilteredSkills(skills, popoverQuery),
    [skills, popoverQuery]
  );

  const filteredFiles = useMemo(
    () => getFilteredFiles(workspaceFiles, popoverQuery),
    [workspaceFiles, popoverQuery]
  );

  const activeItemCount = popoverType === 'slash' ? filteredSkills.length : filteredFiles.length;

  // Keyboard Navigation
  const handleKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (popoverType && activeItemCount > 0) {
      if (e.key === 'ArrowDown') {
        e.preventDefault();
        setSelectedIndex((prev) => (prev + 1) % activeItemCount);
        return;
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault();
        setSelectedIndex((prev) => (prev - 1 + activeItemCount) % activeItemCount);
        return;
      }
      if (e.key === 'Enter' || e.key === 'Tab') {
        e.preventDefault();
        if (popoverType === 'slash') {
          const item = filteredSkills[selectedIndex] || filteredSkills[0];
          if (item) {
            handleSelectSkill(item);
          }
        } else if (popoverType === 'mention') {
          const file = filteredFiles[selectedIndex] || filteredFiles[0];
          if (file) {
            handleSelectFile(file);
          }
        }
        return;
      }
      if (e.key === 'Escape') {
        e.preventDefault();
        setPopoverType(null);
        return;
      }
    }

    if (e.key === 'Enter' && !e.shiftKey) {
      if (popoverType) {
        setPopoverType(null);
      }
      e.preventDefault();
      if ((value.trim() || attachments.length > 0) && !loading) {
        handleSubmit(e as unknown as FormEvent);
      }
    }
  };

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault();
    if ((!value.trim() && attachments.length === 0) || loading) return;
    if (isRecording) {
      stopRecording();
    }
    setPopoverType(null);
    const currentAttachments = [...attachments];
    setAttachments([]);
    onSubmit(e, currentAttachments);
  };

  const formatTimer = (secs: number) => {
    const m = Math.floor(secs / 60);
    const s = secs % 60;
    return `${m.toString().padStart(2, '0')}:${s.toString().padStart(2, '0')}`;
  };

  return (
    <form
      onSubmit={handleSubmit}
      onDragOver={handleDragOver}
      onDragLeave={handleDragLeave}
      onDrop={handleDrop}
      className="sticky bottom-0 border-t border-onyx/10 pt-2.5 z-20"
    >
      <div
        className={`relative rounded-[24px] border bg-soft-meadow p-2 shadow-xs transition-all ${isDragging
          ? 'border-hi-yellow ring-2 ring-hi-yellow bg-soft-meadow/80'
          : 'border-onyx/15 focus-within:ring-2 focus-within:ring-deep-ink'
          }`}
      >
        {/* Autocomplete Popover */}
        <ChatAutocompletePopover
          type={popoverType}
          query={popoverQuery}
          filteredSkills={filteredSkills}
          filteredFiles={filteredFiles}
          selectedIndex={selectedIndex}
          onSelectSkill={handleSelectSkill}
          onSelectFile={handleSelectFile}
          onHoverIndex={setSelectedIndex}
          onClose={() => setPopoverType(null)}
        />

        {/* Attached Files Chips */}
        {attachments.length > 0 && (
          <div className="flex flex-wrap gap-2 px-2.5 pt-1.5 pb-2 border-b border-onyx/10 mb-1.5">
            {attachments.map((att) => {
              const Icon = getFileIcon(att.name);
              return (
                <div
                  key={att.id}
                  className="flex items-center gap-2 rounded-full border border-onyx/15 bg-canvas px-3 py-1 text-xs font-medium text-deep-ink shadow-xs animate-fadeIn"
                >
                  {att.previewUrl ? (
                    <img
                      src={att.previewUrl}
                      alt={att.name}
                      className="w-4 h-4 rounded-full object-cover shrink-0"
                    />
                  ) : att.isWorkspace ? (
                    <Folder className="w-3.5 h-3.5 text-hi-yellow shrink-0" />
                  ) : (
                    <Icon className="w-3.5 h-3.5 text-slate shrink-0" />
                  )}
                  <span className="font-mono text-[11px] truncate max-w-[140px]">
                    {att.name}
                  </span>
                  {att.isWorkspace && (
                    <span className="px-1.5 py-0.5 rounded-sm bg-hi-yellow/20 text-deep-ink text-[9px] font-semibold uppercase tracking-wider">
                      Workspace
                    </span>
                  )}
                  <span className="text-[10px] text-slate/70">
                    ({formatBytes(att.size)})
                  </span>
                  <button
                    type="button"
                    onClick={() => handleRemoveAttachment(att.id)}
                    className="ml-0.5 rounded-full p-0.5 text-slate hover:bg-onyx/10 hover:text-deep-ink transition-colors"
                    aria-label={t('attachments.removeFile')}
                  >
                    <X className="w-3 h-3" />
                  </button>
                </div>
              );
            })}
          </div>
        )}

        {/* Main Input Textarea */}
        <div className="flex items-end gap-2">
          {isRecording ? (
            /* Voice Recording Bar */
            <div className="flex-1 flex items-center justify-between px-3 py-2 bg-canvas rounded-2xl border border-onyx/10 animate-pulse">
              <div className="flex items-center gap-3">
                <span className="relative flex h-3 w-3">
                  <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-red-400 opacity-75" />
                  <span className="relative inline-flex rounded-full h-3 w-3 bg-red-500" />
                </span>
                <span className="font-sans text-body-sm font-semibold text-deep-ink">
                  {t('voice.recording')}
                </span>
                <span className="font-mono text-xs font-semibold text-slate bg-onyx/5 px-2 py-0.5 rounded-full">
                  {formatTimer(recordingSeconds)}
                </span>
              </div>
              <div className="flex items-center gap-1.5">
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  onClick={handleCancelRecording}
                  icon={<X className="w-3.5 h-3.5" />}
                  className="text-slate hover:text-deep-ink"
                >
                  {t('voice.cancelRecording')}
                </Button>
                <Button
                  type="button"
                  variant="primary"
                  size="sm"
                  onClick={stopRecording}
                  icon={<Check className="w-3.5 h-3.5" />}
                  className="font-semibold px-3"
                >
                  {t('voice.stopRecording')}
                </Button>
              </div>
            </div>
          ) : (
            /* Normal Textarea */
            <textarea
              ref={textareaRef}
              rows={1}
              placeholder={t('placeholder')}
              value={value}
              onChange={(e) => handleTextChange(e.target.value)}
              onKeyDown={handleKeyDown}
              onPaste={handlePaste}
              className="min-w-0 flex-1 bg-transparent px-3 py-1.5 text-body-sm text-deep-ink placeholder:text-slate focus:outline-none resize-none leading-relaxed overflow-y-auto"
              disabled={loading}
            />
          )}

          {/* Action Buttons */}
          <div className="flex items-center gap-1 shrink-0 pb-0.5">
            {/* Hidden File Input (Code, text, documents, and images) */}
            <input
              ref={fileInputRef}
              type="file"
              multiple
              accept=".txt,.md,.markdown,.json,.yaml,.yml,.toml,.xml,.csv,.tsv,.sql,.ts,.tsx,.js,.jsx,.py,.go,.rs,.c,.cpp,.h,.hpp,.java,.kt,.cs,.php,.rb,.swift,.dart,.html,.css,.scss,.sh,.bash,.ps1,.env,.log,.diff,.patch,.pdf,.docx,.xlsx,.xls,.ods,.png,.jpg,.jpeg,.webp,.gif,.svg,.bmp,image/*,text/*"
              className="hidden"
              onChange={(e) => {
                if (e.target.files && e.target.files.length > 0) {
                  handleAddFiles(e.target.files);
                  e.target.value = '';
                }
              }}
            />

            {!isRecording && (
              <>
                {/* Attach File Button */}
                <button
                  type="button"
                  onClick={() => fileInputRef.current?.click()}
                  disabled={loading}
                  title={t('attachments.attachFile')}
                  aria-label={t('attachments.attachFile')}
                  className="p-2 rounded-full text-slate hover:text-deep-ink hover:bg-canvas/80 transition-colors disabled:opacity-40"
                >
                  <Paperclip className="w-4 h-4" />
                </button>

                {/* Voice Input Button */}
                <button
                  type="button"
                  onClick={startRecording}
                  disabled={loading}
                  title={t('voice.startRecording')}
                  aria-label={t('voice.startRecording')}
                  className="p-2 rounded-full text-slate hover:text-deep-ink hover:bg-canvas/80 transition-colors disabled:opacity-40"
                >
                  <Mic className="w-4 h-4" />
                </button>
              </>
            )}

            {/* Send or Stop Button */}
            {loading ? (
              <Button
                type="button"
                variant="danger"
                size="sm"
                onClick={onCancelLoading}
                icon={<Square className="h-3.5 w-3.5 fill-current" />}
                className="shrink-0 rounded-full px-3.5 py-2 font-semibold"
              >
                {t('stop', 'Stop')}
              </Button>
            ) : (
              <Button
                type="submit"
                variant="primary"
                size="sm"
                disabled={!value.trim() && attachments.length === 0}
                icon={<Send className="h-3.5 w-3.5" />}
                className="shrink-0 rounded-full px-4 py-2 font-semibold"
              >
                {t('send')}
              </Button>
            )}
          </div>
        </div>
      </div>
    </form>
  );
}
