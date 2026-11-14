# Scry specification

Scry helps its Owner memorize knowledge that someone else wrote. Agents supply
the content. Scry presents practice, judges every answer itself, tells the
truth about help, and schedules recall. It is private, for one person, on his
phone and computer and through an API and command line.

This file is Scry's present-tense target state: what a rebuild must do for the
Owner to experience the same product. [VISION](VISION.md) explains why,
[DESIGN](DESIGN.md) owns visual decisions,
[the decision records](docs/adr/README.md) own technical ones,
[QA](docs/qa/system.md) owns verification and
[the runbook](docs/runbook.md) owns operations.

## Purpose and non-goals

The Owner can receive material an agent wrote, study it, answer from memory
with one typed answer, see Scry's Verdict with a useful correction, and return
when recall is due. Learning effort is intentional; interface effort is not.

Scry is not, and does not become by implication:

- **A content generator.** Scry never writes, drafts, transcribes, researches,
  refines or repairs learning content. Every request to do so is refused
  before any job, record, upload, reservation or provider call exists.
- **Self-graded.** There is no self-rating, client-supplied Verdict,
  expected-answer override or Owner Correction of a grade. A Verdict is never
  overturned.
- **A hint ladder.** Practice has two forms, Source study and Quiz. Scry does
  not fade cues, show first letters or offer multiple choice.
- **A media product.** Content is text. Scry does not accept, store or play
  audio, images or video for practice.
- **A product for anyone else.** No accounts or tokens for other people,
  signup, anonymous or public reading, public Library, sharing, billing or
  collaboration.
- **A speech product.** No microphone integration, speech recognition or
  pronunciation validation. Dictated input is text and is graded as typed.
- **A study administrator.** No deck choice, scheduler tuning, approval
  inbox, mandatory taxonomy, setup questionnaire, streaks, due-count chrome,
  swipe grading or automatic advance.
- **A general knowledge tool.** No universal import, course or chat product,
  manual graph or relations editor, or meaning-based search.
- **A source of advice.** Baby-care lessons are memory practice, not medical
  advice or hands-on certification.
- **A predictor.** Scry shows observed evidence and never claims mastery,
  readiness or a guaranteed recall probability.
- **An offline queue.** Unsent answers are never queued invisibly and sent
  later.

## Terms

Specification, code, commits and conversation use these words.

### People and access

- **Owner:** the one person Scry is for. Every Study, Attempt and Library
  entry is his and visible only to him. Avoid: user, account, learner.
- **Owner token:** a revocable bearer credential that lets a program, such as
  an agent using the CLI, act as the Owner. Avoid: API key, admin token.

### Library

- **Library:** the Owner's private store of Works and their Versions, kept
  apart from his study state. Avoid: deck, collection, catalogue.
- **Work:** the abstract thing to learn, such as "Nicene Creed" or "NATO
  spelling alphabet". It has a title, aliases, a Kind and a short context note.
  Avoid: topic, goal, item.
- **Kind:** the shape of a Work's content: a text (prayer, creed, poem), an
  ordered list, or a keyed mapping. Avoid: type, mode, category.
- **Version:** one exact wording of a Work, such as "English, Roman Missal
  2011". It never changes; a correction is a new Version that supersedes it.
  Avoid: edition (unless naming one), variant.
- **Section:** an ordered group of Units within a Version: a creed article, a
  stanza, a set of mysteries. Avoid: chunk, part.
- **Unit:** the smallest piece of a Version: a clause or line of a text, also
  called a phrase; an item of a list; a key and value of a mapping. A
  Version's Units, joined in order, reproduce its text exactly. Avoid: card,
  chunk.
- **Relation:** a typed link between Works or Versions: version of, part of,
  related to, supersedes. Avoid: tag, link.
- **Provenance:** how a Version's text was obtained: supplied by the Owner or
  his agent, written by a model from memory, or checked against cited sources.
- **Rights status:** what a Version's copyright would allow: public domain,
  licensed, or private only. Private only is the default.
- **Spelling-strict:** said of a Version whose spelling is itself the thing
  being learned, such as the NATO words "Alfa" and "Juliett".
