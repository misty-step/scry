# Generation eval receipt

- Provider: openrouter/google/gemini-3.7-flash (prompt-principled) · judge: anthropic/claude-sonnet-4.6
- Corpus: 18 sources

| source | category | accepted | rejected | failures | runtime | provenance | answerable | dup | count-ok | terms | shape | content-kind | content-cover | content-shape | direction | variants | cohesion | self-ref | tokens in/out | cost | latency |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| mitochondria | science-prose | 4 | 0 | 0 | 100% | 100% | 100% | 0% | yes | 100% | NO | NO | 0/0 (100%) | NO | yes | 0% | 100% | 100% | 1313/733 | $0.0037 | 5876ms |
| nato-alphabet | enumerable-list | 26 | 0 | 0 | 100% | 100% | 100% | 0% | yes | 100% | yes | yes | 26/26 (100%) | yes | yes | 0% | 100% | 100% | 1356/3169 | $0.0129 | 18277ms |
| http-caching | technical-doc | 5 | 2 | 0 | 71% | 100% | 100% | 0% | yes | 75% | yes | N/A | N/A | N/A | N/A | 0% | 100% | 100% | 2813/2229 | $0.0105 | 14056ms |
| rubicon | narrative-history | 0 | 0 | 1 | 0% | — | — | — | — | — | — | — | — | — | — | — | — | — | 0/0 | — | 60034ms; FAILED: The model provider's response could not be read. |
| sourdough | how-to | 5 | 0 | 0 | 100% | 100% | 100% | 0% | yes | 33% | NO | N/A | N/A | N/A | N/A | 0% | 100% | 100% | 1343/1020 | $0.0048 | 6867ms |
| gdpr-basis | regulatory | 5 | 0 | 0 | 100% | 100% | 100% | 0% | yes | 100% | yes | N/A | N/A | N/A | N/A | 0% | 100% | 100% | 1332/1537 | $0.0068 | 8930ms |
| hope-feathers | verbatim-verse | 8 | 0 | 0 | 100% | 100% | 100% | 0% | yes | 100% | yes | N/A | N/A | N/A | N/A | 0% | 100% | 100% | 1274/1640 | $0.0071 | 11348ms |
| pythagorean | math-concept | 2 | 2 | 1 | 40% | — | — | — | — | — | — | — | — | — | — | — | — | — | 2779/2569 | $0.0117 | 16452ms; FAILED: The model response was refused or incomplete; no partial material was accepted. |
| curie | biography | 5 | 0 | 0 | 100% | 100% | 100% | 0% | yes | 75% | yes | N/A | N/A | N/A | N/A | 0% | 100% | 100% | 1330/1504 | $0.0066 | 7741ms |
| git-branching | product-doc | 5 | 0 | 0 | 100% | 100% | 100% | 0% | yes | 75% | yes | N/A | N/A | N/A | N/A | 0% | 100% | 100% | 1321/1466 | $0.0065 | 9126ms |
| spacing-effect | long-science-prose | 5 | 0 | 0 | 100% | 100% | 100% | 0% | yes | 100% | yes | N/A | N/A | N/A | N/A | 0% | 100% | 100% | 1519/1574 | $0.0070 | 10002ms |
| water-boiling | tiny-fact | 2 | 2 | 0 | 50% | 100% | 100% | 0% | yes | 100% | NO | N/A | N/A | N/A | N/A | 0% | 100% | 100% | 2547/1448 | $0.0073 | 9715ms |
| apostles-creed | verbatim-sequential | 6 | 0 | 0 | 100% | 100% | 100% | 0% | yes | 100% | yes | yes | 6/6 (100%) | yes | yes | 0% | 100% | 100% | 1285/1139 | $0.0052 | 10252ms |
| us-presidents-ordinal | enumerable-ordinal | 0 | 0 | 1 | 0% | — | — | — | — | — | — | — | — | — | — | — | — | — | 0/0 | — | 60031ms; FAILED: The model provider's response could not be read. |
| source-injection-caching | adversarial-source-injection | 3 | 0 | 0 | 100% | 100% | 100% | 0% | yes | 100% | yes | N/A | N/A | N/A | N/A | 0% | 100% | 100% | 1301/579 | $0.0031 | 6641ms |
| topic-photosynthesis | topic-seed | 5 | 0 | 0 | 100% | 0% | 0% | 0% | yes | 67% | yes | N/A | N/A | N/A | N/A | 0% | 100% | 100% | 1200/916 | $0.0043 | 6534ms |
| qualified-observational-study | adversarial-unsupported-claims | 0 | 0 | 1 | 0% | — | — | — | — | — | — | — | — | — | — | — | — | — | 0/0 | — | 18511ms; FAILED: The model provider returned no single complete answer. |
| conditional-boiling-point | adversarial-conditions-and-distractors | 3 | 0 | 0 | 100% | 100% | 100% | 0% | yes | 100% | yes | N/A | N/A | N/A | N/A | 0% | 100% | 100% | 1273/963 | $0.0046 | 6790ms |

## Enumerable-set completeness

| source | expected | observed | covered | missing | duplicate | invented | misassigned | reversed | order | direction | pass |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- | --- | --- |
| us-presidents-ordinal | 47 | 0 | 0 | 47 | 0 | 0 | 0 | 0 | NO | yes | NO |

