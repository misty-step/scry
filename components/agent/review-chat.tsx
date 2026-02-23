'use client';

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import Image from 'next/image';
import { useUser } from '@clerk/nextjs';
import type { UIMessage } from '@convex-dev/agent/react';
import { useUIMessages } from '@convex-dev/agent/react';
import { useMutation, useQuery } from 'convex/react';
import {
  ArrowLeft,
  ArrowRight,
  BarChart3,
  BookOpen,
  Layers,
  Loader2,
  MessageCircle,
  MessageSquare,
  Send,
} from 'lucide-react';
import { api } from '@/convex/_generated/api';
import { cn } from '@/lib/utils';
import { FeedbackCard } from './feedback-card';
import { MessageBubble } from './message-bubble';
import { QuestionCard } from './question-card';
import { extractLatestToolResults } from './tool-results';

const START_PROMPT = 'Start my review session. Fetch the first concept and present it.';

const SUGGESTION_CHIPS = [
  { label: 'Explain this concept', text: 'Explain this concept in more detail' },
  { label: 'Weak areas', text: 'Show my weak areas' },
  { label: 'Reschedule', text: 'Reschedule this deck' },
];

export function ReviewChat() {
  const { user } = useUser();
  const [threadId, setThreadId] = useState<string | null>(null);
  const [input, setInput] = useState('');
  const [isStarting, setIsStarting] = useState(false);
  const submittedAnswers = useRef(new Set<string>());
  const chatScrollRef = useRef<HTMLDivElement>(null);
  const chatInputRef = useRef<HTMLInputElement>(null);

  const createThread = useMutation(api.agents.reviewStreaming.createReviewThread);
  const sendMessage = useMutation(api.agents.reviewStreaming.sendMessage);
  const dueCount = useQuery(api.concepts.getConceptsDueCount);

  const messages = useUIMessages(
    api.agents.reviewStreaming.listMessages,
    threadId ? { threadId } : 'skip',
    { initialNumItems: 50, stream: true }
  );

  // Auto-scroll chat panel
  useEffect(() => {
    if (chatScrollRef.current) {
      chatScrollRef.current.scrollTop = chatScrollRef.current.scrollHeight;
    }
  }, [messages?.results]);

  const handleStart = useCallback(async () => {
    setIsStarting(true);
    try {
      const { threadId: newThreadId } = await createThread();
      setThreadId(newThreadId);
    } finally {
      setIsStarting(false);
    }
  }, [createThread]);

  const handleSend = useCallback(
    async (text: string) => {
      if (!threadId || !text.trim()) return;
      setInput('');
      await sendMessage({ threadId, prompt: text.trim() });
    },
    [threadId, sendMessage]
  );

  const handleAnswer = useCallback(
    async (text: string) => {
      submittedAnswers.current.add(text);
      return handleSend(text);
    },
    [handleSend]
  );

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault();
        handleSend(input);
      }
    },
    [handleSend, input]
  );

  // ---- PRE-SESSION: Start Screen (5D locked design) ----
  if (!threadId) {
    const count = dueCount?.conceptsDue ?? 0;
    return (
      <div className="flex h-[calc(100dvh-4rem)] flex-col">
        <main className="grid flex-1 grid-cols-1 overflow-auto md:grid-cols-[2fr_1fr] md:overflow-hidden">
          {/* Left: Editorial content */}
          <div className="flex flex-col justify-center p-6 py-12 md:px-20 md:py-0">
            <div className="mb-8 h-0.5 w-16 bg-primary" />
            <h1 className="mb-4 font-serif text-3xl leading-[1.05] tracking-tight md:text-[3.25rem]">
              Review
            </h1>
            <p className="mb-10 max-w-[32ch] font-serif text-base leading-[1.7] text-muted-foreground md:text-[1.0625rem]">
              {count > 0
                ? 'Your deck has been building overnight. Pick up where you left off.'
                : 'No cards are due right now. Check back later.'}
            </p>
            <button
              onClick={handleStart}
              disabled={isStarting || count === 0}
              className="inline-flex w-fit items-center gap-3 bg-primary px-8 py-3.5 text-[0.9375rem] font-medium text-primary-foreground hover:opacity-90 disabled:opacity-50"
            >
              {isStarting ? (
                <>
                  <Loader2 className="size-[18px] animate-spin" />
                  <span>Starting...</span>
                </>
              ) : (
                <>
                  <span>Begin Session</span>
                  <ArrowRight className="size-[18px]" />
                </>
              )}
            </button>
          </div>

          {/* Right: Stats panel */}
          <ReviewStatsPanel count={count} />
        </main>
      </div>
    );
  }

  // ---- ACTIVE SESSION: Split Layout ----
  return (
    <ActiveSession
      messages={messages}
      user={user}
      input={input}
      setInput={setInput}
      chatScrollRef={chatScrollRef}
      chatInputRef={chatInputRef}
      submittedAnswers={submittedAnswers}
      handleSend={handleSend}
      handleAnswer={handleAnswer}
      handleKeyDown={handleKeyDown}
    />
  );
}

