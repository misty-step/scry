# Generation eval receipt

- Provider: openrouter/google/gemini-3.7-flash (prompt-principled) · judge: openai/gpt-5.4
- Corpus: 18 sources

| source | category | accepted | rejected | failures | runtime | provenance | answerable | dup | count-ok | terms | shape | content-kind | content-cover | content-shape | direction | variants | cohesion | self-ref | tokens in/out | cost | latency |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| mitochondria | science-prose | 4 | 1 | 0 | 80% | 100% | 100% | 0% | yes | 100% | NO | yes | 0/0 (100%) | NO | yes | 0% | 100% | 100% | 2781/1474 | $0.0076 | 11345ms |
| nato-alphabet | enumerable-list | 26 | 0 | 0 | 100% | 100% | 100% | 0% | yes | 100% | yes | yes | 26/26 (100%) | yes | yes | 0% | 100% | 100% | 1405/3119 | $0.0127 | 15331ms |
| http-caching | technical-doc | 5 | 1 | 0 | 83% | 100% | 100% | 0% | yes | 75% | yes | N/A | N/A | N/A | N/A | 0% | 100% | 100% | 2847/1973 | $0.0095 | 13568ms |
| rubicon | narrative-history | 6 | 1 | 0 | 86% | 100% | 100% | 0% | yes | 100% | yes | N/A | N/A | N/A | N/A | 0% | 100% | 100% | 2793/2140 | $0.0101 | 14163ms |
| sourdough | how-to | 5 | 2 | 0 | 71% | 100% | 100% | 0% | yes | 33% | NO | N/A | N/A | N/A | N/A | 0% | 100% | 100% | 2839/2184 | $0.0103 | 14455ms |
| gdpr-basis | regulatory | 4 | 0 | 0 | 100% | 100% | 100% | 0% | yes | 100% | yes | N/A | N/A | N/A | N/A | 0% | 100% | 100% | 1381/1188 | $0.0055 | 7935ms |
| hope-feathers | verbatim-verse | 8 | 0 | 0 | 100% | 100% | 100% | 0% | yes | 100% | yes | N/A | N/A | N/A | N/A | 0% | 100% | 100% | 1323/1374 | $0.0061 | 8721ms |
| pythagorean | math-concept | 2 | 5 | 0 | 29% | 100% | 100% | 0% | yes | 0% | yes | N/A | N/A | N/A | N/A | 0% | 100% | 100% | 2959/2780 | $0.0126 | 17580ms |
| curie | biography | 5 | 0 | 0 | 100% | 100% | 100% | 0% | yes | 75% | yes | N/A | N/A | N/A | N/A | 0% | 100% | 100% | 1379/1162 | $0.0054 | 7164ms |
| git-branching | product-doc | 5 | 0 | 0 | 100% | 100% | 100% | 0% | yes | 100% | yes | N/A | N/A | N/A | N/A | 0% | 100% | 100% | 1370/1337 | $0.0060 | 8641ms |
| spacing-effect | long-science-prose | 5 | 0 | 0 | 100% | 100% | 100% | 0% | yes | 50% | yes | N/A | N/A | N/A | N/A | 0% | 100% | 100% | 1568/966 | $0.0048 | 7325ms |
| water-boiling | tiny-fact | 3 | 3 | 0 | 50% | 100% | 100% | 0% | yes | 100% | NO | N/A | N/A | N/A | N/A | 0% | 100% | 100% | 2758/1977 | $0.0095 | 13083ms |
| apostles-creed | verbatim-sequential | 6 | 0 | 0 | 100% | 100% | 100% | 0% | yes | 100% | yes | yes | 6/6 (100%) | yes | yes | 0% | 100% | 100% | 1334/957 | $0.0046 | 5944ms |
| us-presidents-ordinal | enumerable-ordinal | 47 | 0 | 0 | 100% | 100% | 100% | 0% | yes | 100% | NO | N/A | N/A | N/A | N/A | 0% | 100% | 100% | 2120/5189 | $0.0210 | 25374ms |
| source-injection-caching | adversarial-source-injection | 3 | 0 | 0 | 100% | 100% | 100% | 0% | yes | 100% | yes | N/A | N/A | N/A | N/A | 0% | 100% | 100% | 1350/858 | $0.0042 | 6027ms |
| topic-photosynthesis | topic-seed | 5 | 0 | 0 | 100% | 0% | 0% | 0% | yes | 100% | yes | N/A | N/A | N/A | N/A | 0% | 100% | 100% | 1249/758 | $0.0038 | 6029ms |
| qualified-observational-study | adversarial-unsupported-claims | 3 | 0 | 0 | 100% | 100% | 100% | 0% | yes | 33% | yes | N/A | N/A | N/A | N/A | 0% | 100% | 100% | 1318/837 | $0.0041 | 6722ms |
| conditional-boiling-point | adversarial-conditions-and-distractors | 3 | 0 | 0 | 100% | 100% | 100% | 0% | yes | 100% | yes | N/A | N/A | N/A | N/A | 0% | 100% | 100% | 1322/927 | $0.0045 | 6344ms |

## Enumerable-set completeness

| source | expected | observed | covered | missing | duplicate | invented | misassigned | reversed | order | direction | pass |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- | --- | --- |
| us-presidents-ordinal | 47 | 47 | 47 | 0 | 0 | 0 | 0 | 0 | yes | yes | yes |

## Model judge (rubric 1-5)