- **Starter shelf:** the curated Works Scry offers without an agent: the
  NATO alphabet, the four Rosary sets, the Saint Michael prayer in named
  English and Latin editions, and the baby-care mappings. Their wording ships
  with Scry as supplied content, written outside it.
- **Lesson:** practice of one Work the Owner chose from the lesson shelf.
- **Focused practice:** practice kept to one Lesson until the Owner leaves
  it, as opposed to Study's mix.
- **Notebook:** everything Scry holds for the Owner: his Library, Studies,
  drafts and history.

### Study and practice

- **Study:** the Owner's decision to learn one Version, with its Pace and
  whether it is paused or dropped. Avoid: goal, deck, course.
- **Card:** the Owner's memory state for one Unit in one direction. Avoid:
  flashcard, question.
- **Prompt:** what Scry presents for the Owner to answer: the Work's title,
  the scope asked for and the context around it. Avoid: challenge.
- **Source study:** reading supplied material with no answer field. It earns
  no Attempt and no schedule credit.
- **Quiz:** answering a Prompt with the source hidden.
- **Free recall:** an answer given with no answer-bearing help. Only Free
  recall moves the schedule. Avoid: test mode.
- **Recitation:** one answer that covers several Units in order, such as a
  stanza, a Rosary set or a whole prayer.
- **Attempt:** one immutable record of an answer: what was asked, in which
  scope, the raw answer, the Verdict, the Diff where one applies, the help
  given and the Authority. Avoid: review, response.
- **Verdict:** the outcome of an Attempt, and of each Unit within a
  Recitation. Good means the answer satisfies the
  Learning objective; Again, also called a miss, means it demonstrably does
  not. An answer Scry cannot judge has no Verdict. Avoid: score, mark.
- **Learning objective:** what an answer must show, set by the content's
  Kind and flags. A text asks for its exact words; a Spelling-strict Version
  for exact spelling; an ordered list for the right items in the right order;
  a mapping for the right value. A safety-bearing pair, flagged by its
  author, asks for the safe practice itself.
- **Authority:** who decided a Verdict, an exact match or Jev, with the
  policy and version it was decided under. Older records also name a reveal
  or the Owner. Avoid: grader.
- **Jev:** the judgment service Scry's server uses to assess an answer against
  its learning objective.
- **Slip:** a typing error in a word that was clearly recalled, such as
  "concieved". It is shown and counts as recalled, except in a Spelling-strict
  Version. Avoid: typo, fuzzy match.
- **Diff:** the word-by-word comparison of an answer with the expected text,
  marking each word correct, slipped, wrong, missing or extra.
- **Assistance:** anything answer-bearing shown before or during an answer: a
  reveal, a cue, a guided retry, reference material. An assisted Attempt is
  recorded as assisted.
- **Pace:** how many new Units a Study introduces per rolling day: 3 for
  light, 6 for steady, 12 for intense.
- **Spend ledger:** the record of money reserved and spent on paid calls, by
  day, purpose and model.

### Earlier material

- **Concept:** a teachable idea saved in the earlier concept notebook, with
  its notes, questions and prerequisite links. It is not a Work or Unit.
- **Goal:** a saved request from the earlier concept notebook that groups its
  Concepts. Memorization uses Study instead.
- **Correction:** an Owner's override of a Verdict, found only in older
  records. Scry accepts none; saved ones stay readable and unchanged.
- **Cue level:** how much of a Unit a Prompt showed, found only in older
  records. Scry shows it in history and never uses it to choose what to
  present.
- **Archived notebook:** the oldest notebook, kept read-only with nothing
  deleted, its final verified backup retained and no writer attached.

## Invariants

1. **Scry owns every Verdict.** The Owner supplies an answer, never a grade.
   Nothing a client sends as a Verdict, rating, expected answer, assistance
   flag or schedule is accepted.
2. **Uncertainty is not a grade.** A failed, unavailable, malformed or
   inconclusive assessment leaves the answer saved and ungraded, with no
   schedule effect and no fallback to self-grading.
3. **Help is recorded before it is shown.** An assisted Attempt never earns
   unaided credit and never extends the schedule. Reading, studying, revealing
   and navigating are not recall.
