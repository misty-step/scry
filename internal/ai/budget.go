package ai

// Conservative price ceiling: 8 micros/input token, 40/output token.
// Input UTF-8 bytes bound tokens; output limits are enforced in the adapter.
const JudgeBudgetMicros int64 = 8192*8 + 1024*40
const DraftBudgetMicros int64 = 32000*8 + 16000*40
