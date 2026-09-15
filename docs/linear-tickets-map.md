# Scry Delivery Slices & Linear Tickets Map

This document tracks the delivery slices and proposed Linear tickets for Scry under the Misty Step project.
Canonical specifications live in [SPEC.md](SPEC.md) and [VISION.md](VISION.md).

---

## Active & Prior Slices

- **MIS-48:** Accepted Go/SQLite/HTMX rewrite baseline, phone acceptance, and zero-downtime cutover.
- **MIS-42:** Estate inventory and deployment reconciliation.

---

## Knowledge-Centered & Multi-Scale Learning Pipeline (Proposed Linear Cards)

### 1. `[Scry] Multi-scale learning horizons & campaign tagging (KC19)`
- **Scope:** Extend `Goal` and `PlanSettings` in `internal/store/knowledge_models.go` to support campaign horizons (`quarter`, `semester`, `ad-hoc`) and target time containers.
- **User Outcome:** Learners can group active goals into a quarterly campaign (e.g., *Q4: Complex Systems & Modern Mathematics*) while keeping older goals in low-overhead background maintenance review.
- **Acceptance:**
  - `PlanSettings` allows specifying an optional `campaign` slug/label alongside `time_budget_seconds`.
  - Campaign metadata is visible in `/goals/{id}` and `/goals/{id}/plan` views without crowding the primary one-learning-moment phone loop.
  - Retained cards from inactive campaigns continue to be scheduled by FSRS; shifting campaign focus does not archive or penalize historical retention.

### 2. `[Scry] Daily Pomodoro session boundaries & time-budget enforcement (KC12, KC19)`
- **Scope:** Wire goal-level `time_budget_seconds` (30–3600s) directly into the study/review session loop.
- **User Outcome:** During a workday Pomodoro reading/study slice (e.g., 20–30 min), Scry paces reviews and bridge instructions to fit the exact available window, offering a clean, deliberate "Session Complete" checkpoint rather than an endless review queue.
- **Acceptance:**
  - Session view respects the chosen `time_budget_seconds` and estimates remaining time based on median response times.
  - Reaching the session budget provides a clean stopping state with session stats (items reviewed, new units touched, FSRS intervals updated) and a deliberate Next action.

### 3. `[Scry] Readwise & external reading ingestion bridge (KC04, KC05)`
- **Scope:** Build an automated or one-click ingestion path from Readwise highlights / Daybook reading notes into Scry `sources` and candidate `KnowledgeUnit`s.
- **User Outcome:** Books, papers, and essays highlighted during evening reading or article triage automatically stage candidate knowledge units and reference excerpts for review, eliminating manual card creation friction.
- **Acceptance:**
  - Stored references retain external provenance (URL, author, book title, highlight offset).
  - Highlights create draft sources that require explicit learner confirmation/goal attachment before entering the active quiz queue.
  - Generation adheres to existing spend ceilings ($1.00/day, $0.20 reservation) and conservative retry policies.

### 4. `[Scry] Cross-domain interleaving & anti-clumping queue policy (KC10, KC19)`
- **Scope:** Enhance `internal/learning/selection.go` to balance domain variety during daily sweeps (mixing mathematics, history, theology, systems design) based on recent interaction history.
- **User Outcome:** Prevents cognitive fatigue by avoiding 20 consecutive flashcards on the same narrow topic, interleaving contrasting disciplines while honoring due dates.
- **Acceptance:**
  - Queue selection penalizes items sharing the same immediate domain or goal if alternative due items exist.
  - Never defers an overdue item past its critical forgetting threshold merely for variety.
