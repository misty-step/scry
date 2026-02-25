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

    expect(screen.getByText('Answer Breakdown')).toBeInTheDocument();
    expect(screen.getByRole('list')).toBeInTheDocument();
    expect(screen.getAllByRole('listitem')).toHaveLength(2);
    expect(screen.getByText('Stage')).toBeInTheDocument();
    expect(screen.getByText('Reviews')).toBeInTheDocument();
    const strongText = screen.getByText('Mars', { selector: 'strong' });
    const codeText = screen.getByText('mnemonic', { selector: 'code' });
    expect(strongText.tagName).toBe('STRONG');
    expect(codeText.tagName).toBe('CODE');
  });

  it('renders mixed paragraphs and lists from one explanation block', () => {
    render(
      <FeedbackCard
        data={{
          isCorrect: false,
          userAnswer: 'Mercury',
          correctAnswer: 'Mars',
          explanation:
            'Quick summary before the list:\n- First bullet\n- Second bullet\nFinal reminder line.',
          scheduledDays: 2,
          newState: 'learning',
          reps: 2,
        }}
      />
    );

    expect(screen.getByText('Quick summary before the list:')).toBeInTheDocument();
    expect(screen.getByRole('list')).toBeInTheDocument();
    expect(screen.getByText('Final reminder line.')).toBeInTheDocument();
  });

  it('keeps markdown markers inside inline code and supports nested emphasis', () => {
    render(
      <FeedbackCard
        data={{
          isCorrect: true,
          correctAnswer: 'Mars',
          explanation: 'Use `**literal**` and **focus on *orbit* cues**.',
          scheduledDays: 1,
          newState: 'review',
          reps: 8,
        }}
      />
    );

    const codeText = screen.getByText('**literal**', { selector: 'code' });
    expect(codeText.tagName).toBe('CODE');
    expect(screen.getByText(/focus on/i, { selector: 'strong' })).toBeInTheDocument();
    expect(screen.getByText('orbit', { selector: 'em' })).toBeInTheDocument();
  });

  it('returns null for invalid payloads', () => {
    const { container } = render(
      <FeedbackCard data={null as unknown as Record<string, unknown>} />
    );
    expect(container.firstChild).toBeNull();
  });
});
