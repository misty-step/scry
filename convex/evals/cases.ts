export type EvalCase = {
  prompt: string;
  expectedMin: number;
  description: string;
};

export const EVAL_CASES: EvalCase[] = [
  {
    prompt: 'NATO Phonetic Alphabet',
    expectedMin: 26,
    description: 'Should generate all 26 letter codes',
  },
  {
    prompt: 'The planets of the solar system',
    expectedMin: 8,
    description: 'Should generate at least 8 planets (Pluto optional)',
  },
  {
    prompt: "Isaac Asimov's Three Laws of Robotics",
    expectedMin: 3,
    description: 'Should generate exactly 3 laws',
  },
];
