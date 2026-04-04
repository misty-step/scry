# Context Packet: Content Type Completeness

## Spec

### Cloze Format Convention

**Blank marker syntax:** `{{c::answer}}` in the stored question text. Simplified from Anki's `{{c1::answer}}` numbering since Scry phrasings are single-card (one concept, one phrasing) -- no multi-card cloze notes.

- Stored question: `The capital of France is {{c::Paris}}.`
- Rendered question: `The capital of France is _______.`
- `correctAnswer` field: `"Paris"` (extracted from the marker)
- `options` field: `[]` (empty array; cloze has no MC options)
- Optional hint syntax for future use: `{{c::Paris::European capital}}` -- hint displayed as placeholder text inside the blank. v1 does not implement hints.

**Constraints:**
- Exactly one `{{c::...}}` marker per question (v1). Multiple-blank cloze is a follow-up.
- The answer inside the marker must be 1-80 characters.
- The surrounding context must be at least 12 characters (excluding the marker).

**Grading:** Case-insensitive, whitespace-normalized exact match. `"paris"` matches `"Paris"`, `" Paris "` matches `"Paris"`. No fuzzy matching for cloze -- the answer is unambiguous by design.

### Free-Response (short-answer) Format

- Stored question: A question expecting a brief typed answer (1-3 words typical, up to a short phrase).
- `correctAnswer` field: The canonical correct answer string.
- `options` field: `[]` (empty array; no MC options).
- `explanation` field: Why the answer is correct, as with all types.

**Grading:** Fuzzy string matching with normalization pipeline:
1. Trim whitespace
2. Lowercase
3. Strip leading/trailing articles ("the", "a", "an")
4. Collapse internal whitespace to single space
5. Strip trailing punctuation (period, comma, exclamation, question mark)
6. Compare with Levenshtein distance threshold: distance <= 1 for answers <= 5 chars, distance <= 2 for answers > 5 chars

This catches typos ("Paries" -> "Paris") while rejecting genuinely wrong answers. LLM-based semantic grading is explicitly a follow-up (per non-goals).

## Schema Changes

### `convex/lib/generationContracts.ts`

**Before:**
```typescript
export const generatedPhrasingSchema = z.object({
  question: z.string(),
  explanation: z.string(),
  type: z.enum(['multiple-choice', 'true-false']),
  options: z.array(z.string()).min(2).max(4),
  correctAnswer: z.string(),
});
```

**After:**
```typescript
const multipleChoiceSchema = z.object({
  question: z.string(),
  explanation: z.string(),
  type: z.literal('multiple-choice'),
  options: z.array(z.string()).min(3).max(4),
  correctAnswer: z.string(),
});

const trueFalseSchema = z.object({
  question: z.string(),
  explanation: z.string(),
  type: z.literal('true-false'),
  options: z.array(z.string()).length(2),
  correctAnswer: z.string(),
});

const clozeSchema = z.object({
  question: z.string().regex(/\{\{c::[\s\S]{1,80}\}\}/),
  explanation: z.string(),
  type: z.literal('cloze'),
  options: z.array(z.string()).length(0),
  correctAnswer: z.string().min(1).max(80),
});

const shortAnswerSchema = z.object({
  question: z.string(),
  explanation: z.string(),
  type: z.literal('short-answer'),
  options: z.array(z.string()).length(0),
  correctAnswer: z.string().min(1),
});

export const generatedPhrasingSchema = z.discriminatedUnion('type', [
  multipleChoiceSchema,
  trueFalseSchema,
  clozeSchema,
  shortAnswerSchema,
]);
export type GeneratedPhrasing = z.infer<typeof generatedPhrasingSchema>;
```

**Rationale:** Discriminated union gives the AI SDK precise per-type constraints. The cloze regex enforces the marker is present in the question. Empty `options` arrays for cloze/short-answer prevent the AI from generating spurious distractors.

**Note on `phrasingBatchSchema`:** No change needed -- it wraps `generatedPhrasingSchema` which now includes all four types.

