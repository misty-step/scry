import { cronJobs } from 'convex/server';
import { internal } from './_generated/api';

const crons = cronJobs();

// Schedule rate limit cleanup to run every hour
// This helps prevent the rateLimits table from growing unbounded
crons.hourly(
  'cleanupExpiredRateLimits',
  {
    minuteUTC: 0, // Run at the top of every hour
  },
  internal.rateLimit.cleanupExpiredRateLimits
);

// Schedule job cleanup to run daily at 3 AM UTC
// Removes old completed jobs (7 days) and failed jobs (30 days)
crons.daily(
  'cleanupOldJobs',
  {
    hourUTC: 3,
    minuteUTC: 0,
  },
  internal.generationJobs.cleanup
);

// Schedule embedding sync to run daily at 3:30 AM UTC
// Backfills embeddings for concepts/phrasings that don't have them
// Processes up to configured limits in batches of 10
crons.daily(
  'syncEmbeddings',
  {
    hourUTC: 3,
    minuteUTC: 30, // 30 minutes after job cleanup
  },
  internal.embeddings.syncMissingEmbeddings
);

// Schedule IQC candidate scan to run daily at 4:00 AM UTC
// Finds near-duplicate concepts and enqueues MERGE action cards
crons.daily(
  'scanForIqcCandidates',
  {
    hourUTC: 4,
    minuteUTC: 0,
  },
  internal.iqc.scanAndPropose
);

export default crons;
