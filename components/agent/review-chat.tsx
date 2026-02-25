'use client';

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useUser } from '@clerk/nextjs';
import type { UIMessage } from '@convex-dev/agent/react';
import { useUIMessages } from '@convex-dev/agent/react';
import { useMutation, useQuery } from 'convex/react';
import {
  ArrowRight,
  BarChart3,
  BookOpen,
  Bot,
  CalendarClock,
  Loader2,
  MessageSquare,
  Send,
  Target,
  TriangleAlert,
  X,
} from 'lucide-react';
import { api } from '@/convex/_generated/api';
import type { Id } from '@/convex/_generated/dataModel';
import { formatReviewStageLabel } from '@/lib/review-stage';
import { cn } from '@/lib/utils';
import { FeedbackCard } from './feedback-card';
import { MessageBubble } from './message-bubble';
import { QuestionCard } from './question-card';
import { parseRescheduleIntent } from './review-intent';

type SuggestionChip = {
  id: 'explain' | 'weak-areas' | 'reschedule';
  label: string;
  intent: 'chat' | 'action';
  prompt?: string;
  chatIntent?: ChatIntent;
  needsConceptContext?: boolean;
};

type ChatIntent = 'general' | 'explain' | 'stats';

interface SendChatOptions {
  allowRescheduleIntent?: boolean;
  intent?: ChatIntent;
}

const MAX_VISIBLE_CHAT_MESSAGES = 10;
const MAX_ARTIFACT_ENTRIES = 60;

const SUGGESTION_CHIPS: SuggestionChip[] = [
  {
    id: 'explain',
    label: 'Explain this concept',
    intent: 'chat',
    prompt: 'Explain this concept briefly with one memorable analogy.',
    chatIntent: 'explain',
    needsConceptContext: true,
  },
  { id: 'weak-areas', label: 'Weak areas', intent: 'action' },
  { id: 'reschedule', label: 'Reschedule', intent: 'action', needsConceptContext: true },
];

interface ReviewFeedbackData extends Record<string, unknown> {
  conceptId?: Id<'concepts'>;
  conceptTitle?: string;
  isCorrect?: boolean;
  correctAnswer?: string;
  userAnswer?: string;
  nextReview?: number;
  scheduledDays?: number;
  lapses?: number;
  reps?: number;
}

interface ReviewFeedbackState {
  data: ReviewFeedbackData;
  questionText: string | null;
}

type ActionPanelState =
  | {
      type: 'weak-areas';
      generatedAt: number;
      itemCount: number;
      items: Array<{
        title: string;
        state: string;
        lapses: number;
        reps: number;
        dueNow: boolean;
      }>;
    }
  | {
      type: 'rescheduled';
      conceptTitle: string;
      nextReview: number;
      scheduledDays: number;
    }
  | {
      type: 'notice';
      title: string;
      description: string;
    };

interface PendingFeedbackState {
  questionText: string | null;
  conceptTitle: string | null;
  userAnswer: string;
}

interface ActiveQuestionState extends Record<string, unknown> {
  conceptId: Id<'concepts'>;
  phrasingId: Id<'phrasings'>;
  question: string;
  conceptTitle?: string;
  conceptDescription?: string;
  recentAttempts?: number;
  recentCorrect?: number;
  lapses?: number;
  reps?: number;
}

interface RescheduleTarget {
  conceptId: Id<'concepts'>;
  conceptTitle: string;
}

interface ActionReplyState {
  title: string;
  body: string;
}

interface RescheduleMutationResult {
  conceptId?: Id<'concepts'>;
  conceptTitle?: string;
  nextReview?: number;
  scheduledDays?: number;
}

interface WeakAreasMutationResult {
  generatedAt?: number;
  itemCount?: number;
  items?: Array<{
    title?: string;
    state?: string;
    lapses?: number;
    reps?: number;
    dueNow?: boolean;
  }>;
}

type ArtifactEntry =
  | { id: string; createdAt: number; type: 'question'; data: Record<string, unknown> }
  | { id: string; createdAt: number; type: 'feedback'; data: ReviewFeedbackState }
  | { id: string; createdAt: number; type: 'action'; data: ActionPanelState }
  | { id: string; createdAt: number; type: 'complete' };