## Prompt Template Changes

### `convex/lib/promptTemplates.ts` -- `buildPhrasingGenerationPrompt`

Replace the `# Requirements` section (lines 108-131). Full replacement text:

```
# Requirements
- Select question type and template family by content type:
  - Verbatim/Enumerable:
    - Cloze (fill-in-blank): "In [source], [context] {{c::[answer]}} [more context]." — one blank per question.
    - Sequential recall MC: "In [source], after '[span]', what comes next?" — 3-4 options, one correct.
    - Key line recognition TF: true/false assertion about exact text.
  - Conceptual:
    - Short-answer (free-response): "What is [concept]?" or "Name the [property] of [concept]." — answer is 1-5 words.
    - Definition MC: concept definition with plausible alternatives.
    - Misconception check TF: common misunderstanding framed as true/false.
    - Application MC: apply concept to novel scenario.
- Distribute question types across the batch:
  - Verbatim/Enumerable: ~40% cloze, ~40% MC, ~20% TF
  - Conceptual: ~30% short-answer, ~40% MC, ~30% TF
  - Mixed: blend freely, at least 3 types represented.
- Each question must be standalone and identify its source/topic explicitly; never rely on surrounding context.
- MC: 3-4 options, one correct. Distractors must be semantically adjacent (common confusions), not punctuation/format variants.
- TF: exactly two options ("True","False").
- Cloze format: question text MUST contain exactly one {{c::answer}} marker. The answer inside the marker is the correctAnswer. Set options to empty array [].
- Short-answer: question text is a direct question. correctAnswer is a brief canonical answer (1-5 words). Set options to empty array [].
- Generate rationale first (why correct, why others are wrong) to focus the item, then write the final question/options. Return only the final question payload.
- Keep stems concise (<200 chars). No answer leakage in the stem.
- Do not repeat existing phrasing wording. Vary cognitive skill (recall, recognition, application, misconception).

# Output Format (JSON)
{
  "phrasings": [
    {
      "question": "Question text (for cloze: include {{c::answer}} marker)",
      "explanation": "Why the correct answer is right.",
      "type": "multiple-choice" | "true-false" | "cloze" | "short-answer",
      "options": ["A", "B", "C", "D"] | ["True", "False"] | [],
      "correctAnswer": "The correct answer string"
    }
  ]
}
```

## Pipeline Changes

### `convex/aiGeneration.ts` -- `PreparedPhrasing` type and `prepareGeneratedPhrasings`

**`PreparedPhrasing` type (line 162-168). Before:**
```typescript
type PreparedPhrasing = {
  question: string;
  explanation: string;
  type: 'multiple-choice' | 'true-false';
  options: string[];
  correctAnswer: string;
};
```

**After:**
```typescript
type PreparedPhrasing = {
  question: string;
  explanation: string;
  type: 'multiple-choice' | 'true-false' | 'cloze' | 'short-answer';
  options: string[];
  correctAnswer: string;
};
```

**`prepareGeneratedPhrasings` (line 170-232). Replace the validation body with type-dispatched logic:**

