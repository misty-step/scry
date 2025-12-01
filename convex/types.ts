// Import Id for use in this file
import type { Id } from './_generated/dataModel';

/**
 * Convex type exports
 *
 * This file re-exports commonly used Convex types for easy importing
 * throughout the application.
 */

export type { Doc, Id } from './_generated/dataModel';
export { api } from './_generated/api';

// Define specific document types based on schema
export type User = {
  _id: Id<'users'>;
  _creationTime: number;
  email: string;
  name?: string;
  emailVerified?: boolean;
  image?: string;
  createdAt?: number;
};

// Re-export Id type with specific table names for convenience
export type UserId = Id<'users'>;
