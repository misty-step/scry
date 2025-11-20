'use client';

import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { cn } from '@/lib/utils';

export interface ViewOption {
  value: string;
  label: string;
}

interface ViewSelectorProps {
  value: string;
  onValueChange: (value: string) => void;
  options: ViewOption[];
}

/**
 * ViewSelector - Responsive view switching control
 *
 * Deep module: Simple interface (value/onChange) hides responsive complexity.
 * Mobile: Horizontal scroll pills for touch-friendly, space-efficient selection.
 * Desktop: Standard TabsList for familiar keyboard-navigable tabs.
 *
 * Uses CSS-only responsive switching (no JS media queries).
 */
export function ViewSelector({ value, onValueChange, options }: ViewSelectorProps) {
  return (
    <>
      {/* Mobile: Horizontal scroll pills */}
      <div className="md:hidden">
        <div
          className={cn(
            'flex gap-1.5 overflow-x-auto pb-2 -mx-4 px-4',
            'scrollbar-hide scroll-smooth snap-x snap-mandatory'
          )}
        >
          {options.map((option) => (
            <button
              key={option.value}
              onClick={() => onValueChange(option.value)}
              className={cn(
                'shrink-0 snap-start px-3 py-1.5 rounded-full text-sm font-medium',
                'whitespace-nowrap transition-colors',
                'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
                value === option.value
                  ? 'bg-primary text-primary-foreground'
                  : 'bg-muted text-muted-foreground hover:bg-muted/80'
              )}
            >
              {option.label}
            </button>
          ))}
        </div>
      </div>

      {/* Desktop: Standard tabs */}
      <div className="hidden md:block">
        <Tabs value={value} onValueChange={onValueChange}>
          <TabsList className="inline-flex">
            {options.map((option) => (
              <TabsTrigger
                key={option.value}
                value={option.value}
                className="px-3 py-1.5"
              >
                {option.label}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
      </div>
    </>
  );
}
