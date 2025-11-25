import { act, renderHook } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { useKeyboardShortcuts } from './use-keyboard-shortcuts';

const push = vi.fn();
const toastInfo = vi.fn();
const dispatchEventSpy = vi.spyOn(window, 'dispatchEvent');

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push }),
}));

vi.mock('sonner', () => ({
  toast: { info: (...args: unknown[]) => toastInfo(...args) },
}));

describe('useKeyboardShortcuts', () => {
  beforeEach(() => {
    push.mockReset();
    toastInfo.mockReset();
    dispatchEventSpy.mockClear();
  });

  const triggerKey = (
    key: string,
    options: KeyboardEventInit = {},
    target: EventTarget = window
  ) => {
    const event = new KeyboardEvent('keydown', { key, ...options });
    (target as EventTarget).dispatchEvent(event);
    return event;
  };

  it('toggles help visibility with "?" shortcut', () => {
    const { result } = renderHook(() => useKeyboardShortcuts([]));

    act(() => {
      triggerKey('?');
    });

    expect(result.current.showHelp).toBe(true);

    act(() => {
      triggerKey('?');
    });

    expect(result.current.showHelp).toBe(false);
  });

  it('ignores shortcuts while typing without modifiers', () => {
    const input = document.createElement('input');
    document.body.appendChild(input);

    renderHook(() => useKeyboardShortcuts([]));

    act(() => {
      input.focus();
      triggerKey('h', {}, input);
    });

    expect(push).not.toHaveBeenCalled();
  });

  it('handles settings shortcut with ctrl+s and shows toast', () => {
    renderHook(() => useKeyboardShortcuts([]));

    act(() => {
      triggerKey('s', { ctrlKey: true });
    });

    expect(push).toHaveBeenCalledWith('/settings');
    expect(toastInfo).toHaveBeenCalledWith('Opening settings...');
  });

  it('dispatches generation modal event on "g"', () => {
    renderHook(() => useKeyboardShortcuts([]));

    act(() => {
      triggerKey('g');
    });

    expect(dispatchEventSpy).toHaveBeenCalledWith(
      expect.objectContaining({ type: 'open-generation-modal' })
    );
  });
});