| source | faithfulness | question quality | distractors | keep | judge cost |
| --- | --- | --- | --- | --- | --- |
| mitochondria | 5.0 | 4.8 | 4.2 | 100% | $0.0059 |
| nato-alphabet | 5.0 | 5.0 | 5.0 | 100% | $0.0195 |
| http-caching | 5.0 | 5.0 | 4.6 | 100% | $0.0071 |
| rubicon | 5.0 | 4.8 | 5.0 | 83% | $0.0071 |
| sourdough | 5.0 | 5.0 | 4.4 | 100% | $0.0066 |
| gdpr-basis | 5.0 | 4.8 | 4.8 | 100% | $0.0059 |
| hope-feathers | 5.0 | 4.0 | 5.0 | 100% | $0.0080 |
| pythagorean | 4.5 | 4.0 | 3.5 | 50% | $0.0045 |
| curie | 5.0 | 4.8 | 3.8 | 100% | $0.0069 |
| git-branching | 4.6 | 4.0 | 3.6 | 60% | $0.0068 |
| spacing-effect | 5.0 | 4.8 | 4.6 | 100% | $0.0066 |
| water-boiling | 5.0 | 5.0 | 4.7 | 100% | $0.0047 |
| apostles-creed | 5.0 | 4.2 | 5.0 | 100% | $0.0069 |
| us-presidents-ordinal | 5.0 | 4.0 | 5.0 | 100% | $0.0406 |
| source-injection-caching | 5.0 | 5.0 | 4.3 | 100% | $0.0046 |
| topic-photosynthesis | 4.8 | 4.8 | 4.8 | 100% | $0.0057 |
| qualified-observational-study | 5.0 | 5.0 | 3.7 | 100% | $0.0047 |
| conditional-boiling-point | 5.0 | 5.0 | 3.3 | 67% | $0.0046 |

- Judge means: faithfulness 4.9 · question quality 4.7 · distractors 4.4 · keep rate 92% · judge cost $0.1567
- Keep rate (source-clustered, n=18): 92% (95% CI ±8pp)
- Power: ~18 source clusters resolves only large regressions. Small changes need more paired sources and repeated runs; the exact sample requirement depends on observed variance, not a universal draft-count rule.

Judge would not keep:

- rubicon: draft 6: Accurate, but weakest aspect is redundancy: it duplicates index 1 with the same fact in a weaker format.
- pythagorean: draft 1: Weakest aspect: distractors are uneven and partly silly; one option about multiple 90-degree angles is definitionally absurd. Also the answer is slightly imprecise because the source says the relation fails on curved surfaces/on a sphere, not that angle sum alone is the full reason.
- git-branching: draft 2: Weakest aspect is faithfulness/question wording: HEAD tracks the current checkout, not always strictly a branch; distractors are also alias-like refs from the same namespace.
- git-branching: draft 5: Weakest aspect is answer imprecision: it states what rebase does, but not explicitly that hashes change because replaying creates new commits atop a different base.
- conditional-boiling-point: draft 1: Weakest aspect: distractors are implausible or off-target; critical pressure and zero surface tension are unrelated to the source condition for boiling.

## Totals

- Provider failures: 0/18 sources
- Mean provenance: 94% · mean answerability: 94% · mean key-term coverage: 81% · count-in-range: 18/18
- Intent shape matches: 14/18 sources
- Content fit matches: 2/3 sources · mean required-unit coverage 100%
- Bridge fixture: FAILED: The model provider's response could not be read.
- Quiz reported cost subtotal: $0.1426 · unreported/uncertain: 0/18 sources. This is not a total when any usage is missing; zero tokens can mean unreported, not free.
- Latency p50: 8641ms · p95: 25374ms

## Provenance and adversarial oracles

Quote presence verifies attribution, not factual entailment. Model-expanded topic facts have no source evidence and require human review. Key-term coverage excludes distractors. Failures and empty output are not perfect acceptance.

| source | source-supported | model-expanded | expected grounding | forbidden claims absent |
| --- | --- | --- | --- | --- |
| mitochondria | 4 | 0 | unassessed | unassessed |
| nato-alphabet | 26 | 0 | unassessed | unassessed |
| http-caching | 5 | 0 | unassessed | unassessed |
| rubicon | 6 | 0 | unassessed | unassessed |
| sourdough | 5 | 0 | unassessed | unassessed |
| gdpr-basis | 4 | 0 | unassessed | unassessed |
| hope-feathers | 8 | 0 | unassessed | unassessed |
| pythagorean | 2 | 0 | unassessed | unassessed |
| curie | 5 | 0 | unassessed | unassessed |
| git-branching | 5 | 0 | unassessed | unassessed |
| spacing-effect | 5 | 0 | unassessed | unassessed |
| water-boiling | 3 | 0 | unassessed | unassessed |
| apostles-creed | 6 | 0 | unassessed | unassessed |
| us-presidents-ordinal | 47 | 0 | unassessed | unassessed |
| source-injection-caching | 3 | 0 | pass | pass |
| topic-photosynthesis | 0 | 5 | pass | unassessed |
| qualified-observational-study | 3 | 0 | pass | pass |
| conditional-boiling-point | 3 | 0 | pass | pass |
- Runtime acceptance: 88.8% (source-clustered n=18, 95% CI ±10.1pp; small-n interval, not a truth guarantee)

## Accepted quiz material for blinded human review

Human quality is unassessed unless a calibrated review is recorded. Inspect atomicity, supported claims, conditions, answer leakage, plausible mutually exclusive distractors, and retrieval depth. Compare paired sources with labels hidden and order randomized.

### mitochondria

> Question: Which molecule do mitochondria primarily generate to supply chemical energy to the cell?
> Answer: Adenosine triphosphate (ATP)
> Distractors: Ribonucleic acid (RNA) | Guanosine diphosphate (GDP) | Nicotinamide adenine dinucleotide (NADH)
> Grounding: Mitochondria are organelles that generate most of the cell's supply of adenosine triphosphate, the molecule cells use as chemical energy.

> Question: Which human cell type contains no mitochondria?
> Answer: Red blood cells
> Distractors: Liver cells | Muscle cells | Neuron cells
> Grounding: The number of mitochondria in a cell varies widely: red blood cells have none, while liver cells can contain more than a thousand.

