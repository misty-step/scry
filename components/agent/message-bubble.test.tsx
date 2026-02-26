import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { MessageBubble } from './message-bubble';

describe('MessageBubble', () => {
  it('renders inline markdown emphasis and code snippets', () => {
    render(<MessageBubble text="Use **focus** and *recall* with `cue`." />);

    expect(screen.getByText('focus', { selector: 'strong' })).toBeInTheDocument();
    expect(screen.getByText('recall', { selector: 'em' })).toBeInTheDocument();
    expect(screen.getByText('cue', { selector: 'code' })).toBeInTheDocument();
  });

  it('returns null for blank text', () => {
    const { container } = render(<MessageBubble text="   " />);
    expect(container.firstChild).toBeNull();
  });
});
