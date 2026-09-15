package generation

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/misty-step/scry/internal/store"
)

var (
	markup         = regexp.MustCompile(`(?i)<[/!]?[a-z][^>]*>|\[[^\]]+\]\([^\)]+\)|` + "```")
	quotedText     = regexp.MustCompile(`["“]([^"”\n]{2,})["”]|‘([^’\n]{2,})’|(?:^|[\s(])'([^'\n]{3,})'(?:$|[\s.,!?:;)])`)
	citations      = regexp.MustCompile(`(?i)(?:https?://|www\.)[^\s<>()]+|\bdoi:[^\s]+|\[[0-9]+\]`)
	inlineURL      = regexp.MustCompile(`(?i)\b(?:[a-z][a-z0-9+.-]*://|www\.)`)
	falseSource    = regexp.MustCompile(`(?i)\b(according to (the |this )?(source|passage|excerpt|text)|the (source|passage|excerpt|text) (says|states|shows|proves))\b`)
	catchAll       = regexp.MustCompile(`(?i)\b(all|none|both|any) of (the |these )?(above|below|options|choices)|\b(not enough information|cannot be determined)\b`)
	keyedOption    = regexp.MustCompile(`^[A-Ea-e][.)]\s`)
	numbers        = regexp.MustCompile(`[-+]?[0-9]+(?:\.[0-9]+)?%?`)
	qualifications = regexp.MustCompile(`(?i)\b(may|might|could|sometimes|often|usually|typically|generally|suggests?|associated|association|correlat\w*|observational|not|no|never|cannot)\b`)
	strongClaims   = regexp.MustCompile(`\b(always|never|everyone|proves?|guarantees?|causes?|causal|causation|certainly|definitely)\b`)
	numericRange   = regexp.MustCompile(`^\s*(-?[0-9]+(?:\.[0-9]+)?)\s*(?:-|–|—|to)\s*(-?[0-9]+(?:\.[0-9]+)?)\s*([^0-9]*)$`)
	numericPoint   = regexp.MustCompile(`^\s*(-?[0-9]+(?:\.[0-9]+)?)\s*([^0-9]*)$`)
	batchKey       = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)
	vagueUnit      = regexp.MustCompile(`(?i)^(understand|learn|know|study|basics of|introduction to)\b`)
	falseResearch  = regexp.MustCompile(`(?i)\b(I|we) (fetched|browsed|verified|researched|watched|looked up)|\b(verified (fact|citation|transcript)|the (article|video|transcript) (says|states|shows|proves))\b`)
	fakeCertainty  = regexp.MustCompile(`(?i)\b(mastered|mastery|permanently knows|expert learner)\b|\b[0-9]{1,3}% (confident|confidence|mastery)\b`)
)

