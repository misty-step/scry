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

  it('navigates to home with "h" shortcut', () => {
    renderHook(() => useKeyboardShortcuts([]));

    act(() => {
      triggerKey('h');
    });

    expect(push).toHaveBeenCalledWith('/');
  });

  it('navigates to concepts with "c" shortcut', () => {
    renderHook(() => useKeyboardShortcuts([]));

    act(() => {
      triggerKey('c');
    });

    expect(push).toHaveBeenCalledWith('/concepts');
  });

  it('dispatches escape-pressed event on Escape', () => {
    renderHook(() => useKeyboardShortcuts([]));

    act(() => {
      triggerKey('Escape');
    });

    expect(dispatchEventSpy).toHaveBeenCalledWith(
      expect.objectContaining({ type: 'escape-pressed' })
    );
  });

  it('does not handle shortcuts when disabled', () => {
    renderHook(() => useKeyboardShortcuts([], false));

    act(() => {
      triggerKey('h');
    });

    expect(push).not.toHaveBeenCalled();
  });

  it('handles metaKey as ctrl modifier (Mac)', () => {
    renderHook(() => useKeyboardShortcuts([]));

    act(() => {
      triggerKey('s', { metaKey: true });
    });

    expect(push).toHaveBeenCalledWith('/settings');
  });

  it('executes custom shortcuts', () => {
    const customAction = vi.fn();

    renderHook(() =>
      useKeyboardShortcuts([
        {
          key: 'p',
          description: 'Custom action',
          action: customAction,
        },
      ])
    );

    act(() => {
      triggerKey('p');
    });

    expect(customAction).toHaveBeenCalledTimes(1);
  });

  it('handles alt modifier shortcuts', () => {
    const altAction = vi.fn();

    renderHook(() =>
      useKeyboardShortcuts([
        {
          key: 'a',
          alt: true,
          description: 'Alt shortcut',
          action: altAction,
        },
      ])
    );

    act(() => {
      triggerKey('a', { altKey: true });
    });

    expect(altAction).toHaveBeenCalledTimes(1);
  });

  it('handles shift modifier shortcuts', () => {
    const shiftAction = vi.fn();

    renderHook(() =>
      useKeyboardShortcuts([
        {
          key: 'S',
          shift: true,
          description: 'Shift shortcut',
          action: shiftAction,
        },
      ])
    );

    act(() => {
      triggerKey('S', { shiftKey: true });
    });

    expect(shiftAction).toHaveBeenCalledTimes(1);
  });

  it('returns combined shortcuts list', () => {
    const { result } = renderHook(() =>
      useKeyboardShortcuts([{ key: 'custom', description: 'Custom', action: vi.fn() }])
    );

    // Should have both global shortcuts and the custom one
    expect(result.current.shortcuts.length).toBeGreaterThan(1);
    expect(result.current.shortcuts.some((s) => s.key === 'custom')).toBe(true);
    expect(result.current.shortcuts.some((s) => s.key === '?')).toBe(true);
  });

  it('allows modifier shortcuts while typing in input', () => {
    const input = document.createElement('input');
    document.body.appendChild(input);

    renderHook(() => useKeyboardShortcuts([]));

    // Simulate typing in input with modifier - events bubble to window
    act(() => {
      input.focus();
      // Dispatch on window but with input as target simulation
      const event = new KeyboardEvent('keydown', {
        key: 's',
        ctrlKey: true,
        bubbles: true,
      });
      Object.defineProperty(event, 'target', { value: input, writable: false });
      window.dispatchEvent(event);
    });

    expect(push).toHaveBeenCalledWith('/settings');
  });
});