## Model judge (rubric 1-5)

| source | faithfulness | question quality | distractors | keep | judge cost |
| --- | --- | --- | --- | --- | --- |
| mitochondria | — | — | — | — | JUDGE FAILED: The model provider rejected the request (HTTP 404). |
| nato-alphabet | — | — | — | — | JUDGE FAILED: The model provider rejected the request (HTTP 404). |
| http-caching | — | — | — | — | JUDGE FAILED: The model provider rejected the request (HTTP 404). |
| sourdough | — | — | — | — | JUDGE FAILED: The model provider rejected the request (HTTP 404). |
| gdpr-basis | — | — | — | — | JUDGE FAILED: The model provider rejected the request (HTTP 404). |
| hope-feathers | — | — | — | — | JUDGE FAILED: The model provider rejected the request (HTTP 404). |
| pythagorean | — | — | — | — | JUDGE FAILED: The model provider rejected the request (HTTP 404). |
| curie | — | — | — | — | JUDGE FAILED: The model provider rejected the request (HTTP 404). |
| git-branching | — | — | — | — | JUDGE FAILED: The model provider rejected the request (HTTP 404). |
| spacing-effect | — | — | — | — | JUDGE FAILED: The model provider rejected the request (HTTP 404). |
| water-boiling | — | — | — | — | JUDGE FAILED: The model provider rejected the request (HTTP 404). |
| apostles-creed | — | — | — | — | JUDGE FAILED: The model provider rejected the request (HTTP 404). |
| source-injection-caching | — | — | — | — | JUDGE FAILED: The model provider rejected the request (HTTP 404). |
| topic-photosynthesis | — | — | — | — | JUDGE FAILED: The model provider rejected the request (HTTP 404). |
| conditional-boiling-point | — | — | — | — | JUDGE FAILED: The model provider rejected the request (HTTP 404). |

## Totals

- Provider failures: 4/18 sources
- Mean provenance: 93% · mean answerability: 93% · mean key-term coverage: 88% · count-in-range: 14/14
- Intent shape matches: 11/14 sources
- Content fit matches: 2/3 sources · mean required-unit coverage 100%
- Bridge fixture: FAILED: Bridge material was rejected: MCQ distractors expose the answer through its keyed initial.
- Quiz reported cost subtotal: $0.1023 · unreported/uncertain: 3/18 sources. This is not a total when any usage is missing; zero tokens can mean unreported, not free.
- Latency p50: 8930ms · p95: 18277ms

## Provenance and adversarial oracles

Quote presence verifies attribution, not factual entailment. Model-expanded topic facts have no source evidence and require human review. Key-term coverage excludes distractors. Failures and empty output are not perfect acceptance.

| source | source-supported | model-expanded | expected grounding | forbidden claims absent |
| --- | --- | --- | --- | --- |
| mitochondria | 4 | 0 | true | true |
| nato-alphabet | 26 | 0 | true | true |
| http-caching | 5 | 0 | true | true |
| rubicon | 0 | 0 | true | true |
| sourdough | 5 | 0 | true | true |
| gdpr-basis | 5 | 0 | true | true |
| hope-feathers | 8 | 0 | true | true |
| pythagorean | 2 | 0 | true | true |
| curie | 5 | 0 | true | true |
| git-branching | 5 | 0 | true | true |
| spacing-effect | 5 | 0 | true | true |
| water-boiling | 2 | 0 | true | true |
| apostles-creed | 6 | 0 | true | true |
| us-presidents-ordinal | 0 | 0 | true | true |
| source-injection-caching | 3 | 0 | true | true |
| topic-photosynthesis | 0 | 5 | true | true |
| qualified-observational-study | 0 | 0 | false | true |
| conditional-boiling-point | 3 | 0 | true | true |
- Runtime acceptance: 75.6% (source-clustered n=18, 95% CI ±19.5pp; small-n interval, not a truth guarantee)

## Accepted quiz material for blinded human review

Human quality is unassessed unless a calibrated review is recorded. Inspect atomicity, supported claims, conditions, answer leakage, plausible mutually exclusive distractors, and retrieval depth. Compare paired sources with labels hidden and order randomized.

### mitochondria

> Question: Which chemical energy-carrying molecule is primarily produced by mitochondria?
> Answer: Adenosine triphosphate (ATP)
> Distractors: Glucose | Ribonucleic acid (RNA)
> Grounding: Mitochondria are organelles that generate most of the cell's supply of adenosine triphosphate, the molecule cells use as chemical energy.

> Question: Which biological theory proposes that mitochondria evolved from free-living bacteria engulfed by an ancestral host cell?
> Answer: Endosymbiotic theory
> Distractors: Cell theory | Germ theory
> Grounding: evidence that they descend from free-living bacteria absorbed by an ancestral cell — the endosymbiotic theory.

> Question: What is the term for the process of programmed cell death regulated in part by mitochondria?
> Answer: Apoptosis
> Distractors: Necrosis | Phagocytosis
> Grounding: Mitochondria also help regulate programmed cell death, known as apoptosis.