export function ReviewChat() {
  const { user } = useUser();
  const [threadId, setThreadId] = useState<string | null>(null);
  const [input, setInput] = useState('');
  const [isStarting, setIsStarting] = useState(false);
  const [startError, setStartError] = useState<string | null>(null);
  const [activeQuestion, setActiveQuestion] = useState<ActiveQuestionState | null>(null);
  const [latestFeedback, setLatestFeedback] = useState<ReviewFeedbackState | null>(null);
  const [pendingFeedback, setPendingFeedback] = useState<PendingFeedbackState | null>(null);
  const [actionReply, setActionReply] = useState<ActionReplyState | null>(null);
  const [rescheduleTarget, setRescheduleTarget] = useState<RescheduleTarget | null>(null);
  const [activeChipAction, setActiveChipAction] = useState<SuggestionChip['id'] | null>(null);
  const [isFetchingQuestion, setIsFetchingQuestion] = useState(false);
  const [isSubmittingAnswer, setIsSubmittingAnswer] = useState(false);
  const [artifactFeed, setArtifactFeed] = useState<ArtifactEntry[]>([]);
  const [isChatSendPending, setIsChatSendPending] = useState(false);
  const pendingChatBaselineRef = useRef<number | null>(null);
  const chatScrollRef = useRef<HTMLDivElement>(null);
  const chatInputRef = useRef<HTMLInputElement>(null);

  const createThread = useMutation(api.agents.reviewStreaming.createReviewThread);
  const fetchNextQuestion = useMutation(api.agents.reviewStreaming.fetchNextQuestion);
  const submitAnswerDirect = useMutation(api.agents.reviewStreaming.submitAnswerDirect);
  const getWeakAreasDirect = useMutation(api.agents.reviewStreaming.getWeakAreasDirect);
  const rescheduleConceptDirect = useMutation(api.agents.reviewStreaming.rescheduleConceptDirect);
  const sendMessage = useMutation(api.agents.reviewStreaming.sendMessage);
  const dueCount = useQuery(api.concepts.getConceptsDueCount);

  const messages = useUIMessages(
    api.agents.reviewStreaming.listMessages,
    threadId ? { threadId } : 'skip',
    { initialNumItems: 25, stream: true }
  );

  // Auto-scroll chat panel
  useEffect(() => {
    if (chatScrollRef.current) {
      chatScrollRef.current.scrollTop = chatScrollRef.current.scrollHeight;
    }
  }, [messages?.results]);

  const messageList = useMemo(() => messages?.results ?? [], [messages?.results]);
  const assistantStreaming = messageList.some(
    (message: UIMessage) => message.role === 'assistant' && message.status === 'streaming'
  );
  const appendArtifact = useCallback((entry: ArtifactEntry) => {
    setArtifactFeed((prev) => {
      if (prev.some((item) => item.id === entry.id)) {
        return prev;
      }
      const next = [...prev, entry];
      return next.length > MAX_ARTIFACT_ENTRIES
        ? next.slice(next.length - MAX_ARTIFACT_ENTRIES)
        : next;
    });
  }, []);

  useEffect(() => {
    if (!isChatSendPending || assistantStreaming) return;
    const baseline = pendingChatBaselineRef.current ?? 0;
    const assistantCount = messageList.filter(
      (message: UIMessage) => message.role === 'assistant'
    ).length;
    if (assistantCount > baseline) {
      setIsChatSendPending(false);
      pendingChatBaselineRef.current = null;
    }
  }, [assistantStreaming, isChatSendPending, messageList]);

  const loadNextQuestion = useCallback(
    async (targetThreadId: string) => {
      setIsFetchingQuestion(true);
      setRescheduleTarget(null);
      try {
        const next = await fetchNextQuestion({ threadId: targetThreadId });
        if (next) {
          const typedNext = next as ActiveQuestionState;
          setActiveQuestion(typedNext);
          const questionId = `question:${String(typedNext.conceptId ?? '')}:${String(typedNext.phrasingId ?? '')}:${String(typedNext.question ?? '')}:${Date.now()}`;
          appendArtifact({
            id: questionId,
            createdAt: Date.now(),
            type: 'question',
            data: typedNext,
          });
          return;
        }
        setActiveQuestion(null);
        appendArtifact({ id: `complete:${Date.now()}`, createdAt: Date.now(), type: 'complete' });
      } finally {
        setIsFetchingQuestion(false);
      }
    },
    [appendArtifact, fetchNextQuestion]
  );

  const handleStart = useCallback(async () => {
    setIsStarting(true);
    setStartError(null);
    try {
      setArtifactFeed([]);
      const { threadId: newThreadId } = await createThread();
      setThreadId(newThreadId);
      await loadNextQuestion(newThreadId);
    } catch (error) {
      setThreadId(null);
      setStartError(error instanceof Error ? error.message : 'Unable to start review session.');
    } finally {
      setIsStarting(false);
    }
  }, [createThread, loadNextQuestion]);

  const handleSendChat = useCallback(
    async (text: string, options?: SendChatOptions) => {
      if (!threadId || !text.trim()) return;
      const trimmed = text.trim();
      const allowRescheduleIntent = options?.allowRescheduleIntent ?? true;
      const intent = options?.intent ?? 'general';
      const requestedDays = allowRescheduleIntent ? parseRescheduleIntent(trimmed) : null;
      const currentConceptId = activeQuestion?.conceptId ?? latestFeedback?.data.conceptId;
      const currentConceptTitle =
        activeQuestion?.conceptTitle ?? latestFeedback?.data.conceptTitle ?? 'this concept';

      if (requestedDays && currentConceptId) {
        setInput('');
        setRescheduleTarget({ conceptId: currentConceptId, conceptTitle: currentConceptTitle });
        setActionReply({
          title: 'Reschedule ready',
          body: `Picked up your request. Choose an interval below to move ${currentConceptTitle}.`,
        });
        return;
      }

      setInput('');
      setActionReply(null);
      pendingChatBaselineRef.current = messageList.filter(
        (message: UIMessage) => message.role === 'assistant'
      ).length;
      setIsChatSendPending(true);
      try {
        await sendMessage({ threadId, prompt: trimmed, intent });
      } catch (error) {
        setIsChatSendPending(false);
        pendingChatBaselineRef.current = null;
        setActionReply({
          title: 'Message not sent',
          body: error instanceof Error ? error.message : 'Try sending again in a moment.',
        });
      }
    },
    [threadId, activeQuestion, latestFeedback, sendMessage, messageList]
  );

  const handleAnswer = useCallback(
    async (text: string) => {
      if (!threadId || !activeQuestion) return;
      const questionSnapshot = activeQuestion;
      setPendingFeedback({
        questionText: questionSnapshot.question ?? null,
        conceptTitle: questionSnapshot.conceptTitle ?? null,
        userAnswer: text,
      });
      setActiveQuestion(null);
      setRescheduleTarget(null);
      setIsSubmittingAnswer(true);
      try {
        const result: ReviewFeedbackData = await submitAnswerDirect({
          threadId,
          conceptId: questionSnapshot.conceptId,
          phrasingId: questionSnapshot.phrasingId,
          userAnswer: text,
          conceptTitle: questionSnapshot.conceptTitle ?? '',
          conceptDescription: questionSnapshot.conceptDescription ?? '',
          recentAttempts: questionSnapshot.recentAttempts ?? 0,
          recentCorrect: questionSnapshot.recentCorrect ?? 0,
          lapses: questionSnapshot.lapses ?? 0,
          reps: questionSnapshot.reps ?? 0,
        });
        const feedbackEntry: ReviewFeedbackState = {
          data: result,
          questionText: questionSnapshot.question ?? null,
        };
        setLatestFeedback(feedbackEntry);
        appendArtifact({
          id: `feedback:${String(questionSnapshot.conceptId ?? '')}:${String(result.reps ?? '')}:${Date.now()}`,
          createdAt: Date.now(),
          type: 'feedback',
          data: feedbackEntry,
        });
        setPendingFeedback(null);
      } catch (error) {
        setActiveQuestion(questionSnapshot);
        setPendingFeedback(null);
        setActionReply({
          title: 'Answer not submitted',
          body: error instanceof Error ? error.message : 'Try again in a moment.',
        });
      } finally {
        setIsSubmittingAnswer(false);
      }
    },
    [threadId, activeQuestion, submitAnswerDirect, appendArtifact]
  );

  const executeReschedule = useCallback(
    async (days: number) => {
      if (!threadId || !rescheduleTarget) return;
      const target = rescheduleTarget;
      const normalizedDays = Math.max(1, Math.min(30, Math.round(days)));
      setActiveChipAction('reschedule');
      try {
        const result: RescheduleMutationResult = await rescheduleConceptDirect({
          threadId,
          conceptId: target.conceptId,
          days: normalizedDays,
        });
        const nextReview = result.nextReview ?? Date.now();
        const scheduledDays = result.scheduledDays ?? normalizedDays;
        const conceptTitle = result.conceptTitle ?? target.conceptTitle;

        appendArtifact({
          id: `action:rescheduled:${String(result.conceptId ?? target.conceptId)}:${String(nextReview)}`,
          createdAt: Date.now(),
          type: 'action',
          data: {
            type: 'rescheduled',
            conceptTitle,
            nextReview,
            scheduledDays,
          },
        });
        setActionReply({
          title: 'Rescheduled',
          body: `${conceptTitle} moved by ${scheduledDays} day${scheduledDays === 1 ? '' : 's'}.`,
        });
        setLatestFeedback((prev) => {
          if (!prev) return prev;
          if (String(prev.data.conceptId ?? '') !== String(target.conceptId)) {
            return prev;
          }
          return {
            ...prev,
            data: {
              ...prev.data,
              scheduledDays: scheduledDays ?? prev.data.scheduledDays,
              nextReview: nextReview ?? prev.data.nextReview,
            },
          };
        });
        setRescheduleTarget(null);
        if (activeQuestion && String(activeQuestion.conceptId) === String(target.conceptId)) {
          await loadNextQuestion(threadId);
        }
      } catch (error) {
        setActionReply({
          title: 'Reschedule failed',
          body: error instanceof Error ? error.message : 'Try again in a moment.',
        });
      } finally {
        setActiveChipAction(null);
      }
    },
    [
      threadId,
      rescheduleTarget,
      rescheduleConceptDirect,
      activeQuestion,
      loadNextQuestion,
      appendArtifact,
    ]
  );

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault();
        void handleSendChat(input);
      }
    },
    [handleSendChat, input]
  );

  const buildSuggestionPrompt = useCallback(
    (text: string) => {
      const conceptTitle = activeQuestion?.conceptTitle ?? latestFeedback?.data.conceptTitle;
      const userAnswer = latestFeedback?.data.userAnswer;
      const isCorrect = latestFeedback?.data.isCorrect;
      const reps = activeQuestion?.reps ?? latestFeedback?.data.reps;
      const lapses = activeQuestion?.lapses ?? latestFeedback?.data.lapses;
      const contextBits = [
        conceptTitle ? `concept=${conceptTitle}` : null,
        userAnswer ? `my_answer=${userAnswer}` : null,
        typeof isCorrect === 'boolean' ? `result=${isCorrect ? 'correct' : 'incorrect'}` : null,
        typeof reps === 'number' ? `reviews=${reps}` : null,
        typeof lapses === 'number' ? `lapses=${lapses}` : null,
      ].filter(Boolean);

      const responseFormat =
        'Reply format: 1) one-sentence TL;DR 2) 2-4 short bullets 3) one memory cue.';
      if (contextBits.length === 0) {
        return `${text}\n\n${responseFormat}`;
      }
      return `${text}\n\n${responseFormat}\nContext: ${contextBits.join(' | ')}`;
    },
    [activeQuestion, latestFeedback]
  );

  const handleSuggestionChip = useCallback(
    async (chip: SuggestionChip) => {
      if (!threadId) return;

      if (chip.intent === 'chat') {
        await handleSendChat(buildSuggestionPrompt(chip.prompt ?? chip.label), {
          allowRescheduleIntent: false,
          intent: chip.chatIntent ?? 'general',
        });
        return;
      }

      if (chip.id === 'weak-areas') {
        setActiveChipAction(chip.id);
        try {
          const result: WeakAreasMutationResult = await getWeakAreasDirect({ threadId, limit: 5 });
          const generatedAt = result.generatedAt ?? Date.now();
          const itemCount = result.itemCount ?? 0;
          const items = (result.items ?? []).map((item) => ({
            title: item.title ?? 'Untitled',
            state: item.state ?? 'new',
            lapses: item.lapses ?? 0,
            reps: item.reps ?? 0,
            dueNow: Boolean(item.dueNow),
          }));

          appendArtifact({
            id: `action:weak-areas:${String(generatedAt)}`,
            createdAt: Date.now(),
            type: 'action',
            data: {
              type: 'weak-areas',
              generatedAt,
              itemCount,
              items,
            },
          });
          setActionReply({
            title: 'Weak areas ready',
            body:
              itemCount > 0
                ? `I ranked ${itemCount} concepts by lapses and difficulty.`
                : 'No major weak areas right now. You are in good shape.',
          });
        } catch (error) {
          setActionReply({
            title: 'Weak areas unavailable',
            body: error instanceof Error ? error.message : 'Try again in a moment.',
          });
        } finally {
          setActiveChipAction(null);
        }
        return;
      }

      if (chip.id === 'reschedule') {
        const conceptId = activeQuestion?.conceptId ?? latestFeedback?.data.conceptId;
        const conceptTitle =
          activeQuestion?.conceptTitle ?? latestFeedback?.data.conceptTitle ?? 'this concept';

        if (!conceptId) {
          appendArtifact({
            id: `action:notice:no-concept:${Date.now()}`,
            createdAt: Date.now(),
            type: 'action',
            data: {
              type: 'notice',
              title: 'No concept selected',
              description: 'Answer or load a question first, then reschedule.',
            },
          });
          setActionReply({
            title: 'Reschedule unavailable',
            body: 'Load a concept first, then choose Reschedule.',
          });
          return;
        }

        setRescheduleTarget({ conceptId, conceptTitle });
        setActionReply({
          title: 'Choose a new interval',
          body: `Pick how long to postpone ${conceptTitle}.`,
        });
      }
    },
    [
      activeQuestion,
      buildSuggestionPrompt,
      getWeakAreasDirect,
      handleSendChat,
      latestFeedback,
      appendArtifact,
      threadId,
    ]
  );

  // ---- PRE-SESSION: Start Screen (5D locked design) ----
  if (!threadId) {
    const count = dueCount?.conceptsDue ?? 0;
    return (
      <div className="relative flex h-full min-h-0 flex-col">
        <div className="pointer-events-none fixed inset-x-0 top-[var(--navbar-height)] bottom-0 bg-[radial-gradient(circle_at_top_left,hsl(var(--primary)/0.09),transparent_45%)]" />
        <main className="relative z-10 flex-1 overflow-auto">
          <div className="mx-auto grid w-full max-w-7xl grid-cols-1 p-4 md:grid-cols-[2fr_1fr] md:gap-4 md:p-6">
            {/* Left: Editorial content */}
            <div className="flex flex-col justify-center rounded-3xl border border-border/70 bg-background/80 p-6 py-10 shadow-sm backdrop-blur-sm md:px-14 md:py-0">
              <div className="mb-7 h-0.5 w-14 rounded-full bg-primary/70" />
              <h1 className="mb-4 font-serif text-3xl leading-[1.05] tracking-tight md:text-[3.2rem]">
                Review
              </h1>
              <p className="mb-8 max-w-[34ch] font-serif text-base leading-[1.7] text-muted-foreground md:text-[1.0625rem]">
                {count > 0
                  ? 'Your deck is ready. Start with one card and momentum follows.'
                  : 'No cards are due right now. Come back when your next review opens.'}
              </p>
              <button
                onClick={handleStart}
                disabled={isStarting || count === 0}
                className="inline-flex w-fit items-center gap-3 rounded-full bg-primary px-7 py-3 text-[0.9375rem] font-semibold text-primary-foreground shadow-sm transition hover:opacity-90 disabled:opacity-50"
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
              {startError && (
                <p className="mt-3 max-w-[36ch] text-sm text-destructive">{startError}</p>
              )}
            </div>

            {/* Right: Stats panel */}
            <ReviewStatsPanel count={count} />
          </div>
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
      activeQuestion={activeQuestion}
      latestFeedback={latestFeedback}
      pendingFeedback={pendingFeedback}
      artifactFeed={artifactFeed}
      actionReply={actionReply}
      rescheduleTarget={rescheduleTarget}
      isFetchingQuestion={isFetchingQuestion}
      isSubmittingAnswer={isSubmittingAnswer}
      isChatThinking={isChatSendPending || assistantStreaming}
      activeChipAction={activeChipAction}
      onDismissActionReply={() => setActionReply(null)}
      onDismissReschedule={() => setRescheduleTarget(null)}
      onSubmitReschedule={executeReschedule}
      onSuggestionChip={handleSuggestionChip}
      handleSend={handleSendChat}
      handleAnswer={handleAnswer}
      handleNextQuestion={async () => {
        if (!threadId) return;
        setLatestFeedback(null);
        setActionReply(null);
        setRescheduleTarget(null);
        setPendingFeedback(null);
        await loadNextQuestion(threadId);
      }}
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
  activeQuestion,
  latestFeedback,
  pendingFeedback,
  artifactFeed,
  actionReply,
  rescheduleTarget,
  isFetchingQuestion,
  isSubmittingAnswer,
  isChatThinking,
  activeChipAction,
  onDismissActionReply,
  onDismissReschedule,
  onSubmitReschedule,
  onSuggestionChip,
  handleSend,
  handleAnswer,
  handleNextQuestion,
  handleKeyDown,
}: {
  messages: ReturnType<typeof useUIMessages>;
  user: ReturnType<typeof useUser>['user'];
  input: string;
  setInput: (v: string) => void;
  chatScrollRef: React.RefObject<HTMLDivElement | null>;
  chatInputRef: React.RefObject<HTMLInputElement | null>;
  activeQuestion: ActiveQuestionState | null;
  latestFeedback: ReviewFeedbackState | null;
  pendingFeedback: PendingFeedbackState | null;
  artifactFeed: ArtifactEntry[];
  actionReply: ActionReplyState | null;
  rescheduleTarget: RescheduleTarget | null;
  isFetchingQuestion: boolean;
  isSubmittingAnswer: boolean;
  isChatThinking: boolean;
  activeChipAction: SuggestionChip['id'] | null;
  onDismissActionReply: () => void;
  onDismissReschedule: () => void;
  onSubmitReschedule: (days: number) => Promise<void>;
  onSuggestionChip: (chip: SuggestionChip) => Promise<void>;
  handleSend: (text: string, options?: SendChatOptions) => Promise<void>;
  handleAnswer: (text: string) => Promise<void>;
  handleNextQuestion: () => Promise<void>;
  handleKeyDown: (e: React.KeyboardEvent) => void;
}) {
  const messageList = useMemo(() => messages?.results ?? [], [messages?.results]);
  const [showFullHistory, setShowFullHistory] = useState(false);
  const focusChat = useCallback(() => {
    requestAnimationFrame(() => {
      chatInputRef.current?.focus();
    });
  }, [chatInputRef]);
  const hiddenMessageCount = Math.max(0, messageList.length - MAX_VISIBLE_CHAT_MESSAGES);
  const chatMessages = useMemo(
    () =>
      showFullHistory
        ? messageList
        : messageList.slice(Math.max(0, messageList.length - MAX_VISIBLE_CHAT_MESSAGES)),
    [messageList, showFullHistory]
  );

  useEffect(() => {
    setShowFullHistory(false);
  }, [artifactFeed.length]);

  const latestQuestionArtifactId = useMemo(() => {
    for (let i = artifactFeed.length - 1; i >= 0; i -= 1) {
      const entry = artifactFeed[i];
      if (entry?.type === 'question') return entry.id;
    }
    return null;
  }, [artifactFeed]);
  const timelineItems = useMemo(() => {
    const items: Array<
      | { key: string; createdAt: number; kind: 'message'; message: UIMessage }
      | { key: string; createdAt: number; kind: 'artifact'; artifact: ArtifactEntry }
    > = [];
    for (const message of chatMessages) {
      items.push({
        key: `message:${message.key}`,
        createdAt: message._creationTime ?? 0,
        kind: 'message',
        message,
      });
    }
    for (const artifact of artifactFeed) {
      items.push({
        key: `artifact:${artifact.id}`,
        createdAt: artifact.createdAt,
        kind: 'artifact',
        artifact,
      });
    }
    items.sort((a, b) => a.createdAt - b.createdAt);
    return items;
  }, [artifactFeed, chatMessages]);

  return (
    <div className="flex h-full min-h-0 flex-col overflow-hidden">
      {/* Main: Asymmetric Split */}
      <main className="flex min-h-0 flex-1 overflow-hidden">
        {/* Chat Panel */}
        <div className="flex min-h-0 w-full flex-1 flex-col border-border bg-background md:border-l">
          {/* Chat header */}
          <div className="sticky top-0 z-30 flex items-center justify-between border-b border-border bg-background/95 px-4 py-2.5 backdrop-blur md:backdrop-blur-0">
            <div className="flex items-center gap-2.5">
              <div className="flex h-7 w-7 items-center justify-center rounded-lg bg-primary/15 text-primary">
                <Bot className="h-4 w-4" />
              </div>
              <h3 className="text-sm font-semibold leading-none">Willow</h3>
            </div>
            {isChatThinking && <p className="text-xs text-muted-foreground">Replying...</p>}
          </div>

          {/* Chat messages */}
          <div
            ref={chatScrollRef}
            className="min-h-0 flex-1 space-y-4 overflow-auto p-4 md:space-y-5 md:p-5"
          >
            {messages?.status === 'LoadingFirstPage' && (
              <div className="flex items-center justify-center py-8">
                <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
              </div>
            )}

            <div className="mx-auto w-full max-w-4xl space-y-3">
              {hiddenMessageCount > 0 && !showFullHistory && (
                <button
                  onClick={() => setShowFullHistory(true)}
                  className="rounded-full border border-border bg-secondary px-3 py-1 text-xs text-muted-foreground hover:text-foreground"
                >
                  Show {hiddenMessageCount} earlier message{hiddenMessageCount === 1 ? '' : 's'}
                </button>
              )}

              {showFullHistory && hiddenMessageCount > 0 && (
                <button
                  onClick={() => setShowFullHistory(false)}
                  className="rounded-full border border-border bg-secondary px-3 py-1 text-xs text-muted-foreground hover:text-foreground"
                >
                  Collapse earlier history
                </button>
              )}

              {pendingFeedback && (
                <PendingFeedbackCard
                  questionText={pendingFeedback.questionText}
                  conceptTitle={pendingFeedback.conceptTitle}
                  userAnswer={pendingFeedback.userAnswer}
                />
              )}

              {timelineItems.map((item) => {
                if (item.kind === 'message') {
                  return (
                    <ChatMessage
                      key={item.key}
                      message={item.message}
                      userAvatarUrl={user?.imageUrl}
                    />
                  );
                }
                const entry = item.artifact;
                if (entry.type === 'feedback') {
                  return (
                    <FeedbackCard
                      key={item.key}
                      data={entry.data.data}
                      questionText={entry.data.questionText ?? undefined}
                      compact
                    />
                  );
                }
                if (entry.type === 'question') {
                  const isLatestQuestion = entry.id === latestQuestionArtifactId;
                  return (
                    <QuestionCard
                      key={item.key}
                      data={entry.data}
                      onAnswer={handleAnswer}
                      disabled={!isLatestQuestion || isSubmittingAnswer}
                    />
                  );
                }
                if (entry.type === 'action') {
                  return (
                    <ActionPanelCard
                      key={item.key}
                      data={entry.data}
                      className="mb-0 border-primary/20"
                    />
                  );
                }
                return (
                  <div
                    key={item.key}
                    className="rounded-2xl border border-border bg-background p-5 shadow-sm"
                  >
                    <h3 className="font-serif text-xl text-foreground">All done for now</h3>
                    <p className="mt-2 text-sm text-muted-foreground">
                      You have no additional due cards in this session.
                    </p>
                  </div>
                );
              })}

              {isFetchingQuestion && (
                <div className="mt-2 flex items-center gap-2 text-sm text-muted-foreground">
                  <Loader2 className="h-4 w-4 animate-spin" />
                  Finding next concept...
                </div>
              )}

              {isSubmittingAnswer && !pendingFeedback && (
                <div className="mt-2 flex items-center gap-2 text-sm text-muted-foreground">
                  <Loader2 className="h-4 w-4 animate-spin" />
                  Checking answer...
                </div>
              )}
            </div>

            {isChatThinking && (
              <div className="mx-auto flex w-full max-w-4xl items-center gap-1 px-1 py-2">
                <span className="h-1.5 w-1.5 animate-bounce rounded-full bg-muted-foreground/50 [animation-delay:0ms]" />
                <span className="h-1.5 w-1.5 animate-bounce rounded-full bg-muted-foreground/50 [animation-delay:150ms]" />
                <span className="h-1.5 w-1.5 animate-bounce rounded-full bg-muted-foreground/50 [animation-delay:300ms]" />
              </div>
            )}
          </div>

          {/* Suggestion chips */}
          <div className="border-t border-border bg-secondary/95 px-4 py-2.5 backdrop-blur md:bg-secondary md:px-5 md:py-3 md:backdrop-blur-0">
            <div className="flex flex-wrap items-center gap-2">
              {SUGGESTION_CHIPS.map((chip) => (
                <button
                  key={chip.label}
                  onClick={() => {
                    if (chip.intent === 'chat') {
                      focusChat();
                    }
                    void onSuggestionChip(chip);
                  }}
                  disabled={
                    activeChipAction === chip.id ||
                    (chip.needsConceptContext && !activeQuestion && !latestFeedback)
                  }
                  className="rounded-full border border-border bg-background px-3 py-1.5 text-xs text-muted-foreground transition hover:border-primary hover:text-primary disabled:cursor-not-allowed disabled:opacity-50"
                >
                  <span className="inline-flex items-center gap-1.5">
                    {activeChipAction === chip.id && <Loader2 className="h-3 w-3 animate-spin" />}
                    {chip.label}
                  </span>
                </button>
              ))}
              {latestFeedback && (
                <button
                  onClick={() => {
                    void handleNextQuestion();
                  }}
                  className="ml-auto inline-flex items-center gap-1.5 rounded-full bg-primary px-3 py-1.5 text-xs font-semibold text-primary-foreground shadow-sm transition hover:bg-primary/90"
                >
                  <span>Next question</span>
                  <ArrowRight className="h-3.5 w-3.5" />
                </button>
              )}
            </div>
            {rescheduleTarget && (
              <div className="mt-3 rounded-xl border border-border bg-background p-2.5">
                <div className="mb-2 flex items-center justify-between">
                  <p className="text-xs font-medium text-muted-foreground">
                    Reschedule {rescheduleTarget.conceptTitle}
                  </p>
                  <button
                    onClick={onDismissReschedule}
                    className="rounded-md p-1 text-muted-foreground hover:bg-secondary hover:text-foreground"
                  >
                    <X className="h-3.5 w-3.5" />
                  </button>
                </div>
                <div className="flex flex-wrap gap-2">
                  {[1, 3, 7].map((days) => (
                    <button
                      key={days}
                      onClick={() => void onSubmitReschedule(days)}
                      disabled={activeChipAction === 'reschedule'}
                      className="rounded-full border border-border bg-secondary px-2.5 py-1 text-xs text-foreground hover:border-primary disabled:opacity-50"
                    >
                      +{days} day{days === 1 ? '' : 's'}
                    </button>
                  ))}
                </div>
              </div>
            )}
            {actionReply && (
              <div className="mt-3 rounded-xl border border-border bg-background p-3">
                <div className="mb-1 flex items-center justify-between gap-2">
                  <p className="text-xs font-semibold text-foreground">{actionReply.title}</p>
                  <button
                    onClick={onDismissActionReply}
                    className="rounded-md p-1 text-muted-foreground hover:bg-secondary hover:text-foreground"
                  >
                    <X className="h-3.5 w-3.5" />
                  </button>
                </div>
                <p className="text-xs text-muted-foreground">{actionReply.body}</p>
              </div>
            )}
          </div>

          {/* Chat input */}
          <div className="sticky bottom-0 z-30 border-t border-border bg-background/95 p-3 pb-[calc(0.75rem+env(safe-area-inset-bottom))] backdrop-blur md:bg-background md:p-4 md:pb-4 md:backdrop-blur-0">
            <div className="flex items-center gap-2 rounded-2xl border border-border bg-background px-2 py-2 shadow-sm">
              <input
                ref={chatInputRef}
                type="text"
                value={input}
                onChange={(e) => setInput(e.target.value)}
                onKeyDown={handleKeyDown}
                placeholder="Ask anything..."
                className="flex-1 bg-transparent py-2.5 pl-2 pr-2 text-sm outline-none"
              />
              <button
                onClick={() => handleSend(input)}
                disabled={!input.trim()}
                className="shrink-0 rounded-xl bg-primary p-2.5 text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
              >
                <Send className="h-4 w-4" />
              </button>
            </div>
          </div>
        </div>
      </main>
    </div>
  );
}