4. **History is never rewritten.** Attempts, Verdicts, Authorities, Diffs,
   saved Corrections, Versions and past schedules keep their original content
   and meaning. A result is never rejudged; it keeps the policy it was decided
   under. An older version of Scry refuses saved records it cannot honor in
   full rather than dropping what it does not understand.
5. **Supplied content is authoritative.** Scry never silently substitutes,
   samples, reorders or paraphrases a Version. Named editions stay distinct.
6. **Each action happens once.** Every mutation carries an operation identity.
   Repeating it returns the committed result; reusing it with a different
   payload is an explicit conflict. A lost response, reload, restart, second
   tab or stale page never duplicates an Attempt, schedule effect or Study,
   and never grades a Prompt other than the one answered.
7. **Only a deliberate action advances.** Reading, navigating, reloading and
   status checks never select a Prompt, reveal an answer, clear a draft,
   start a Study, write evidence or change a schedule.
8. **Protected recall stays protected.** While a Prompt awaits an unaided
   answer, nothing answer-bearing is reachable from it: not in the page
   source, the Library, progress, history, export or reference pages.
9. **Evidence is observed, never invented.** Counts come from saved Attempts.
   Phrase, joined-passage and whole-text evidence stay separate; success on a
   part never becomes credit for the whole.
10. **Only the Owner reaches his Notebook.** No other identity reads,
    exports or changes it, through any hostname, listener or port.
11. **Acknowledged work survives.** An answer Scry has acknowledged survives
    restart, release and loss of the serving machine, within the stated
    backup limits.
12. **Paid work is bounded.** Spend never exceeds its caps, and a sent request
    with an unknown cost keeps its reservation until deliberately resolved.

## Experience

Scry opens on practice, not a dashboard: the held Prompt or result if there
is one, otherwise one Study action. An ordinary session
needs no choice of deck, no configuration and no reading of telemetry. Scry is
quiet: few components, little text, no decorative panels, no repeated labels
and no reassurance copy.
 One thing leads at a time. Practice shows one Work and scope, one clear task
(study this, or answer from memory), one answer field when quizzed, and one
primary next action. The Work's identity leads visually. Lessons, progress and
optional details sit behind compact disclosures on the same screen. Library,
history, settings, the earlier notebook and the changelog live in a quiet
menu, and navigation is never duplicated in a footer.

The loop is study, then **Try from memory**, then one typed answer and
**Check**, then a visible checking state, then Scry's Verdict with its
correction, held on screen until the Owner chooses **Next**. Studying a
source, answering a question, checking, a held result, an answer saved but
ungraded, nothing left to do, and a conflict or error are visibly different
states.
 It feels immediate with a keyboard: Enter submits a one-line answer and
chooses Next on a result, while in a several-line answer Enter starts a new
line and Check submits. Focus is visible and restored, and transitions are
subtle and respect reduced-motion preferences. Every control is labelled, a
result is announced to assistive technology, and contrast is adequate. Icons
are decorative beside readable labels and are never the only name of an
action. Status never depends on colour alone.

It works equally on a phone and a desktop, in light and dark, with JavaScript
or without it: every core action is a native form. Inputs are at least 16 CSS
pixels, zoom stays enabled, primary targets are at least 44 CSS pixels, and
the layout holds at 320 CSS pixels wide and at 200% text zoom without
horizontal clipping, including with long questions and explanations.

## Capabilities

### Open into study

- Home offers one **Study** action and a closed, secondary **Browse**, which
  opens the lesson shelf and the Library. It resumes a held Prompt or result
  without showing its answer or choosing another one. Scry holds at most one
  open Prompt or held result at a time.
- Study and Next serve genuinely due, previously practised material first,
  interleaved across Studies: the least recently served Study wins, then due
  time, then a stable identity, so one overdue Work cannot monopolize a
  session. After that they rotate unfinished Studies and Works not yet
  started, by least recent service.
- Available material is the Library's Works and the Starter shelf. When it
  is the turn of a Work not yet started, Study starts that one Study and
  nothing else. Home and opening practice with nothing held never start a
  Study or introduce a Unit.
- A Work not yet started offers one unsuperseded Version, the first by
  edition name, never all its Versions. A Version the Owner chose is kept.
  The Starter shelf defaults to the supplied English Saint Michael prayer;
  its other Versions are explicit alternatives, and choosing one suppresses
  the default.