// Separated to avoid hooks-after-early-return issues
function ActiveSession({
  messages,
  user,
  input,
  setInput,
  chatScrollRef,
  chatInputRef,
  submittedAnswers,
  handleSend,
  handleAnswer,
  handleKeyDown,
}: {
  messages: ReturnType<typeof useUIMessages>;
  user: ReturnType<typeof useUser>['user'];
  input: string;
  setInput: (v: string) => void;
  chatScrollRef: React.RefObject<HTMLDivElement | null>;
  chatInputRef: React.RefObject<HTMLInputElement | null>;
  submittedAnswers: React.MutableRefObject<Set<string>>;
  handleSend: (text: string) => Promise<void>;
  handleAnswer: (text: string) => Promise<void>;
  handleKeyDown: (e: React.KeyboardEvent) => void;
}) {
  const [showChat, setShowChat] = useState(false);
  const messageList = messages?.results ?? [];
  const focusChat = useCallback(() => {
    setShowChat(true);
    requestAnimationFrame(() => {
      chatInputRef.current?.focus();
    });
  }, [chatInputRef]);

  // Extract tool results for left panel
  const { latestQuestion, latestFeedback } = useMemo(
    () => extractLatestToolResults(messageList),
    [messageList]
  );

  // Filter chat-only messages (text from assistant, user messages minus auto-prompts/answers)
  const chatMessages = useMemo(() => {
    return messageList.filter((m: UIMessage) => {
      if (m.role === 'user') {
        if (m.text === START_PROMPT) return false;
        if (submittedAnswers.current.has(m.text)) return false;
        return true;
      }
      return true;
    });
  }, [messageList, submittedAnswers]);

  const isStreaming = messageList.some(
    (m: UIMessage) => m.role === 'assistant' && m.status === 'streaming'
  );

  // Check if agent is currently executing a tool
  const isToolRunning = messageList.some(
    (m: UIMessage) =>
      m.role === 'assistant' &&
      m.parts.some(
        (part) =>
          typeof part.type === 'string' &&
          part.type.startsWith('tool-') &&
          'state' in part &&
          ((part as { state: string }).state === 'input-streaming' ||
            (part as { state: string }).state === 'input-available')
      )
  );

  return (
    <div className="flex h-[calc(100dvh-4rem)] flex-col">
      {/* Main: Asymmetric Split */}
      <main className="flex flex-1 overflow-hidden flex-col md:flex-row">
        {/* LEFT: Review Content */}
        <div
          className={cn(
            'flex-1 min-w-0 overflow-auto bg-background p-4 md:p-8',
            showChat ? 'hidden md:block' : 'block'
          )}
        >
          {/* Loading state */}
          {!latestQuestion && !latestFeedback && (
            <div className="flex items-center gap-2 pt-4 md:pt-8 text-sm text-muted-foreground">
              <Loader2 className="h-4 w-4 animate-spin" />
              Finding next concept...
            </div>
          )}

          {/* Feedback card */}
          {latestFeedback && (
            <>
              <FeedbackCard
                data={latestFeedback.data}
                questionText={latestFeedback.questionText ?? undefined}
              />
              <div className="mt-6 flex items-center justify-between max-w-3xl">
                <button
                  onClick={focusChat}
                  className="flex items-center gap-2 text-sm text-muted-foreground hover:text-primary"
                >
                  <MessageCircle className="h-4 w-4" />
                  <span>Discuss this topic</span>
                </button>
              </div>
            </>
          )}

          {/* Quiz card */}
          {latestQuestion?.toolName === 'fetchDueConcept' && (
            <QuestionCard
              key={(latestQuestion.data.conceptId as string) ?? ''}
              data={latestQuestion.data}
              onAnswer={handleAnswer}
            />
          )}

          {/* Tool execution indicator */}
          {isToolRunning && latestFeedback && !latestQuestion && (
            <div className="mt-4 flex items-center gap-2 text-sm text-muted-foreground">
              <Loader2 className="h-4 w-4 animate-spin" />
              Loading next question...
            </div>
          )}
        </div>

        {/* RIGHT: Chat Panel */}
        <div
          className={cn(
            'flex flex-col border-border bg-background',
            'w-full md:w-[420px] md:shrink-0 md:border-l',
            showChat ? 'flex' : 'hidden md:flex'
          )}
        >
          {/* Mobile back button */}
          <div className="flex items-center gap-2 border-b border-border px-4 py-3 md:hidden">
            <button
              onClick={() => setShowChat(false)}
              className="flex items-center gap-2 text-sm text-muted-foreground hover:text-foreground"
            >
              <ArrowLeft className="h-4 w-4" />
              Back to review
            </button>
          </div>

          {/* Chat header */}
          <div className="flex items-center justify-between border-b border-border px-5 py-4">
            <div className="flex items-center gap-3">
              <div className="flex h-8 w-8 items-center justify-center bg-primary">
                <Layers className="h-4 w-4 text-primary-foreground" />
              </div>
              <div>
                <h3 className="text-sm font-medium">Scry Agent</h3>
                <p className="font-mono text-xs text-muted-foreground">
                  {isStreaming ? 'Thinking...' : 'Online'}
                </p>
              </div>
            </div>
          </div>

          {/* Chat messages */}
          <div ref={chatScrollRef} className="flex-1 space-y-5 overflow-auto p-5">
            {messages?.status === 'LoadingFirstPage' && (
              <div className="flex items-center justify-center py-8">
                <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
              </div>
            )}

            {chatMessages.map((message: UIMessage) => (
              <ChatMessage key={message.key} message={message} userAvatarUrl={user?.imageUrl} />
            ))}

            {/* Streaming dots */}
            {isStreaming && (
              <div className="flex items-center gap-1 px-1 py-2">
                <span className="h-1.5 w-1.5 animate-bounce rounded-full bg-muted-foreground/50 [animation-delay:0ms]" />
                <span className="h-1.5 w-1.5 animate-bounce rounded-full bg-muted-foreground/50 [animation-delay:150ms]" />
                <span className="h-1.5 w-1.5 animate-bounce rounded-full bg-muted-foreground/50 [animation-delay:300ms]" />
              </div>
            )}
          </div>

          {/* Suggestion chips */}
          <div className="border-t border-border bg-secondary px-5 py-3">
            <div className="flex flex-wrap gap-2">
              {SUGGESTION_CHIPS.map((chip) => (
                <button
                  key={chip.label}
                  onClick={() => handleSend(chip.text)}
                  className="border border-border bg-background px-2.5 py-1 text-xs text-muted-foreground hover:border-primary hover:text-primary"
                >
                  {chip.label}
                </button>
              ))}
            </div>
          </div>

          {/* Chat input */}
          <div className="border-t border-border p-4">
            <div className="flex items-center gap-2 border border-border p-1.5">
              <input
                ref={chatInputRef}
                type="text"
                value={input}
                onChange={(e) => setInput(e.target.value)}
                onKeyDown={handleKeyDown}
                placeholder="Ask anything..."
                className="flex-1 bg-transparent py-2 pl-3 pr-2 text-sm outline-none"
              />
              <button
                onClick={() => handleSend(input)}
                disabled={!input.trim()}
                className="shrink-0 bg-primary p-2 text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
              >
                <Send className="h-4 w-4" />
              </button>
            </div>
          </div>
        </div>
      </main>

      {/* Mobile FAB — show chat toggle when on content view */}
      {!showChat && (
        <button
          onClick={() => setShowChat(true)}
          className="fixed bottom-6 right-6 z-50 flex h-14 w-14 items-center justify-center rounded-full bg-primary text-primary-foreground shadow-lg hover:bg-primary/90 md:hidden"
        >
          <MessageCircle className="h-6 w-6" />
        </button>
      )}
    </div>
  );
}