```typescript
export function prepareGeneratedPhrasings(
  generated: GeneratedPhrasing[],
  existingQuestions: string[],
  targetCount: number
): PreparedPhrasing[] {
  const normalized: PreparedPhrasing[] = [];
  const seen = new Set(existingQuestions.map((q) => q.trim().toLowerCase()));

  const CLOZE_MARKER_RE = /\{\{c::([\s\S]{1,80})\}\}/;

  for (const phrasing of generated) {
    if (normalized.length >= targetCount) break;

    const question = phrasing.question.trim();
    const explanation = phrasing.explanation.trim();
    if (question.length < 12 || question.length > 400) continue;
    if (explanation.length < 12) continue;

    const questionKey = question.toLowerCase();
    if (seen.has(questionKey)) continue;

    const options = phrasing.options.map((opt) => opt.trim()).filter(Boolean);

    // Type-specific validation
    if (phrasing.type === 'multiple-choice') {
      if (options.length < 3 || options.length > 5) continue;
      if (!options.some((opt) => opt.toLowerCase() === phrasing.correctAnswer.trim().toLowerCase())) continue;
    } else if (phrasing.type === 'true-false') {
      if (options.length !== 2) continue;
      if (!options.some((opt) => opt.toLowerCase() === phrasing.correctAnswer.trim().toLowerCase())) continue;
    } else if (phrasing.type === 'cloze') {
      // Cloze: marker must be present, correctAnswer must match marker content
      const match = question.match(CLOZE_MARKER_RE);
      if (!match) continue;
      const markerAnswer = match[1].trim();
      if (markerAnswer.toLowerCase() !== phrasing.correctAnswer.trim().toLowerCase()) continue;
      // Context outside the marker must be substantive
      const contextLength = question.replace(CLOZE_MARKER_RE, '').trim().length;
      if (contextLength < 12) continue;
    } else if (phrasing.type === 'short-answer') {
      // Short-answer: correctAnswer must be non-empty
      if (!phrasing.correctAnswer.trim()) continue;
    } else {
      // Unknown type -- drop
      continue;
    }

    // Deduplicate options for MC/TF
    const uniqueOptions = (phrasing.type === 'multiple-choice' || phrasing.type === 'true-false')
      ? Array.from(new Set(options.map((opt) => opt.toLowerCase()))).map(
          (lower) => options.find((opt) => opt.toLowerCase() === lower) || lower
        )
      : [];

    const prepared: PreparedPhrasing = {
      question,
      explanation,
      type: phrasing.type,
      options: uniqueOptions,
      correctAnswer:
        (phrasing.type === 'multiple-choice' || phrasing.type === 'true-false')
          ? (uniqueOptions.find(
              (opt) => opt.toLowerCase() === phrasing.correctAnswer.trim().toLowerCase()
            ) ?? phrasing.correctAnswer.trim())
          : phrasing.correctAnswer.trim(),
    };

    normalized.push(prepared);
    seen.add(questionKey);
  }

  return normalized;
}
```

## UI Components

### Cloze Input Component -- `components/review/cloze-input.tsx`

**Rendering strategy:**
1. Parse question text by splitting on `{{c::...}}` marker.
2. Render: `<span>{prefix}</span><input /><span>{suffix}</span>` inline.
3. The input field is sized to the answer length (min 4ch, max 20ch) to avoid layout leaking.
4. On submit, compare input value against `correctAnswer` using the grading function.

**Feedback states reuse existing visual language:**
- Unanswered: input with subtle border (`border-input`)
- Selected (pre-submit): `border-info-border bg-info-background`
- Correct: `border-success-border bg-success-background`, show check icon
- Incorrect: `border-error-border bg-error-background`, show X icon, reveal correct answer below

**Component signature:**
```typescript
interface ClozeInputProps {
  question: string;           // Contains {{c::answer}} marker
  correctAnswer: string;
  selectedAnswer: string;
  showFeedback: boolean;
  onAnswerChange: (value: string) => void;
  instantFeedback?: { isCorrect: boolean; visible: boolean };
}
```

**Key UX decision:** User types into the blank and presses Enter or clicks Submit (same button as existing flow). No intermediate "check" step -- this keeps the interaction model identical to MC/TF from the session context's perspective.

### Free-Response Component -- `components/review/short-answer-input.tsx`

**Rendering strategy:**
1. Display question text as `<h2>` (same as MC/TF).
2. Render a single-line `<Input>` (not textarea -- answers are short) with placeholder "Type your answer..."
3. Submit on Enter keypress or Submit button.

**Feedback states:** Same visual language as cloze. On incorrect, display the correct answer below the input for learning.

**Component signature:**
```typescript
interface ShortAnswerInputProps {
  question: string;
  correctAnswer: string;
  selectedAnswer: string;
  showFeedback: boolean;
  onAnswerChange: (value: string) => void;
  instantFeedback?: { isCorrect: boolean; visible: boolean };
}
```