- A paused Study gets no new material but keeps its due reviews. A dropped
  Study gets neither, and its Attempts are kept. Another Version never
  bypasses a paused or dropped Work.
- Due review and new learning stay distinct: an introduction is never
  labelled or counted as review evidence.
- When nothing is eligible, Scry says **Rest for now**, offers Browse and,
  when a review is scheduled, states when the next one falls due. It shows no
  ticking countdown, invents no due work and claims no completion. This holds
  under Study and inside a focused lesson alike.
- Material Scry cannot grade stays in the Library and is never served by
  Study.

### Study, then quiz from memory

- Anything not yet practised is first shown as Source study, whatever its
  Kind. Source study shows the supplied material with no answer field and no
  grading controls. Scry refuses an answer submitted against a source being
  studied, from any client.
- **Try from memory** deliberately replaces the held source with a Quiz of
  the same scope, source hidden. Studying and making that transition create
  no Attempt, success, review count or schedule credit.
- The Quiz that follows Source study is Free recall and schedules its Card
  like any other. Every later review is a Quiz too. Revealing during a Quiz
  marks that retry assisted.
- A mapping's source study keeps its authored question and any
  safety-bearing cue.
- A Prompt still held from an older record in a cue-bearing form can be
  answered once; an answer given beside an answer-bearing cue is assisted.
  Every Prompt after it is a Quiz. The app shows any visible source as study,
  never as a form to copy from.

### Grading

- Scry assesses each submitted answer on the server against the frozen
  Version, the Prompt's scope and the Learning objective, in this order. An
  answer that matches the expected content or an authored alias is Good, once
  case, spacing and a hyphen or space inside a word are set aside. For
  Spelling-strict content any other answer is Again. Every other answer is
  judged by Jev.
- Valid recall is never failed merely because its wording is absent from a
  list of accepted answers. A related but different identity is never
  accepted as a match, and broad string similarity never decides
  correctness.
- Objectives keep their real distinctions. Exact-word practice,
  Spelling-strict content, ordered identity and safety-bearing answers are
  not interchangeable with loose meaning.
- A Slip is distinguished from a content error and counts as recalled, except
  in a Spelling-strict Version, where the misspelling is a miss and the
  official form is shown.
- An answer that belongs to another Version of the same Work is identified as
  that other wording, never silently accepted or substituted.
- Dictated text is assessed exactly as typed text.
- The result shows the expected content and a concise, useful correction. For
  exact-word practice it includes the Diff. A mechanical Diff never
  masquerades as the judgment and never contradicts it without explanation.
- Each Verdict keeps its actual Authority and the policy it was decided
  under. Replay, display, export and recovery reproduce the stored judgment
  without judging again.
- When Jev is unavailable, fails or is inconclusive, the answer stays saved
  as an Attempt with no Verdict, Scry says so honestly, and the saved
  material and history stay reachable. Next moves on and the Card's schedule
  is untouched, so it comes round again.
- The Owner can ask for an ungraded Attempt to be checked again. That is a
  new, deliberate check of the same saved answer, not a second Attempt. Scry
  never retries by itself. A check that resolves gives the Attempt its
  Verdict once; it changes the schedule only if the Card has no newer
  Attempt.
- A Verdict cannot be overturned. Where the content was at fault, an agent
  supplies a corrected Version, with any alias it lacked; later practice
  uses it, and the recorded Attempt stays as it was.

### Assistance

- A reveal, a cue or answer-bearing reference is recorded as Assistance
  before it is displayed. Reload, another tab or a later correct answer
  cannot turn that occurrence into unaided recall.
- A mismatched phrase shows the expected phrase immediately and offers a
  guided retry of the same phrase; continuing is an explicit secondary
  choice. The retry is answered with the source hidden again. It records
  nothing until it is submitted and never turns disclosed wording into
  unaided credit.
- A reveal on its own is a record of help. It is not a completed review and
  not a wrong answer.
- Optional reference, such as the whole alphabet, is absent during protected
  recall. A result shows only its own correction.

### After an answer

