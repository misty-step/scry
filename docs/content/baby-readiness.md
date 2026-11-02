# Baby care readiness

Parent preparation for newborn and early infancy; individual prompts specify broader infant or age-specific safety scope. US safety sources.

Memory practice, not medical advice or certification of hands-on care. Follow the baby’s clinician for individual needs. Karp is an optional soothing framework; Montessori is an educational approach. Safety guidance takes precedence.

## Content and grading

Existing deterministic keyed mapping. These are deliberately bounded clozes/labels, not open-ended clinical judgment. Case and punctuation normalization permitted; exact tokens expected. A recall grade is not evidence that a caregiver can safely perform a technique.

Original prompts and notes based on factual ideas and brief labels. No book excerpts, page designs, diagrams or commercial promises reproduced. References identify sources, not endorsement.

The three supplied manifests in `examples/baby-*.json` use the existing external mapping import contract. Selecting a lesson creates its Study; browsing never enrolls anyone. Successful recall is not hands-on competency.

## Checked sources

- **karp**: [The 5 S’s for Soothing Babies](https://www.happiestbaby.com/blogs/baby/the-5-s-s-for-soothing-babies) — Happiest Baby / Harvey Karp. Checked 2026-11-02. Originator of optional soothing framework; not the safety authority.

- **sleep**: [How to Keep Your Sleeping Baby Safe: AAP Policy Explained](https://www.healthychildren.org/English/ages-stages/baby/sleep/Pages/a-parents-guide-to-safe-sleep.aspx) — American Academy of Pediatrics. Checked 2026-11-02. Infant sleep safety.

- **swaddle**: [Swaddling: Is it Safe for Your Baby?](https://www.healthychildren.org/English/ages-stages/baby/diapers-clothing/Pages/Swaddling-Is-it-Safe.aspx) — American Academy of Pediatrics. Checked 2026-11-02. Swaddle safety.

- **noise**: [Preventing Excessive Noise Exposure in Infants, Children, and Adolescents](https://publications.aap.org/pediatrics/article/152/5/e2023063752/194392/Preventing-Excessive-Noise-Exposure-in-Infants) — American Academy of Pediatrics. Checked 2026-11-02. Noise safety.

- **feeding**: [Signs Your Child Is Hungry or Full](https://www.cdc.gov/infant-toddler-nutrition/mealtime/signs-your-child-is-hungry-or-full.html) — CDC. Checked 2026-11-02. Infant feeding cues.

- **fever**: [Fever and Your Baby](https://www.healthychildren.org/english/health-issues/conditions/fever/pages/fever-and-your-baby.aspx) — American Academy of Pediatrics. Checked 2026-11-02. Clinical escalation.

- **crying**: [Responding To Your Baby’s Cries](https://www.healthychildren.org/English/ages-stages/baby/crying-colic/Pages/Responding-to-Your-Babys-Cries.aspx) — American Academy of Pediatrics. Checked 2026-11-02. Caregiver coping and safe handling.

- **ami_observe**: [See, Support, Share: Observation and Intervention](https://montessori-ami.org/node/8070) — Association Montessori Internationale. Checked 2026-11-02. Montessori educational approach, not clinical guidance.

- **ams**: [Montessori for Infants & Toddlers](https://amshq.org/about-us/inside-the-montessori-classroom/infant-and-toddler/) — American Montessori Society. Checked 2026-11-02. Montessori educational approach, not clinical guidance.

- **movement**: [Movement Mat](https://aidtolife.org/movement/mat) — AMI Aid to Life. Checked 2026-11-02. Montessori awake movement; safety conditions supplied by NIH.

- **nih_awake**: [Ways to Reduce Baby's Risk](https://safetosleep.nichd.nih.gov/reduce-risk/reduce) — NIH NICHD Safe to Sleep. Checked 2026-11-02. Awake supervised tummy time and infant safe sleep.

- **conversation**: [Making Conversation: Young Babies](https://aidtolife.org/communication/conversations-young-babies) — AMI Aid to Life. Checked 2026-11-02. Montessori-informed responsive language.


## Authored items and notes

### karp-swaddle

Karp's five S's: unweighted wrapping of a young infant is which S? Stop at any signs of trying to roll. Use the verb.

Answer: **swaddle**. Sources: karp, swaddle.

Teach proper technique with a qualified caregiver; hips must move freely. No weighted swaddles; stop at signs of trying to roll.

### karp-position

Karp's held-soothing position S is side or ___. Only while held by an awake caregiver; never a sleep position.

Answer: **stomach**. Sources: karp, sleep.

Only while held by an awake caregiver. Place the baby on their back for sleep; do not leave a baby sleeping on their side or stomach.

### karp-shush

Karp's five S's: a gentle calming sound is which S? Use the verb.

Answer: **shush**. Sources: karp, noise.

If using a sound machine: far from baby, low volume, limited duration. Do not teach loudness targets from commercial soothing claims.

### karp-swing

Karp's five S's: gentle held movement, head and neck supported, is which S? Never shake; not a swing seat for sleep. Use the verb.

Answer: **swing**. Sources: karp, crying.

Gentle held movement with head and neck supported, never shaking. This names a soothing action, not permission to sleep in a swing.

### karp-suck

Karp's five S's: comfort from sucking is which S? Use the verb.

Answer: **suck**. Sources: karp, sleep, feeding.

Check feeding needs first; soothing must not delay a needed feed. If breastfeeding, AAP advises waiting until breastfeeding is established before offering a pacifier. Never attach it to clothing or bedding for sleep.

### sleep-position

For every infant sleep, place the baby on their ___.

Answer: **back**. Sources: sleep.



### swaddle-stop

Stop swaddling at the first signs of trying to do what movement? Use the -ing word.

Answer: **rolling**. Sources: swaddle.

Do not wait for a successful roll or a fixed age.

### noise-volume

If using a baby sound machine, keep volume as ___ as possible.

Answer: **low**. Sources: noise.

Place it far from the baby and limit duration; a louder sound is not a better soothing strategy.

### never-shake

When is shaking a baby safe?

Answer: **never**. Sources: crying.



### sleep-surface

An approved infant sleep surface should be firm and ___.

Answer: **flat**. Sources: sleep.

Use a safety-approved crib, bassinet or play yard with a fitted sheet, no loose bedding or soft objects. Do not substitute a floor mattress or futon.

### hunger-cue

Birth to 5 months: rooting toward breast or bottle can signal what need?

Answer: **hunger**. Sources: feeding.

Read cues together rather than treating one motion as proof. Crying can be a late hunger sign.

### fullness-cue

Birth to 5 months: turning away and closing the mouth during a feed can signal what?

Answer: **fullness**. Sources: feeding.

Do not pressure the baby to finish a bottle; discuss feeding concerns with their clinician.

### young-fever

A baby 3 months or younger has a rectal temperature of 100.4°F / 38°C or higher. Call the pediatrician when?

Answer: **immediately**. Sources: fever.

Do not wait for additional symptoms or use soothing techniques instead of seeking care. This is an escalation reminder, not diagnosis or medication advice.

### caregiver-reset

Overwhelmed by crying? For a brief calming break, put baby on their back in an empty crib or on a couch? Answer crib or couch.

Answer: **crib**. Sources: crying, sleep.

A safe bassinet or play yard is also appropriate; the question contrasts two specific locations. Seek another adult’s help and return/check on baby once calm. Never shake.

### montessori-observe

Montessori approach: before choosing an activity, first ___ the baby’s interests and cues.

Answer: **observe**. Sources: ami_observe.

Observation supports responsive choices, not ignoring distress or postponing feeding/medical care.

### montessori-environment

Montessori calls a thoughtfully arranged, development-appropriate space the prepared ___.

Answer: **environment**. Sources: ams.

For awake play: simple, orderly and safe, with appropriate materials. Parenting philosophy does not replace sleep-safety standards.

### awake-movement

Infant floor movement and tummy time belong in supervised ___ time, not sleep time.

Answer: **awake**. Sources: movement, nih_awake.

Provide safe floor space and stay attentive. Follow the baby’s tolerance; do not prescribe a fixed duration from the Montessori page.

### responsive-conversation

Montessori-informed conversation: respond to baby’s sounds, then pause so you can take ___.

Answer: **turns**. Sources: conversation.

Warm face-to-face talk during ordinary routines; no need for special products or claims of accelerated development.


## Deliberately excluded

- No Montessori floor-bed/futon recommendation for infants.

- No instructions to jiggle quickly, shake, use loud white noise, weighted swaddles, or keep swaddling after rolling signs.

- No SNOO-specific exemptions or efficacy guarantees.

- No treatment/dosing, sleep-training regime, or infant CPR technique.

- No exact age prediction for milestones; no toys with choking/cord hazards.