### Integration into `review-phrasing-display.tsx`

Add type-dispatched rendering in the display mode section (after the existing TF/MC branches):

```typescript
// Display Mode Rendering
if (question.type === 'cloze') {
  return (
    <>
      <ClozeInput
        question={question.question}
        correctAnswer={question.correctAnswer}
        selectedAnswer={selectedAnswer}
        showFeedback={displayFeedback}
        onAnswerChange={onAnswerSelect}
        instantFeedback={instantFeedback}
      />
    </>
  );
}

if (question.type === 'short-answer') {
  return (
    <>
      <h2 className="text-xl font-semibold">{question.question}</h2>
      <ShortAnswerInput
        question={question.question}
        correctAnswer={question.correctAnswer}
        selectedAnswer={selectedAnswer}
        showFeedback={displayFeedback}
        onAnswerChange={onAnswerSelect}
        instantFeedback={instantFeedback}
      />
    </>
  );
}
```

### Changes to `components/review/session-context.tsx`

**Client-side grading (line 293):** Replace the direct comparison with a grading function that dispatches by type:

```typescript
// Before:
const isCorrect = selectedAnswer === question.correctAnswer;

// After:
const isCorrect = gradeAnswerClient(selectedAnswer, question.correctAnswer, question.type);
```

Where `gradeAnswerClient` is a shared utility (see Grading Logic section) imported into both client and server.

**`handleAnswerSelect` (line 281-286):** No change needed -- already accepts arbitrary string. Cloze and short-answer components call `onAnswerSelect(inputValue)` with the typed text.

**`questionType` derivation (line 157-158):** Extend to include new types:

```typescript
// Before:
const questionType: QuestionType =
  question?.type === 'true-false' ? 'true-false' : 'multiple-choice';

// After:
const questionType: QuestionType = question?.type ?? 'multiple-choice';
```

### Changes to `lib/unified-edit-validation.ts`

**`QuestionType` (line 24):** Extend the union:

```typescript
// Before:
export type QuestionType = 'multiple-choice' | 'true-false';

// After:
export type QuestionType = 'multiple-choice' | 'true-false' | 'cloze' | 'short-answer';
```

**`validateUnifiedEdit`:** Add cloze/short-answer validation rules:

```typescript
// Rule 5: Cloze specific validation
if (questionType === 'cloze') {
  if (!/\{\{c::[\s\S]{1,80}\}\}/.test(data.question)) {
    errors.push({
      field: 'question',
      message: 'Cloze question must contain a {{c::answer}} blank marker',
    });
  }
  if (!data.correctAnswer.trim()) {
    errors.push({
      field: 'correctAnswer',
      message: 'Correct answer is required for cloze questions',
    });
  }
}

// Rule 6: Short-answer specific validation
if (questionType === 'short-answer') {
  if (!data.correctAnswer.trim()) {
    errors.push({
      field: 'correctAnswer',
      message: 'Correct answer is required for short-answer questions',
    });
  }
}
```

## Grading Logic

### `convex/agents/reviewToolHelpers.ts` -- type-dispatched `gradeAnswer`

**Before (line 22-28):**
```typescript
export function gradeAnswer(userAnswer: string, correctAnswer: string) {
  const normalizedCorrect = normalizeAnswer(correctAnswer);
  if (!normalizedCorrect) return false;
  return normalizeAnswer(userAnswer) === normalizedCorrect;
}
```

