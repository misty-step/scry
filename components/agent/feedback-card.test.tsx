import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { FeedbackCard } from './feedback-card';

describe('FeedbackCard', () => {
  it('renders markdown-like explanation with lists and inline formatting', () => {
    render(
      <FeedbackCard
        data={{
          isCorrect: false,
          userAnswer: 'Venus',
          correctAnswer: 'Mars',
          explanation:
            'Remember the core rule.\n\n- **Mars** is called the red planet\n- Use `mnemonic` anchors',
          scheduledDays: 3,
          newState: 'learning',
          reps: 4,
        }}
        questionText="Which planet is known as the red planet?"
      />
    );

    expect(screen.getByText('Why This Matters')).toBeInTheDocument();
    expect(screen.getByRole('list')).toBeInTheDocument();
    expect(screen.getAllByRole('listitem')).toHaveLength(2);
    const strongText = screen.getByText('Mars', { selector: 'strong' });
    const codeText = screen.getByText('mnemonic', { selector: 'code' });
    expect(strongText.tagName).toBe('STRONG');
    expect(codeText.tagName).toBe('CODE');
  });

  it('returns null for invalid payloads', () => {
    const { container } = render(
      <FeedbackCard data={null as unknown as Record<string, unknown>} />
    );
    expect(container.firstChild).toBeNull();
  });
});