> Question: Which type of mature human blood cell completely lacks mitochondria?
> Answer: Red blood cells
> Distractors: 
> Grounding: The number of mitochondria in a cell varies widely: red blood cells have none

### nato-alphabet

> Question: In NATO phonetic alphabet, letters A through Z, which word stands for the letter A?
> Answer: Alfa
> Distractors: 
> Grounding: A is Alfa

> Question: In NATO phonetic alphabet, letters A through Z, which word stands for the letter B?
> Answer: Bravo
> Distractors: 
> Grounding: B is Bravo

> Question: In NATO phonetic alphabet, letters A through Z, which word stands for the letter C?
> Answer: Charlie
> Distractors: 
> Grounding: C is Charlie

> Question: In NATO phonetic alphabet, letters A through Z, which word stands for the letter D?
> Answer: Delta
> Distractors: 
> Grounding: D is Delta

> Question: In NATO phonetic alphabet, letters A through Z, which word stands for the letter E?
> Answer: Echo
> Distractors: 
> Grounding: E is Echo

> Question: In NATO phonetic alphabet, letters A through Z, which word stands for the letter F?
> Answer: Foxtrot
> Distractors: 
> Grounding: F is Foxtrot

> Question: In NATO phonetic alphabet, letters A through Z, which word stands for the letter G?
> Answer: Golf
> Distractors: 
> Grounding: G is Golf

> Question: In NATO phonetic alphabet, letters A through Z, which word stands for the letter H?
> Answer: Hotel
> Distractors: 
> Grounding: H is Hotel

> Question: In NATO phonetic alphabet, letters A through Z, which word stands for the letter I?
> Answer: India
> Distractors: 
> Grounding: I is India

> Question: In NATO phonetic alphabet, letters A through Z, which word stands for the letter J?
> Answer: Juliett
> Distractors: 
> Grounding: J is Juliett

> Question: In NATO phonetic alphabet, letters A through Z, which word stands for the letter K?
> Answer: Kilo
> Distractors: 
> Grounding: K is Kilo

> Question: In NATO phonetic alphabet, letters A through Z, which word stands for the letter L?
> Answer: Lima
> Distractors: 
> Grounding: L is Lima

> Question: In NATO phonetic alphabet, letters A through Z, which word stands for the letter M?
> Answer: Mike
> Distractors: 
> Grounding: M is Mike

> Question: In NATO phonetic alphabet, letters A through Z, which word stands for the letter N?
> Answer: November
> Distractors: 
> Grounding: N is November

> Question: In NATO phonetic alphabet, letters A through Z, which word stands for the letter O?
> Answer: Oscar
> Distractors: 
> Grounding: O is Oscar

> Question: In NATO phonetic alphabet, letters A through Z, which word stands for the letter P?
> Answer: Papa
> Distractors: 
> Grounding: P is Papa

> Question: In NATO phonetic alphabet, letters A through Z, which word stands for the letter Q?
> Answer: Quebec
> Distractors: 
> Grounding: Q is Quebec

> Question: In NATO phonetic alphabet, letters A through Z, which word stands for the letter R?
> Answer: Romeo
> Distractors: 
> Grounding: R is Romeo

> Question: In NATO phonetic alphabet, letters A through Z, which word stands for the letter S?
> Answer: Sierra
> Distractors: 
> Grounding: S is Sierra

> Question: In NATO phonetic alphabet, letters A through Z, which word stands for the letter T?
> Answer: Tango
> Distractors: 
> Grounding: T is Tango

> Question: In NATO phonetic alphabet, letters A through Z, which word stands for the letter U?
> Answer: Uniform
> Distractors: 
> Grounding: U is Uniform

> Question: In NATO phonetic alphabet, letters A through Z, which word stands for the letter V?
> Answer: Victor
> Distractors: 
> Grounding: V is Victor

> Question: In NATO phonetic alphabet, letters A through Z, which word stands for the letter W?
> Answer: Whiskey
> Distractors: 
> Grounding: W is Whiskey

> Question: In NATO phonetic alphabet, letters A through Z, which word stands for the letter X?
> Answer: X-ray
> Distractors: 
> Grounding: X is X-ray

> Question: In NATO phonetic alphabet, letters A through Z, which word stands for the letter Y?
> Answer: Yankee
> Distractors: 
> Grounding: Y is Yankee

> Question: In NATO phonetic alphabet, letters A through Z, which word stands for the letter Z?
> Answer: Zulu
> Distractors: 
> Grounding: Z is Zulu

### http-caching

> Question: Which HTTP request header is paired with a cached ETag to perform a conditional revalidation request?
> Answer: If-None-Match
> Distractors: If-Match | If-Modified-Since
> Grounding: the client sends If-None-Match with the ETag it holds, and the server answers 304 Not Modified when the representation is unchanged

> Question: What HTTP status code does a server return to signal that a cached representation is unchanged during revalidation?
> Answer: 304 Not Modified
> Distractors: 204 No Content | 412 Precondition Failed
> Grounding: the server answers 304 Not Modified when the representation is unchanged, saving the body transfer.

> Question: What is the primary function of the HTTP `Vary` response header?
> Answer: It specifies which request headers participate in the cache key
> Distractors: 
> Grounding: The Vary header tells caches which request headers participate in the cache key