func strictObject(data []byte, target any, fields ...string) error {
	var object map[string]json.RawMessage
	if json.Unmarshal(data, &object) != nil || object == nil || len(object) != len(fields) {
		return errors.New("missing or unknown JSON fields")
	}
	for _, field := range fields {
		value, found := object[field]
		if !found || (field != "diagram" && bytes.Equal(bytes.TrimSpace(value), []byte("null"))) {
			return errors.New("missing or null JSON field")
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

// Decode the entire bundle before inspecting content. Unlike candidate filtering,
// atomic rejection cannot leave a relation or suggestion pointing at removed work.
func decodeBundle(data []byte) (store.GenerationResult, error) {
	var result store.GenerationResult
	var raw struct {
		Coverage    json.RawMessage   `json:"coverage"`
		Units       []json.RawMessage `json:"units"`
		Relations   []json.RawMessage `json:"relations"`
		Materials   []json.RawMessage `json:"materials"`
		Quizzes     []json.RawMessage `json:"quizzes"`
		Suggestions []json.RawMessage `json:"suggestions"`
	}
	if err := strictObject(data, &raw, "coverage", "units", "relations", "materials", "quizzes", "suggestions"); err != nil {
		return result, err
	}
	if len(raw.Units) > maxUnits || len(raw.Relations) > maxRelations || len(raw.Materials) > maxMaterials || len(raw.Quizzes) > maxQuizzes || len(raw.Suggestions) > maxSuggestions {
		return result, errors.New("bundle count exceeds limit")
	}
	if err := strictObject(raw.Coverage, &result.Coverage, "kind", "complete", "missing"); err != nil {
		return result, err
	}
	for _, value := range raw.Units {
		var unit store.GeneratedUnit
		if err := strictObject(value, &unit, "key", "reuse_id", "statement", "kind"); err != nil {
			return result, err
		}
		result.Units = append(result.Units, unit)
	}
	for _, value := range raw.Relations {
		var relation store.GeneratedRelation
		if err := strictObject(value, &relation, "from", "to", "kind", "evidence"); err != nil {
			return result, err
		}
		result.Relations = append(result.Relations, relation)
	}
	for _, value := range raw.Materials {
		var material store.GeneratedMaterial
		if err := strictObject(value, &material, "key", "reuse_id", "kind", "title", "body", "basis", "evidence", "reference_url", "start_seconds", "end_seconds", "estimated_seconds", "diagram", "links"); err != nil {
			return result, err
		}
		if err := decodeResourceDetails(value, material.Diagram != nil); err != nil {
			return result, err
		}
		result.Materials = append(result.Materials, material)
	}
	for _, value := range raw.Quizzes {
		var quiz store.GeneratedQuiz
		if err := strictObject(value, &quiz, "key", "reuse_id", "level", "estimated_seconds", "evidence", "basis", "kind", "prompt", "answer", "explanation", "choices", "variants", "links"); err != nil {
			return result, err
		}
		if err := decodeResourceDetails(value, false); err != nil {
			return result, err
		}
		result.Quizzes = append(result.Quizzes, quiz)
	}
	for _, value := range raw.Suggestions {
		var suggestion store.GeneratedSuggestion
		if err := strictObject(value, &suggestion, "key", "kind", "title", "reason", "unit_keys", "material_keys"); err != nil {
			return result, err
		}
		result.Suggestions = append(result.Suggestions, suggestion)
	}
	return result, nil
}

func decodeResourceDetails(data []byte, diagram bool) error {
	var raw struct {
		Links   []json.RawMessage `json:"links"`
		Diagram json.RawMessage   `json:"diagram"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if len(raw.Links) > maxLinks {
		return errors.New("coverage link count exceeds limit")
	}
	for _, value := range raw.Links {
		var link store.GeneratedLink
		if err := strictObject(value, &link, "unit_key", "role"); err != nil {
			return err
		}
	}
	if diagram {
		var parts struct {
			Nodes   []json.RawMessage `json:"nodes"`
			Edges   []json.RawMessage `json:"edges"`
			Caption string            `json:"caption"`
		}
		if err := strictObject(raw.Diagram, &parts, "nodes", "edges", "caption"); err != nil {
			return err
		}
		if len(parts.Nodes) > 16 || len(parts.Edges) > 32 {
			return errors.New("diagram count exceeds limit")
		}
		for _, value := range parts.Nodes {
			var node store.DiagramNode
			if err := strictObject(value, &node, "id", "label"); err != nil {
				return err
			}
		}
		for _, value := range parts.Edges {
			var edge store.DiagramEdge
			if err := strictObject(value, &edge, "from", "to", "label"); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateOutput(content string, job *store.Job, plan coveragePlan) (store.GenerationResult, []string, error) {
	empty := store.GenerationResult{}
	data := []byte(content)
	if len(data) > maxContentBytes || !utf8.Valid(data) || checkJSON(data, 12) != nil {
		return empty, nil, errors.New("invalid bundle JSON")
	}
	result, err := decodeBundle(data)
	if err != nil {
		return empty, nil, err
	}
	coverage := &result.Coverage
	if !slices.Contains([]string{"concepts", "vocabulary", "procedure", "complete_set", "exact_text"}, coverage.Kind) || len(coverage.Missing) > maxQuizzes {
		return empty, nil, errors.New("unsupported task classification or coverage count")
	}
	missingSeen := make(map[string]bool)
	for _, missing := range coverage.Missing {
		if !plainText(missing, 512, true) || missingSeen[normalized(missing)] {
			return empty, nil, errors.New("invalid coverage detail")
		}
		missingSeen[normalized(missing)] = true
	}
	issues := make([]string, 0)
	addIssue := func(issue string) {
		if issue != "" && !slices.Contains(issues, issue) {
			issues = append(issues, issue)
		}
	}
	if plan.Task != "infer" && coverage.Kind != plan.Task {
		addIssue("task_mismatch")
	}
	units := make(map[string]store.GeneratedUnit, len(result.Units))
	keys, statements := make(map[string]bool), make(map[string]bool)
	required := make(map[string]string, len(plan.Units))
	for _, unit := range plan.Units {
		required[unit.ID] = unit.Text
	}
	claimKey := func(key string) bool {
		if !batchKey.MatchString(key) || keys[key] {
			return false
		}
		keys[key] = true
		return true
	}
	for _, unit := range result.Units {
		if !claimKey(unit.Key) || !slices.Contains([]string{"foundation", "concept", "composition", "procedure", "exact_text"}, unit.Kind) {
			addIssue("invalid_unit_identity")
		}
		atomic := len(meaningful(normalized(unit.Statement))) >= 3 && !vagueUnit.MatchString(unit.Statement)
		if required[unit.Key] == unit.Statement {
			atomic = true // An explicit finite inventory can contain short mappings.
		}
		if !plainText(unit.Statement, 2048, true) || !atomic || statements[normalized(unit.Statement)] {
			addIssue("vague_or_duplicate_unit")
		}
		if unit.ReuseID == "" && unsupportedCitation(unit.Statement, job, job.SourceKind == "source") {
			addIssue("invented_unit_citation")
		}
		if text, found := required[unit.Key]; found && text != unit.Statement {
			addIssue("required_unit_changed")
		}
		addIssue(validateUnitReuse(unit, job))
		statements[normalized(unit.Statement)] = true
		units[unit.Key] = unit
	}
	if len(units) == 0 {
		addIssue("missing_knowledge_units")
	}
	relations, dependencies := make(map[string]bool), make(map[string][]string)
	used := make(map[string]bool)
	for _, relation := range result.Relations {
		_, from := units[relation.From]
		_, to := units[relation.To]
		key := relation.From + "\x00" + relation.To + "\x00" + relation.Kind
		if !from || !to || relation.From == relation.To || relations[key] || !slices.Contains([]string{"prerequisite", "composition", "contrast"}, relation.Kind) {
			addIssue("invalid_knowledge_relation")
		}
		if !plainText(relation.Evidence, 8192, true) || len(meaningful(normalized(relation.Evidence))) < 4 || (!strings.HasPrefix(relation.Evidence, "Proposed relationship:") && (job.SourceKind != "source" || !strings.Contains(job.SourceText, relation.Evidence))) {
			addIssue("unsubstantiated_relation")
		}
		if relation.Kind == "composition" && units[relation.To].Kind != "composition" {
			addIssue("invalid_composition_target")
		}
		if relation.Kind != "contrast" {
			dependencies[relation.From] = append(dependencies[relation.From], relation.To)
		}
		relations[key], used[relation.From], used[relation.To] = true, true, true
	}
	if dependencyCycle(dependencies) {
		addIssue("cyclic_knowledge_dependencies")
	}
	materials := make(map[string]bool)
	seenContent := make(map[string]bool)
	for _, material := range result.Materials {
		if !claimKey(material.Key) {
			addIssue("invalid_material_identity")
		}
		addIssue(validateLinks(material.Links, units, false, used))
		addIssue(validateMaterial(material, job))
		addIssue(validateMaterialReuse(material, job))
		if material.Basis == "background" {
			for _, link := range material.Links {
				if link.Role == "teaches" && units[link.UnitKey].Kind != "foundation" {
					addIssue("background_target_laundering")
				}
			}
		}
		contentKey := material.Kind + "\x00" + normalized(material.Body) + "\x00" + material.ReferenceURL + "\x00" + strconv.Itoa(material.StartSeconds) + "\x00" + strconv.Itoa(material.EndSeconds)
		if seenContent[contentKey] {
			addIssue("duplicate_material")
		}
		seenContent[contentKey], materials[material.Key] = true, true
	}
	lastUnit, lastExactOffset := -1, -1
	covered := make(map[string]bool, len(plan.Units))
	for _, quiz := range result.Quizzes {
		if !claimKey(quiz.Key) {
			addIssue("invalid_quiz_identity")
		}
		if !slices.Contains([]string{"foundation", "target", "extension"}, quiz.Level) || quiz.EstimatedSeconds < 1 || quiz.EstimatedSeconds > 3600 {
			addIssue("invalid_quiz_pacing")
		}
		addIssue(validateLinks(quiz.Links, units, true, used))
		if quiz.ReuseID == "" {
			addIssue(validateQuiz(quiz, job))
		}
		addIssue(validateQuizReuse(quiz, job))
		if quiz.Basis == "background" {
			for _, link := range quiz.Links {
				if link.Role == "assesses" && units[link.UnitKey].Kind != "foundation" {
					addIssue("background_target_laundering")
				}
			}
		}
		contentKey := "quiz\x00" + normalized(quiz.Prompt)
		if seenContent[contentKey] {
			addIssue("duplicate_question")
		}
		seenContent[contentKey], materials[quiz.Key] = true, true
		if quiz.Level != "target" {
			continue
		}
		assessed := assessedKeys(quiz.Links)
		unit := -1
		if len(plan.Units) > 0 {
			if len(assessed) != 1 {
				addIssue("missing_coverage_unit")
			} else {
				for index, requiredUnit := range plan.Units {
					if requiredUnit.ID == assessed[0] {
						unit = index
					}
				}
				if unit < 0 || covered[assessed[0]] || (plan.Ordered && unit <= lastUnit) {
					addIssue("repeated_or_reordered_unit")
				} else if !strings.Contains(quiz.Evidence, plan.Units[unit].Text) || !unitIsTested(plan.Units[unit].Text, quiz, coverage.Kind) {
					addIssue("unit_not_actually_tested")
				} else {
					covered[assessed[0]], lastUnit = true, unit
				}
			}
		}
		if coverage.Kind == "exact_text" {
			if quiz.Kind != "recall" || len(quiz.Variants) != 0 || job.SourceKind != "source" || !strings.Contains(job.SourceText, quiz.Answer) {
				addIssue("exact_text_changed")
			}
			if unit < 0 {
				offset := strings.Index(job.SourceText, quiz.Answer)
				if offset <= lastExactOffset {
					addIssue("exact_text_reordered")
				}
				lastExactOffset = offset
			}
		}
	}
	for _, suggestion := range result.Suggestions {
		if !claimKey(suggestion.Key) || !slices.Contains([]string{"advance", "lateral"}, suggestion.Kind) || !plainText(suggestion.Title, 240, true) || !plainText(suggestion.Reason, 2048, true) || len(meaningful(normalized(suggestion.Reason))) < 6 || fakeCertainty.MatchString(suggestion.Reason) {
			addIssue("invalid_goal_suggestion")
		}
		if len(suggestion.UnitKeys) == 0 || len(suggestion.UnitKeys) > maxLinks || len(suggestion.MaterialKeys) > maxLinks {
			addIssue("invalid_suggestion_scope")
		}
		seen := make(map[string]bool)
		for _, key := range suggestion.UnitKeys {
			if _, exists := units[key]; !exists || seen[key] {
				addIssue("dangling_suggestion_unit")
			}
			seen[key], used[key] = true, true
		}
		for _, key := range suggestion.MaterialKeys {
			if !materials[key] || seen[key] {
				addIssue("dangling_suggestion_material")
			}
			seen[key] = true
		}
	}
	for key := range units {
		if !used[key] {
			addIssue("unconnected_knowledge_unit")
		}
	}
	addIssue(validateJobBundle(&result, job, units))
	if len(result.Materials)+len(result.Quizzes) == 0 {
		addIssue("no_usable_material")
	}
	if len(issues) > 0 {
		return empty, issues, nil
	}
	addMissing := func(detail string) {
		if !missingSeen[normalized(detail)] {
			coverage.Missing = append(coverage.Missing, detail)
			missingSeen[normalized(detail)] = true
		}
		coverage.Complete = false
	}
	finite := coverage.Kind == "complete_set" || coverage.Kind == "exact_text"
	if finite && (plan.Unverified || len(plan.Units) == 0) {
		addMissing("Complete coverage cannot be established without a complete authoritative finite input.")
	}
	missingUnits := make([]string, 0, len(plan.Units))
	for _, unit := range plan.Units {
		if !covered[unit.ID] {
			missingUnits = append(missingUnits, unit.ID)
		}
	}
	if len(missingUnits) > 0 {
		addMissing("Required source units lack validated target assessments: " + strings.Join(missingUnits, ", ") + ".")
	}
	unavailable := 0
	for _, material := range result.Materials {
		if material.Basis == "reference" {
			unavailable++
		}
	}
	if unavailable > 0 {
		addMissing(fmt.Sprintf("%d reference(s) have no supplied article body or video transcript; no content was fetched.", unavailable))
	}
	if job.Kind == "capture" && !finite {
		foundation := false
		for _, unit := range result.Units {
			foundation = foundation || unit.Kind == "foundation"
		}
		if !foundation {
			addMissing("The bounded capture has not yet represented its foundations.")
		}
		if len(result.Materials) == 0 {
			addMissing("Durable foundation instruction remains missing from this capture.")
		}
		if len(result.Quizzes) == 0 {
			addMissing("Practice assessments remain missing from this bounded capture.")
		}
	}
	if len(coverage.Missing) > 0 {
		coverage.Complete = false
	}
	if !coverage.Complete && len(coverage.Missing) == 0 {
		addMissing("The provider reports incomplete bounded coverage without further detail.")
	}
	if len(coverage.Missing) > maxQuizzes {
		return empty, []string{"coverage_detail_limit"}, nil
	}
	return result, nil, nil
}

func plainText(value string, limit int, required bool) bool {
	if len(value) > limit || !utf8.ValidString(value) || (required && strings.TrimSpace(value) == "") || hasUnsafeControl(value) || markup.MatchString(value) || inlineURL.MatchString(value) {
		return false
	}
	lower := strings.ToLower(value)
	return !strings.Contains(lower, "javascript:") && !strings.Contains(lower, "data:")
}

func unsupportedCitation(text string, job *store.Job, sourceBacked bool) bool {
	for _, citation := range citations.FindAllString(text, -1) {
		if !sourceBacked || !strings.Contains(job.SourceText, citation) {
			return true
		}
	}
	return false
}

func assessedKeys(links []store.GeneratedLink) []string {
	keys := make([]string, 0, len(links))
	for _, link := range links {
		if link.Role == "assesses" {
			keys = append(keys, link.UnitKey)
		}
	}
	return keys
}

func validateLinks(links []store.GeneratedLink, units map[string]store.GeneratedUnit, quiz bool, used map[string]bool) string {
	if len(links) == 0 || len(links) > maxLinks {
		return "missing_or_excessive_coverage"
	}
	seen, useful := make(map[string]bool), false
	for _, link := range links {
		key := link.UnitKey + "\x00" + link.Role
		if _, found := units[link.UnitKey]; !found || !slices.Contains([]string{"assesses", "teaches", "assumes", "mentions"}, link.Role) || seen[key] {
			return "invalid_coverage_link"
		}
		if !quiz && link.Role == "assesses" {
			return "instruction_cannot_assess"
		}
		useful = useful || (quiz && link.Role == "assesses") || (!quiz && link.Role == "teaches")
		seen[key], used[link.UnitKey] = true, true
	}
	if !useful {
		return "no_direct_material_coverage"
	}
	return ""
}

func dependencyCycle(edges map[string][]string) bool {
	state := make(map[string]uint8)
	var visit func(string) bool
	visit = func(key string) bool {
		if state[key] == 1 {
			return true
		}
		if state[key] == 2 {
			return false
		}
		state[key] = 1
		for _, next := range edges[key] {
			if visit(next) {
				return true
			}
		}
		state[key] = 2
		return false
	}
	for key := range edges {
		if visit(key) {
			return true
		}
	}
	return false
}

func safeReference(raw string) bool {
	if raw == "" || len(raw) > 2048 || !utf8.ValidString(raw) || strings.IndexFunc(raw, unicode.IsSpace) >= 0 || hasUnsafeControl(raw) {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Opaque != "" || strings.ContainsAny(raw, "<>\"\\") {
		return false
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsPrivate() || ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
			return false
		}
	}
	return true
}

func validateMaterial(material store.GeneratedMaterial, job *store.Job) string {
	if !slices.Contains([]string{"explanation", "worked_example", "diagram", "article", "video"}, material.Kind) || !plainText(material.Title, 240, true) || !plainText(material.Body, 16384, material.Basis != "reference") || material.EstimatedSeconds < 1 || material.EstimatedSeconds > 3600 {
		return "invalid_material_text"
	}
	if len(material.Evidence) > 8192 || !utf8.ValidString(material.Evidence) || hasUnsafeControl(material.Evidence) {
		return "invalid_material_evidence"
	}
	reference := material.Kind == "article" || material.Kind == "video"
	if reference {
		if !safeReference(material.ReferenceURL) || !suppliedReference(material, job) {
			return "invented_or_unsafe_reference"
		}
	} else if material.ReferenceURL != "" {
		return "unexpected_external_reference"
	}
	if material.StartSeconds < 0 || material.EndSeconds < 0 || material.EndSeconds > 86400 || (material.StartSeconds != 0 || material.EndSeconds != 0) && (material.Kind != "video" || material.EndSeconds <= material.StartSeconds) {
		return "invalid_video_segment"
	}
	if (material.Kind == "diagram") != (material.Diagram != nil) {
		return "invalid_diagram_kind"
	}
	// Immutable reuse still passes executable-content, reference, shape and
	// bounds gates. New semantic/source attribution checks must not reinterpret
	// a preserved resource from a different source as a newly authored one.
	if material.ReuseID != "" {
		if material.Diagram != nil {
			return validateDiagram(material.Diagram, material.Body)
		}
		return ""
	}
	combined := material.Title + "\n" + material.Body
	if unsupportedCitation(combined, job, material.Basis == "source" && job.SourceKind == "source") {
		return "invented_citation"
	}
	if material.Diagram != nil {
		if issue := validateDiagram(material.Diagram, material.Body); issue != "" {
			return issue
		}
		combined += "\n" + material.Diagram.Caption
		for _, node := range material.Diagram.Nodes {
			combined += "\n" + node.Label
		}
		for _, edge := range material.Diagram.Edges {
			combined += "\n" + edge.Label
		}
	}
	if falseResearch.MatchString(combined) {
		return "invented_reference_research"
	}
	if material.Basis != "reference" && (len(meaningful(normalized(material.Body))) < 6 || normalized(material.Body) == normalized(material.Title)) {
		return "uninformative_instruction"
	}
	switch material.Basis {
	case "reference":
		if !reference || material.Body != "" || material.Evidence != "" || material.Diagram != nil {
			return "invented_reference_content"
		}
	case "background":
		if material.Evidence != "" || !strings.HasPrefix(material.Body, "Generated background:") || falseSource.MatchString(combined) {
			return "false_background_evidence"
		}
	case "topic":
		if job.SourceKind != "topic" || reference || material.Evidence != "" || falseSource.MatchString(combined) {
			return "false_topic_evidence"
		}
	case "source":
		if job.SourceKind != "source" || len(strings.Fields(material.Evidence)) < 2 || !strings.Contains(job.SourceText, material.Evidence) {
			return "unverified_source_quote"
		}
		evidence := normalized(material.Evidence)
		if !answerSupported(normalized(material.Body), evidence) || !numbersSupported(combined, material.Evidence) {
			return "unsupported_source_instruction"
		}
		if qualifications.MatchString(material.Evidence) && strengthensQualification(normalized(combined), evidence) {
			return "strengthened_source_claim"
		}
		for _, match := range quotedText.FindAllStringSubmatch(combined, -1) {
			for _, quote := range match[1:] {
				if quote != "" && !strings.Contains(material.Evidence, quote) {
					return "invented_quoted_text"
				}
			}
		}
	default:
		return "invalid_material_basis"
	}
	return ""
}

func validateDiagram(diagram *store.Diagram, body string) string {
	if len(diagram.Nodes) < 2 || len(diagram.Nodes) > 16 || len(diagram.Edges) == 0 || len(diagram.Edges) > 32 || !plainText(diagram.Caption, 1024, true) {
		return "invalid_diagram_bounds"
	}
	bodyKey := normalized(body)
	nodes, labels, edges := make(map[string]bool), make(map[string]bool), make(map[string]bool)
	neighbors := make(map[string][]string)
	for _, node := range diagram.Nodes {
		label := normalized(node.Label)
		if !batchKey.MatchString(node.ID) || nodes[node.ID] || !plainText(node.Label, 240, true) || labels[label] || !phraseContains(bodyKey, label) {
			return "invalid_diagram_node"
		}
		nodes[node.ID], labels[label] = true, true
	}
	for _, edge := range diagram.Edges {
		key := edge.From + "\x00" + edge.To
		if !nodes[edge.From] || !nodes[edge.To] || edge.From == edge.To || edges[key] || !plainText(edge.Label, 240, true) || !phraseContains(bodyKey, normalized(edge.Label)) {
			return "invalid_diagram_edge"
		}
		edges[key] = true
		neighbors[edge.From] = append(neighbors[edge.From], edge.To)
		neighbors[edge.To] = append(neighbors[edge.To], edge.From)
	}
	visited := make(map[string]bool)
	queue := []string{diagram.Nodes[0].ID}
	for len(queue) > 0 {
		key := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		if visited[key] {
			continue
		}
		visited[key] = true
		queue = append(queue, neighbors[key]...)
	}
	if len(visited) != len(nodes) {
		return "disconnected_diagram"
	}
	return ""
}

func validateQuiz(q store.GeneratedQuiz, job *store.Job) string {
	for _, field := range []struct {
		value string
		limit int
	}{{q.Prompt, 4096}, {q.Answer, 1024}, {q.Explanation, 8192}} {
		if !plainText(field.value, field.limit, true) {
			return "invalid_quiz_text"
		}
	}
	if len(q.Evidence) > 8192 || hasUnsafeControl(q.Evidence) || len(q.Variants) > 8 || len(q.Choices) > 5 {
		return "quiz_bounds"
	}
	prompt, answer, explanation := normalized(q.Prompt), normalized(q.Answer), normalized(q.Explanation)
	leaksAnswer := phraseContains(prompt, answer)
	// Preserve case for single-letter identifiers: an ordinary article does
	// not reveal an uppercase label, but naming that label explicitly does.
	if letter, size := utf8.DecodeRuneInString(q.Answer); leaksAnswer && size == len(q.Answer) && unicode.IsUpper(letter) {
		leaksAnswer = false
		for word := range strings.FieldsFuncSeq(q.Prompt, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) }) {
			if word == q.Answer {
				leaksAnswer = true
				break
			}
		}
	}
	if len(strings.Fields(prompt)) < 4 || answer == "" || leaksAnswer {
		return "answer_leakage_or_vague_prompt"
	}
	if len(strings.Fields(explanation)) < 6 || len(meaningful(explanation)) < 3 || explanation == prompt || explanation == answer {
		return "uninformative_explanation"
	}
	if q.Kind == "choice" {
		if len(q.Choices) < 3 || len(q.Variants) != 0 {
			return "invalid_choice_shape"
		}
		found := false
		keys := make([]string, 0, len(q.Choices))
		for _, choice := range q.Choices {
			key := normalized(choice)
			if strings.TrimSpace(choice) != choice || key == "" || !plainText(choice, 1024, true) || catchAll.MatchString(choice) || keyedOption.MatchString(choice) {
				return "invalid_distractor"
			}
			for index, previous := range keys {
				if previous == key || overlappingOptions(q.Choices[index], choice, previous, key) {
					return "overlapping_distractors"
				}
			}
			keys = append(keys, key)
			found = found || choice == q.Answer
		}
		if !found {
			return "answer_not_displayed_choice"
		}
	} else if q.Kind == "recall" {
		if len(q.Choices) != 0 {
			return "invalid_recall_shape"
		}
		seen := map[string]bool{answer: true}
		for _, variant := range q.Variants {
			key := normalized(variant)
			if key == "" || strings.TrimSpace(variant) != variant || !plainText(variant, 1024, true) || seen[key] || phraseContains(prompt, key) || phraseContains(answer, key) || phraseContains(key, answer) || strings.ContainsAny(variant, "*|") {
				return "invalid_recall_variant"
			}
			seen[key] = true
		}
	} else {
		return "unsupported_quiz_kind"
	}
	combined := q.Prompt + "\n" + q.Answer + "\n" + q.Explanation + "\n" + strings.Join(q.Choices, "\n") + "\n" + strings.Join(q.Variants, "\n")
	if unsupportedCitation(combined, job, q.Basis == "source" && job.SourceKind == "source") {
		return "invented_citation"
	}
	if q.Basis == "background" {
		if q.Level != "foundation" || q.Evidence != "" || !strings.HasPrefix(q.Explanation, "Generated background:") || falseSource.MatchString(combined) || falseResearch.MatchString(combined) || citations.MatchString(combined) {
			return "false_background_evidence"
		}
		return ""
	}
	if job.SourceKind == "topic" {
		if q.Basis != "topic" || q.Evidence != "" || falseSource.MatchString(combined) || falseResearch.MatchString(combined) {
			return "false_topic_evidence"
		}
		return ""
	}
	if q.Basis != "source" || strings.TrimSpace(q.Evidence) == "" || !strings.Contains(job.SourceText, q.Evidence) || len(strings.Fields(q.Evidence)) < 2 {
		return "unverified_source_quote"
	}
	evidence := normalized(q.Evidence)
	if !answerSupported(answer, evidence) || !numbersSupported(q.Answer, q.Evidence) || !numbersSupported(q.Explanation, q.Evidence) {
		return "unsupported_source_answer"
	}
	for _, variant := range q.Variants {
		if !answerSupported(normalized(variant), evidence) || !numbersSupported(variant, q.Evidence) {
			return "unsupported_source_variant"
		}
	}
	for _, match := range quotedText.FindAllStringSubmatch(combined, -1) {
		for _, quote := range match[1:] {
			if quote != "" && !strings.Contains(job.SourceText, quote) {
				return "invented_quoted_text"
			}
		}
	}
	if qualifications.MatchString(q.Evidence) && strengthensQualification(answer+" "+explanation, evidence) {
		return "strengthened_source_claim"
	}
	return ""
}

func normalized(text string) string {
	var value strings.Builder
	value.Grow(len(text))
	space := true
	for _, char := range text {
		if unicode.IsLetter(char) || unicode.IsNumber(char) {
			value.WriteRune(unicode.ToLower(char))
			space = false
		} else if !space {
			value.WriteByte(' ')
			space = true
		}
	}
	return strings.TrimSpace(value.String())
}

func phraseContains(text, phrase string) bool {
	if phrase == "" {
		return false
	}
	for offset := 0; offset <= len(text)-len(phrase); {
		index := strings.Index(text[offset:], phrase)
		if index < 0 {
			return false
		}
		index += offset
		end := index + len(phrase)
		if (index == 0 || text[index-1] == ' ') && (end == len(text) || text[end] == ' ') {
			return true
		}
		offset = index + 1
	}
	return false
}

func meaningful(text string) []string {
	words := strings.Fields(text)
	out := words[:0]
	for _, word := range words {
		switch word {
		case "a", "an", "the", "of", "to", "in", "on", "at", "by", "and", "or", "for", "is", "are", "was", "were", "be", "it", "its", "this", "that", "with", "from", "as", "has", "have", "had", "which", "what", "does", "do", "can":
			continue
		}
		out = append(out, word)
	}
	return out
}

func answerSupported(answer, evidence string) bool {
	if phraseContains(evidence, answer) {
		return true
	}
	words := meaningful(answer)
	if len(words) == 0 {
		return false
	}
	matched := 0
	for _, word := range words {
		if phraseContains(evidence, word) {
			matched++
		}
	}
	return matched*3 >= len(words)*2
}

func numbersSupported(text, evidence string) bool {
	available := numbers.FindAllString(evidence, -1)
	for _, number := range numbers.FindAllString(text, -1) {
		found := false
		for _, permitted := range available {
			found = found || number == permitted
		}
		if !found {
			return false
		}
	}
	return true
}

func unitIsTested(unit string, q store.GeneratedQuiz, task string) bool {
	if task == "exact_text" {
		return q.Answer == unit
	}
	questionAndAnswer := normalized(q.Prompt + " " + q.Answer)
	words := meaningful(normalized(unit))
	if len(words) == 0 {
		return false
	}
	for _, word := range words {
		if !phraseContains(questionAndAnswer, word) {
			return false
		}
	}
	return true
}

func strengthensQualification(claim, evidence string) bool {
	if !qualifications.MatchString(claim) {
		return true
	}
	for _, location := range strongClaims.FindAllStringIndex(claim, -1) {
		word := claim[location[0]:location[1]]
		if phraseContains(evidence, word) && (word == "never" || word == "always" || word == "everyone") {
			continue
		}
		previous := strings.Fields(claim[:location[0]])
		if len(previous) > 3 {
			previous = previous[len(previous)-3:]
		}
		negated := false
		for _, token := range previous {
			negated = negated || token == "not" || token == "no" || token == "cannot" || token == "t" || token == "may" || token == "might" || token == "could" || token == "sometimes"
		}
		if !negated {
			return true
		}
	}
	return false
}

func interval(option string) (float64, float64, string, bool) {
	if match := numericRange.FindStringSubmatch(option); match != nil {
		start, err1 := strconv.ParseFloat(match[1], 64)
		end, err2 := strconv.ParseFloat(match[2], 64)
		return start, end, normalized(match[3]), err1 == nil && err2 == nil && start <= end
	}
	if match := numericPoint.FindStringSubmatch(option); match != nil {
		point, err := strconv.ParseFloat(match[1], 64)
		return point, point, normalized(match[2]), err == nil
	}
	return 0, 0, "", false
}

func overlappingOptions(left, right, leftKey, rightKey string) bool {
	leftMin, leftMax, leftUnit, leftNumeric := interval(left)
	rightMin, rightMax, rightUnit, rightNumeric := interval(right)
	if leftNumeric && rightNumeric {
		return leftUnit == rightUnit && leftMin <= rightMax && rightMin <= leftMax
	}
	return phraseContains(leftKey, rightKey) || phraseContains(rightKey, leftKey)
}
