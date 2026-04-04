# Context Packet: Content Type Eval Suite

## Spec

### Relationship to Item 003

Item 003 establishes the phrasing generation eval pattern: a separate
`evals/promptfoo-phrasing.yaml` config with structural assertions (S1-S6) and
four LLM rubric dimensions (standalone clarity, distractor quality, explanation
value, difficulty calibration). Item 004 does NOT create new eval infrastructure.
It adds test cases to the existing configs.

### Where Cases Land

| Content type | Stage tested | Config file | Prompt template |
|---|---|---|---|
| Poetry | concept-synthesis + phrasing | both `promptfoo.yaml` and `promptfoo-phrasing.yaml` |
| Prayers | concept-synthesis + phrasing | both |
| NATO (extended) | phrasing only (concept-synthesis already has a NATO case) | `promptfoo-phrasing.yaml` |
| Book analysis | concept-synthesis + phrasing | both |
| Vocabulary | concept-synthesis + phrasing | both |
| Trivia/facts | concept-synthesis + phrasing | both |

**Concept-synthesis cases** use `intentJson` as input and assert on concept
count, content type preservation, and domain-specific keywords.

**Phrasing cases** use `conceptTitle`, `contentType`, `originIntent`,
`existingQuestions`, `targetCount` as inputs and inherit all structural/quality
assertions from 003's `defaultTest`, plus per-case content-specific assertions.

### Generation Stages

The Scry pipeline has three stages:

1. **Intent extraction** (`buildIntentExtractionPrompt`) -- classifies raw user input
2. **Concept synthesis** (`buildConceptSynthesisPrompt`) -- produces atomic concepts from intent
3. **Phrasing generation** (`buildPhrasingGenerationPrompt`) -- produces quiz questions from one concept

This eval suite tests stages 2 and 3. Stage 1 (intent extraction) is not
tested here because the test cases provide pre-classified intent JSON directly,
isolating concept synthesis and phrasing generation from classification errors.

---

## Test Content Library

### Poetry

#### Case P1: "Ozymandias" by Percy Bysshe Shelley (1818, public domain)

**Full text:**

```
I met a traveller from an antique land
Who said: "Two vast and trunkless legs of stone
Stand in the desert . . . Near them, on the sand,
Half sunk, a shattered visage lies, whose frown,
And wrinkled lip, and sneer of cold command,
Tell that its sculptor well those passions read
Which yet survive, stamped on these lifeless things,
The hand that mocked them, and the heart that fed:
And on the pedestal these words appear:
'My name is Ozymandias, king of kings:
Look on my works, ye Mighty, and despair!'
Nothing beside remains. Round the decay
Of that colossal wreck, boundless and bare
The lone and level sands stretch far away."
```

**Intent JSON for concept-synthesis:**

```json
{
  "content_type": "verbatim",
  "goal": "memorize",
  "atomic_units": [
    "Two vast and trunkless legs of stone",
    "Half sunk, a shattered visage lies",
    "My name is Ozymandias, king of kings",
    "Look on my works, ye Mighty, and despair!",
    "Nothing beside remains",
    "The lone and level sands stretch far away"
  ],
  "synthesis_ops": [
    "irony of Ozymandias's boast vs. ruin",
    "theme of impermanence of power",
    "frame narrative structure (traveller)"
  ],
  "confidence": 0.95
}
```

**Expected concept extraction:** 6 concepts, one per atomic unit. Each should
be `verbatim` type. Descriptions must attribute to Shelley/Ozymandias.

**Phrasing test concept:** `"My name is Ozymandias, king of kings" (Ozymandias, Shelley)`
- contentType: `verbatim`
- Questions should test: what line comes after the pedestal inscription, who
  spoke the words, what the inscription says, irony of the boast.
- Distractors should draw from adjacent lines in the poem, not random content.

#### Case P2: "The Road Not Taken" by Robert Frost (1916)

**Key lines for atomic units** (used as eval input, not full reproduction):

```json
{
  "content_type": "verbatim",
  "goal": "memorize",
  "atomic_units": [
    "Two roads diverged in a yellow wood",
    "And sorry I could not travel both",
    "I took the one less traveled by",
    "And that has made all the difference",
    "Yet knowing how way leads on to way",
    "I doubted if I should ever come back"
  ],
  "synthesis_ops": [
    "common misreading as celebration of nonconformity",
    "irony of the speaker's future retelling",
    "the roads were actually 'really about the same'"
  ],
  "confidence": 0.9
}
```

**Expected concept extraction:** 6 concepts, `verbatim` type. Descriptions
must attribute to Robert Frost / "The Road Not Taken."

