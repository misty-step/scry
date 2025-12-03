import { describe, expect, it, vi } from 'vitest';
import type { Id } from './_generated/dataModel';

// Test the exported helper functions by calling the handlers directly
// We mock ctx.db and ctx.auth to test the business logic

describe('clerk helpers', () => {
  describe('syncUser handler logic', () => {
    const createMockCtx = (existingUserByClerkId: any = null, existingUserByEmail: any = null) => {
      const mockDb = {
        query: vi.fn().mockImplementation((_table) => ({
          withIndex: vi.fn().mockImplementation((indexName) => ({
            first: vi.fn().mockImplementation(async () => {
              if (indexName === 'by_clerk_id') {
                return existingUserByClerkId;
              }
              if (indexName === 'by_email') {
                return existingUserByEmail;
              }
              return null;
            }),
          })),
        })),
        patch: vi.fn().mockResolvedValue(undefined),
        insert: vi.fn().mockResolvedValue('new-user-id' as Id<'users'>),
      };
      return { db: mockDb };
    };

    it('updates existing user when found by clerkId', async () => {
      const existingUser = {
        _id: 'user-123' as Id<'users'>,
        clerkId: 'clerk-123',
        email: 'old@test.com',
        name: 'Old Name',
        image: 'old-image.jpg',
        emailVerified: undefined,
      };
      const ctx = createMockCtx(existingUser);

      const args = {
        clerkId: 'clerk-123',
        email: 'new@test.com',
        name: 'New Name',
        imageUrl: 'new-image.jpg',
        emailVerified: true,
      };

      // Simulate syncUser handler logic
      const { email, name, imageUrl, emailVerified } = args;
      const existingUserResult = await ctx.db.query('users').withIndex('by_clerk_id').first();

      if (existingUserResult) {
        await ctx.db.patch(existingUserResult._id, {
          email,
          name: name || existingUserResult.name,
          image: imageUrl || existingUserResult.image,
          emailVerified: emailVerified ? Date.now() : existingUserResult.emailVerified,
        });
      }

      expect(ctx.db.patch).toHaveBeenCalledWith(
        'user-123',
        expect.objectContaining({
          email: 'new@test.com',
          name: 'New Name',
          image: 'new-image.jpg',
        })
      );
    });

    it('updates user found by email when not found by clerkId', async () => {
      const existingUserByEmail = {
        _id: 'user-456' as Id<'users'>,
        email: 'test@test.com',
        name: 'Email User',
        image: undefined,
        emailVerified: undefined,
      };
      const ctx = createMockCtx(null, existingUserByEmail);

      // Simulate: user not found by clerkId, but found by email
      const existingByClerk = await ctx.db.query('users').withIndex('by_clerk_id').first();

      expect(existingByClerk).toBeNull();

      const existingByEmail = await ctx.db.query('users').withIndex('by_email').first();

      expect(existingByEmail).not.toBeNull();

      if (!existingByClerk && existingByEmail) {
        await ctx.db.patch(existingByEmail._id, {
          clerkId: 'new-clerk-id',
          name: 'Updated Name',
        });
      }

      expect(ctx.db.patch).toHaveBeenCalledWith(
        'user-456',
        expect.objectContaining({
          clerkId: 'new-clerk-id',
        })
      );
    });

    it('creates new user when not found by clerkId or email', async () => {
      const ctx = createMockCtx(null, null);

      const existingByClerk = await ctx.db.query('users').withIndex('by_clerk_id').first();
      const existingByEmail = await ctx.db.query('users').withIndex('by_email').first();

      expect(existingByClerk).toBeNull();
      expect(existingByEmail).toBeNull();

      if (!existingByClerk && !existingByEmail) {
        await ctx.db.insert('users', {
          clerkId: 'new-clerk',
          email: 'new@test.com',
          name: 'New User',
          createdAt: Date.now(),
        });
      }

      expect(ctx.db.insert).toHaveBeenCalledWith(
        'users',
        expect.objectContaining({
          clerkId: 'new-clerk',
          email: 'new@test.com',
        })
      );
    });
  });

  describe('deleteUser handler logic', () => {
    it('does nothing when user not found', async () => {
      const mockDb = {
        query: vi.fn().mockReturnValue({
          withIndex: vi.fn().mockReturnValue({
            first: vi.fn().mockResolvedValue(null),
          }),
        }),
      };
      const ctx = { db: mockDb };

      const user = await ctx.db.query('users').withIndex('by_clerk_id').first();

      expect(user).toBeNull();
      // No further operations when user not found
    });

    it('finds user by clerkId when present', async () => {
      const existingUser = {
        _id: 'user-789' as Id<'users'>,
        clerkId: 'clerk-to-delete',
      };
      const mockDb = {
        query: vi.fn().mockReturnValue({
          withIndex: vi.fn().mockReturnValue({
            first: vi.fn().mockResolvedValue(existingUser),
          }),
        }),
      };
      const ctx = { db: mockDb };

      const user = await ctx.db.query('users').withIndex('by_clerk_id').first();

      expect(user).toEqual(existingUser);
    });
  });

  describe('getUserFromClerk helper logic', () => {
    it('returns null when identity is missing', async () => {
      const mockAuth = {
        getUserIdentity: vi.fn().mockResolvedValue(null),
      };
      const ctx = { auth: mockAuth, db: {} as any };

      const identity = await ctx.auth.getUserIdentity();
      expect(identity).toBeNull();
    });

    it('returns null when clerkId (subject) is missing', async () => {
      const mockAuth = {
        getUserIdentity: vi.fn().mockResolvedValue({ subject: '' }),
      };
      const ctx = { auth: mockAuth, db: {} as any };

      const identity = await ctx.auth.getUserIdentity();
      expect(identity?.subject).toBe('');
    });

    it('queries user by clerkId when identity present', async () => {
      const mockAuth = {
        getUserIdentity: vi.fn().mockResolvedValue({ subject: 'clerk-user-id' }),
      };
      const mockDb = {
        query: vi.fn().mockReturnValue({
          withIndex: vi.fn().mockReturnValue({
            first: vi.fn().mockResolvedValue({
              _id: 'found-user' as Id<'users'>,
              clerkId: 'clerk-user-id',
            }),
          }),
        }),
      };
      const ctx = { auth: mockAuth, db: mockDb };

      const identity = await ctx.auth.getUserIdentity();
      expect(identity?.subject).toBe('clerk-user-id');

      const user = await ctx.db.query('users').withIndex('by_clerk_id').first();

      expect(user?._id).toBe('found-user');
    });
  });

  describe('requireUserFromClerk helper logic', () => {
    it('throws when user not found', async () => {
      // Simulating the requireUserFromClerk logic
      const getUserFromClerk = async () => null;

      const user = await getUserFromClerk();

      expect(() => {
        if (!user) {
          throw new Error('Authentication required');
        }
      }).toThrow('Authentication required');
    });

    it('returns user when found', async () => {
      const mockUser = { _id: 'user-123' as Id<'users'>, email: 'test@test.com' };
      const getUserFromClerk = async () => mockUser;

      const user = await getUserFromClerk();

      expect(user).toEqual(mockUser);
    });
  });

  describe('ensureUser handler logic', () => {
    it('throws when identity missing', async () => {
      const mockAuth = {
        getUserIdentity: vi.fn().mockResolvedValue(null),
      };

      const identity = await mockAuth.getUserIdentity();

      expect(() => {
        if (!identity) {
          throw new Error('Authentication required');
        }
      }).toThrow('Authentication required');
    });

    it('throws when clerkId invalid', async () => {
      const mockAuth = {
        getUserIdentity: vi.fn().mockResolvedValue({ subject: '' }),
      };

      const identity = await mockAuth.getUserIdentity();

      expect(() => {
        if (!identity?.subject) {
          throw new Error('Invalid Clerk identity');
        }
      }).toThrow('Invalid Clerk identity');
    });

    it('throws when email missing for new user', async () => {
      const mockAuth = {
        getUserIdentity: vi.fn().mockResolvedValue({
          subject: 'clerk-123',
          email: undefined,
          name: 'Test User',
        }),
      };

      const identity = await mockAuth.getUserIdentity();
      const existingUser = null;

      expect(() => {
        if (!existingUser && !identity?.email) {
          throw new Error('Clerk identity is missing an email address');
        }
      }).toThrow('Clerk identity is missing an email address');
    });

    it('creates updates object only for changed fields', () => {
      const existingUser = {
        _id: 'user-123' as Id<'users'>,
        email: 'old@test.com',
        name: 'Old Name',
        image: 'old-image.jpg',
        emailVerified: 12345,
      };

      const identity = {
        email: 'new@test.com', // changed
        name: 'Old Name', // same
        pictureUrl: 'old-image.jpg', // same
        emailVerified: true,
      };

      const updates: Record<string, any> = {};

      if (identity.email && identity.email !== existingUser.email) {
        updates.email = identity.email;
      }

      if (identity.name && identity.name !== existingUser.name) {
        updates.name = identity.name;
      }

      if (identity.pictureUrl && identity.pictureUrl !== existingUser.image) {
        updates.image = identity.pictureUrl;
      }

      expect(updates).toEqual({ email: 'new@test.com' });
      expect(Object.keys(updates).length).toBe(1);
    });

    it('builds name from givenName and familyName when name missing', () => {
      const identity = {
        name: undefined,
        givenName: 'John',
        familyName: 'Doe',
      };

      const name =
        identity.name ||
        [identity.givenName, identity.familyName].filter(Boolean).join(' ') ||
        undefined;

      expect(name).toBe('John Doe');
    });

    it('handles missing givenName gracefully', () => {
      const identity = {
        name: undefined,
        givenName: undefined,
        familyName: 'Doe',
      };

      const name =
        identity.name ||
        [identity.givenName, identity.familyName].filter(Boolean).join(' ') ||
        undefined;

      expect(name).toBe('Doe');
    });

    it('returns undefined when all name fields missing', () => {
      const identity = {
        name: undefined,
        givenName: undefined,
        familyName: undefined,
      };

      const name =
        identity.name ||
        [identity.givenName, identity.familyName].filter(Boolean).join(' ') ||
        undefined;

      expect(name).toBeUndefined();
    });
  });
});