> Question: What is the biological term for programmed cell death regulated in part by mitochondria?
> Answer: Apoptosis
> Distractors: 
> Grounding: Mitochondria also help regulate programmed cell death, known as apoptosis.

> Question: Which theory proposes that mitochondria originated from ancestral cells absorbing free-living bacteria?
> Answer: Endosymbiotic theory
> Distractors: RNA world hypothesis | Invagination hypothesis | Panspermia theory
> Grounding: evidence that they descend from free-living bacteria absorbed by an ancestral cell — the endosymbiotic theory.

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

> Question: Which HTTP Cache-Control directive completely prohibits caches from storing the response?
> Answer: no-store
> Distractors: no-cache | must-revalidate | max-age=0
> Grounding: The no-store directive forbids caching entirely

> Question: When revalidating a cached response using an ETag, which HTTP request header does the client send?
> Answer: If-None-Match
> Distractors: If-Match | If-Modified-Since | ETag
> Grounding: the client sends If-None-Match with the ETag it holds

> Question: Which HTTP status code does a server return during revalidation to indicate the representation is unchanged, avoiding body retransmission?
> Answer: 304 Not Modified
> Distractors: 204 No Content | 200 OK | 412 Precondition Failed
> Grounding: the server answers 304 Not Modified when the representation is unchanged, saving the body transfer.

> Question: What role does the Vary response header serve in HTTP caching?
> Answer: Specifies which request headers participate in the cache key
> Distractors: Sets the maximum duration a response remains fresh | Lists which proxies are allowed to cache the response | Instructs the client to purge all cached variants
> Grounding: The Vary header tells caches which request headers participate in the cache key

> Question: Under HTTP Cache-Control, what requirement does the no-cache directive impose before a cache reuses a stored response?
> Answer: Revalidate before each reuse
> Distractors: Forbid storing the response entirely | Serve stored responses without revalidation until max-age expires | Strip conditional request headers
> Grounding: no-cache allows storage but requires revalidation before each reuse

### rubicon

> Question: Which military unit did Julius Caesar lead across the Rubicon in January of 49 BC?
> Answer: The Thirteenth Legion
> Distractors: The Tenth Legion | The Ninth Legion | The Sixth Legion
> Grounding: Julius Caesar led the Thirteenth Legion across the Rubicon

> Question: Under Roman law, what specific crime was committed when a general brought an army into Italy proper?
> Answer: Treason
> Distractors: 
> Grounding: crossing under arms was treason and made civil war inevitable.

> Question: According to Suetonius, which phrase translated as "the die is cast" did Julius Caesar say upon crossing the Rubicon?
> Answer: Alea iacta est
> Distractors: Veni, vidi, vici | Et tu, Brute | Carthago delenda est
> Grounding: Suetonius reports that Caesar said "alea iacta est" — the die is cast.

> Question: To which southern port city did Pompey and senators retreat before crossing the Adriatic Sea in 49 BC?
> Answer: Brundisium
> Distractors: 
> Grounding: Pompey and much of the Senate abandoned Rome rather than face him, retreating south to Brundisium and then across the Adriatic.

> Question: How long after crossing the Rubicon did it take Julius Caesar to control the entire Italian peninsula?
> Answer: Within three months
> Distractors: Within one year | Within three weeks | Within six months
> Grounding: Within three months Caesar controlled the whole Italian peninsula without a major battle.

> Question: Which Roman legion did Julius Caesar lead across the Rubicon in 49 BC?
> Answer: The Thirteenth Legion
> Distractors: 
> Grounding: Julius Caesar led the Thirteenth Legion across the Rubicon

### sourdough

> Question: Which two types of microorganisms comprise a sourdough starter culture?
> Answer: Wild yeast and lactic acid bacteria
> Distractors: 
> Grounding: A sourdough starter is a stable culture of wild yeast and lactic acid bacteria living in a paste of flour and water.

> Question: Within what time frame after feeding should a healthy sourdough starter double in volume?
> Answer: 4 to 8 hours
> Distractors: 1 to 2 hours | 12 to 16 hours | 20 to 24 hours
> Grounding: A healthy starter doubles in volume within 4 to 8 hours of feeding

> Question: What smell indicates that a sourdough starter is underfed?
> Answer: Acetone or nail polish remover
> Distractors: Rotten eggs or sulfur | Sweet vanilla | Fresh cut grass
> Grounding: If it smells of acetone or nail polish remover, it is underfed: shorten the feeding interval.

> Question: How often does a refrigerated sourdough starter need to be fed?
> Answer: Once per week
> Distractors: Once every 24 hours | Once every 2 to 3 days | Once per month
> Grounding: Refrigeration slows the culture enough that one feeding per week suffices.

> Question: Besides returning to room temperature, what must a refrigerated sourdough starter do before being used for baking?
> Answer: Pass the doubling test
> Distractors: Develop an acetone aroma | Form a surface hooch layer | Triple in total volume
> Grounding: Always let a refrigerated starter return to room temperature and pass the doubling test before baking with it.

### gdpr-basis

> Question: Under Article 6 of the GDPR, how many distinct lawful bases for processing personal data are established?
> Answer: Six
> Distractors: Four | Eight | Twelve
> Grounding: Article 6 of the GDPR makes processing of personal data lawful only when at least one of six bases applies

> Question: What assessment must a controller document when relying on legitimate interest as a lawful basis under the GDPR?
> Answer: A documented balancing test against the data subject's reasonable expectations
> Distractors: A formal security audit certified by a national supervisory authority | A binding contractual agreement signed by all affected data subjects | A statutory clearance notice published in an official public register
> Grounding: Legitimate interest is the most flexible basis but requires a documented balancing test against the data subject's reasonable expectations.

> Question: Under the GDPR, what standard must be met regarding the ease of withdrawing consent?
> Answer: As easy as giving it
> Distractors: 
> Grounding: withdrawing consent must be as easy as giving it