**Phrasing test concept:** `"Two roads diverged in a yellow wood" (The Road Not Taken, Frost)`
- contentType: `verbatim`
- Questions should test: opening line recall, what comes next, which stanza
  a line belongs to, the poem's actual meaning vs. popular misreading.
- Key quality signal: the poem is widely misread as a celebration of
  individualism; good questions should test understanding that the roads
  were "really about the same."

---

### Prayers

#### Case R1: Hail Mary (Traditional English)

**Full text (USCCB source):**

```
Hail, Mary, full of grace,
the Lord is with thee.
Blessed art thou among women
and blessed is the fruit of thy womb, Jesus.
Holy Mary, Mother of God,
pray for us sinners,
now and at the hour of our death.
Amen.
```

**Intent JSON for concept-synthesis:**

```json
{
  "content_type": "verbatim",
  "goal": "memorize",
  "atomic_units": [
    "Hail, Mary, full of grace, the Lord is with thee",
    "Blessed art thou among women and blessed is the fruit of thy womb, Jesus",
    "Holy Mary, Mother of God, pray for us sinners",
    "now and at the hour of our death. Amen."
  ],
  "synthesis_ops": [
    "biblical source: Luke 1:28 (Annunciation) and Luke 1:42 (Visitation)",
    "two-part structure: scriptural greeting + petitionary prayer"
  ],
  "confidence": 0.95
}
```

**Expected concept extraction:** 4 concepts, all `verbatim`. The system MUST
NOT paraphrase or modernize the language ("thee," "thou," "art," "womb").

**Phrasing test concept:** `"Hail, Mary, full of grace, the Lord is with thee" (Hail Mary prayer)`
- contentType: `verbatim`
- Critical assertion: correct answer options must use EXACT traditional wording.
  "Hail, Mary, full of grace" not "Hail Mary, you are full of grace."
- Distractors should be plausible misquotations (word order swaps, substitutions
  from the Our Father, etc.), NOT random phrases.

#### Case R2: Our Father / Lord's Prayer (Traditional English)

**Full text (USCCB source):**

```
Our Father, who art in heaven,
hallowed be thy name;
thy kingdom come,
thy will be done
on earth as it is in heaven.
Give us this day our daily bread,
and forgive us our trespasses,
as we forgive those who trespass against us;
and lead us not into temptation,
but deliver us from evil.
Amen.
```

**Intent JSON for concept-synthesis:**

```json
{
  "content_type": "verbatim",
  "goal": "memorize",
  "atomic_units": [
    "Our Father, who art in heaven, hallowed be thy name",
    "thy kingdom come, thy will be done on earth as it is in heaven",
    "Give us this day our daily bread",
    "and forgive us our trespasses, as we forgive those who trespass against us",
    "and lead us not into temptation, but deliver us from evil"
  ],
  "synthesis_ops": [
    "biblical source: Matthew 6:9-13",
    "petition structure: three God-directed then four human-need petitions"
  ],
  "confidence": 0.95
}
```

**Expected concept extraction:** 5 concepts, all `verbatim`. Same archaic
language preservation rule as Hail Mary.

**Phrasing test concept:** `"Give us this day our daily bread" (Our Father / Lord's Prayer)`
- contentType: `verbatim`
- Questions should test: sequential recall (what petition comes before/after),
  exact wording, source attribution (Matthew 6).
- Critical: "trespasses" not "debts" or "sins" (denominational variants are
  distinct; the eval tests the Catholic/traditional English form).

---

### NATO Phonetic Alphabet

#### Case N1: Full Alphabet (concept-synthesis, extending existing case)

The existing `promptfoo.yaml` already has a NATO case. This eval suite adds
**phrasing generation** cases for individual NATO entries, plus a stricter
concept-synthesis case.

**Complete NATO phonetic alphabet (official ICAO/NATO spellings):**

| Letter | Code Word | Letter | Code Word |
|--------|-----------|--------|-----------|
| A | Alfa | N | November |
| B | Bravo | O | Oscar |
| C | Charlie | P | Papa |
| D | Delta | Q | Quebec |
| E | Echo | R | Romeo |
| F | Foxtrot | S | Sierra |
| G | Golf | T | Tango |
| H | Hotel | U | Uniform |
| I | India | V | Victor |
| J | Juliett | W | Whiskey |
| K | Kilo | X | X-ray |
| L | Lima | Y | Yankee |
| M | Mike | Z | Zulu |

Note: Official spelling is "Alfa" (not "Alpha") and "Juliett" (not "Juliet").
The existing eval case uses "Alpha" and "Juliet" — consider correcting in the
existing case as well.

**Intent JSON for concept-synthesis (stricter version):**