// ---- Chat sub-components ----

function formatActionDate(value: number) {
  return new Date(value).toLocaleString('en-US', {
    weekday: 'short',
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  });
}

function ActionPanelCard({ data, className }: { data: ActionPanelState; className?: string }) {
  if (data.type === 'notice') {
    return (
      <section
        className={cn(
          'mb-6 max-w-3xl rounded-2xl border border-border bg-background p-5 shadow-sm',
          className
        )}
      >
        <div className="mb-2 inline-flex items-center gap-2 text-xs font-medium text-muted-foreground">
          <TriangleAlert className="h-3.5 w-3.5" />
          Action needed
        </div>
        <h3 className="text-sm font-semibold">{data.title}</h3>
        <p className="mt-1 text-sm text-muted-foreground">{data.description}</p>
      </section>
    );
  }

  if (data.type === 'rescheduled') {
    return (
      <section
        className={cn(
          'mb-6 max-w-3xl rounded-2xl border border-border bg-background p-5 shadow-sm',
          className
        )}
      >
        <div className="mb-2 inline-flex items-center gap-2 text-xs font-medium text-muted-foreground">
          <CalendarClock className="h-3.5 w-3.5" />
          Schedule updated
        </div>
        <h3 className="text-sm font-semibold">{data.conceptTitle}</h3>
        <p className="mt-1 text-sm text-muted-foreground">
          Moved to{' '}
          <span className="font-medium text-foreground">{formatActionDate(data.nextReview)}</span> (
          {data.scheduledDays === 1 ? '1 day' : `${data.scheduledDays} days`}).
        </p>
      </section>
    );
  }

  return (
    <section
      className={cn(
        'mb-6 max-w-3xl rounded-2xl border border-border bg-background p-5 shadow-sm',
        className
      )}
    >
      <div className="mb-3 inline-flex items-center gap-2 text-xs font-medium text-muted-foreground">
        <Target className="h-3.5 w-3.5" />
        Weak areas ({data.itemCount})
      </div>
      <div className="space-y-2">
        {data.items.length === 0 && (
          <p className="text-sm text-muted-foreground">No weak concepts detected yet.</p>
        )}
        {data.items.map((item) => (
          <div
            key={`${item.title}:${item.reps}:${item.lapses}`}
            className="flex items-center justify-between rounded-xl border border-border bg-secondary/40 px-3 py-2"
          >
            <div>
              <p className="text-sm font-medium">{item.title}</p>
              <p className="text-xs text-muted-foreground">
                {formatReviewStageLabel(item.state)} · {item.lapses} lapses · {item.reps} reviews
              </p>
            </div>
            {item.dueNow && (
              <span className="rounded-full bg-primary/10 px-2 py-0.5 text-[11px] font-medium text-primary">
                Due now
              </span>
            )}
          </div>
        ))}
      </div>
    </section>
  );
}