// ---- Chat sub-components ----

function ChatMessage({ message, userAvatarUrl }: { message: UIMessage; userAvatarUrl?: string }) {
  if (message.role === 'user') {
    return (
      <div className="flex justify-end gap-3">
        <div className="flex flex-1 justify-end">
          <div className="max-w-[85%] bg-primary px-4 py-3">
            <p className="text-sm leading-relaxed text-primary-foreground">{message.text}</p>
          </div>
        </div>
        {userAvatarUrl ? (
          <Image
            src={userAvatarUrl}
            alt=""
            width={28}
            height={28}
            className="mt-0.5 h-7 w-7 shrink-0 rounded-full"
          />
        ) : (
          <div className="mt-0.5 h-7 w-7 shrink-0 rounded-full bg-muted" />
        )}
      </div>
    );
  }

  // Assistant message — render only text parts (tool cards go to left panel)
  const textParts = message.parts.filter(
    (part): part is { type: 'text'; text: string } =>
      part.type === 'text' && 'text' in part && !!(part as { text: string }).text?.trim()
  );

  // Tool loading states shown inline in chat
  const toolLoadingParts = message.parts.filter(
    (part) =>
      typeof part.type === 'string' &&
      part.type.startsWith('tool-') &&
      'state' in part &&
      ((part as { state: string }).state === 'input-streaming' ||
        (part as { state: string }).state === 'input-available')
  );

  if (textParts.length === 0 && toolLoadingParts.length === 0) return null;

  return (
    <div className="flex gap-3">
      <div className="flex h-7 w-7 shrink-0 items-center justify-center bg-primary">
        <Layers className="h-3.5 w-3.5 text-primary-foreground" />
      </div>
      <div className="min-w-0 flex-1 space-y-2">
        {textParts.map((part, i) => (
          <MessageBubble key={i} text={part.text} />
        ))}
        {toolLoadingParts.map((part, i) => {
          const toolName = (part.type as string).slice(5);
          return (
            <div
              key={`tool-${i}`}
              className="flex items-center gap-2 text-xs text-muted-foreground"
            >
              <Loader2 className="h-3 w-3 animate-spin" />
              {toolName === 'fetchDueConcept' && 'Finding next concept...'}
              {toolName === 'submitAnswer' && 'Checking answer...'}
              {toolName === 'getSessionStats' && 'Getting stats...'}
            </div>
          );
        })}
      </div>
    </div>
  );
}

