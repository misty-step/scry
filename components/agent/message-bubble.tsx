'use client';

/**
 * Renders agent text with basic markdown-like formatting.
 * Handles **bold**, *italic*, and line breaks without a full markdown parser.
 */
function renderFormattedText(text: string) {
  const parts = text.split(/(\*\*[^*]+\*\*|\*[^*]+\*)/g);
  return parts.map((part, i) => {
    if (part.startsWith('**') && part.endsWith('**')) {
      return <strong key={i}>{part.slice(2, -2)}</strong>;
    }
    if (part.startsWith('*') && part.endsWith('*')) {
      return <em key={i}>{part.slice(1, -1)}</em>;
    }
    return part;
  });
}

export function MessageBubble({ text }: { text: string }) {
  if (!text.trim()) return null;
  return (
    <div className="border border-border bg-muted px-4 py-3">
      <p className="whitespace-pre-wrap text-sm leading-relaxed text-foreground">
        {renderFormattedText(text)}
      </p>
    </div>
  );
}
