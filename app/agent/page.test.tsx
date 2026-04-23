import { permanentRedirect } from 'next/navigation';
import { describe, expect, it, vi } from 'vitest';
import AgentPage from './page';

vi.mock('next/navigation', () => ({
  permanentRedirect: vi.fn(),
}));

describe('AgentPage', () => {
  it('permanently redirects legacy agent links to home', () => {
    AgentPage();

    expect(permanentRedirect).toHaveBeenCalledWith('/');
  });
});
