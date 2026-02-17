'use client';

import { useCallback, useEffect, useRef, useState } from 'react';
import type { UIMessage } from '@convex-dev/agent/react';
import { useUIMessages } from '@convex-dev/agent/react';
import { useAction, useMutation } from 'convex/react';
import { Send } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { api } from '@/convex/_generated/api';
import { FeedbackCard } from './feedback-card';
import { MessageBubble } from './message-bubble';
import { QuestionCard } from './question-card';

export function ReviewChat() {
  const [threadId, setThreadId] = useState<string | null>(null);
  const [input, setInput] = useState('');
  const [isStarting, setIsStarting] = useState(false);
  const scrollRef = useRef<HTMLDivElement>(null);

  const createThread = useMutation(api.agents.reviewStreaming.createReviewThread);
  const sendMessage = useMutation(api.agents.reviewStreaming.sendMessage);
  const startSession = useAction(api.agents.reviewStreaming.startSession);

  const messages = useUIMessages(
    api.agents.reviewStreaming.listMessages,
    threadId ? { threadId } : 'skip',
    { initialNumItems: 50, stream: true }
  );

  // Auto-scroll on new messages
  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [messages?.results]);

  const handleStart = useCallback(async () => {
    setIsStarting(true);
    try {
      const { threadId: newThreadId } = await createThread();
      setThreadId(newThreadId);
      await startSession({ threadId: newThreadId });
    } finally {
      setIsStarting(false);
    }
  }, [createThread, startSession]);

  const handleSend = useCallback(
    async (text: string) => {
      if (!threadId || !text.trim()) return;
      setInput('');
      await sendMessage({ threadId, prompt: text.trim() });
    },
    [threadId, sendMessage]
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

  // Pre-session: show start button
  if (!threadId) {
    return (
      <div className="flex min-h-[60vh] flex-col items-center justify-center gap-4">
        <h1 className="text-2xl font-semibold">Review Session</h1>
        <p className="text-muted-foreground max-w-md text-center">
          Your AI tutor will present concepts, evaluate your answers, and provide feedback.
        </p>
        <Button size="lg" onClick={handleStart} disabled={isStarting}>
          {isStarting ? 'Starting...' : 'Start Review'}
        </Button>
      </div>
    );
  }

  const messageList = messages?.results ?? [];

  return (
    <div className="mx-auto flex h-[calc(100vh-8rem)] max-w-2xl flex-col">
      {/* Messages */}
      <div ref={scrollRef} className="flex-1 space-y-4 overflow-y-auto px-4 py-6">
        {messageList.map((message: UIMessage) => (
          <div key={message.key}>
            {message.role === 'user' ? (
              <div className="flex justify-end">
                <div className="bg-primary text-primary-foreground max-w-[80%] rounded-2xl rounded-br-sm px-4 py-2">
                  <p>{message.text}</p>
                </div>
              </div>
            ) : (
              <div className="flex flex-col gap-2">
                {message.parts.map((part, i) => {
                  if (part.type === 'text' && 'text' in part && part.text) {
                    return <MessageBubble key={i} text={part.text} />;
                  }
                  if (part.type === 'tool-invocation' && 'state' in part) {
                    return renderToolResult(part, i);
                  }
                  return null;
                })}
              </div>
            )}
          </div>
        ))}

        {messages?.status === 'LoadingFirstPage' && (
          <div className="text-muted-foreground animate-pulse text-sm">Thinking...</div>
        )}
      </div>

      {/* Input */}
      <div className="border-t px-4 py-3">
        <div className="flex gap-2">
          <input
            type="text"
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="Type your answer or message..."
            className="bg-muted flex-1 rounded-lg px-4 py-2 text-sm outline-none"
          />
          <Button size="icon" onClick={() => handleSend(input)} disabled={!input.trim()}>
            <Send className="h-4 w-4" />
          </Button>
        </div>
      </div>
    </div>
  );
}

function renderToolResult(
  part: { type: string; toolName?: string; state?: string; result?: unknown },
  key: number
) {
  if (part.state !== 'result') return null;

  if (part.toolName === 'fetchDueConcept' && part.result) {
    return <QuestionCard key={key} data={part.result as Record<string, unknown>} />;
  }
  if (part.toolName === 'evaluateAnswer' && part.result) {
    return <FeedbackCard key={key} data={part.result as Record<string, unknown>} />;
  }
  return null;
}