**After:**
```typescript
/** Strip leading/trailing articles and trailing punctuation. */
function normalizeForFuzzy(value: string): string {
  return value
    .trim()
    .toLowerCase()
    .replace(/^(the|a|an)\s+/i, '')
    .replace(/\s+/g, ' ')
    .replace(/[.,!?]+$/, '');
}

/** Levenshtein distance (standard DP). */
function levenshtein(a: string, b: string): number {
  const m = a.length, n = b.length;
  const dp: number[][] = Array.from({ length: m + 1 }, (_, i) =>
    Array.from({ length: n + 1 }, (_, j) => (i === 0 ? j : j === 0 ? i : 0))
  );
  for (let i = 1; i <= m; i++) {
    for (let j = 1; j <= n; j++) {
      dp[i][j] = a[i - 1] === b[j - 1]
        ? dp[i - 1][j - 1]
        : 1 + Math.min(dp[i - 1][j], dp[i][j - 1], dp[i - 1][j - 1]);
    }
  }
  return dp[m][n];
}

export function gradeAnswer(
  userAnswer: string,
  correctAnswer: string,
  questionType?: 'multiple-choice' | 'true-false' | 'cloze' | 'short-answer'
): boolean {
  const normalizedCorrect = normalizeAnswer(correctAnswer);
  if (!normalizedCorrect) return false;

  // MC, TF, and cloze: exact match (case-insensitive, trimmed)
  if (!questionType || questionType === 'multiple-choice' || questionType === 'true-false' || questionType === 'cloze') {
    return normalizeAnswer(userAnswer) === normalizedCorrect;
  }

  // Short-answer: fuzzy match
  const normUser = normalizeForFuzzy(userAnswer);
  const normCorrect = normalizeForFuzzy(correctAnswer);
  if (!normUser) return false;

  // Exact match after normalization
  if (normUser === normCorrect) return true;

  // Levenshtein threshold: <=1 for short answers, <=2 for longer
  const threshold = normCorrect.length <= 5 ? 1 : 2;
  return levenshtein(normUser, normCorrect) <= threshold;
}
```

**Backward compatible:** The `questionType` parameter is optional with undefined defaulting to exact match. All existing call sites (which pass no type) continue to work identically.

### Call-site changes

**`convex/agents/reviewStreaming.ts` (line 118) and `convex/agents/reviewAgent.ts` (line 85):**

```typescript
// Before:
const isCorrect = gradeAnswer(args.userAnswer, correctAnswer);

// After:
const isCorrect = gradeAnswer(args.userAnswer, correctAnswer, phrasing.type);
```

The phrasing document already contains `type` from the database.

**`components/review/session-context.tsx` (line 293):**

```typescript
// Before:
const isCorrect = selectedAnswer === question.correctAnswer;

// After:
import { gradeAnswerClient } from '@/lib/grading';
const isCorrect = gradeAnswerClient(selectedAnswer, question.correctAnswer, question.type);
```

### Client-side grading utility -- `lib/grading.ts`

Extract `gradeAnswer`, `normalizeForFuzzy`, and `levenshtein` into a shared pure module at `lib/grading.ts`. Both `reviewToolHelpers.ts` and `session-context.tsx` import from it. The Convex agent-side module re-exports for backward compatibility:

```typescript
// convex/agents/reviewToolHelpers.ts
export { gradeAnswer } from '../../lib/grading';
```

**Architectural decision:** Grading must be deterministic and identical on client and server. A shared pure module guarantees this. The server-side grading is the authoritative source of truth for FSRS scheduling; the client-side grading is for instant feedback only.

## Implementation Sequence

TDD throughout. Each step: write failing test -> implement -> green -> refactor.

### Phase 1: Schema + Contracts (backend, no UI)

1. **Test:** Add test cases to `tests/convex/aiGeneration.prep.test.ts` for cloze and short-answer validation in `prepareGeneratedPhrasings` (blank marker presence, correctAnswer extraction, context length, empty options rejection, fuzzy-match irrelevance for cloze).
2. **Implement:** Update `generatedPhrasingSchema` in `generationContracts.ts` to discriminated union.
3. **Implement:** Update `PreparedPhrasing` type and `prepareGeneratedPhrasings` in `aiGeneration.ts`.
4. **Verify:** All existing tests still pass (backward compatible).

### Phase 2: Grading (shared pure module)