- Once an answer commits, the result shows automatically, beside the
  correction and the next action: the count of unaided correct answers out of
  unaided answers for that exact scope, and the recent outcomes. No summary
  appears before a commit or on an unanswered Prompt.
- It shows the actual saved next review time for the answered item. An answer
  covering several Units shows the earliest scheduled part, never an invented
  whole-text schedule. Unscheduled or inactive work is labelled as such.
- The result, the question and the answer stay until the Owner chooses Next.

### Scheduling

- The memory model is FSRS, pinned: the same Card state, Verdict and time
  always produce the same next schedule, including learning and relearning
  steps within a day. Existing Cards carry their state forward unchanged.
- A Free recall answer to a genuinely due Card schedules it from the Verdict.
  An optional early answer is recorded as extra practice and leaves the
  schedule alone.
- Under Study, a text or mapping introduces its Units in reading order, at
  most its Pace per rolling 24 hours, and an ordered list is introduced
  whole. Each Study keeps its own limit; one reaching it does not block the
  others. Focused practice differs only as Lessons states.
- The Owner can pause, resume or drop a Study and choose its Pace, which
  starts at steady. Resuming returns a paused or dropped Study to practice
  with its schedule intact.
- Asking what is due lists due Cards and upcoming times without presenting,
  consuming or changing anything.

### Lessons

- A quiet lesson shelf on the practice screen groups the Starter shelf by
  Work rather than as a flat catalogue, with essential wording and language
  labels explicit. Current practice is distinguished from starting another
  lesson.
- Browsing or dismissing the shelf keeps the typed draft and the held
  question or result, makes no request and writes no evidence. Returning to
  current practice does not start a lesson.
- Switching away from an unanswered question says plainly that it stays
  ungraded and closes that exact Prompt: it can no longer be answered, so a
  late answer cannot grade a different question.
- Browsing never imports content or starts a Study. Deliberate selection
  imports the immutable supplied content once and enters scheduled practice
  for that lesson. Selecting again returns the same content and Study without
  resetting anything.
- A mapping lesson is introduced at its Pace. A text lesson and focused NATO
  practice make their whole content available at once, without changing the
  stored Pace. Once a text lesson has been entered, Study may serve any of its
  phrases.
- Agent-supplied content stays reachable beside the Starter shelf wherever
  protected recall permits. A shared title or edition name never hides custom
  content as a starter.

### Texts and prayers

- A text is learned phrase by phrase through its authored, meaning-based
  clauses: Source study of a phrase, then its Quiz. The Owner may begin at
  any Section instead of the start.
- Reciting a Section or the whole Version is an optional challenge the Owner
  chooses. It stays secondary to the next phrase.
- A phrase Quiz names the Work. The previous phrase opens as context only
  when the Owner asks, without changing the draft, the Prompt or what counts
  as help.
- A Recitation is prompted from its heading or title alone. It has one
  Verdict for the whole and one for each Unit, consistent with each other.
- Supplied Latin starter wording is unaccented. A named English Version is
  never substituted for another.

### Ordered lists

- An ordered list is recalled as one whole sequence and is never drilled item
  by item. Each Rosary set is one recall of its five mysteries in order.
- Grading recognizes the intended item identities and their order. Authored
  aliases are accepted, and the result shows the Version's own form.
- The result reports missing, repeated and misplaced items. One omission does
  not cascade across the later correctly ordered items. A duplicate or a
  transposition cannot earn a correct whole result.
- Each item keeps its own Card. Only items that are genuinely due change
  their schedule; the rest count as optional early practice.

### Keyed mappings

- A mapping is practised one pair at a time, in each direction its Version
  declares. A forward Prompt shows the key alone.
- A value that belongs to another pair is identified as the wrong key, never
  accepted as a near match.
- A year is accepted with or without AD or CE and graded as an exact number,
  unless the Version declares a range.
- A mapping is never recited as a whole set.

### NATO alphabet

- Choosing the alphabet offers one **Start practice**. It imports the
  canonical alphabet, starts or resumes its Study and begins with its first
  eligible letter, with no Library, Version, Pace or setup screen. Starting
  again never duplicates content, resets a schedule or replaces another
  Study's held Prompt.