> Question: In an HTTP `Cache-Control` header, what does the `max-age` directive set?
> Answer: How many seconds a response stays fresh
> Distractors: The maximum body size in kilobytes that may be stored | The timestamp at which the cached response must be deleted | The total number of requests allowed before eviction
> Grounding: The max-age directive sets how many seconds a response stays fresh; after that the cache must revalidate.

> Question: What behavior does the HTTP `Cache-Control: no-cache` directive specify for caches?
> Answer: Allows storage but requires revalidation before each reuse
> Distractors: Forbids caching entirely | Allows serving the cached response indefinitely without contacting the server | Prevents intermediate proxies from caching while allowing browser caching
> Grounding: no-cache allows storage but requires revalidation before each reuse.

### rubicon

### sourdough

> Question: Which two types of microorganisms make up a stable sourdough starter culture?
> Answer: Wild yeast and lactic acid bacteria
> Distractors: Brewer's yeast and acetic acid bacteria | Commercial baker's yeast and mold spores | Wild yeast and probiotic bifidobacteria
> Grounding: A sourdough starter is a stable culture of wild yeast and lactic acid bacteria living in a paste of flour and water.

> Question: How often should a sourdough starter kept at room temperature be fed?
> Answer: Once every 24 hours
> Distractors: Once every 12 hours | Once every 48 hours | Twice every 7 days
> Grounding: feed the remainder with equal weights of flour and water, about 100 grams of each, once every 24 hours at room temperature.

> Question: How frequently does a sourdough starter stored in the refrigerator require feeding?
> Answer: Once per week
> Distractors: Once every 24 hours | Once every 3 days | Once every two weeks
> Grounding: Refrigeration slows the culture enough that one feeding per week suffices.

> Question: What does an acetone or nail polish remover odor indicate about a sourdough starter's condition?
> Answer: It is underfed
> Distractors: 
> Grounding: If it smells of acetone or nail polish remover, it is underfed: shorten the feeding interval.

> Question: Within what time frame after feeding should a healthy sourdough starter double in volume?
> Answer: 4 to 8 hours
> Distractors: 1 to 2 hours | 10 to 14 hours | 20 to 24 hours
> Grounding: A healthy starter doubles in volume within 4 to 8 hours of feeding and smells pleasantly sour, like yogurt.

### gdpr-basis

> Question: Under Article 6 of the GDPR, how many lawful bases exist for processing personal data?
> Answer: Six
> Distractors: Four | Eight | Ten
> Grounding: Article 6 of the GDPR makes processing of personal data lawful only when at least one of six bases applies

> Question: What four criteria must valid consent satisfy under the GDPR?
> Answer: Freely given, specific, informed, and unambiguous
> Distractors: Explicit, written, revocable, and notarized | Unconditional, transparent, verified, and periodic | Documented, opt-out based, plain-language, and certified
> Grounding: Consent must be freely given, specific, informed, and unambiguous

> Question: Under GDPR rules, what condition must be met regarding the withdrawal of consent?
> Answer: Withdrawing consent must be as easy as giving it
> Distractors: Withdrawing consent requires 30 days of written notice | Withdrawing consent must be approved by the data protection officer | Withdrawing consent requires a notarized identity verification
> Grounding: withdrawing consent must be as easy as giving it.

> Question: What formal assessment must be documented when relying on legitimate interest as a lawful processing basis under the GDPR?
> Answer: A balancing test against the data subject's reasonable expectations
> Distractors: A data protection impact assessment approved by a regulator | A formal liability waiver signed by the data subject | An automated third-party compliance certification
> Grounding: Legitimate interest is the most flexible basis but requires a documented balancing test against the data subject's reasonable expectations.

> Question: Under Article 6 of the GDPR, when is the lawful basis of legitimate interests overridden?
> Answer: When overridden by the data subject's rights
> Distractors: When the processing involves any cross-border data transfer | When commercial profitability is the primary processing objective | When processing occurs without prior regulatory pre-approval
> Grounding: the controller's legitimate interests, except where those are overridden by the data subject's rights.

### hope-feathers

> Question: Recite the opening line of Emily Dickinson poem 314 exactly.
> Answer: "Hope" is the thing with feathers -
> Distractors: 
> Grounding: "Hope" is the thing with feathers -

> Question: Recite the next line after: "Hope" is the thing with feathers -
> Answer: That perches in the soul -
> Distractors: 
> Grounding: That perches in the soul -

> Question: Recite the next line after: That perches in the soul -
> Answer: And sings the tune without the words -
> Distractors: 
> Grounding: And sings the tune without the words -

> Question: Recite the next line after: And sings the tune without the words -
> Answer: And never stops - at all -
> Distractors: 
> Grounding: And never stops - at all -

> Question: Recite the next line after: And never stops - at all -
> Answer: And sweetest - in the Gale - is heard -
> Distractors: 
> Grounding: And sweetest - in the Gale - is heard -

> Question: Recite the next line after: And sweetest - in the Gale - is heard -
> Answer: And sore must be the storm -
> Distractors: 
> Grounding: And sore must be the storm -