> Question: Under GDPR Article 6, when can a controller's legitimate interests NOT serve as a lawful basis for processing personal data?
> Answer: When overridden by the data subject's rights
> Distractors: When personal data is transferred across borders | When processing involves public interest tasks | When the processing is not performed under a commercial contract
> Grounding: the controller's legitimate interests, except where those are overridden by the data subject's rights

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

> Question: Why does the Pythagorean theorem fail for triangles drawn on the surface of a sphere?
> Answer: The angles of the triangle sum to more than 180 degrees
> Distractors: The angles of the triangle sum to less than 180 degrees | The hypotenuse is always equal to the sum of the other two sides | The triangle contains more than one 90-degree angle by definition
> Grounding: The theorem fails on curved surfaces; on a sphere, the angles of a triangle sum to more than 180 degrees and the relation no longer applies.

> Question: Why does the Pythagorean relation fail for triangles on a sphere?
> Answer: The angles of a triangle sum to more than 180 degrees
> Distractors: The angles of a triangle sum to less than 180 degrees | Right angles cannot be constructed | Curved edges prevent forming closed polygons
> Grounding: on a sphere, the angles of a triangle sum to more than 180 degrees and the relation no longer applies.

### curie

> Question: In which two scientific fields did Marie Curie win Nobel Prizes?
> Answer: Physics and Chemistry
> Distractors: Physics and Medicine | Chemistry and Physiology | Physics and Mathematics
> Grounding: She shared the 1903 Physics prize with Pierre Curie and Henri Becquerel for research on radiation, and won the 1911 Chemistry prize alone

> Question: Which two chemical elements did Marie Curie discover, leading to her 1911 Nobel Prize in Chemistry?
> Answer: Polonium and radium
> Distractors: Uranium and thorium | Actinium and radium | Polonium and francium
> Grounding: won the 1911 Chemistry prize alone for discovering the elements polonium and radium.

> Question: Which scientific term did Marie Curie coin during her research on radiation?
> Answer: Radioactivity
> Distractors: Radioluminescence | Phosphorescence | Ionizing radiation
> Grounding: She coined the term radioactivity.

> Question: What nickname was given to the mobile X-ray vehicles equipped by Marie Curie during World War I?
> Answer: Petites Curies
> Distractors: Ambulances Curie | Radiomobiles | Unités Curie
> Grounding: During the First World War she equipped mobile X-ray units, nicknamed petites Curies, and drove them to the front herself.

> Question: Which specific radioactive isotope contaminates Marie Curie's laboratory notebooks, necessitating their storage in lead-lined boxes?
> Answer: Radium-226
> Distractors: Polonium-210 | Uranium-235 | Thorium-232
> Grounding: Her laboratory notebooks remain so contaminated with radium-226 that they are stored in lead-lined boxes and require protective equipment to consult.

### git-branching

> Question: In Git, what is a branch fundamentally represented as?
> Answer: A movable pointer to a commit
> Distractors: A full snapshot of the repository files | An isolated append-only commit log | A compressed copy of the staging area
> Grounding: A Git branch is a movable pointer to a commit

> Question: In Git, which reference tracks the branch you are currently on?
> Answer: HEAD
> Distractors: FETCH_HEAD | ORIG_HEAD | MERGE_HEAD
> Grounding: The branch you are on is tracked by HEAD.

> Question: Under what condition does Git perform a fast-forward merge by simply moving the branch pointer?
> Answer: The target is a direct descendant
> Distractors: Both branches have diverged with independent commits | The target branch shares no common ancestor | The working directory has uncommitted staged changes
> Grounding: a fast-forward merge simply moves the branch pointer when the target is a direct descendant

> Question: In Git, how many parents does a newly created three-way merge commit have?
> Answer: Two parents
> Distractors: One parent | Three parents | Zero parents
> Grounding: a three-way merge creates a new merge commit with two parents using the common ancestor as the base.

> Question: What mechanism causes Git rebasing to rewrite commit hashes while producing a linear history?
> Answer: It replays commits on top of another branch
> Distractors: It generates a new commit with two parents from a common ancestor | It updates the branch pointer forward without altering commit objects | It discards all unmerged changes from the working tree
> Grounding: Rebasing replays commits on top of another branch, producing a linear history but rewriting commit hashes

### spacing-effect

> Question: What cognitive psychology phenomenon describes superior retention when study sessions are distributed over time rather than concentrated in a single massed session?
> Answer: Spacing effect
> Distractors: 
> Grounding: The spacing effect is one of the most replicated findings in cognitive psychology: information reviewed in sessions spread over time is retained far better than the same information massed into one session.

> Question: What term describes the phenomenon where the act of actively retrieving information from memory strengthens the memory trace more than passive rereading?
> Answer: Testing effect
> Distractors: 
> Grounding: The effort of retrieval itself strengthens the memory trace, a phenomenon called the testing effect.

> Question: What term did Robert Bjork coin for learning conditions that require cognitive effort during recall, thereby producing more durable memory retention?
> Answer: Desirable difficulty
> Distractors: 
> Grounding: Difficulty helps: recall that requires effort produces more durable learning than recall that is easy, which Robert Bjork termed a desirable difficulty.

> Question: Why do spaced repetition schedules use progressively longer intervals between successive reviews?
> Answer: Each successful recall slows the subsequent rate of forgetting
> Distractors: Massed exposure creates stronger initial neural consolidation | Short delays prevent interference from newly acquired memories
> Grounding: Each successful recall after a delay slows subsequent forgetting, which is why review schedules expand

> Question: According to cognitive research on spaced repetition, why does lenient self-grading undermine the benefits of retrieval practice?
> Answer: Recognition is mistaken for recall
> Distractors: Memory traces degrade due to cognitive fatigue | Interval algorithms incorrectly shorten scheduled review gaps
> Grounding: systems that let learners rate their own answers leniently lose much of the testing effect, because recognition masquerades as recall.

### water-boiling

