'use client';

export function MessageBubble({ text }: { text: string }) {
  if (!text.trim()) return null;
  return (
    <div className="bg-muted max-w-[85%] rounded-2xl rounded-bl-sm px-4 py-2">
      <p className="whitespace-pre-wrap text-sm">{text}</p>
    </div>
  );
}
