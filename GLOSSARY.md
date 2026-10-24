# Scry

Scry helps its owner understand and remember concepts. Exact text memorization
remains a distinct activity. These are the words its specification, tickets
and code use; future ambitions are not implemented data contracts.

## People and access

**Owner**:
The one person Scry is for (phaedrus). Every Study, Attempt and Library entry
is his and visible only to him.
_Avoid_: user, account, learner (in new text)

**Owner token**:
A revocable bearer credential that lets a program, such as an agent using the
CLI, act as the Owner.
_Avoid_: API key, admin token, account token

## Library

**Library**:
The Owner's private store of Works and their Versions. It is kept apart from
his study state so it could be shared later without exposing him; nothing in
it is public today.
_Avoid_: public library, catalogue, deck, collection

**Work**:
The abstract thing to learn, such as "Nicene Creed" or "NATO spelling
alphabet". It has a title, aliases, a kind and a short context note.
_Avoid_: topic, goal, item

**Kind**:
The shape of a Work's content: a text (prayer, creed, poem), an ordered list,
or a keyed mapping.
_Avoid_: type, mode, category

**Version**:
One exact wording of a Work, such as "English, Roman Missal 2011". It never
changes; a correction is a new Version that supersedes it.
_Avoid_: edition, translation (unless it is one), variant

**Section**:
An ordered group of Units within a Version: a creed article, a stanza, a set
of mysteries.
_Avoid_: chunk, part

**Unit**:
The smallest piece of a Version practised on its own: a clause or line of a
text, an item of a list, a key and value of a mapping. A Version's Units,
joined in order, reproduce its text exactly.
_Avoid_: card, chunk, segment, question

**Relation**:
A typed link between Works or Versions: version of, part of, related to,
supersedes.
_Avoid_: tag, link

**Provenance**:
How a Version's text was obtained: supplied by the Owner or his agent, written
by the generation agent from model memory, or checked against cited sources.
_Avoid_: source quality, trust level

**Rights status**:
What a Version's copyright allows if the Library is ever shared: public domain,
licensed, or private only.
_Avoid_: licence flag, visibility

**Spelling-strict**:
Said of a Version whose spelling is itself the thing being learned, such as the
NATO words "Alfa" and "Juliett". Slips count as misses there.
_Avoid_: strict mode

## Study and practice

**Study**:
The Owner's decision to learn one Version, with its pace and whether it is
paused.
_Avoid_: goal, deck, course, enrolment

**Card**:
The Owner's memory state and current Cue level for one Unit in one direction.
_Avoid_: flashcard, item, note, question

**Ladder**:
The ordered Cue levels for a Kind, from reading the text with full help to
reciting it from its title alone.
_Avoid_: difficulty, levels (alone)

**Cue level**:
One rung of the Ladder: how much of a Unit is shown when it is asked, such as
first letters of every word, or only the previous line. Only the rungs without
answer-bearing help are free recall.
_Avoid_: hint level, difficulty

**Free recall**:
Answering a Unit with no answer-bearing cue. Only free recall moves the
schedule.
_Avoid_: cold recall (in new text), test mode

**Prompt**:
What Scry presents for one Card at its Cue level: the Work title, the context
around the Unit, and the cue.
_Avoid_: question (for exact recall), challenge

**Recitation**:
One answer that covers several Units in order, such as a whole stanza or a
whole Creed. Each Unit in it still gets its own verdict.
_Avoid_: full test, run-through

**Attempt**:
One immutable record of an answer: what was asked, at which Cue level, the
raw answer, the diff, the verdict and its authority.
_Avoid_: review, response, try

**Verdict**:
The outcome of an Attempt: Good when every word is right, Again for anything
else.
_Avoid_: score, mark, grade (as a noun)

**Slip**:
A typed spelling error in a word that was clearly recalled, such as
"concieved". It is shown and counts correct, except in a Spelling-strict
Version.
_Avoid_: typo, near miss, fuzzy match

**Diff**:
The word-by-word comparison of an answer with the expected text, marking each
word correct, slipped, wrong, missing or extra.
_Avoid_: feedback, correction

**Authority**:
Who decided a verdict: the exact grader, a model judge, the Owner, or a reveal.
_Avoid_: grader (for the decider), source

**Correction**:
The Owner's later override of a verdict. It is appended, and the original
Attempt stays as it was.
_Avoid_: edit, regrade

**Pace**:
How many new Units a Study introduces per rolling day.
_Avoid_: speed, intensity

## Generation and spend

**Generation agent**:
The model, with tools such as web search, that turns a request for a Work into
a draft Version. It decides when to rely on memory and when to check sources.
_Avoid_: generator pipeline, sourcing step, AI

**Generation run**:
One bounded attempt by the Generation agent to produce a Version, kept with
its tool calls, self-check results and cost whether or not it succeeds.
_Avoid_: job, task, preparation (for exact recall)

**Self-check**:
A deterministic test a draft Version must pass before it enters the Library,
such as Units joining to the text or a set having its declared count.
_Avoid_: critic, validation (alone)

**Spend ledger**:
The record of money reserved and spent on model and tool calls, by day,
purpose and model.
_Avoid_: usage log, billing

## Concepts and their material

**Concept**:
The primary teachable idea the owner wants to understand. Generated Concepts
have a Goal, a current explanation, saved questions and directed prerequisites.
The compatibility floor also retains explicitly owner-authored Concepts with
Materials and no invented Goal or practice. Normal authoring, quiz collections,
varied sources and related-concept prediction remain future work. A Concept is
not an exact-recall Work or Unit.
_Avoid_: using it for a Work or Unit

**Material**:
An independently addressable reference belonging to one Concept, with a current
revision and immutable wording history. Supplied text is not a fetched or
verified Source. An existing generated Note chain is one read-only Material
projection, preserving its original records rather than copying or splitting it.
_Avoid_: saved text (as the primary learning object), source (without provenance)

**Material revision**:
One immutable title and exact supplied text, with its Material owner, ordinal,
predecessor, supplied provenance and timestamp. Revising wording keeps earlier
revisions; separate Materials do not collapse because their wording matches.
_Avoid_: new Material (for a revision of the same reference)

**Goal**:
The saved preparation request that groups generated concepts. Exact
recall uses Study instead.
_Avoid_: using it for a Study

**Archived notebook**:
The Go-era notebook revived on 2026-10-16. It is kept read-only, nothing
deleted, and the exact-recall model starts in a fresh space.
_Avoid_: old data, legacy store