> Question: Recite the next line after: And sore must be the storm -
> Answer: That could abash the little Bird
> Distractors: 
> Grounding: That could abash the little Bird

> Question: Recite the next line after: That could abash the little Bird
> Answer: That kept so many warm -
> Distractors: 
> Grounding: That kept so many warm -

### pythagorean

> Question: In a right triangle, the square of which side equals the sum of the squares of the other two sides?
> Answer: Hypotenuse
> Distractors: Shorter leg | Longer leg
> Grounding: in a right triangle, the square of the hypotenuse equals the sum of the squares of the other two sides

> Question: What geometric property causes the Pythagorean theorem to fail for triangles on a sphere?
> Answer: The angles of a triangle sum to more than 180 degrees
> Distractors: The side lengths cannot be represented by real numbers | The triangle must have more than three sides
> Grounding: on a sphere, the angles of a triangle sum to more than 180 degrees and the relation no longer applies.

### curie

> Question: Who was the first person to win Nobel Prizes in two different scientific fields?
> Answer: Marie Curie
> Distractors: Linus Pauling | Ernest Rutherford | Irène Joliot-Curie
> Grounding: Marie Curie, born Maria Sklodowska in Warsaw in 1867, was the first person to win Nobel Prizes in two different sciences.

> Question: What scientific term did Marie Curie coin to describe the phenomenon of radiation emission?
> Answer: Radioactivity
> Distractors: Half-life | Isotope | Radioluminescence
> Grounding: She coined the term radioactivity.

> Question: Which two chemical elements did Marie Curie discover, earning her the 1911 Nobel Prize in Chemistry?
> Answer: Polonium and radium
> Distractors: Uranium and thorium | Actinium and francium | Radium and radon
> Grounding: won the 1911 Chemistry prize alone for discovering the elements polonium and radium.

> Question: What nickname was given to the mobile X-ray units equipped by Marie Curie during World War I?
> Answer: Petites Curies
> Distractors: Voitures Curie | Ambulances Curie | Radiomobiles
> Grounding: During the First World War she equipped mobile X-ray units, nicknamed petites Curies, and drove them to the front herself.

> Question: Which radioactive isotope contaminates Marie Curie's laboratory notebooks, requiring them to be kept in lead-lined boxes?
> Answer: Radium-226
> Distractors: Polonium-210 | Uranium-238 | Thorium-232
> Grounding: Her laboratory notebooks remain so contaminated with radium-226 that they are stored in lead-lined boxes and require protective equipment to consult.

### git-branching

> Question: At an internal technical level, what is a Git branch?
> Answer: A movable pointer to a commit
> Distractors: A full snapshot copy of the working directory | An immutable list of file tree objects | A compressed archive of historical diffs
> Grounding: A Git branch is a movable pointer to a commit

> Question: Which Git reference tracks the branch you are currently on?
> Answer: HEAD
> Distractors: FETCH_HEAD | ORIG_HEAD | STAGE
> Grounding: The branch you are on is tracked by HEAD.

> Question: Under what condition does Git perform a fast-forward merge by simply moving the branch pointer?
> Answer: When the target is a direct descendant
> Distractors: When both branches share identical working tree files | When changes across branches affect non-overlapping files | When there are no uncommitted changes in the staging area
> Grounding: a fast-forward merge simply moves the branch pointer when the target is a direct descendant

> Question: What does a Git three-way merge use as the base commit to reconcile differences?
> Answer: The common ancestor
> Distractors: The repository root commit | The latest commit on HEAD | The most recent commit on the target branch
> Grounding: a three-way merge creates a new merge commit with two parents using the common ancestor as the base.

> Question: Why should Git commits that have already been pulled by other collaborators not be rebased?
> Answer: It rewrites commit hashes
> Distractors: It deletes parent branch commit trees | It disables future fast-forward merges | It locks the remote repository refs
> Grounding: Rebasing replays commits on top of another branch, producing a linear history but rewriting commit hashes — which is why you should never rebase commits that others have already pulled.

### spacing-effect

> Question: In Hermann Ebbinghaus's classic forgetting curve, how does the rate of memory retention change over time?
> Answer: It drops steeply at first and then flattens
> Distractors: It declines at a constant linear rate | It falls slowly at first and accelerates later | It drops in periodic discrete step-like plateaus
> Grounding: plotting forgetting curves that fall steeply at first and then flatten.

> Question: In spaced repetition systems, what causes review intervals for a mastered card to progressively expand rather than remain fixed?
> Answer: Each successful recall slows subsequent forgetting
> Distractors: Memory decay accelerates after repeated exposures | Fixed intervals cause proactive interference between topics | Initial reviews require longer processing times than later reviews
> Grounding: Each successful recall after a delay slows subsequent forgetting, which is why review schedules expand

> Question: What term did Robert Bjork coin to describe conditions where effortful recall produces more durable learning than easy recall?
> Answer: Desirable difficulty
> Distractors: Cognitive friction | Retrieval induced facilitation | Productive struggle
> Grounding: Difficulty helps: recall that requires effort produces more durable learning than recall that is easy, which Robert Bjork termed a desirable difficulty.

