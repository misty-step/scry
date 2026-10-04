# S2 learning boundary contract

This slice is a pure Go module API in `internal/learning`. Inputs are Go
arguments; outputs are return values. It adds no stdin/argv, stdout JSON, or
process exit-code interface. Persisted strings and JSON remain unchanged.

`QuizKind` and `GradingPolicy` are distinct named string types. Supported kinds
are `ChoiceKind` (`choice`) and `RecallKind` (`recall`). Supported policies are
`ExactPolicy` (`exact-v1`), `ShortPolicy` (`short-v1`), `SemanticPolicy`
(`semantic-v1`), and `LearnerPolicy` (`learner-v1`). The checked-out S1 base has
no policy/kind types, so these definitions belong in the scoped `learning.go`.

```go
func GradeTyped(kind QuizKind, expected string, variants []string, answer string, reveal bool) (outcome string, rating int)
func EventAlgorithmTyped(policy GradingPolicy) string
func GradeShort(j ShortJudgments, p ShortParams) SemanticDecision
func GradeSemantic(j SemanticJudgments, p Params) SemanticDecision
```

`ShortParams.PolicyVersion` and `Params.PolicyVersion` have type `GradingPolicy`.
Short grading accepts only `ShortPolicy`; semantic grading accepts only
`SemanticPolicy`. Unknown, empty, or inappropriate policies return an unapplied
`ungraded` decision with rating zero. Validation happens inside the typed
functions, including when callers explicitly convert an unknown string.
`EventAlgorithmTyped` returns an empty string for an unsupported policy;
supported policies retain the pinned scheduler and historical exact identity.
`GradeTyped` returns `ungraded`, rating zero, for an unsupported kind.

The existing string APIs are single delegation wrappers:

```go
func Grade(kind, expected string, variants []string, answer string, reveal bool) (outcome string, rating int)
func EventAlgorithm(grading string) string
```

For supported kinds, reveal returns `revealed`/Again. Choices compare exactly.
Recall accepts only the key or an authored variant after trimming surrounding
space; all other recall answers remain ungraded. Short thresholds stay accept
≥0.85, identity risk ≤0.35, injection risk ≤0.20, and reject ≥0.90 with injection
risk ≤0.20. Semantic thresholds and shadow-class behavior remain unchanged.

No transition, store, schema, scheduler, network, or deployment change is part
of this contract.