> Question: At sea-level atmospheric pressure, at what temperature does water boil?
> Answer: 100 degrees Celsius
> Distractors: 90 degrees Celsius | 110 degrees Celsius | 120 degrees Celsius
> Grounding: Water boils at 100 degrees Celsius at sea-level atmospheric pressure.

> Question: Why does water boil at a lower temperature at higher altitudes?
> Answer: Atmospheric pressure is lower
> Distractors: Atmospheric pressure is higher | Oxygen concentration is higher | Ambient humidity is lower
> Grounding: At higher altitudes the pressure is lower, so water boils at a lower temperature

> Question: At an altitude of 3,000 meters, what is the approximate boiling point of water?
> Answer: Roughly 90 degrees Celsius
> Distractors: Roughly 80 degrees Celsius | Roughly 100 degrees Celsius | Roughly 70 degrees Celsius
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

> Question: In U.S. presidents by current ordinal, what is the entry for number 1?
> Answer: George Washington
> Distractors: 
> Grounding: 1. George Washington is the 1st president of the United States.

> Question: In U.S. presidents by current ordinal, what is the entry for number 2?
> Answer: John Adams
> Distractors: 
> Grounding: 2. John Adams is the 2nd president of the United States.

> Question: In U.S. presidents by current ordinal, what is the entry for number 3?
> Answer: Thomas Jefferson
> Distractors: 
> Grounding: 3. Thomas Jefferson is the 3rd president of the United States.

> Question: In U.S. presidents by current ordinal, what is the entry for number 4?
> Answer: James Madison
> Distractors: 
> Grounding: 4. James Madison is the 4th president of the United States.

> Question: In U.S. presidents by current ordinal, what is the entry for number 5?
> Answer: James Monroe
> Distractors: 
> Grounding: 5. James Monroe is the 5th president of the United States.

> Question: In U.S. presidents by current ordinal, what is the entry for number 6?
> Answer: John Quincy Adams
> Distractors: 
> Grounding: 6. John Quincy Adams is the 6th president of the United States.

> Question: In U.S. presidents by current ordinal, what is the entry for number 7?
> Answer: Andrew Jackson
> Distractors: 
> Grounding: 7. Andrew Jackson is the 7th president of the United States.

> Question: In U.S. presidents by current ordinal, what is the entry for number 8?
> Answer: Martin Van Buren
> Distractors: 
> Grounding: 8. Martin Van Buren is the 8th president of the United States.

> Question: In U.S. presidents by current ordinal, what is the entry for number 9?
> Answer: William Henry Harrison
> Distractors: 
> Grounding: 9. William Henry Harrison is the 9th president of the United States.

> Question: In U.S. presidents by current ordinal, what is the entry for number 10?
> Answer: John Tyler
> Distractors: 
> Grounding: 10. John Tyler is the 10th president of the United States.

> Question: In U.S. presidents by current ordinal, what is the entry for number 11?
> Answer: James K. Polk
> Distractors: 
> Grounding: 11. James K. Polk is the 11th president of the United States.

> Question: In U.S. presidents by current ordinal, what is the entry for number 12?
> Answer: Zachary Taylor
> Distractors: 
> Grounding: 12. Zachary Taylor is the 12th president of the United States.

> Question: In U.S. presidents by current ordinal, what is the entry for number 13?
> Answer: Millard Fillmore
> Distractors: 
> Grounding: 13. Millard Fillmore is the 13th president of the United States.

> Question: In U.S. presidents by current ordinal, what is the entry for number 14?
> Answer: Franklin Pierce
> Distractors: 
> Grounding: 14. Franklin Pierce is the 14th president of the United States.

> Question: In U.S. presidents by current ordinal, what is the entry for number 15?
> Answer: James Buchanan
> Distractors: 
> Grounding: 15. James Buchanan is the 15th president of the United States.

> Question: In U.S. presidents by current ordinal, what is the entry for number 16?
> Answer: Abraham Lincoln
> Distractors: 
> Grounding: 16. Abraham Lincoln is the 16th president of the United States.

> Question: In U.S. presidents by current ordinal, what is the entry for number 17?
> Answer: Andrew Johnson
> Distractors: 
> Grounding: 17. Andrew Johnson is the 17th president of the United States.

> Question: In U.S. presidents by current ordinal, what is the entry for number 18?
> Answer: Ulysses S. Grant
> Distractors: 
> Grounding: 18. Ulysses S. Grant is the 18th president of the United States.

> Question: In U.S. presidents by current ordinal, what is the entry for number 19?
> Answer: Rutherford B. Hayes
> Distractors: 
> Grounding: 19. Rutherford B. Hayes is the 19th president of the United States.

> Question: In U.S. presidents by current ordinal, what is the entry for number 20?
> Answer: James A. Garfield
> Distractors: 
> Grounding: 20. James A. Garfield is the 20th president of the United States.

> Question: In U.S. presidents by current ordinal, what is the entry for number 21?
> Answer: Chester A. Arthur
> Distractors: 
> Grounding: 21. Chester A. Arthur is the 21st president of the United States.

> Question: In U.S. presidents by current ordinal, what is the entry for number 22?
> Answer: Grover Cleveland
> Distractors: 
> Grounding: 22. Grover Cleveland is the 22nd president of the United States.

> Question: In U.S. presidents by current ordinal, what is the entry for number 23?
> Answer: Benjamin Harrison
> Distractors: 
> Grounding: 23. Benjamin Harrison is the 23rd president of the United States.

> Question: In U.S. presidents by current ordinal, what is the entry for number 24?
> Answer: Grover Cleveland
> Distractors: 
> Grounding: 24. Grover Cleveland is the 24th president of the United States.

> Question: In U.S. presidents by current ordinal, what is the entry for number 25?
> Answer: William McKinley
> Distractors: 
> Grounding: 25. William McKinley is the 25th president of the United States.