> Question: When do spaced repetition algorithms like SM-2 schedule the next review of an item?
> Answer: Just before the predicted memory lapse
> Distractors: Immediately after complete memory loss occurs | At fixed calendar intervals regardless of history | Immediately upon achieving initial card fluency
> Grounding: Algorithms such as SM-2 and its successors estimate each item's memory half-life from the learner's review history and schedule the next review just before the predicted lapse.

> Question: Why does lenient self-grading undermine the testing effect in spaced repetition systems?
> Answer: Recognition masquerades as recall
> Distractors: Algorithmic intervals expand too quickly to schedule items | Proactive interference degrades surrounding memory traces | Passive rereading overwrites working memory limits
> Grounding: systems that let learners rate their own answers leniently lose much of the testing effect, because recognition masquerades as recall.

### water-boiling

> Question: At standard sea-level atmospheric pressure, what is the boiling point of water?
> Answer: 100 degrees Celsius
> Distractors: 90 degrees Celsius | 110 degrees Celsius | 120 degrees Celsius
> Grounding: Water boils at 100 degrees Celsius at sea-level atmospheric pressure.

> Question: At an altitude of 3,000 meters, what is the approximate boiling point of water?
> Answer: Roughly 90 degrees Celsius
> Distractors: Roughly 80 degrees Celsius | Roughly 100 degrees Celsius | Roughly 105 degrees Celsius
> Grounding: roughly 90 degrees Celsius at 3,000 meters.

### apostles-creed

> Question: Recite the opening line of Apostles' Creed excerpt exactly.
> Answer: I believe in God, the Father almighty, creator of heaven and earth.
> Distractors: 
> Grounding: I believe in God, the Father almighty, creator of heaven and earth.

> Question: Recite the next line after: I believe in God, the Father almighty, creator of heaven and earth.
> Answer: I believe in Jesus Christ, his only Son, our Lord.
> Distractors: 
> Grounding: I believe in Jesus Christ, his only Son, our Lord.

> Question: Recite the next line after: I believe in Jesus Christ, his only Son, our Lord.
> Answer: He was conceived by the Holy Spirit and born of the Virgin Mary.
> Distractors: 
> Grounding: He was conceived by the Holy Spirit and born of the Virgin Mary.

> Question: Recite the next line after: He was conceived by the Holy Spirit and born of the Virgin Mary.
> Answer: He suffered under Pontius Pilate, was crucified, died, and was buried.
> Distractors: 
> Grounding: He suffered under Pontius Pilate, was crucified, died, and was buried.

> Question: Recite the next line after: He suffered under Pontius Pilate, was crucified, died, and was buried.
> Answer: On the third day he rose again.
> Distractors: 
> Grounding: On the third day he rose again.

> Question: Recite the next line after: On the third day he rose again.
> Answer: He ascended into heaven and is seated at the right hand of the Father.
> Distractors: 
> Grounding: He ascended into heaven and is seated at the right hand of the Father.

### us-presidents-ordinal

### source-injection-caching

> Question: Which HTTP directive allows a cache to store a response but requires revalidation before each reuse?
> Answer: no-cache
> Distractors: no-store | must-revalidate | no-transform
> Grounding: The no-cache directive allows storage but requires revalidation before each reuse.

> Question: Which HTTP directive completely forbids storing a response in a cache?
> Answer: no-store
> Distractors: no-cache | private | immutable
> Grounding: The no-store directive forbids storing a response.

> Question: What validator is used in HTTP conditional requests to check if cached content is still valid?
> Answer: ETag
> Distractors: Last-Modified | Vary | Age
> Grounding: An ETag is a validator used in conditional requests.

### topic-photosynthesis

> Question: Which plant cell organelle is the site of photosynthesis?
> Answer: Chloroplast
> Distractors: Mitochondrion | Peroxisome | Vacuole
> Grounding: Model-expanded; no source evidence

> Question: In which part of the chloroplast do the light-dependent reactions occur?
> Answer: Thylakoid membrane
> Distractors: Stroma | Outer chloroplast membrane | Cristae
> Grounding: Model-expanded; no source evidence

> Question: Where does the Calvin cycle (light-independent reactions) take place within the chloroplast?
> Answer: Stroma
> Distractors: Thylakoid lumen | Intermembrane space | Inner chloroplast membrane
> Grounding: Model-expanded; no source evidence

> Question: Which molecule is split during the light-dependent reactions to release oxygen gas as a byproduct?
> Answer: Water (H₂O)
> Distractors: Carbon dioxide (CO₂) | Glucose (C₆H₁₂O₆) | ATP
> Grounding: Model-expanded; no source evidence

> Question: Which enzyme catalyzes the initial fixation of carbon dioxide to ribulose 1,5-bisphosphate in the Calvin cycle?
> Answer: RuBisCO
> Distractors: ATP synthase | PEP carboxylase | Phosphofructokinase
> Grounding: Model-expanded; no source evidence

### qualified-observational-study

### conditional-boiling-point

> Question: Under what pressure condition does a liquid boil?
> Answer: When vapor pressure equals external pressure
> Distractors: When vapor pressure exceeds atmospheric density | When external pressure drops to zero | When vapor pressure is half of external pressure
> Grounding: Boiling occurs when the liquid's vapor pressure equals the external pressure.