- Starting focused practice deliberately allows all 26 letters to be
  introduced. Under Study, the alphabet follows its Pace like any mapping.
- The alphabet has all 26 official words, including Alfa, Juliett, Whiskey
  and X-ray, and is Spelling-strict. Case and surrounding space do not
  matter, and "X-ray" and "X ray" are the same answer. "Xray", "Alpha",
  "Juliet" and letters split apart by punctuation are misses.
- Practice serves due letters, then new ones, and never presents a letter
  that is not yet due as ordinary review. It prefers a different letter from
  the one just answered, returning to that one only when it is due and
  nothing else is eligible, and does not restart at A.
- The practised count is the number of distinct letters with an unaided,
  graded Attempt, labelled "practised", never "correct" or "mastered".
  Importing, revealing and repeating cannot inflate it.
- A read-only progress disclosure on the same screen shows all 26 letters.
  For each: answered Prompts, including assisted and early practice; accuracy
  as correct out of unaided answers with the denominator visible, or a dash
  with no evidence; assisted answers counted separately; recent outcomes; the
  last review time; and the actual next due time, or "Not scheduled".
- Opening, selecting or closing that disclosure writes nothing and changes no
  draft, held Prompt, Assistance or schedule. It never exposes a canonical
  word or a past raw answer during protected recall. It shows no invented
  queue position; only the held letter is labelled as on screen or as
  answered and saved. Arrow keys move between letters.

### Baby-care readiness

- The Starter shelf includes three original, sourced mappings for newborn and
  early-infant preparation: safe soothing, including the five S's; everyday
  safety; and Montessori-inspired awake interaction. Their source notes stay
  accessible.
- Safety guidance takes precedence over optional soothing and educational
  approaches, and a critical safety condition stays in the question itself.
- A safety-bearing answer must state the safe practice itself. A dangerous
  alternative, a negation or added unsafe words is a miss. Scry makes no
  approximate clinical judgment.
- Whole-text and joined-passage modes are not applied to these factual
  questions.

### Progress

- Progress separates unaided whole attempts from assisted and phrase
  practice. It claims neither mastery nor predicted readiness.
- History lists each Study, its Cards with their schedules and any Cue
  level, every Attempt with its answer, Verdict, Diff, Assistance and
  Authority, and the Versions they refer to. Reading it changes nothing.

### Library

- Searching by a title, an alias or opening words returns matching Works with
  their Versions, and says plainly when nothing matches.
- A Version added under an existing Work's title or alias attaches to that
  Work. Adding a Version whose text equals an existing one, ignoring case,
  punctuation and spacing, returns the existing Version and creates nothing.
- A Version stores its Kind, ordered Sections and Units, Provenance, Rights
  status, edition name and sources. If its Units joined in order do not
  reproduce its text exactly, it is refused and nothing is stored.
- Ordered aliases are optional, bounded, unambiguous after normalization and
  fixed with their Version.
- Correcting a named Version always creates a new Version that supersedes
  it, even when only case, punctuation or spacing changed. The old Version
  and every Attempt against it stay unchanged.
- Text supplied without Unit boundaries is divided at its line breaks and
  clause punctuation, and its Units still rejoin to the text exactly.
- Showing a Work or Version shows its Versions, Units, Provenance, Rights
  status and Relations without changing any Study. A Relation appears on both
  of the Works it links, such as the Glorious Mysteries and the Rosary.
- A Work or Version that no Study has ever used, dropped ones included, can
  be deleted. Any other cannot.
- Learning a Version creates one Study for it. Learning it again returns that
  Study.
- The Library exported on its own contains no Study, Card, Attempt, request
  or token.
- With no current Study, the app offers the Library: search, inspect, add
  supplied text with an optional edition and source, and learn a chosen
  Version. Adding supplied text calls no paid service.
- Reading the Library never imports the Starter shelf.

### Agent access

- Every memorization action is available through a versioned API and the
  `scry` command line with machine-readable output, driven by an Owner token.
  The app uses the same behavior, and no such action is web-only. A CLI
  command asked for machine-readable output exits non-zero on any refusal or
  failure.