> Question: In U.S. presidents by current ordinal, what is the entry for number 26?
> Answer: Theodore Roosevelt
> Distractors: 
> Grounding: 26. Theodore Roosevelt is the 26th president of the United States.

> Question: In U.S. presidents by current ordinal, what is the entry for number 27?
> Answer: William Howard Taft
> Distractors: 
> Grounding: 27. William Howard Taft is the 27th president of the United States.

> Question: In U.S. presidents by current ordinal, what is the entry for number 28?
> Answer: Woodrow Wilson
> Distractors: 
> Grounding: 28. Woodrow Wilson is the 28th president of the United States.

> Question: In U.S. presidents by current ordinal, what is the entry for number 29?
> Answer: Warren G. Harding
> Distractors: 
> Grounding: 29. Warren G. Harding is the 29th president of the United States.

> Question: In U.S. presidents by current ordinal, what is the entry for number 30?
> Answer: Calvin Coolidge
> Distractors: 
> Grounding: 30. Calvin Coolidge is the 30th president of the United States.

> Question: In U.S. presidents by current ordinal, what is the entry for number 31?
> Answer: Herbert Hoover
> Distractors: 
> Grounding: 31. Herbert Hoover is the 31st president of the United States.

> Question: In U.S. presidents by current ordinal, what is the entry for number 32?
> Answer: Franklin D. Roosevelt
> Distractors: 
> Grounding: 32. Franklin D. Roosevelt is the 32nd president of the United States.

> Question: In U.S. presidents by current ordinal, what is the entry for number 33?
> Answer: Harry S. Truman
> Distractors: 
> Grounding: 33. Harry S. Truman is the 33rd president of the United States.

> Question: In U.S. presidents by current ordinal, what is the entry for number 34?
> Answer: Dwight D. Eisenhower
> Distractors: 
> Grounding: 34. Dwight D. Eisenhower is the 34th president of the United States.

> Question: In U.S. presidents by current ordinal, what is the entry for number 35?
> Answer: John F. Kennedy
> Distractors: 
> Grounding: 35. John F. Kennedy is the 35th president of the United States.

> Question: In U.S. presidents by current ordinal, what is the entry for number 36?
> Answer: Lyndon B. Johnson
> Distractors: 
> Grounding: 36. Lyndon B. Johnson is the 36th president of the United States.

> Question: In U.S. presidents by current ordinal, what is the entry for number 37?
> Answer: Richard Nixon
> Distractors: 
> Grounding: 37. Richard Nixon is the 37th president of the United States.

> Question: In U.S. presidents by current ordinal, what is the entry for number 38?
> Answer: Gerald Ford
> Distractors: 
> Grounding: 38. Gerald Ford is the 38th president of the United States.

> Question: In U.S. presidents by current ordinal, what is the entry for number 39?
> Answer: Jimmy Carter
> Distractors: 
> Grounding: 39. Jimmy Carter is the 39th president of the United States.

> Question: In U.S. presidents by current ordinal, what is the entry for number 40?
> Answer: Ronald Reagan
> Distractors: 
> Grounding: 40. Ronald Reagan is the 40th president of the United States.

> Question: In U.S. presidents by current ordinal, what is the entry for number 41?
> Answer: George H. W. Bush
> Distractors: 
> Grounding: 41. George H. W. Bush is the 41st president of the United States.

> Question: In U.S. presidents by current ordinal, what is the entry for number 42?
> Answer: Bill Clinton
> Distractors: 
> Grounding: 42. Bill Clinton is the 42nd president of the United States.

> Question: In U.S. presidents by current ordinal, what is the entry for number 43?
> Answer: George W. Bush
> Distractors: 
> Grounding: 43. George W. Bush is the 43rd president of the United States.

> Question: In U.S. presidents by current ordinal, what is the entry for number 44?
> Answer: Barack Obama
> Distractors: 
> Grounding: 44. Barack Obama is the 44th president of the United States.

> Question: In U.S. presidents by current ordinal, what is the entry for number 45?
> Answer: Donald J. Trump
> Distractors: 
> Grounding: 45. Donald J. Trump is the 45th president of the United States.

> Question: In U.S. presidents by current ordinal, what is the entry for number 46?
> Answer: Joe Biden
> Distractors: 
> Grounding: 46. Joe Biden is the 46th president of the United States.

> Question: In U.S. presidents by current ordinal, what is the entry for number 47?
> Answer: Donald J. Trump
> Distractors: 
> Grounding: 47. Donald J. Trump is the 47th president of the United States.

### source-injection-caching

> Question: In HTTP caching, which directive permits response storage while requiring revalidation before each reuse?
> Answer: no-cache
> Distractors: no-store | must-revalidate | private
> Grounding: The no-cache directive allows storage but requires revalidation before each reuse.

> Question: In HTTP caching, which directive completely forbids storing a response?
> Answer: no-store
> Distractors: no-cache | no-transform | private
> Grounding: The no-store directive forbids storing a response.

> Question: In HTTP conditional requests, which entity serves as a validator for caching?
> Answer: ETag
> Distractors: Vary | Age | Max-Age
> Grounding: An ETag is a validator used in conditional requests.

### topic-photosynthesis

> Question: Which plant cell organelle is the site where photosynthesis takes place?
> Answer: Chloroplast
> Distractors: Mitochondrion | Ribosome
> Grounding: Model-expanded; no source evidence

> Question: What is the primary green pigment in plants that absorbs light energy for photosynthesis?
> Answer: Chlorophyll
> Distractors: Carotenoid | Anthocyanin
> Grounding: Model-expanded; no source evidence

> Question: Which gas is released as a byproduct during the light-dependent reactions of photosynthesis?
> Answer: Oxygen
> Distractors: Carbon dioxide | Nitrogen
> Grounding: Model-expanded; no source evidence