```json
{
  "content_type": "enumerable",
  "goal": "memorize",
  "atomic_units": [
    "A - Alfa", "B - Bravo", "C - Charlie", "D - Delta",
    "E - Echo", "F - Foxtrot", "G - Golf", "H - Hotel",
    "I - India", "J - Juliett", "K - Kilo", "L - Lima",
    "M - Mike", "N - November", "O - Oscar", "P - Papa",
    "Q - Quebec", "R - Romeo", "S - Sierra", "T - Tango",
    "U - Uniform", "V - Victor", "W - Whiskey", "X - X-ray",
    "Y - Yankee", "Z - Zulu"
  ],
  "synthesis_ops": [
    "letter-to-word mapping",
    "word-to-letter mapping"
  ],
  "confidence": 0.95
}
```

**Expected concept extraction:** Exactly 26 concepts, one per letter-word pair.
Each must be `enumerable` type. No merging (e.g., grouping A-E together = failure).

**Phrasing test concepts (3 representative cases):**

1. `"Alfa (NATO Phonetic Alphabet: A)"` — contentType: `enumerable`
   - Tests letter-to-word direction: "In the NATO phonetic alphabet, what word represents the letter A?"
   - Tests word-to-letter direction: "In the NATO phonetic alphabet, which letter does 'Alfa' represent?"
   - Distractors: other NATO code words (Bravo, Charlie, Delta), NOT random words.

2. `"November (NATO Phonetic Alphabet: N)"` — contentType: `enumerable`
   - Tests potential confusion: "November" is also a month, questions should
     disambiguate by referencing NATO context explicitly.

3. `"X-ray (NATO Phonetic Alphabet: X)"` — contentType: `enumerable`
   - Tests a hyphenated entry; questions should handle the hyphen correctly.

---

### Book Analysis

#### Case B1: Fiction — "1984" by George Orwell (themes)

**Source material (themes for concept extraction):**

Major themes of George Orwell's *Nineteen Eighty-Four* (1949):

1. **Totalitarianism and absolute power:** The Party controls every aspect of
   life in Oceania. Big Brother is the symbolic figurehead. The Inner Party
   maintains power through surveillance, propaganda, and terror. The novel
   warns that totalitarian regimes can acquire and maintain terrifying degrees
   of control.

2. **Language as mind control (Newspeak):** The Party systematically reduces
   the English language to eliminate words that could express rebellious
   thought. "Newspeak" aims to make thoughtcrime literally impossible by
   removing the vocabulary for dissent. This is the most original and
   prescient theme in the novel.

3. **Surveillance and privacy (telescreens, Thought Police):** Citizens are
   monitored by telescreens in every room. The Thought Police detect and
   punish unorthodox thinking. The novel argues that constant surveillance
   creates a prison of the mind — people self-censor even their thoughts.

4. **Reality control and historical revisionism ("doublethink"):** The Party
   rewrites history continuously. "Who controls the past controls the future;
   who controls the present controls the past." Doublethink is the ability
   to hold two contradictory beliefs simultaneously and accept both.

5. **Individuality vs. conformity:** The Party destroys all sense of personal
   identity. Winston's rebellion is ultimately about asserting that he exists
   as an individual. His defeat proves the Party's total dominance.

**Intent JSON for concept-synthesis:**

```json
{
  "content_type": "conceptual",
  "goal": "understand",
  "atomic_units": [
    "Totalitarianism and absolute power in 1984",
    "Newspeak as language-based mind control",
    "Surveillance state: telescreens and Thought Police",
    "Doublethink and historical revisionism",
    "Destruction of individuality (Winston's arc)"
  ],
  "synthesis_ops": [
    "how themes reinforce each other",
    "relevance to modern surveillance technology",
    "comparison: Orwell's predictions vs. actual history"
  ],
  "confidence": 0.9
}
```

**Expected concept extraction:** 5 concepts, all `conceptual`. Each must
capture a THEME, not just a plot point. For example, "Newspeak as mind control"
is good; "Winston works at the Ministry of Truth" is bad (plot summary, not theme).

**Phrasing test concept:** `"Newspeak as language-based mind control (1984, Orwell)"`
- contentType: `conceptual`
- Questions should test UNDERSTANDING: why the Party reduces vocabulary, what
  "thoughtcrime" means, how Newspeak relates to the Party's power.