- An agent supplies content as a manifest that Scry validates as a whole
  before admitting anything: text with its clause boundaries and Sections, an
  ordered list with aliases, or typed pairs with a declared direction. A
  malformed or duplicate entry refuses the whole manifest. Scry fetches no
  remote source to admit it.
- An agent can start and continue a lesson for any supplied Version, not only
  the Starter shelf: learn, ask what is due, take the next Prompt, move from
  study to Quiz, submit a raw answer, reveal, and pause, resume or drop a
  Study.
- A bounded local tool server for agents exposes the same validation, import,
  search, show, learn, due, next, Quiz, answer and reveal actions under the
  same rules. It offers no generation, self-rating, Correction, credential
  creation or arbitrary network tool.

### Owner tokens

- The signed-in Owner creates an Owner token. Its secret is shown once; only
  a verifier is kept. Tokens are then listed by name, creation time, last use
  and expiry, without the secret.
- A request with an unexpired, unrevoked token acts as the Owner for every
  API action, including creating, listing and revoking tokens.
- A missing, malformed, expired or revoked token is refused before any state
  is read or changed, with the same refusal whether or not the token ever
  existed. Revocation takes effect on that token's next request.
- A committed mutation records which token acted, visible in history and
  export.
- The signed-in Owner always carries paid permission. A token carries it
  only if it was created with it, and a token cannot create one with more
  permission than it holds. An answer submitted without paid permission is
  saved and judged without Jev, and no allowance is reserved.
- A token is never kept in browser storage, and browser sessions keep their
  cross-site protection.

### Drafts and interruptions

- A held draft, question or result survives reload, navigation and return.
- **Save draft** checkpoints the text typed for the exact unanswered Prompt.
  Saving never grades, advances, schedules or calls a paid service. Anyone
  acting with the Owner's authority, on any device or through a token, can
  read it. The Owner can discard it; competing saves and discards settle on
  the newest. A committed answer or a deliberate change of Prompt clears the
  checkpoint.
- With JavaScript, an edited answer is also checkpointed quietly after five
  seconds in the foreground. The editor stays usable, and the status
  distinguishes newer unsaved text from the acknowledged snapshot. Automatic
  saving does not start on load, reconnect, return to the foreground or
  unload. It pauses when Scry refuses it for capacity; Save draft stays
  available.
- Only acknowledged text can be relied on. Text typed after the last
  acknowledged save exists only in the open page. A lost response may have
  committed, and the Owner reconciles it deliberately against its original
  operation. Reconciliation never sends edits made while the outcome was
  unknown or the device was offline.
- Pending is distinguishable from saved. A definite rejection keeps the
  editable answer. Only one unresolved action is permitted at a time, and
  practice pauses offline.
- A competing tab or stale page cannot overwrite a newer Prompt, and a late
  response cannot revert the current view, dismiss a held result or advance.
- Each check is sent to Jev once. A concurrent duplicate is refused, a
  replay reconciles the saved state without sending again, and an
  interrupted send becomes a definite failure with its reservation kept as
  unknown spend.

### Earlier concept notebook

- Everything saved in the earlier concept notebook stays reachable through
  secondary navigation and in export: Goals, Concepts with their notes and
  prerequisite links, saved questions with the wording, choices and answers
  that were presented, and every recorded answer, result, Correction and
  observation. Nothing is imported, rewritten or deleted.
- The Archived notebook is kept the same way, read-only.
- It is history, not practice. Scry does not serve, schedule or grade its
  questions, and Study never draws on it. A question that was left unanswered
  is shown as unanswered.
- A note that quotes supplied material or a web excerpt keeps the exact
  evidence and its provenance. General knowledge is labelled and never
  presented as quoted support.

### Retired generation records

- Saved requests, drafts, photos and transcripts, generated candidates,
  their defect reasons, model attribution, raw responses and known or unknown
  usage stay readable and exportable, unchanged.
- An unsent draft saved from the old creation screen stays copyable and is
  never submitted.
- Generation work saved as queued, sent, uncertain or paused is never
  claimed, published or restarted by a read, a timer, a restart, a recovery
  or the resumption of paused judgment.

### Privacy

- Only the approved Owner identity, signed in or holding a valid Owner token,
  can read, export or change anything.
- Cross-site requests are rejected. Text from the Owner, an agent or a model
  is always rendered inert.
