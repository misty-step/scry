'use client';

import { CheckCircle2, XCircle } from 'lucide-react';
import { Card, CardContent } from '@/components/ui/card';

interface FeedbackData {
  isCorrect?: boolean;
  correctAnswer?: string;
}

export function FeedbackCard({ data }: { data: Record<string, unknown> }) {
  const fb = data as unknown as FeedbackData;

  return (
    <Card className={`my-2 ${fb.isCorrect ? 'border-green-500/30' : 'border-red-500/30'}`}>
      <CardContent className="flex items-start gap-3 pt-4">
        {fb.isCorrect ? (
          <CheckCircle2 className="mt-0.5 h-5 w-5 shrink-0 text-green-500" />
        ) : (
          <XCircle className="mt-0.5 h-5 w-5 shrink-0 text-red-500" />
        )}
        <div>
          <p className="font-medium">{fb.isCorrect ? 'Correct!' : 'Incorrect'}</p>
          {!fb.isCorrect && fb.correctAnswer && (
            <p className="text-muted-foreground mt-1 text-sm">
              The correct answer is: <span className="font-medium">{fb.correctAnswer}</span>
            </p>
          )}
        </div>
      </CardContent>
    </Card>
  );
}
