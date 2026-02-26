'use client';

import { renderInlineMarkdown } from './rich-text';

export function MessageBubble({ text }: { text: string }) {
  if (!text.trim()) return null;
  return (
    <div className="rounded-2xl border border-border bg-muted px-4 py-3">
      <p className="whitespace-pre-wrap text-sm leading-relaxed text-foreground">
        {renderInlineMarkdown(text)}
      </p>
    </div>
  );
}