> Question: What six-carbon sugar is the primary organic energy storage molecule synthesized by photosynthesis?
> Answer: Glucose
> Distractors: 
> Grounding: Model-expanded; no source evidence

> Question: What is the name of the light-independent stage of photosynthesis where carbon dioxide is fixed into carbohydrates?
> Answer: Calvin cycle
> Distractors: Krebs cycle | Glycolysis
> Grounding: Model-expanded; no source evidence

### qualified-observational-study

> Question: Why does an observational study linking frequent retrieval practice to better delayed recall fail to establish a causal relationship?
> Answer: Practice frequency was not randomly assigned
> Distractors: Delayed recall cannot be measured quantitatively | The study lacked an observational baseline | Participants were evaluated immediately rather than delayed
> Grounding: The researchers did not randomly assign practice frequency, so the association cannot establish causation.

> Question: What research design is required to test whether retrieval practice directly causes improvements in delayed recall?
> Answer: A randomized study
> Distractors: A qualitative case series | A larger observational cohort study | A cross-sectional survey
> Grounding: A larger randomized study would be needed to test a causal explanation.

> Question: In the 24-adult observational pilot study, what outcome was associated with more frequent retrieval practice?
> Answer: Better delayed recall
> Distractors: Faster initial learning speed | Lower testing anxiety | Higher immediate recall accuracy
> Grounding: More frequent retrieval practice was associated with better delayed recall in this sample.

### conditional-boiling-point

> Question: At what point relative to pressure does a liquid boil?
> Answer: When vapor pressure equals external pressure
> Distractors: When vapor pressure exceeds critical pressure | When liquid density matches vapor density | When surface tension drops to zero
> Grounding: Boiling occurs when the liquid's vapor pressure equals the external pressure.

> Question: How does decreasing the external pressure affect the boiling point of pure water?
> Answer: Lowers the boiling point
> Distractors: Raises the boiling point | Has no effect on the boiling point
> Grounding: Lower external pressure lowers its boiling point.

> Question: At what temperature does pure water boil when under a standard atmospheric pressure of 101.325 kilopascals?
> Answer: 100 degrees Celsius
> Distractors: 80 degrees Celsius | 120 degrees Celsius | 150 degrees Celsius
> Grounding: Pure water boils at 100 degrees Celsius at standard atmospheric pressure of 101.325 kilopascals.


## Reference quality

Mechanical probes are necessary, not sufficient: factual accuracy, example usefulness, and pedagogical depth remain **human-unassessed** until blinded review. The fake provider is deliberately not a quality baseline. Quotes are checked against actual authorized source bodies; topic expansion must not claim quotations.

| reference | words explanation/example/distinctions | explanation | example | distinctions | retrieval | provenance | forbidden-free | pass | tokens in/out | cost | latency |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| http-caching | 76/35/44 | 67% | 50% | 100% | true | true | true | 1 | 946/521 | $0.0027 | 6185ms |
| pythagorean | 74/28/36 | 100% | 100% | 100% | true | true | true | 1 | 932/606 | $0.0030 | 4534ms |
| source-injection-caching | 84/36/41 | 100% | 100% | 100% | true | true | true | 1 | 904/553 | $0.0028 | 4802ms |
| topic-photosynthesis | 68/30/35 | 100% | 50% | 50% | true | true | true | 1 | 801/720 | $0.0033 | 6001ms |
| qualified-observational-study | 50/35/41 | 100% | 0% | 50% | true | true | true | 0 | 868/379 | $0.0021 | 4057ms |
| conditional-boiling-point | 67/23/41 | 100% | 100% | 50% | true | true | true | 1 | 876/817 | $0.0037 | 6820ms |

- Reference reported cost subtotal: $0.0175 · unreported/uncertain: 0/6 calls. Failed responses retain reported usage; zero tokens may mean unreported, not free.
- Reference mechanical pass: 83.3% (source-clustered n=6, 95% CI ±42.9pp; small-n interval, not a truth guarantee)

### Reference material for blinded human review

Judge each note for factual faithfulness/provenance, explanatory depth, a useful example, an accurate distinction, and a retrieval cue. Compare paired old/new notes with provider labels hidden and order randomized; record keep/revise/reject and the concrete defect. Do not treat this automated pass as human approval.

#### http-caching

> Source-informed model explanation. The source quotations below are verified; the explanation and example are generated study aids, not quotations.
> 
> Explanation
> In HTTP caching mechanics, the Cache-Control header dictates how intermediate and client caches retain and serve representations. The no-cache directive does not forbid storing the response; rather, it instructs caches that they cannot serve the stored copy without first checking with the origin server via revalidation. This process typically uses conditional request headers, such as If-None-Match with ETags or If-Modified-Since with timestamps, allowing servers to return a lightweight 304 Not Modified status when content is unchanged.
> 
> Example
> A browser saves an API response marked with Cache-Control: no-cache and an ETag header; on the next fetch, it sends an If-None-Match header to the origin server to verify freshness before reusing the cached content.
> 
> Key distinctions
> - The no-cache directive permits storing responses but forces revalidation before reuse, whereas no-store completely disallows storing responses on disk or in memory.
> - Freshness directives like max-age permit serving cached responses directly while valid, whereas no-cache requires a conditional request every single time.
> 
> Retrieval cue
> What mandatory verification step does Cache-Control: no-cache require before a client or intermediate cache can serve a stored response?
> 
> Source context
> HTTP caching headers [http-caching]:
> while no-cache allows storage but requires revalidation before each reuse.
> 
> HTTP caching headers [http-caching]:
> The no-store directive forbids caching entirely, while no-cache allows storage but requires revalidation before each reuse.

#### pythagorean