- Private pages are not cached. After sign-out, loss of access or a change of
  identity, Back, history and cached pages reveal nothing, and returning
  revalidates access.
- An alternate hostname may redirect a read, never replay a mutation.
- Build and preview environments use synthetic data and hold no production
  data, model, backup or deployment authority.
- A verification-only sign-in reaches only an isolated practice space with
  synthetic data. It never reaches the Owner's material, operator actions or
  backups.
- The signed-in Owner can ask for a verified backup from settings. Signing in
  grants no restore capability.

### Backup and recovery

- A restart keeps every acknowledged answer and exposes unfinished work as
  recoverable or failed, never forgotten. An incompatible saved state stops
  Scry before it serves or changes anything.
- A complete, integrity-checked backup is taken daily, stored away from the
  serving machine and kept for 30 days. The targets are at most 24 hours of
  lost work and 60 minutes to restore; they are targets, not guarantees.
- A backup counts only after its remote copy is read back and verified,
  including every referenced file. An interrupted or failed backup is never
  advertised as usable, stays visibly failed in settings, and leaves study
  available.
- Health reports unhealthy, naming a stale backup, when no verified remote
  backup exists or the last is older than 26 hours. A separately authorized
  trigger may request a backup; it cannot read the Owner's data or act as him.
- A fresh, isolated environment can be restored from off-machine material
  alone. A restore never overwrites acknowledged live work or reattaches an
  old writer. Uncertain paid work stays paused after a restore and never
  resends on its own. While it is paused Scry says so and points to
  settings, never to indefinite progress. There the Owner can deliberately
  resume future work, which leaves historical paused and unknown records
  unchanged; resuming twice is a conflict that changes nothing.
- Growth beyond storage capacity is refused before it commits, with the draft
  kept. Records of paid work are kept even at capacity. A capacity failure
  never becomes a backup success or a silent deletion.
- A personal export contains the supplied text, current content, history and
  schedules in a documented portable form. Exporting changes nothing.

### Spend

- Jev is paid judgment. It runs, and an answer leaves Scry to be judged,
  only where the Owner has turned paid judgment on and the request carries
  paid permission. Otherwise an answer that would need Jev stays saved and
  ungraded.
- Two caps both apply: at most 3.50 US dollars in any rolling 24 hours,
  enforced by Scry, and at most 25 US dollars a week, enforced on the
  provider account. When a cap is reached, Scry says so and answers are
  handled as they are without Jev until the cap allows more.
- Each paid request reserves against the allowance before it is sent. A
  request that was sent with an unknown outcome keeps its reservation until
  the Owner settles it as spent or released; it is never retried for free.
- The Spend ledger records the purpose and model of every entry and reports
  spend by day, purpose and model.

### Build identity

- A Changelog link in the app's navigation shows the release notes included
  in the serving build, never newer unpublished work, with the version and
  the full source revision. It does not read or change study state and stays
  behind the same private access.
- Health responses identify that same version and revision without exposing
  any of the Owner's material.

### Failure reporting

- An unexpected failure at the application boundary is reported to the
  Owner's triage route with only a fixed failure category and the deployment
  identity: never content, addresses, headers, credentials, raw errors or
  response bodies. An ordinary access denial is not an incident.
- Reporting is bounded and asynchronous. An unavailable or rejecting sink
  never delays or changes a response, and study never depends on it.

## Quality bars

- **Acknowledgement:** a tap or key press shows a pressed or pending response
  within 100 ms at the 95th percentile on the Owner's phone, including with
  300 ms of network delay, without waiting for the network.
- **Result:** when Scry can judge an answer without a paid call, the
  committed result is visible within 300 ms at the 95th percentile on a
  connection with at most 100 ms round trip. A Jev-backed judgment has no
  time bar; the checking state shows until it resolves.
- **Next:** a prepared Next shows within 150 ms and an existing first review
  within 2 seconds, both at the 95th percentile.
- **Honest continuity:** at most one speculative next Prompt, never a
  speculative success. A cancelled gesture or stale response never submits.
- **Approval:** the Owner approves aesthetics and usefulness. Verification
  can find defects and measure these bars; it cannot grant that approval.