- Distractors should be plausible misinterpretations (e.g., "Newspeak was
  designed to make communication more efficient" — wrong, but sounds reasonable).
- NOT plot trivia ("What floor does Winston live on?").

#### Case B2: Non-fiction — "Thinking, Fast and Slow" by Daniel Kahneman (key arguments)

**Source material (key arguments for concept extraction):**

Core arguments from Daniel Kahneman's *Thinking, Fast and Slow* (2011):

1. **System 1 vs. System 2:** The brain uses two distinct cognitive systems.
   System 1 is fast, automatic, intuitive, and emotional. System 2 is slow,
   deliberate, analytical, and effortful. Most daily decisions use System 1.
   System 2 is "lazy" and defers to System 1 unless forced to engage.

2. **Cognitive biases stem from System 1 heuristics:** System 1 uses mental
   shortcuts (heuristics) that are fast but error-prone. Anchoring, availability,
   representativeness — these are not "mistakes" but systematic patterns.
   "WYSIATI" (What You See Is All There Is) — System 1 builds coherent stories
   from limited evidence without seeking missing information.

3. **Prospect Theory (loss aversion):** People evaluate outcomes relative to a
   reference point, not in absolute terms. Losses loom larger than equivalent
   gains (roughly 2:1). This explains risk-averse behavior for gains and
   risk-seeking behavior for losses. Won Kahneman the Nobel Prize in Economics.

4. **Overconfidence and the illusion of understanding:** Humans systematically
   overestimate their ability to predict and explain. Narrative fallacy: we
   construct causal stories after the fact and believe them. Expert predictions
   are often no better than chance (especially in "low-validity environments").

5. **Experiencing self vs. remembering self:** The experiencing self lives in
   the present. The remembering self constructs stories about the past. They
   often disagree (peak-end rule: memories are dominated by the peak intensity
   and the ending, not the duration). Life decisions are dominated by the
   remembering self.

**Intent JSON for concept-synthesis:**

```json
{
  "content_type": "conceptual",
  "goal": "understand",
  "atomic_units": [
    "System 1 vs. System 2 thinking",
    "Cognitive heuristics and WYSIATI",
    "Prospect Theory and loss aversion",
    "Overconfidence and narrative fallacy",
    "Experiencing self vs. remembering self"
  ],
  "synthesis_ops": [
    "how System 1 produces systematic biases",
    "practical implications for decision-making",
    "relationship between prospect theory and risk behavior"
  ],
  "confidence": 0.9
}
```

**Expected concept extraction:** 5 concepts, all `conceptual`. Each must
capture an ARGUMENT, not just a topic label. "System 1 vs. System 2" is a
topic; "System 1 is fast/automatic and System 2 is slow/deliberate; most
decisions use System 1" is an argument.

**Phrasing test concept:** `"Prospect Theory and loss aversion (Thinking, Fast and Slow, Kahneman)"`
- contentType: `conceptual`
- Questions should test: what prospect theory claims (relative not absolute
  evaluation), the approximate loss-aversion ratio (2:1), why people are
  risk-seeking when facing losses.
- Distractors should be common misconceptions: "people are always risk-averse,"
  "losses and gains are weighted equally," "prospect theory applies only to
  financial decisions."

---

### Vocabulary

#### Case V1: Foreign Language — Spanish Food Vocabulary (10 words)

**Word list:**

| Spanish | English | Example sentence |
|---------|---------|-----------------|
| el pan | bread | Compro el pan en la panaderia. |
| la leche | milk | Me gusta la leche fria. |
| el queso | cheese | El queso manchego es delicioso. |
| la manzana | apple | Quiero una manzana roja. |
| el pollo | chicken | El pollo asado es mi favorito. |
| el arroz | rice | El arroz con frijoles es tipico. |
| la carne | meat/beef | La carne esta muy tierna. |
| el pescado | fish | El pescado fresco es mejor. |
| la sopa | soup | La sopa de tomate esta caliente. |
| el huevo | egg | Necesito un huevo para la receta. |

**Intent JSON for concept-synthesis:**

```json
{
  "content_type": "enumerable",
  "goal": "memorize",
  "atomic_units": [
    "el pan - bread",
    "la leche - milk",
    "el queso - cheese",
    "la manzana - apple",
    "el pollo - chicken",
    "el arroz - rice",
    "la carne - meat/beef",
    "el pescado - fish",
    "la sopa - soup",
    "el huevo - egg"
  ],
  "synthesis_ops": [
    "grammatical gender (el vs. la)",
    "Spanish-to-English and English-to-Spanish directions"
  ],
  "confidence": 0.95
}
```

**Expected concept extraction:** 10 concepts, one per word pair. Each `enumerable`.
No merging ("food vocabulary" as one concept = failure).

**Phrasing test concept:** `"el queso - cheese (Spanish food vocabulary)"`
- contentType: `enumerable`
- Questions should test BOTH directions:
  - Spanish-to-English: "What does 'el queso' mean in English?"
  - English-to-Spanish: "How do you say 'cheese' in Spanish?"
- Should also test usage in context: "In the sentence 'El ____ manchego es
  delicioso,' what word fills the blank?"
- Distractors: other Spanish food words from the same set (el pan, la leche),
  NOT random Spanish words from unrelated categories.

#### Case V2: English — SAT-Level Vocabulary (10 words)

**Word list:**

| Word | Part of speech | Definition | Example sentence |
|------|---------------|------------|-----------------|
| ubiquitous | adj. | present, appearing, or found everywhere | Smartphones have become ubiquitous in modern society. |
| ephemeral | adj. | lasting for a very short time | The beauty of cherry blossoms is ephemeral, lasting only a few days. |
| pragmatic | adj. | dealing with things sensibly and realistically | She took a pragmatic approach to solving the budget crisis. |
| ambivalent | adj. | having mixed feelings or contradictory ideas | He felt ambivalent about accepting the promotion. |
| perfunctory | adj. | carried out with minimum effort or reflection | The security guard gave a perfunctory glance at the ID badge. |
| sycophant | n. | a person who acts obsequiously to gain advantage | The CEO surrounded himself with sycophants who never challenged his ideas. |
| esoteric | adj. | intended for or understood by only a small number of people | The professor's lectures on quantum topology were esoteric even for graduate students. |
| pernicious | adj. | having a harmful effect, especially in a gradual or subtle way | The pernicious effects of misinformation erode public trust over time. |
| equivocate | v. | use ambiguous language so as to conceal the truth or avoid committing oneself | The politician continued to equivocate when asked about the policy. |
| laconic | adj. | using very few words | The laconic reply — "Fine." — told her everything she needed to know. |

**Intent JSON for concept-synthesis:**

```json
{
  "content_type": "enumerable",
  "goal": "memorize",
  "atomic_units": [
    "ubiquitous - present everywhere",
    "ephemeral - lasting a very short time",
    "pragmatic - sensible and realistic",
    "ambivalent - having mixed feelings",
    "perfunctory - with minimum effort",
    "sycophant - obsequious flatterer",
    "esoteric - understood by few",
    "pernicious - harmful in a gradual way",
    "equivocate - use ambiguous language",
    "laconic - using very few words"
  ],
  "synthesis_ops": [
    "usage in context sentences",
    "distinguish from near-synonyms"
  ],
  "confidence": 0.9
}
```

**Expected concept extraction:** 10 concepts, one per word. Each `enumerable`.

**Phrasing test concept:** `"ephemeral - lasting for a very short time (SAT vocabulary)"`
- contentType: `enumerable`
- Questions should test: definition recall, usage in context (fill-in-the-blank),
  and distinguishing from near-synonyms.
- Good distractor set for "ephemeral": "eternal" (antonym), "sporadic"
  (related but different), "perpetual" (antonym), "evanescent" (near-synonym,
  tests precision).
- NOT definition-only recall: at least one question should test usage in a
  sentence or distinguish it from a similar word.

---

### Trivia

#### Case T1: Historical Dates

**Fact set (12 entries):**

| Event | Date | Significance |
|-------|------|-------------|
| Fall of Constantinople | 1453 | End of the Byzantine Empire; Ottoman Empire rises |
| Columbus reaches the Americas | 1492 | Beginning of European colonization of the New World |
| American Declaration of Independence | July 4, 1776 | United States declares independence from Britain |
| French Revolution begins | 1789 | Storming of the Bastille; end of absolute monarchy in France |
| Battle of Waterloo | 1815 | Napoleon's final defeat; reshapes European balance of power |
| US Civil War begins | 1861 | Fort Sumter attacked; war over slavery and states' rights |
| World War I begins | 1914 | Assassination of Archduke Franz Ferdinand triggers global war |
| Russian Revolution | 1917 | Bolsheviks seize power; Soviet Union formed |
| D-Day (Normandy landings) | June 6, 1944 | Allied invasion of occupied France; turning point of WWII |
| Hiroshima atomic bombing | August 6, 1945 | First nuclear weapon used in warfare |
| Berlin Wall falls | November 9, 1989 | Symbolic end of the Cold War |
| September 11 attacks | September 11, 2001 | Deadliest terrorist attack in history; reshaped US foreign policy |

**Intent JSON for concept-synthesis:**

```json
{
  "content_type": "enumerable",
  "goal": "memorize",
  "atomic_units": [
    "Fall of Constantinople - 1453",
    "Columbus reaches the Americas - 1492",
    "American Declaration of Independence - 1776",
    "French Revolution begins - 1789",
    "Battle of Waterloo - 1815",
    "US Civil War begins - 1861",
    "World War I begins - 1914",
    "Russian Revolution - 1917",
    "D-Day Normandy landings - June 6, 1944",
    "Hiroshima atomic bombing - August 6, 1945",
    "Berlin Wall falls - November 9, 1989",
    "September 11 attacks - 2001"
  ],
  "synthesis_ops": [
    "chronological ordering",
    "cause-and-effect chains between events"
  ],
  "confidence": 0.9
}
```

**Expected concept extraction:** 12 concepts, each `enumerable`. Each must
pair the event with its date. Lumping multiple events into one concept = failure.

**Phrasing test concept:** `"D-Day Normandy landings - June 6, 1944 (World History)"`
- contentType: `enumerable`
- Questions should test: exact date, what the event was, who was involved,
  what conflict it was part of.
- Distractors for the date: other WWII dates (1941, 1943, 1945), NOT random
  years. Distractors for the event: other WWII operations (Market Garden,
  Torch, Overlord vs. Barbarossa).
- Critical: all facts in correct answers MUST be historically accurate.

#### Case T2: Science Facts

**Fact set (10 entries):**

| Fact | Category | Common misconception |
|------|----------|---------------------|
| Water boils at 100C (212F) at sea level | Physics | "Water always boils at 100C" (ignores altitude/pressure) |
| The speed of light in vacuum is approximately 299,792 km/s | Physics | "Nothing can travel faster than light" (phase velocity can) |
| DNA stands for deoxyribonucleic acid | Biology | Often confused with RNA (ribonucleic acid) |
| Mitochondria are the primary site of cellular energy (ATP) production | Biology | "Mitochondria are the powerhouse of the cell" (oversimplified) |
| The periodic table has 118 confirmed elements | Chemistry | Number changes as new elements are confirmed |
| Diamonds are composed of carbon atoms in a crystal lattice | Chemistry | "Diamonds are the hardest substance" (not universally true) |
| Humans have 23 pairs of chromosomes (46 total) | Biology | "All organisms have the same number" (varies widely) |
| The Earth's core is primarily iron and nickel | Geology | "The core is liquid" (inner core is solid, outer core is liquid) |
| Photosynthesis converts CO2 and water into glucose and oxygen | Biology | "Plants don't respire" (they do both) |
| Absolute zero is 0 Kelvin (-273.15C) | Physics | "Absolute zero means no energy" (quantum zero-point energy persists) |

**Intent JSON for concept-synthesis:**

```json
{
  "content_type": "enumerable",
  "goal": "memorize",
  "atomic_units": [
    "Water boils at 100C at sea level",
    "Speed of light: ~299,792 km/s in vacuum",
    "DNA: deoxyribonucleic acid",
    "Mitochondria: primary ATP production site",
    "Periodic table: 118 confirmed elements",
    "Diamonds: carbon in crystal lattice",
    "Humans: 23 pairs of chromosomes (46 total)",
    "Earth's core: primarily iron and nickel",
    "Photosynthesis: CO2 + H2O -> glucose + O2",
    "Absolute zero: 0 K (-273.15C)"
  ],
  "synthesis_ops": [
    "common misconceptions for each fact",
    "cross-disciplinary connections"
  ],
  "confidence": 0.9
}
```

**Expected concept extraction:** 10 concepts, each `enumerable`. Facts must be
scientifically accurate in the concept descriptions.

**Phrasing test concept:** `"Mitochondria: primary ATP production site (Biology)"`
- contentType: `enumerable`
- Questions should test: what mitochondria do, what ATP is, the distinction
  between mitochondria and other organelles.
- Distractors should be OTHER organelles (ribosomes, chloroplasts, Golgi apparatus),
  NOT random biology terms.
- The common misconception column provides excellent distractor material.
- Factual accuracy is NON-NEGOTIABLE: if a question states a wrong fact as
  correct, the eval fails regardless of other quality.

---

## Quality Assertions Per Content Type

### Verbatim Content (Poetry, Prayers)

**What GOOD looks like:**
- Questions test recall of EXACT text: "What is the next line after...?"
- Answer options preserve archaic/original language exactly
- Source attribution (author, work title) appears in the question stem
- Distractors are plausible misquotations: wrong word order, substitutions
  from adjacent lines, words from a similar work (mixing up prayers, mixing
  up Frost poems)

**What BAD looks like:**
- Questions paraphrase the original: "What is the general meaning of...?"
- Answer options modernize language: "you" instead of "thou"
- No source attribution — question assumes context
- Distractors are random phrases unrelated to the source text

**Assertion rubric language:**

```
Evaluate whether these questions test VERBATIM RECALL of specific text from
[source]. Good verbatim questions:
(1) Ask for the next line, a specific phrase, or exact wording
(2) Preserve original language in answer options (archaic forms, exact punctuation)
(3) Use distractors from adjacent lines or plausible misquotations
(4) Explicitly identify the source text in the question stem

Bad verbatim questions paraphrase, modernize language, or test general
knowledge about the topic rather than specific text recall.

Score 0.0 if questions are general comprehension, not text recall.
Score 0.5 if questions reference specific text but allow paraphrased answers.
Score 1.0 if questions demand exact wording and use adjacent-text distractors.
```

### Enumerable Content (NATO, Vocabulary, Trivia)

**What GOOD looks like:**
- One concept per item — no merging or grouping
- Questions test BOTH directions where applicable (Spanish->English AND
  English->Spanish; letter->word AND word->letter)
- Distractors drawn from the SAME SET (other NATO words, other Spanish foods,
  other historical dates), creating genuine discrimination challenges
- For vocabulary: at least one question tests usage in context, not just
  definition recall

**What BAD looks like:**
- Multiple items merged into one concept ("A-E in the NATO alphabet")
- Questions test only one direction (always Spanish->English, never reverse)
- Distractors from unrelated domains (a NATO question with chemistry distractors)
- For vocabulary: only "What does X mean?" questions with no context usage

**Assertion rubric language:**

```
Evaluate whether these questions test recall of individual items from an
enumerable set. Good enumerable questions:
(1) Test a single, specific item (not a group)
(2) Use distractors from the same category/set
(3) Test multiple cognitive directions where applicable
(4) Require genuine discrimination between similar items

Bad enumerable questions group items, use unrelated distractors, or only
test in one direction.

Score 0.0 if items are grouped or distractors are from unrelated domains.
Score 0.5 if questions are one-directional but use same-set distractors.
Score 1.0 if questions test discrimination with same-set distractors in
multiple directions.
```

### Conceptual Content (Book Analysis)

**What GOOD looks like:**
- Questions test UNDERSTANDING of themes/arguments, not plot summary or
  factual trivia
- For fiction: "Why does the Party use Newspeak?" not "What is Winston's job?"
- For non-fiction: "What does prospect theory predict about risk behavior?"
  not "In what year was the book published?"
- Distractors are plausible misinterpretations of the theme, not random facts
- Explanations connect the answer back to the broader argument

**What BAD looks like:**
- Questions test surface-level plot recall: "Who is Big Brother?" (trivia)
- Questions are so generic they could apply to any book: "What is the main theme?"
- Distractors are obviously wrong (a question about 1984 with a distractor
  about Harry Potter)
- Explanations just restate the answer without explaining WHY

**Assertion rubric language:**

```
Evaluate whether these questions test THEMATIC UNDERSTANDING of the source
material, not plot summary or surface-level trivia.

For the concept "[conceptTitle]":
Good conceptual questions:
(1) Ask WHY or HOW, not just WHAT
(2) Test understanding of arguments/themes, not factual recall of details
(3) Use distractors that represent plausible misinterpretations
(4) Have explanations that teach the underlying reasoning

Bad conceptual questions ask "Who/What/When" trivia, use obviously wrong
distractors, or have explanations that just restate the answer.

Score 0.0 if questions are factual trivia about the source material.
Score 0.5 if questions touch on themes but don't test deep understanding.
Score 1.0 if questions require genuine thematic understanding and reasoning.
```

### Factual Accuracy (Trivia, Science)

An additional assertion layer for trivia and science content. Applied ON TOP
of the standard quality rubrics.

**Assertion rubric language:**

```
Evaluate the FACTUAL ACCURACY of all questions, correct answers, and
distractors in this output. Every factual claim must be correct.

Check specifically:
(1) The correct answer is actually correct
(2) All distractors are actually wrong (no accidentally correct distractors)
(3) Dates, numbers, names, and scientific facts are precise
(4) Explanations do not contain factual errors

Score 0.0 if any correct answer is wrong or any distractor is accidentally correct.
Score 0.5 if facts are mostly right but imprecise (wrong date by a year, etc.).
Score 1.0 if all facts are verifiably accurate.
```

---

## Implementation Sequence

### Prerequisites
- Item 003 must be complete (phrasing eval infrastructure exists)
- `evals/promptfoo-phrasing.yaml` exists with `defaultTest` assertions
- `pnpm eval:phrasing` command works

### Step 1: Add concept-synthesis cases to `evals/promptfoo.yaml`

Add 11 new test cases to the existing concept-synthesis config:

| ID | Content type | Source |
|----|-------------|--------|
| P1-synth | Poetry/verbatim | Ozymandias |
| P2-synth | Poetry/verbatim | The Road Not Taken |
| R1-synth | Prayer/verbatim | Hail Mary |
| R2-synth | Prayer/verbatim | Our Father |
| B1-synth | Book/conceptual | 1984 themes |
| B2-synth | Book/conceptual | Thinking, Fast and Slow |
| V1-synth | Vocab/enumerable | Spanish food words |
| V2-synth | Vocab/enumerable | SAT vocabulary |
| T1-synth | Trivia/enumerable | Historical dates |
| T2-synth | Trivia/enumerable | Science facts |
| N1-synth | NATO/enumerable | Full 26-letter alphabet (stricter) |

Each case uses the `intentJson` vars from this context packet and asserts on:
- Correct concept count (matching atomic_units length)
- Correct contentType preservation
- Domain-specific keyword presence

### Step 2: Add phrasing generation cases to `evals/promptfoo-phrasing.yaml`

Add 11 new test cases to the phrasing eval config:

| ID | Content type | conceptTitle |
|----|-------------|-------------|
| P1-phrase | Poetry/verbatim | "My name is Ozymandias, king of kings" (Ozymandias, Shelley) |
| P2-phrase | Poetry/verbatim | "Two roads diverged in a yellow wood" (The Road Not Taken, Frost) |
| R1-phrase | Prayer/verbatim | "Hail, Mary, full of grace, the Lord is with thee" (Hail Mary prayer) |
| R2-phrase | Prayer/verbatim | "Give us this day our daily bread" (Our Father / Lord's Prayer) |
| B1-phrase | Book/conceptual | Newspeak as language-based mind control (1984, Orwell) |
| B2-phrase | Book/conceptual | Prospect Theory and loss aversion (Thinking, Fast and Slow, Kahneman) |
| V1-phrase | Vocab/enumerable | el queso - cheese (Spanish food vocabulary) |
| V2-phrase | Vocab/enumerable | ephemeral - lasting for a very short time (SAT vocabulary) |
| T1-phrase | Trivia/enumerable | D-Day Normandy landings - June 6, 1944 (World History) |
| T2-phrase | Trivia/enumerable | Mitochondria: primary ATP production site (Biology) |
| N1-phrase | NATO/enumerable | Alfa (NATO Phonetic Alphabet: A) |

Each case inherits all `defaultTest` assertions from item 003, plus per-case
assertions using the content-type-specific rubric language from this packet.

### Step 3: Add content-type-specific rubrics as per-case assertions

For each phrasing test case, add the appropriate rubric from the
"Quality Assertions Per Content Type" section above:
- Verbatim rubric for P1, P2, R1, R2
- Enumerable rubric for V1, V2, T1, T2, N1
- Conceptual rubric for B1, B2
- Factual accuracy rubric for T1, T2 (additional layer)

### Step 4: Run baseline and capture results

```bash
pnpm eval           # Run full suite (both configs)
pnpm eval:phrasing  # Run phrasing only (faster iteration)
```

Record per-content-type pass rates. Expected baseline:
- Verbatim (poetry/prayers): likely weakest — models tend to paraphrase
- Enumerable (NATO/vocab/trivia): likely strongest — structured input
- Conceptual (books): moderate — depends on distractor quality
- Factual accuracy: high risk for science edge cases

### Step 5: Document results

Add a content-type results table to the eval YAML as a comment block:

```yaml
# CONTENT TYPE BASELINE (captured YYYY-MM-DD):
# Poetry (verbatim):   X/2 synth, X/2 phrase
# Prayers (verbatim):  X/2 synth, X/2 phrase
# NATO (enumerable):   X/1 synth, X/1 phrase
# Books (conceptual):  X/2 synth, X/2 phrase
# Vocab (enumerable):  X/2 synth, X/2 phrase
# Trivia (enumerable): X/2 synth, X/2 phrase
```

This identifies which content types need prompt engineering attention.

---

## Files Modified

| File | Action | Cases added |
|------|--------|-------------|
| `evals/promptfoo.yaml` | EDIT | 11 concept-synthesis cases |
| `evals/promptfoo-phrasing.yaml` | EDIT | 11 phrasing generation cases |

No new files. No infrastructure changes. This is purely additive test cases
using the patterns established by items 003.

## Key Source Files (read-only reference)

| File | Purpose |
|------|---------|
| `evals/prompts/concept-synthesis.txt` | Concept synthesis prompt template |
| `evals/prompts/phrasing-generation.txt` | Phrasing generation prompt template |
| `convex/lib/promptTemplates.ts` | Canonical prompt builders |
| `backlog.d/003-phrasing-generation-evals.ctx.md` | Pattern reference for eval design |