> Source-informed model explanation. The source quotations below are verified; the explanation and example are generated study aids, not quotations.
> 
> Explanation
> A right triangle consists of two perpendicular sides known as legs and one opposing side known as the hypotenuse. The hypotenuse is defined specifically as the side directly opposite the right angle. Because the right angle is the largest interior angle in any Euclidean right triangle, the hypotenuse is always the longest side. In the standard Pythagorean formulation where a squared plus b squared equals c squared, the variable c represents this opposite side.
> 
> Example
> In a right triangle with side lengths 3, 4, and 5, the side of length 5 lies directly across from the 90-degree angle and serves as the hypotenuse.
> 
> Key distinctions
> - The hypotenuse is always opposite the 90-degree angle, while the legs form the right angle itself.
> - The hypotenuse is strictly the longest side of a right triangle in Euclidean geometry, unlike the two legs.
> 
> Retrieval cue
> What is the specific name of the longest side located directly opposite the 90-degree angle in a right triangle?
> 
> Source context
> The Pythagorean theorem [pythagorean]:
> The Pythagorean theorem states that in a right triangle, the square of the hypotenuse equals the sum of the squares of the other two sides: a squared plus b squared equals c squared, where c is the side opposite the right angle.

#### source-injection-caching

> Source-informed model explanation. The source quotations below are verified; the explanation and example are generated study aids, not quotations.
> 
> Explanation
> In HTTP caching mechanisms, storage policies are separated from reuse rules. The no-cache directive indicates that a caching intermediary or browser is permitted to save the response payload locally, but it cannot reuse that cached copy to fulfill subsequent requests without first revalidating freshness with the origin server. Typically, this revalidation occurs via conditional request headers like If-None-Match or If-Modified-Since. If the origin confirms the representation remains unchanged by returning a 304 Not Modified status, the cached payload is safely served to the requester.
> 
> Example
> When a client receives a response with Cache-Control: no-cache and an ETag header, the local cache stores the payload but sends a conditional GET request with If-None-Match to the origin server before serving that payload again.
> 
> Key distinctions
> - The no-cache directive allows storage while mandating revalidation before reuse, whereas no-store completely prevents caching or saving response data.
> - Revalidation checks resource freshness with conditional headers, unlike standard reuse which serves stored responses immediately without contacting the origin server.
> 
> Retrieval cue
> What validation process must a cache perform with the origin server before serving a stored response that contains a no-cache directive?
> 
> Source context
> HTTP reuse notes [source-injection-caching]:
> The no-cache directive allows storage but requires revalidation before each reuse.

#### topic-photosynthesis

> Model-expanded study material. The captured input is a topic seed, not evidence for these factual claims. Check important claims against an authoritative reference.
> 
> Explanation
> Photosynthesis is the process by which autotrophic eukaryotes capture light energy and convert it into chemical energy stored in sugars. In plant and algal cells, this pathway takes place within chloroplasts, specialized double-membrane organelles. Inside each chloroplast, disc-like thylakoid membranes contain chlorophyll pigments that absorb light during the light-dependent reactions, while the surrounding fluid matrix, known as the stroma, contains the enzymes required for the light-independent Calvin cycle.
> 
> Example
> In a green leaf cell exposed to sunlight, chlorophyll pigments inside the chloroplast's thylakoid membranes absorb photons to generate ATP and NADPH, which then power glucose synthesis in the stroma.
> 
> Key distinctions
> - Chloroplasts synthesize carbohydrates using light energy, whereas mitochondria break down sugars to produce ATP via cellular respiration.
> - Chlorophyll is a light-absorbing pigment molecule, whereas the chloroplast is the whole organelle containing that pigment.
> 
> Retrieval cue
> Which specialized double-membrane organelle contains chlorophyll and houses the machinery for photosynthesis in plant cells?

#### qualified-observational-study

> Source-informed model explanation. The source quotations below are verified; the explanation and example are generated study aids, not quotations.
> 
> Explanation
> An observational association reveals that two variables change together, but it cannot establish causation. Because observational studies do not randomly assign conditions or isolate confounding variables, observed differences may stem from unmeasured external factors, pre-existing differences among participants, or reverse causality. Determining a causal link requires experimental manipulation and randomization.
> 
> Example
> In a study observing that students who drink more green tea score higher on exams, the correlation cannot prove tea improves test scores because confounding variables like study hours or sleep could explain the link.
> 
> Key distinctions
> - Association indicates a statistical relationship between two variables, whereas causation proves that changing one directly causes a change in the other.
> - Observational designs observe naturally occurring behavior, whereas randomized controlled experiments actively assign interventions to control for confounding factors.
> 
> Retrieval cue
> Why does an observational correlation fail to prove that one variable causes changes in another?
> 
> Source context
> Reading an observational pilot study [qualified-observational-study]:
> The researchers did not randomly assign practice frequency, so the association cannot establish causation.

#### conditional-boiling-point

> Source-informed model explanation. The source quotations below are verified; the explanation and example are generated study aids, not quotations.
> 
> Explanation
> Boiling is a phase transition in which liquid transforms into gas throughout the bulk of the liquid. For vapor bubbles to form, grow, and rise without being crushed by the surrounding environment, the liquid's internal vapor pressure must equal or exceed the prevailing external pressure. Because heating increases thermal energy and raises vapor pressure, changing the external pressure directly alters the temperature needed to reach this equilibrium.
> 
> Example
> At high mountain altitudes where surrounding atmospheric pressure is lower than at sea level, water boils at a temperature below 100 degrees Celsius.
> 
> Key distinctions
> - Boiling occurs throughout the bulk liquid when vapor pressure equals external pressure, whereas evaporation occurs only at the liquid surface at various temperatures.
> - Boiling temperature is variable with atmospheric conditions rather than an absolute invariant property of a liquid.
> 
> Retrieval cue
> What surrounding force must a liquid's vapor pressure match in order for bulk boiling to begin?
> 
> Source context
> Conditions on a boiling point [conditional-boiling-point]:
> Boiling occurs when the liquid's vapor pressure equals the external pressure.
> 
> Conditions on a boiling point [conditional-boiling-point]:
> Lower external pressure lowers its boiling point.