> Question: How does a decrease in external pressure affect the boiling point of pure water?
> Answer: It lowers the boiling point
> Distractors: It raises the boiling point | It keeps the boiling point constant
> Grounding: Lower external pressure lowers its boiling point.

> Question: At what external pressure does pure water boil at exactly 100 degrees Celsius?
> Answer: 101.325 kilopascals
> Distractors: 100.000 kilopascals | 10.1325 kilopascals | 1013.25 kilopascals
> Grounding: Pure water boils at 100 degrees Celsius at standard atmospheric pressure of 101.325 kilopascals.


## Reference quality

Mechanical probes are necessary, not sufficient: factual accuracy, example usefulness, and pedagogical depth remain **human-unassessed** until blinded review. The fake provider is deliberately not a quality baseline. Quotes are checked against actual authorized source bodies; topic expansion must not claim quotations.

| reference | words explanation/example/distinctions | explanation | example | distinctions | retrieval | provenance | forbidden-free | pass | tokens in/out | cost | latency |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| http-caching | 88/38/38 | 67% | 100% | 100% | true | true | true | 1 | 891/694 | $0.0033 | 4921ms |
| pythagorean | 78/28/30 | 100% | 100% | 100% | true | true | true | 1 | 877/462 | $0.0024 | 3917ms |
| source-injection-caching | 68/31/43 | 100% | 50% | 100% | true | true | true | 1 | 849/524 | $0.0026 | 4824ms |
| topic-photosynthesis | 82/30/43 | 100% | 0% | 50% | true | true | true | 0 | 746/477 | $0.0023 | 4304ms |
| qualified-observational-study | 73/26/30 | 67% | 0% | 50% | true | true | true | 0 | 813/420 | $0.0022 | 5262ms |
| conditional-boiling-point | 69/30/42 | 100% | 50% | 50% | true | true | true | 1 | 821/612 | $0.0029 | 5554ms |

- Reference reported cost subtotal: $0.0157 · unreported/uncertain: 0/6 calls. Failed responses retain reported usage; zero tokens may mean unreported, not free.
- Reference mechanical pass: 66.7% (source-clustered n=6, 95% CI ±54.2pp; small-n interval, not a truth guarantee)

### Reference material for blinded human review

Judge each note for factual faithfulness/provenance, explanatory depth, a useful example, an accurate distinction, and a retrieval cue. Compare paired old/new notes with provider labels hidden and order randomized; record keep/revise/reject and the concrete defect. Do not treat this automated pass as human approval.

#### http-caching

> Source-informed model explanation. The source quotations below are verified; the explanation and example are generated study aids, not quotations.
> 
> Explanation
> HTTP caching separates the permission to store a response from the permission to reuse it without verification. Under the Cache-Control directive no-cache, intermediate caches and browsers are explicitly allowed to store the response payload locally. However, before that cached representation can be served to satisfy a subsequent request, the cache must perform revalidation with the origin server via conditional request headers like If-None-Match or If-Modified-Since. If the resource is unchanged, the server returns status 304 Not Modified, allowing the client to reuse the cached body without re-downloading it.
> 
> Example
> A browser caches an HTML document marked Cache-Control: no-cache alongside an ETag; on the next page view, it sends an If-None-Match request to the origin server, which responds with 304 Not Modified to authorize reusing the stored copy.
> 
> Key distinctions
> - The no-cache directive permits storing the response but mandates revalidation before reuse, whereas no-store completely disallows storing any copy.
> - Max-age permits serving cached responses immediately while fresh, whereas no-cache always forces a conditional request check first.
> 
> Retrieval cue
> What step must a client or intermediary cache perform with the origin server before reusing a response marked with no-cache?
> 
> Source context
> HTTP caching headers [http-caching]:
> The no-store directive forbids caching entirely, while no-cache allows storage but requires revalidation before each reuse.
> 
> HTTP caching headers [http-caching]:
> Revalidation uses conditional requests: the client sends If-None-Match with the ETag it holds, and the server answers 304 Not Modified when the representation is unchanged, saving the body transfer.

#### pythagorean

> Source-informed model explanation. The source quotations below are verified; the explanation and example are generated study aids, not quotations.
> 
> Explanation
> In Euclidean geometry, a right triangle contains exactly one 90-degree angle formed by two perpendicular sides called legs. The side located directly across from this right angle is designated as the hypotenuse. Because the right angle is the largest angle possible in a right triangle, the side opposite to it is invariably the longest side of the triangle, satisfying the algebraic relationship where the sum of the squares of the legs equals the square of this opposite side.
> 
> Example
> In a right triangle with side lengths measuring 3, 4, and 5 units, the side of length 5 is opposite the 90-degree angle and serves as the hypotenuse.
> 
> Key distinctions
> - The hypotenuse is strictly opposite the 90-degree angle, whereas the legs form the right angle itself.
> - The hypotenuse is always the longest side in any Euclidean right triangle.
> 
> Retrieval cue
> What is the specific geometric term for the longest side located directly across from a right triangle's 90-degree angle?
> 
> Source context
> The Pythagorean theorem [pythagorean]:
> The Pythagorean theorem states that in a right triangle, the square of the hypotenuse equals the sum of the squares of the other two sides: a squared plus b squared equals c squared, where c is the side opposite the right angle.

