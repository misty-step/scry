import { beforeEach, describe, expect, it, vi } from 'vitest';

// Create a mock flushAsync that we can track
const mockFlushAsync = vi.fn().mockResolvedValue(undefined);

// Mock the Langfuse module with a proper constructor class
const MockLangfuse = vi.fn().mockImplementation(function () {
  return { flushAsync: mockFlushAsync };
});

vi.mock('langfuse', () => ({
  Langfuse: MockLangfuse,
}));

const originalEnv = { ...process.env };

describe('langfuse module', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.resetModules();
    process.env = { ...originalEnv };
    delete process.env.LANGFUSE_SECRET_KEY;
    delete process.env.LANGFUSE_PUBLIC_KEY;
    delete process.env.LANGFUSE_HOST;
  });

  describe('isLangfuseConfigured', () => {
    it('returns true when both keys are set', async () => {
      process.env.LANGFUSE_SECRET_KEY = 'secret-key';
      process.env.LANGFUSE_PUBLIC_KEY = 'public-key';
      const { isLangfuseConfigured } = await import('./langfuse');

      expect(isLangfuseConfigured()).toBe(true);
    });

    it('returns false when secret key is missing', async () => {
      process.env.LANGFUSE_PUBLIC_KEY = 'public-key';
      const { isLangfuseConfigured } = await import('./langfuse');

      expect(isLangfuseConfigured()).toBe(false);
    });

    it('returns false when public key is missing', async () => {
      process.env.LANGFUSE_SECRET_KEY = 'secret-key';
      const { isLangfuseConfigured } = await import('./langfuse');

      expect(isLangfuseConfigured()).toBe(false);
    });

    it('returns false when both keys are missing', async () => {
      const { isLangfuseConfigured } = await import('./langfuse');

      expect(isLangfuseConfigured()).toBe(false);
    });
  });

  describe('getLangfuse', () => {
    it('throws when not configured', async () => {
      const { getLangfuse } = await import('./langfuse');

      expect(() => getLangfuse()).toThrow(
        'Langfuse not configured: LANGFUSE_SECRET_KEY and LANGFUSE_PUBLIC_KEY required'
      );
    });

    it('creates instance with correct config when configured', async () => {
      process.env.LANGFUSE_SECRET_KEY = 'sk-test';
      process.env.LANGFUSE_PUBLIC_KEY = 'pk-test';
      const { Langfuse } = await import('langfuse');
      const { getLangfuse } = await import('./langfuse');

      const instance = getLangfuse();

      expect(Langfuse).toHaveBeenCalledWith({
        secretKey: 'sk-test',
        publicKey: 'pk-test',
        baseUrl: 'https://cloud.langfuse.com',
        flushAt: 1,
      });
      expect(instance).toBeDefined();
    });

    it('uses custom host when LANGFUSE_HOST is set', async () => {
      process.env.LANGFUSE_SECRET_KEY = 'sk-test';
      process.env.LANGFUSE_PUBLIC_KEY = 'pk-test';
      process.env.LANGFUSE_HOST = 'https://us.cloud.langfuse.com';
      const { Langfuse } = await import('langfuse');
      const { getLangfuse } = await import('./langfuse');

      getLangfuse();

      expect(Langfuse).toHaveBeenCalledWith(
        expect.objectContaining({
          baseUrl: 'https://us.cloud.langfuse.com',
        })
      );
    });

    it('returns singleton instance on subsequent calls', async () => {
      process.env.LANGFUSE_SECRET_KEY = 'sk-test';
      process.env.LANGFUSE_PUBLIC_KEY = 'pk-test';
      const { Langfuse } = await import('langfuse');
      const { getLangfuse } = await import('./langfuse');

      const instance1 = getLangfuse();
      const instance2 = getLangfuse();

      expect(instance1).toBe(instance2);
      expect(Langfuse).toHaveBeenCalledTimes(1);
    });
  });

  describe('flushLangfuse', () => {
    it('does nothing when no instance exists', async () => {
      const { flushLangfuse } = await import('./langfuse');

      // Should not throw
      await expect(flushLangfuse()).resolves.toBeUndefined();
    });

    it('calls flushAsync when instance exists', async () => {
      process.env.LANGFUSE_SECRET_KEY = 'sk-test';
      process.env.LANGFUSE_PUBLIC_KEY = 'pk-test';
      const { getLangfuse, flushLangfuse } = await import('./langfuse');

      getLangfuse();
      await flushLangfuse();

      expect(mockFlushAsync).toHaveBeenCalled();
    });
  });
});