// ---- Start Screen: Stats Panel (5D design) ----

const HEATMAP_LEVELS = [
  'bg-white/[0.06]',
  'bg-white/[0.14]',
  'bg-white/[0.26]',
  'bg-white/[0.42]',
  'bg-white/[0.65]',
];

function generateHeatmapCells() {
  const WEEKS = 28;
  const dayNames = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'];
  let seed = 42;
  const rand = () => {
    seed = (seed * 16807) % 2147483647;
    return seed / 2147483647;
  };

  const cells: { level: number; isToday: boolean; tip: string }[] = [];
  for (let w = 0; w < WEEKS; w++) {
    for (let d = 0; d < 7; d++) {
      const progress = w / WEEKS;
      const isWeekend = d >= 5;
      const r = rand();
      let lvl = 0;
      const threshold = progress * (isWeekend ? 0.6 : 1);
      if (r < threshold * 0.7) lvl = 1;
      if (r < threshold * 0.5) lvl = 2;
      if (r < threshold * 0.3) lvl = 3;
      if (r < threshold * 0.15) lvl = 4;

      if (w === WEEKS - 1 && d === 3) {
        cells.push({ level: 0, isToday: true, tip: 'Today' });
      } else {
        const counts = [0, 6, 16, 30, 42];
        const jitter = Math.round((rand() - 0.5) * 6);
        const count = Math.max(0, counts[lvl] + jitter);
        cells.push({
          level: lvl,
          isToday: false,
          tip: `${dayNames[d]} · ${count} ${count === 1 ? 'card' : 'cards'}`,
        });
      }
    }
  }
  return cells;
}

