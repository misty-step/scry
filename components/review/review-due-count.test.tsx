import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { ReviewDueCount } from './review-due-count';

describe('ReviewDueCount', () => {
  it('renders the due concepts count', () => {
    render(<ReviewDueCount count={12} />);

    expect(screen.getByText('12')).toBeInTheDocument();
    expect(screen.getByText('concepts due')).toBeInTheDocument();
  });
});
