import { render } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';

// Test ensuring Tooltip crashes without Provider (validating our need for the fix)
// and works with Provider (validating our layout fix pattern)
describe('Tooltip Integration', () => {
  const TooltipComponent = () => (
    <Tooltip>
      <TooltipTrigger>Hover me</TooltipTrigger>
      <TooltipContent>Content</TooltipContent>
    </Tooltip>
  );

  it('should throw error when rendered without TooltipProvider', () => {
    // Suppress console.error for this test since we expect a React error
    const originalError = console.error;
    console.error = () => {};

    expect(() => render(<TooltipComponent />)).toThrow();

    console.error = originalError;
  });

  it('should render successfully when wrapped in TooltipProvider', () => {
    const { getByText } = render(
      <TooltipProvider>
        <TooltipComponent />
      </TooltipProvider>
    );

    expect(getByText('Hover me')).toBeInTheDocument();
  });
});