function ReviewStatsPanel({ count }: { count: number }) {
  const dashboard = useQuery(api.concepts.getReviewDashboard);
  const cells = useMemo(() => generateHeatmapCells(), []);

  const totalConcepts = dashboard?.totalConcepts ?? 0;
  const totalPhrasings = dashboard?.totalPhrasings ?? 0;
  const masteryPercent = dashboard?.masteryPercent ?? 0;
  const streak = dashboard?.streak ?? 0;

  return (
    <div className="relative flex flex-col justify-center overflow-hidden bg-primary p-6 text-primary-foreground md:p-10">
      {/* Dot pattern overlay */}
      <div
        className="pointer-events-none absolute inset-0"
        style={{
          backgroundImage: 'radial-gradient(rgba(255,255,255,0.05) 1px, transparent 1px)',
          backgroundSize: '14px 14px',
        }}
      />

      {/* Hero number */}
      <div className="relative mb-6 flex items-baseline gap-2.5">
        <span className="text-5xl font-bold tabular-nums leading-[0.9] md:text-7xl">{count}</span>
        <span className="font-mono text-[0.625rem] uppercase tracking-[0.08em] opacity-70">
          Cards Due
        </span>
      </div>

      {/* Stat cards */}
      <div className="relative mb-5 flex flex-col gap-1.5">
        <div className="flex items-center gap-2.5 bg-white/[0.06] px-3.5 py-2.5">
          <BookOpen className="size-[15px] shrink-0 opacity-[0.35]" />
          <span className="flex-1 font-mono text-[0.5rem] uppercase tracking-[0.06em] opacity-50">
            Concepts
          </span>
          <span className="font-mono text-sm font-semibold tabular-nums">{totalConcepts}</span>
        </div>
        <div className="flex items-center gap-2.5 bg-white/[0.06] px-3.5 py-2.5">
          <MessageSquare className="size-[15px] shrink-0 opacity-[0.35]" />
          <span className="flex-1 font-mono text-[0.5rem] uppercase tracking-[0.06em] opacity-50">
            Phrasings
          </span>
          <span className="font-mono text-sm font-semibold tabular-nums">{totalPhrasings}</span>
        </div>
        <div className="flex items-center gap-2.5 bg-white/[0.06] px-3.5 py-2.5">
          <BarChart3 className="size-[15px] shrink-0 opacity-[0.35]" />
          <span className="flex-1 font-mono text-[0.5rem] uppercase tracking-[0.06em] opacity-50">
            Mastery
          </span>
          <span className="font-mono text-sm font-semibold tabular-nums">{masteryPercent}%</span>
        </div>
      </div>

      {/* Heatmap */}
      <div className="relative border-t border-white/[0.12] pt-3">
        <div className="mb-2 flex items-baseline justify-between">
          <span className="font-mono text-[0.5625rem] uppercase tracking-[0.06em] opacity-60">
            Activity
          </span>
          <span className="font-mono text-[0.6875rem] font-medium">{streak}d streak</span>
        </div>

        {/* Grid: day labels left + heatmap right */}
        <div className="flex gap-1.5">
          <div className="grid shrink-0 grid-rows-7 gap-0.5">
            {['Mon', '', 'Wed', '', 'Fri', '', ''].map((day, i) => (
              <span
                key={i}
                className="flex items-center font-mono text-[0.5rem] uppercase leading-none opacity-40"
              >
                {day}
              </span>
            ))}
          </div>
          <div className="grid flex-1 auto-cols-fr grid-flow-col grid-rows-7 gap-0.5">
            {cells.map((cell, i) => (
              <div
                key={i}
                title={cell.tip}
                className={cn(
                  'aspect-square rounded-sm',
                  cell.isToday
                    ? 'bg-white shadow-[0_0_4px_rgba(255,255,255,0.4)]'
                    : HEATMAP_LEVELS[cell.level]
                )}
              />
            ))}
          </div>
        </div>

        {/* Footer: weeks label + legend */}
        <div className="mt-1.5 flex items-center justify-between">
          <span className="font-mono text-[0.5rem] opacity-[0.35]">28 weeks</span>
          <div className="flex items-center gap-1">
            <span className="font-mono text-[0.5625rem] opacity-[0.45]">Less</span>
            {HEATMAP_LEVELS.map((bg, i) => (
              <div key={i} className={cn('h-[11px] w-[11px] rounded-sm', bg)} />
            ))}
            <span className="font-mono text-[0.5625rem] opacity-[0.45]">More</span>
          </div>
        </div>
      </div>
    </div>
  );
}
