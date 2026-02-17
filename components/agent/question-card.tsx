'use client';

import { Badge } from '@/components/ui/badge';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';

interface QuestionData {
  conceptTitle?: string;
  fsrsState?: string;
  question?: string;
  type?: string;
  options?: string[];
  retrievability?: number;
  lapses?: number;
}

export function QuestionCard({ data }: { data: Record<string, unknown> }) {
  const q = data as unknown as QuestionData;
  if (!q.question) return null;

  const stateColors: Record<string, string> = {
    new: 'bg-blue-500/10 text-blue-500',
    learning: 'bg-yellow-500/10 text-yellow-500',
    review: 'bg-green-500/10 text-green-500',
    relearning: 'bg-red-500/10 text-red-500',
  };

  return (
    <Card className="border-primary/20 my-2">
      <CardHeader className="pb-2">
        <div className="flex items-center justify-between">
          <CardTitle className="text-sm font-medium">{q.conceptTitle}</CardTitle>
          <Badge variant="outline" className={stateColors[q.fsrsState ?? 'new'] ?? ''}>
            {q.fsrsState ?? 'new'}
          </Badge>
        </div>
      </CardHeader>
      <CardContent>
        <p className="mb-3 font-medium">{q.question}</p>
        {q.options && q.options.length > 0 && (
          <div className="space-y-1.5">
            {q.options.map((opt, i) => (
              <div key={i} className="bg-muted/50 rounded-md px-3 py-2 text-sm">
                {i + 1}. {opt}
              </div>
            ))}
          </div>
        )}
        {q.lapses != null && q.lapses > 0 && (
          <p className="text-muted-foreground mt-2 text-xs">
            {q.lapses} lapse{q.lapses !== 1 ? 's' : ''}
          </p>
        )}
      </CardContent>
    </Card>
  );
}