function PendingFeedbackCard({
  questionText,
  conceptTitle,
  userAnswer,
}: {
  questionText: string | null;
  conceptTitle: string | null;
  userAnswer: string;
}) {
  return (
    <section className="mb-6 max-w-3xl overflow-hidden rounded-2xl border border-border bg-background shadow-sm">
      <div className="border-b border-border bg-secondary p-4 md:p-6">
        <p className="mb-2 text-xs font-medium text-muted-foreground">Checking your answer</p>
        <h3 className="font-serif text-xl leading-tight text-foreground">
          {conceptTitle ?? 'Current concept'}
        </h3>
      </div>
      <div className="space-y-3 p-4 md:p-6">
        {questionText && <p className="text-sm text-muted-foreground">{questionText}</p>}
        <p className="rounded-xl border border-border bg-secondary/40 px-3 py-2 text-sm text-foreground">
          Your answer: {userAnswer}
        </p>
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <Loader2 className="h-4 w-4 animate-spin" />
          Grading and updating your schedule...
        </div>
      </div>
    </section>
  );
}

function ChatMessage({ message, userAvatarUrl }: { message: UIMessage; userAvatarUrl?: string }) {
  if (message.role === 'user') {
    return (
      <div className="flex justify-end gap-3">
        <div className="flex flex-1 justify-end">
          <div className="max-w-[85%] rounded-2xl bg-primary px-4 py-3">
            <p className="text-sm leading-relaxed text-primary-foreground">{message.text}</p>
          </div>
        </div>
        {userAvatarUrl ? (
          // eslint-disable-next-line @next/next/no-img-element
          <img src={userAvatarUrl} alt="" className="mt-0.5 h-7 w-7 shrink-0 rounded-full" />
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

  if (textParts.length === 0) return null;

  return (
    <div className="flex gap-3">
      <div className="mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-lg bg-primary/15 text-primary">
        <Bot className="h-3.5 w-3.5" />
      </div>
      <div className="min-w-0 flex-1 space-y-2">
        {textParts.map((part, i) => (
          <MessageBubble key={i} text={part.text} />
        ))}
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
  const jsDay = new Date().getDay();
  const todayIndex = jsDay === 0 ? 6 : jsDay - 1; // Convert Sunday-first to Monday-first index

  const cells: { level: number; isToday: boolean; tip: string }[] = [];
  for (let w = 0; w < WEEKS; w++) {
    for (let d = 0; d < 7; d++) {
      const isToday = w === WEEKS - 1 && d === todayIndex;
      cells.push({
        level: 0,
        isToday,
        tip: isToday
          ? 'Today · Activity history coming soon'
          : `${dayNames[d]} · Activity history coming soon`,
      });
    }
  }
  return cells;
}

function ReviewStatsPanel({ count }: { count: number }) {
  const dashboard = useQuery(api.concepts.getReviewDashboard);
  const cells = useMemo(() => generateHeatmapCells(), []);

  const totalConcepts = dashboard?.totalConcepts ?? 0;
  const totalPhrasings = dashboard?.totalPhrasings ?? 0;
  const totalPhrasingsLabel = totalPhrasings >= 10001 ? '10,000+' : String(totalPhrasings);
  const masteryPercent = dashboard?.masteryPercent ?? 0;
  const streak = dashboard?.streak ?? 0;

  return (
    <div className="relative flex flex-col justify-center overflow-hidden rounded-3xl border border-primary/20 bg-gradient-to-br from-primary to-primary/80 p-6 text-primary-foreground shadow-sm md:p-10">
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
        <div className="flex items-center gap-2.5 rounded-xl bg-white/[0.09] px-3.5 py-2.5">
          <BookOpen className="size-[15px] shrink-0 opacity-[0.35]" />
          <span className="flex-1 font-mono text-[0.5rem] uppercase tracking-[0.06em] opacity-50">
            Concepts
          </span>
          <span className="font-mono text-sm font-semibold tabular-nums">{totalConcepts}</span>
        </div>
        <div className="flex items-center gap-2.5 rounded-xl bg-white/[0.09] px-3.5 py-2.5">
          <MessageSquare className="size-[15px] shrink-0 opacity-[0.35]" />
          <span className="flex-1 font-mono text-[0.5rem] uppercase tracking-[0.06em] opacity-50">
            Phrasings
          </span>
          <span className="font-mono text-sm font-semibold tabular-nums">
            {totalPhrasingsLabel}
          </span>
        </div>
        <div className="flex items-center gap-2.5 rounded-xl bg-white/[0.09] px-3.5 py-2.5">
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
            Activity (preview)
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

        <div className="mt-1.5 flex items-center justify-between">
          <span className="font-mono text-[0.5rem] opacity-[0.35]">28 weeks</span>
          <span className="font-mono text-[0.5rem] opacity-[0.45]">
            Daily activity integration pending
          </span>
        </div>
      </div>
    </div>
  );
}