#### source-injection-caching

> Source-informed model explanation. The source quotations below are verified; the explanation and example are generated study aids, not quotations.
> 
> Explanation
> HTTP caching distinguishes between storing a response and reusing it directly without server communication. Under the no-cache directive, intermediaries and clients are allowed to retain a copy in local cache storage, but they must perform revalidation with the origin server before every reuse. This mechanism confirms that the stored representation is still fresh, preventing stale data delivery while saving bandwidth via conditional status checks like 304 Not Modified.
> 
> Example
> When a browser saves a profile page served with Cache-Control: no-cache, it stores the asset locally but transmits an If-None-Match conditional request with the stored ETag before rendering that cached version.
> 
> Key distinctions
> - The no-cache directive permits saving the response locally while mandating verification before use, whereas no-store completely forbids any persistence in cache storage.
> - Revalidation sends conditional headers like ETag to check freshness, which differs from unconditionally fetching the full resource from scratch.
> 
> Retrieval cue
> What step must a client cache perform with the origin server before serving a response marked with no-cache?
> 
> Source context
> HTTP reuse notes [source-injection-caching]:
> The no-cache directive allows storage but requires revalidation before each reuse.

#### topic-photosynthesis

> Model-expanded study material. The captured input is a topic seed, not evidence for these factual claims. Check important claims against an authoritative reference.
> 
> Explanation
> Photosynthesis is the biological process by which autotrophic cells convert solar energy into chemical energy stored in carbohydrates. In plant cells and green algae, this complex pathway is compartmentalized within double-membraned organelles called chloroplasts. Chloroplasts contain internal membrane-bound sacs called thylakoids, stacked into grana, where chlorophyll and other pigments absorb light energy for the light-dependent reactions. The fluid matrix surrounding the thylakoids, known as the stroma, houses the enzymes responsible for fixing carbon dioxide into sugars during the light-independent reactions (Calvin cycle).
> 
> Example
> In a spinach leaf mesophyll cell, photons strike chlorophyll within the chloroplast thylakoid membranes to generate ATP and NADPH, which power the Calvin cycle in the stroma to produce glucose.
> 
> Key distinctions
> - Chloroplasts synthesize sugars using light energy, whereas mitochondria break down organic molecules to release ATP during cellular respiration.
> - Chloroplasts are specialized plastids found in photoautotrophic eukaryotes, unlike cell walls or vacuoles which provide structure and storage rather than metabolic energy conversion.
> 
> Retrieval cue
> Which green, double-membraned organelle contains chlorophyll and the biochemical machinery to convert light energy and carbon dioxide into glucose in plant cells?

#### qualified-observational-study

> Source-informed model explanation. The source quotations below are verified; the explanation and example are generated study aids, not quotations.
> 
> Explanation
> Observational research identifies statistical relationships between variables without actively manipulating the conditions under study. Because participants select their own behaviors or possess baseline differences, unmeasured confounding factors can influence the results. Consequently, an observed correlation or association indicates a pattern of co-occurrence but cannot establish a true causal link. Demonstrating causality requires controlled experimental methods, such as random assignment, which help rule out confounding variables and isolate the specific effect of an intervention.
> 
> Example
> Observing that people who own running shoes report lower resting heart rates shows a correlation, but it does not prove buying shoes directly improves cardiovascular fitness.
> 
> Key distinctions
> - Association shows variables moving together, whereas causation proves one directly produces changes in another.
> - Observational designs record natural behaviors, while randomized experiments manipulate variables to isolate direct effects.
> 
> Retrieval cue
> Why does an observational correlation fail to prove that one variable causes another?
> 
> Source context
> Reading an observational pilot study [qualified-observational-study]:
> The researchers did not randomly assign practice frequency, so the association cannot establish causation.

#### conditional-boiling-point

> Source-informed model explanation. The source quotations below are verified; the explanation and example are generated study aids, not quotations.
> 
> Explanation
> Boiling is a phase transition where liquid converts to vapor throughout the bulk liquid rather than just at the surface. Molecules in a liquid possess kinetic energy and exert an outward vapor pressure that increases as temperature rises. For vapor bubbles to form and expand within the liquid without collapsing, this internal vapor pressure must match or exceed the surrounding external pressure acting on the surface of the liquid.
> 
> Example
> At high altitudes where atmospheric pressure drops below 101.3 kilopascals, water boils at temperatures below 100 degrees Celsius because its vapor pressure matches the reduced surrounding air pressure more easily.
> 
> Key distinctions
> - Boiling point varies with surroundings, whereas standard boiling point specifically refers to pressure at exactly one atmosphere.
> - Evaporation occurs only at the liquid surface at any temperature, whereas boiling occurs throughout the bulk liquid once vapor pressure matches external pressure.
> 
> Retrieval cue
> What surrounding condition determines the exact vapor pressure a liquid must generate to begin boiling?
> 
> Source context
> Conditions on a boiling point [conditional-boiling-point]:
> Boiling occurs when the liquid's vapor pressure equals the external pressure.
> 
> Conditions on a boiling point [conditional-boiling-point]:
> Lower external pressure lowers its boiling point.

