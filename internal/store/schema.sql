-- Scry schema, version 1. Times are unix seconds, UTC. Money is integer
-- cents; metered model usage is integer micro-dollars.
-- Every table a learner owns carries user_id; see store.userTables.

CREATE TABLE health (id INTEGER PRIMARY KEY, at INTEGER NOT NULL);

CREATE TABLE users (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  email        TEXT NOT NULL UNIQUE COLLATE NOCASE,
  auth_subject TEXT NOT NULL UNIQUE,
  tz           TEXT NOT NULL DEFAULT 'UTC',
  theme        TEXT NOT NULL DEFAULT 'system',
  about        TEXT NOT NULL DEFAULT '',
  created_at   INTEGER NOT NULL
);

-- Browser sessions. Only a hash of the cookie value is stored.
CREATE TABLE sessions (
  token_hash TEXT PRIMARY KEY,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  csrf       TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  expires_at INTEGER NOT NULL
);

-- The test-mode auth adapter's own credential store.
CREATE TABLE local_credentials (
  subject       TEXT PRIMARY KEY,
  email         TEXT NOT NULL UNIQUE COLLATE NOCASE,
  password_hash TEXT NOT NULL
);

-- Processor events already applied, so a replayed webhook is harmless.
CREATE TABLE processed_events (id TEXT PRIMARY KEY, at INTEGER NOT NULL);

CREATE TABLE subscriptions (
  user_id              INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  customer_ref         TEXT NOT NULL,
  subscription_ref     TEXT NOT NULL UNIQUE,
  payment_ref          TEXT NOT NULL DEFAULT '',
  status               TEXT NOT NULL CHECK (status IN ('trialing','active','past_due','ended')),
  trial_end            INTEGER NOT NULL DEFAULT 0,
  period_start         INTEGER NOT NULL,
  period_end           INTEGER NOT NULL,
  cancel_at_period_end INTEGER NOT NULL DEFAULT 0,
  card_brand           TEXT NOT NULL DEFAULT '',
  card_last4           TEXT NOT NULL DEFAULT '',
  card_error           TEXT NOT NULL DEFAULT '',
  allowance_micros     INTEGER NOT NULL,
  period_used_micros   INTEGER NOT NULL DEFAULT 0,
  topup_micros         INTEGER NOT NULL DEFAULT 0,
  past_due_since       INTEGER NOT NULL DEFAULT 0
);

-- topups and subscription_attempts (one row per charge attempt, claimed
-- before the processor is called) are created on every open, so databases
-- from before them gain them too: see store.migrateAdded.

CREATE TABLE ledger (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  at           INTEGER NOT NULL,
  kind         TEXT NOT NULL CHECK (kind IN ('trial','charge','topup')),
  description  TEXT NOT NULL,
  amount_cents INTEGER NOT NULL,
  status       TEXT NOT NULL CHECK (status IN ('paid','failed','none')),
  ref          TEXT NOT NULL UNIQUE
);
CREATE INDEX ledger_user ON ledger(user_id, at);

CREATE TABLE usage (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  at            INTEGER NOT NULL,
  kind          TEXT NOT NULL CHECK (kind IN ('judge','draft','speech')),
  model         TEXT NOT NULL DEFAULT '',
  input_tokens  INTEGER NOT NULL DEFAULT 0,
  output_tokens INTEGER NOT NULL DEFAULT 0,
  seconds       REAL NOT NULL DEFAULT 0,
  cost_micros   INTEGER NOT NULL
);
CREATE INDEX usage_user ON usage(user_id, at);