5. **Test:** Add test cases to `tests/convex/reviewToolHelpers.test.ts` for type-dispatched grading: cloze exact match, short-answer fuzzy match (typo tolerance, article stripping, punctuation stripping, threshold boundaries).
6. **Implement:** Create `lib/grading.ts` with `gradeAnswer`, `normalizeForFuzzy`, `levenshtein`.
7. **Implement:** Update `reviewToolHelpers.ts` to re-export from shared module.
8. **Implement:** Update call sites in `reviewStreaming.ts` and `reviewAgent.ts` to pass `phrasing.type`.

### Phase 3: Prompt Templates

9. **Implement:** Update `buildPhrasingGenerationPrompt` in `promptTemplates.ts` with new requirements section.
10. **Test:** Add promptfoo eval cases for cloze (NATO as cloze: "N is for {{c::November}}") and short-answer (book theme free-response).

### Phase 4: UI Components

11. **Test:** Component tests for `ClozeInput` (renders blank, accepts input, shows feedback).
12. **Implement:** `components/review/cloze-input.tsx`.
13. **Test:** Component tests for `ShortAnswerInput` (renders input, accepts text, shows feedback).
14. **Implement:** `components/review/short-answer-input.tsx`.
15. **Implement:** Integrate into `review-phrasing-display.tsx` with type dispatch.
16. **Implement:** Update `session-context.tsx` client-side grading.

### Phase 5: Validation + Edit Mode

17. **Implement:** Extend `QuestionType` in `unified-edit-validation.ts`.
18. **Implement:** Add cloze/short-answer validation rules.
19. **Implement:** Update `session-context.tsx` questionType derivation.
20. **Test:** Validation tests for cloze marker requirement, short-answer correctAnswer requirement.

### Phase 6: Integration + Evals

21. **End-to-end test:** Input "NATO phonetic alphabet" -> generates mix of all 4 question types -> all reviewable.
22. **Promptfoo evals:** Verify type distribution matches target ratios (+/- 15%).
23. **Manual QA:** Review session with mixed question types, verify grading, feedback, and FSRS scheduling.

## Risks

### Edge Cases

| Risk | Mitigation |
|------|------------|
| **Multiple blanks in cloze** | v1 enforces exactly one `{{c::...}}` marker via regex. Validation drops multi-blank phrasings. Document as future enhancement. |
| **Very long free-response answers** | `MAX_USER_ANSWER_LENGTH` (500 chars) already enforced by `assertUserAnswerLength`. Levenshtein on long strings is O(mn) but with max 500 chars this is <1ms. |
| **Ambiguous correct answers** | Short-answer fuzzy matching has a tight threshold (Levenshtein <= 2). Genuinely ambiguous questions are a prompt quality issue, not a grading issue. Promptfoo evals will catch prompts that generate ambiguous short-answer questions. |
| **AI generates cloze without marker** | `prepareGeneratedPhrasings` drops any cloze phrasing missing the `{{c::...}}` marker. The Zod schema also enforces the regex. Double validation. |
| **AI puts answer in stem (leakage)** | Existing prompt rule ("No answer leakage in the stem") applies. For cloze, the answer IS in the stem by design -- the marker syntax makes this explicit and correct. |
| **Backward compatibility** | `gradeAnswer` optional third parameter defaults to exact-match behavior. All existing MC/TF phrasings unaffected. Schema changes are additive (discriminated union is a superset). `insertGenerated` already accepts all four types. |
| **Client/server grading divergence** | Shared `lib/grading.ts` module guarantees identical logic. Client grading is for instant UX feedback only; server grading is authoritative for FSRS. |
| **Edit mode for cloze/short-answer** | Edit mode in `review-phrasing-display.tsx` currently only handles MC/TF. For v1, cloze/short-answer edit uses the generic `Textarea` for question and a plain `Input` for correctAnswer. No options editor needed. |
| **Discriminated union Zod + AI SDK compatibility** | `z.discriminatedUnion` is supported by AI SDK's `generateObject` with structured output mode. Verify during Phase 1 that the provider (Gemini) handles it correctly; fallback is to keep flat union with runtime validation only in `prepareGeneratedPhrasings`. |
