import { Clock } from 'lucide-react';

interface ReviewDueCountProps {
  count: number;
}

export function ReviewDueCount({ count }: ReviewDueCountProps) {
  return (
    <div className="inline-flex items-center gap-2 px-4 py-2 rounded-full bg-card border border-border/50 shadow-sm">
      <Clock className="h-4 w-4 text-muted-foreground" />
      <span className="text-sm font-medium tabular-nums">
        <span className="text-foreground">{count}</span>
        <span className="text-muted-foreground ml-1">concepts due</span>
      </span>
    </div>
  );
}