-- An idea. Top-level ideas are sets, books and topics; their children are
-- the ideas inside them (one level of nesting).
CREATE TABLE ideas (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  parent_id  INTEGER REFERENCES ideas(id) ON DELETE CASCADE,
  title      TEXT NOT NULL,
  summary    TEXT NOT NULL DEFAULT '',
  origin     TEXT NOT NULL DEFAULT '',
  position   INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL,
  deleted_at INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX ideas_user ON ideas(user_id, parent_id, position);

CREATE TABLE idea_links (
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  a_id    INTEGER NOT NULL REFERENCES ideas(id) ON DELETE CASCADE,
  b_id    INTEGER NOT NULL REFERENCES ideas(id) ON DELETE CASCADE,
  PRIMARY KEY (a_id, b_id),
  CHECK (a_id < b_id)
);
CREATE INDEX idea_links_b ON idea_links(b_id);

-- An item is one question. Every item belongs to an idea.
CREATE TABLE items (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  idea_id     INTEGER NOT NULL REFERENCES ideas(id) ON DELETE CASCADE,
  prompt      TEXT NOT NULL,
  answer      TEXT NOT NULL,
  exact       INTEGER NOT NULL DEFAULT 0,
  text_title  TEXT NOT NULL DEFAULT '',
  line_no     INTEGER NOT NULL DEFAULT 0,
  source      TEXT NOT NULL DEFAULT '',
  created_at  INTEGER NOT NULL,
  due_at      INTEGER NOT NULL,
  stability   REAL NOT NULL DEFAULT 0,
  difficulty  REAL NOT NULL DEFAULT 0,
  reps        INTEGER NOT NULL DEFAULT 0,
  lapses      INTEGER NOT NULL DEFAULT 0,
  last_review INTEGER NOT NULL DEFAULT 0,
  deleted_at  INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX items_due ON items(user_id, deleted_at, due_at);
CREATE INDEX items_idea ON items(idea_id);

CREATE TABLE study_sessions (
  id                INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id           INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  started_at        INTEGER NOT NULL,
  ended_at          INTEGER NOT NULL DEFAULT 0,
  pending_review_id INTEGER NOT NULL DEFAULT 0,
  retry_item_id     INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX study_sessions_user ON study_sessions(user_id, ended_at);

CREATE TABLE session_items (
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  session_id INTEGER NOT NULL REFERENCES study_sessions(id) ON DELETE CASCADE,
  position   INTEGER NOT NULL,
  item_id    INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
  review_id  INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (session_id, position)
);

-- The learner's history: one row per judged answer, never rewritten.
CREATE TABLE reviews (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  item_id    INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
  session_id INTEGER NOT NULL DEFAULT 0,
  at         INTEGER NOT NULL,
  answer     TEXT NOT NULL,
  verdict    TEXT NOT NULL CHECK (verdict IN ('right','close','missed')),
  reason     TEXT NOT NULL,
  detail     TEXT NOT NULL DEFAULT '{}',
  via        TEXT NOT NULL DEFAULT 'typed',
  judge      TEXT NOT NULL DEFAULT '',
  practice   INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX reviews_user ON reviews(user_id, at);
CREATE INDEX reviews_item ON reviews(item_id);

CREATE TABLE drafts (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  query      TEXT NOT NULL,
  kind       TEXT NOT NULL DEFAULT '',
  title      TEXT NOT NULL DEFAULT '',
  status     TEXT NOT NULL CHECK (status IN ('drafting','ready','empty','failed','allowance')),
  note       TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL
);

-- Material waiting for the learner's approval: suggestions from agents
-- (draft_id = 0, shown in the inbox) and the items of a draft outline.
CREATE TABLE suggestions (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  draft_id     INTEGER NOT NULL DEFAULT 0,
  agent        TEXT NOT NULL DEFAULT '',
  source       TEXT NOT NULL DEFAULT '',
  parent_title TEXT NOT NULL DEFAULT '',
  idea_title   TEXT NOT NULL,
  idea_summary TEXT NOT NULL DEFAULT '',
  links        TEXT NOT NULL DEFAULT '[]',
  prompt       TEXT NOT NULL,
  answer       TEXT NOT NULL,
  exact        INTEGER NOT NULL DEFAULT 0,
  status       TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','dismissed')),
  created_at   INTEGER NOT NULL,
  decided_at   INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX suggestions_user ON suggestions(user_id, draft_id, status);

-- Tokens the learner hands to their agents. Only a hash is stored.
CREATE TABLE agent_tokens (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name       TEXT NOT NULL,
  token_hash TEXT NOT NULL UNIQUE,
  can_add    INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL,
  last_used  INTEGER NOT NULL DEFAULT 0,
  revoked_at INTEGER NOT NULL DEFAULT 0
);
