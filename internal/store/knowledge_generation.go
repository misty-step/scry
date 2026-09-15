package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/misty-step/scry/internal/learning"
)

var generatedKey = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)
var referencePattern = regexp.MustCompile(`https://[^\s<>"\x60]+`)

func validRole(role string) bool {
	return role == "assesses" || role == "teaches" || role == "assumes" || role == "mentions"
}
func validRelationKind(kind string) bool {
	return kind == "prerequisite" || kind == "composition" || kind == "contrast"
}
func validateUnit(u GeneratedUnit) error {
	if !generatedKey.MatchString(u.Key) {
		return fmt.Errorf("%w: invalid unit key", ErrInvalid)
	}
	if u.Kind != "foundation" && u.Kind != "concept" && u.Kind != "composition" && u.Kind != "procedure" && u.Kind != "exact_text" {
		return fmt.Errorf("%w: invalid knowledge unit kind", ErrInvalid)
	}
	return validText("unit statement", u.Statement, 2048, true)
}

func safeReference(raw string) error {
	if err := validText("reference URL", raw, 2048, true); err != nil {
		return err
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Opaque != "" {
		return fmt.Errorf("%w: reference requires an actual credential-free HTTPS URL", ErrInvalid)
	}
	host := strings.ToLower(u.Hostname())
	ip := net.ParseIP(host)
	if host == "localhost" || !strings.Contains(host, ".") || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") || ip != nil && (ip.IsPrivate() || ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()) {
		return fmt.Errorf("%w: reference cannot address a private service", ErrInvalid)
	}
	for key := range u.Query() {
		k := strings.ToLower(key)
		if strings.Contains(k, "token") || strings.Contains(k, "secret") || strings.Contains(k, "password") || strings.Contains(k, "signature") || strings.Contains(k, "credential") || k == "key" || k == "auth" || k == "authorization" {
			return fmt.Errorf("%w: reference cannot contain credentials", ErrInvalid)
		}
	}
	return nil
}

func sourceReferences(text string) []string {
	refs := []string{}
	seen := map[string]bool{}
	for _, raw := range referencePattern.FindAllString(text, -1) {
		raw = strings.TrimRight(raw, ".,;!?)\"]}")
		if safeReference(raw) == nil && !seen[raw] {
			seen[raw] = true
			refs = append(refs, raw)
		}
	}
	return refs
}

func validateBasis(basis, evidence string, src Source) error {
	switch basis {
	case "source":
		if src.Kind != "source" || strings.TrimSpace(evidence) == "" || !strings.Contains(src.Text, evidence) {
			return fmt.Errorf("%w: source evidence must be an exact quotation from saved source", ErrInvalid)
		}
	case "topic":
		if src.Kind != "topic" || evidence != "" {
			return fmt.Errorf("%w: topic content cannot relabel supplied-source claims", ErrInvalid)
		}
	case "background", "reference":
		if evidence != "" {
			return fmt.Errorf("%w: generated background or unavailable references cannot claim quoted evidence", ErrInvalid)
		}
	default:
		return fmt.Errorf("%w: unsupported content basis", ErrInvalid)
	}
	return nil
}

func validateMaterial(m GeneratedMaterial, src Source, references []string) error {
	if m.Kind != "explanation" && m.Kind != "worked_example" && m.Kind != "diagram" && m.Kind != "article" && m.Kind != "video" {
		return fmt.Errorf("%w: unsupported material kind", ErrInvalid)
	}
	for _, field := range []struct {
		name, text string
		limit      int
		required   bool
	}{
		{"material title", m.Title, 240, true}, {"material body", m.Body, 16384, m.Basis != "reference"}, {"material evidence", m.Evidence, 8192, false},
	} {
		if err := validText(field.name, field.text, field.limit, field.required); err != nil {
			return err
		}
	}
	if err := validateBasis(m.Basis, m.Evidence, src); err != nil {
		return err
	}
	if m.EstimatedSeconds < 1 || m.EstimatedSeconds > 3600 {
		return fmt.Errorf("%w: material time must be 1..3600 seconds", ErrInvalid)
	}
	external := m.Kind == "article" || m.Kind == "video"
	if external {
		if err := safeReference(m.ReferenceURL); err != nil {
			return err
		}
		known := false
		for _, ref := range sourceReferences(src.Text) {
			known = known || ref == m.ReferenceURL
		}
		for _, ref := range references {
			known = known || ref == m.ReferenceURL
		}
		if !known {
			return fmt.Errorf("%w: reference was not supplied or reused", ErrInvalid)
		}
	} else if m.ReferenceURL != "" {
		return fmt.Errorf("%w: external URLs belong only to article/video references", ErrInvalid)
	}
	if m.Basis == "reference" && (!external || m.Body != "" || m.Diagram != nil) {
		return fmt.Errorf("%w: an unavailable reference cannot claim fetched content", ErrInvalid)
	}
	if m.Basis == "background" && !strings.HasPrefix(m.Body, "Generated background:") {
		return fmt.Errorf("%w: generated background needs an explicit label", ErrInvalid)
	}
	if m.Basis == "topic" && external {
		return fmt.Errorf("%w: topic knowledge is not a fetched reference", ErrInvalid)
	}
	if m.StartSeconds < 0 || m.EndSeconds < 0 || m.EndSeconds > 86400 || ((m.StartSeconds != 0 || m.EndSeconds != 0) && (m.Kind != "video" || m.EndSeconds <= m.StartSeconds)) {
		return fmt.Errorf("%w: invalid supported video segment", ErrInvalid)
	}
	if (m.Kind == "diagram") != (m.Diagram != nil) {
		return fmt.Errorf("%w: structured diagram kind mismatch", ErrInvalid)
	}
	if m.Diagram != nil {
		return validateStructuredDiagram(m.Diagram, m.Body)
	}
	return nil
}

func validateStructuredDiagram(d *Diagram, body string) error {
	if len(d.Nodes) < 2 || len(d.Nodes) > 16 || len(d.Edges) < 1 || len(d.Edges) > 32 {
		return fmt.Errorf("%w: diagram exceeds node/edge bounds", ErrInvalid)
	}
	if err := validText("diagram caption", d.Caption, 1024, true); err != nil {
		return err
	}
	ids, labels := map[string]bool{}, map[string]bool{}
	for _, node := range d.Nodes {
		label := strings.ToLower(strings.TrimSpace(node.Label))
		if !generatedKey.MatchString(node.ID) || ids[node.ID] || labels[label] || !strings.Contains(strings.ToLower(body), label) {
			return fmt.Errorf("%w: diagram nodes must be distinct and explained in the body", ErrInvalid)
		}
		if err := validText("diagram node", node.Label, 240, true); err != nil {
			return err
		}
		ids[node.ID], labels[label] = true, true
	}
	neighbors := map[string][]string{}
	seen := map[string]bool{}
	for _, edge := range d.Edges {
		key := edge.From + "/" + edge.To
		if !ids[edge.From] || !ids[edge.To] || edge.From == edge.To || seen[key] || !strings.Contains(strings.ToLower(body), strings.ToLower(edge.Label)) {
			return fmt.Errorf("%w: invalid or unexplained diagram edge", ErrInvalid)
		}
		if err := validText("diagram edge", edge.Label, 240, true); err != nil {
			return err
		}
		seen[key] = true
		neighbors[edge.From] = append(neighbors[edge.From], edge.To)
		neighbors[edge.To] = append(neighbors[edge.To], edge.From)
	}
	visited := map[string]bool{}
	queue := []string{d.Nodes[0].ID}
	for len(queue) > 0 {
		id := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		if visited[id] {
			continue
		}
		visited[id] = true
		queue = append(queue, neighbors[id]...)
	}
	if len(visited) != len(ids) {
		return fmt.Errorf("%w: disconnected diagram", ErrInvalid)
	}
	return nil
}

func validateGeneration(result GenerationResult, src Source) error {
	if len(result.Units) == 0 || len(result.Units) > MaxGeneratedUnits || len(result.Relations) > MaxGeneratedRelations || len(result.Materials) > MaxGeneratedMaterials || len(result.Quizzes) > MaxGeneratedQuizzes || len(result.Suggestions) > MaxGeneratedSuggestions || len(result.Materials)+len(result.Quizzes) == 0 {
		return fmt.Errorf("%w: bundle needs bounded represented units and useful materials", ErrInvalid)
	}
	if err := validText("model attribution", result.Model, 200, true); err != nil {
		return err
	}
	if err := validText("prompt version", result.PromptVersion, 200, true); err != nil {
		return err
	}
	c := result.Coverage
	if c.Kind != "concepts" && c.Kind != "vocabulary" && c.Kind != "procedure" && c.Kind != "complete_set" && c.Kind != "exact_text" {
		return fmt.Errorf("%w: unsupported coverage task", ErrInvalid)
	}
	if (c.Complete && len(c.Missing) != 0) || (!c.Complete && len(c.Missing) == 0) || len(c.Missing) > 60 {
		return fmt.Errorf("%w: inconsistent coverage completion report", ErrInvalid)
	}
	missingSeen := map[string]bool{}
	for _, missing := range c.Missing {
		if err := validText("missing coverage", missing, 512, true); err != nil {
			return err
		}
		if missingSeen[missing] {
			return fmt.Errorf("%w: repeated missing coverage", ErrInvalid)
		}
		missingSeen[missing] = true
	}
	keys := map[string]bool{}
	units := map[string]GeneratedUnit{}
	materials, used, assessed := map[string]bool{}, map[string]bool{}, map[string]bool{}
	add := func(key string) error {
		if !generatedKey.MatchString(key) || keys[key] {
			return fmt.Errorf("%w: invalid or duplicate bundle key", ErrInvalid)
		}
		keys[key] = true
		return nil
	}
	statements := map[string]bool{}
	for _, unit := range result.Units {
		if err := validateUnit(unit); err != nil {
			return err
		}
		if err := add(unit.Key); err != nil {
			return err
		}
		statement := strings.ToLower(strings.TrimSpace(unit.Statement))
		if statements[statement] {
			return fmt.Errorf("%w: duplicate knowledge definition", ErrInvalid)
		}
		statements[statement] = true
		units[unit.Key] = unit
	}
	links := func(ls []GeneratedLink, quiz bool, basis string) error {
		if len(ls) == 0 || len(ls) > MaxMaterialLinks {
			return fmt.Errorf("%w: materials need 1..32 explicit coverage links", ErrInvalid)
		}
		seen := map[string]bool{}
		useful := false
		for _, link := range ls {
			unit, found := units[link.UnitKey]
			key := link.UnitKey + "/" + link.Role
			if !found || !validRole(link.Role) || seen[key] || (!quiz && link.Role == "assesses") {
				return fmt.Errorf("%w: invalid coverage reference", ErrInvalid)
			}
			if basis == "background" && (link.Role == "teaches" || link.Role == "assesses") && unit.Kind != "foundation" {
				return fmt.Errorf("%w: background cannot launder goal targets", ErrInvalid)
			}
			seen[key], used[link.UnitKey] = true, true
			useful = useful || (quiz && link.Role == "assesses") || (!quiz && link.Role == "teaches")
			if link.Role == "assesses" {
				assessed[link.UnitKey] = true
			}
		}
		if !useful {
			return fmt.Errorf("%w: material lacks direct useful coverage", ErrInvalid)
		}
		return nil
	}
	prompts := map[string]bool{}
	for _, q := range result.Quizzes {
		if err := add(q.Key); err != nil {
			return err
		}
		materials[q.Key] = true
		if q.Level != "foundation" && q.Level != "target" && q.Level != "extension" {
			return fmt.Errorf("%w: invalid quiz level", ErrInvalid)
		}
		if q.EstimatedSeconds < 1 || q.EstimatedSeconds > 3600 {
			return fmt.Errorf("%w: quiz time outside bound", ErrInvalid)
		}
		if err := links(q.Links, true, q.Basis); err != nil {
			return err
		}
		if q.ReuseID == "" {
			if err := validateQuiz(q, src); err != nil {
				return err
			}
		}
		key := strings.TrimSpace(q.Prompt)
		if prompts[key] {
			return fmt.Errorf("%w: duplicate generated prompt", ErrInvalid)
		}
		prompts[key] = true
		if c.Kind == "exact_text" && q.Level == "target" && (q.Kind != "recall" || len(q.Variants) > 0 || q.Basis != "source" || !strings.Contains(src.Text, q.Answer)) {
			return fmt.Errorf("%w: exact-text target changed source text", ErrInvalid)
		}
	}
	seenContent := map[string]bool{}
	for _, m := range result.Materials {
		if err := add(m.Key); err != nil {
			return err
		}
		materials[m.Key] = true
		if err := links(m.Links, false, m.Basis); err != nil {
			return err
		}
		key := m.Kind + "/" + strings.ToLower(strings.TrimSpace(m.Body)) + "/" + m.ReferenceURL
		if seenContent[key] {
			return fmt.Errorf("%w: duplicate material", ErrInvalid)
		}
		seenContent[key] = true
		if m.Basis == "reference" && c.Complete {
			return fmt.Errorf("%w: unavailable reference content must appear in missing coverage", ErrInvalid)
		}
	}
	seenRelations := map[string]bool{}
	dependencies := map[string][]string{}
	for _, relation := range result.Relations {
		_, from := units[relation.From]
		to, hasTo := units[relation.To]
		key := relation.From + "/" + relation.To + "/" + relation.Kind
		if !from || !hasTo || relation.From == relation.To || !validRelationKind(relation.Kind) || seenRelations[key] {
			return fmt.Errorf("%w: invalid relation", ErrInvalid)
		}
		if relation.Kind == "composition" && to.Kind != "composition" {
			return fmt.Errorf("%w: composition edge requires an integrated target", ErrInvalid)
		}
		if err := validText("relation evidence", relation.Evidence, 8192, true); err != nil {
			return err
		}
		if !strings.HasPrefix(relation.Evidence, "Proposed relationship:") && (src.Kind != "source" || !strings.Contains(src.Text, relation.Evidence)) {
			return fmt.Errorf("%w: relationship must retain exact evidence or explicit proposed status", ErrInvalid)
		}
		seenRelations[key], used[relation.From], used[relation.To] = true, true, true
		if relation.Kind != "contrast" {
			dependencies[relation.From] = append(dependencies[relation.From], relation.To)
		}
	}
	visiting, visited := map[string]bool{}, map[string]bool{}
	var cyclic func(string) bool
	cyclic = func(id string) bool {
		if visiting[id] {
			return true
		}
		if visited[id] {
			return false
		}
		visiting[id] = true
		for _, next := range dependencies[id] {
			if cyclic(next) {
				return true
			}
		}
		visiting[id] = false
		visited[id] = true
		return false
	}
	for id := range units {
		if cyclic(id) {
			return fmt.Errorf("%w: cyclic proposed dependencies", ErrInvalid)
		}
	}
	for _, suggestion := range result.Suggestions {
		if err := add(suggestion.Key); err != nil {
			return err
		}
		if suggestion.Kind != "advance" && suggestion.Kind != "lateral" {
			return fmt.Errorf("%w: invalid suggestion kind", ErrInvalid)
		}
		if err := validText("suggestion title", suggestion.Title, 240, true); err != nil {
			return err
		}
		if err := validText("suggestion reason", suggestion.Reason, 2048, true); err != nil {
			return err
		}
		if len(suggestion.UnitKeys) == 0 || len(suggestion.UnitKeys) > 32 || len(suggestion.MaterialKeys) > 32 {
			return fmt.Errorf("%w: suggestion needs bounded represented scope", ErrInvalid)
		}
		seen := map[string]bool{}
		for _, key := range suggestion.UnitKeys {
			if _, found := units[key]; !found || seen[key] {
				return fmt.Errorf("%w: invalid suggestion unit", ErrInvalid)
			}
			seen[key], used[key] = true, true
		}
		for _, key := range suggestion.MaterialKeys {
			if !materials[key] || seen[key] {
				return fmt.Errorf("%w: invalid suggestion material", ErrInvalid)
			}
			seen[key] = true
		}
	}
	for key := range units {
		if !used[key] {
			return fmt.Errorf("%w: unconnected knowledge definition", ErrInvalid)
		}
		if c.Complete && (c.Kind == "complete_set" || c.Kind == "exact_text") && !assessed[key] {
			return fmt.Errorf("%w: finite complete coverage lacks an exact target assessment", ErrInvalid)
		}
	}
	return nil
}

func enqueueKnowledge(ctx context.Context, tx *sql.Tx, sourceID string, revision int, kind, targetID string, targetVersion int, presentationID string, presentationVersion int, now int64) (string, error) {
	goalID, err := ensureGoal(ctx, tx, sourceID, now)
	if err != nil {
		return "", err
	}
	var goalRevision int
	if err = tx.QueryRowContext(ctx, "SELECT revision FROM goals WHERE id=?", goalID).Scan(&goalRevision); err != nil {
		return "", err
	}
	var existing string
	err = tx.QueryRowContext(ctx, `SELECT id FROM jobs WHERE source_id=? AND source_revision=? AND goal_id=? AND goal_revision=? AND kind=? AND target_material_id=? AND target_material_version=? AND target_presentation_id=? AND status IN('queued','running','retry','paused')`, sourceID, revision, goalID, goalRevision, kind, targetID, targetVersion, presentationID).Scan(&existing)
	if err == nil {
		return existing, nil
	}
	if err != sql.ErrNoRows {
		return "", err
	}
	id := newID()
	_, err = tx.ExecContext(ctx, `INSERT INTO jobs(id,source_id,source_revision,kind,goal_id,goal_revision,target_material_id,target_material_version,target_presentation_id,target_presentation_version,status,created_at,updated_at,available_at) VALUES(?,?,?,?,?,?,?,?,?,?,'queued',?,?,?)`, id, sourceID, revision, kind, goalID, goalRevision, targetID, targetVersion, presentationID, presentationVersion, now, now, now)
	return id, err
}

func generationContext(ctx context.Context, tx *sql.Tx, j Job, now int64) (KnowledgeContext, error) {
	c := KnowledgeContext{
		Version: KnowledgeContextVersion, GoalID: j.GoalID, GoalRevision: j.GoalRevision, AsOf: time.UnixMilli(now),
		Units: []KnowledgeUnit{}, Relations: []KnowledgeRelation{}, Materials: []Material{}, References: []string{}, Evidence: []learning.Evidence{},
	}
	var settings string
	if err := tx.QueryRowContext(ctx, "SELECT title,settings_json FROM goals WHERE id=?", j.GoalID).Scan(&c.GoalTitle, &settings); err != nil {
		return c, err
	}
	if err := json.Unmarshal([]byte(settings), &c.Settings); err != nil {
		return c, err
	}
	check := c.Settings
	check.ExpectedRevision = j.GoalRevision
	if err := validatePlan(check); err != nil {
		return c, err
	}
	c.Request = map[string]string{
		"capture": "Represent this chosen goal, including its foundations, instruction and appropriate practice",
		"enrich":  "Map compatible existing unmapped material without inventing historical assessment coverage",
		"bridge":  "The learner requested prerequisite instruction and appropriate practice before returning to the retained target",
		"expand":  "Offer only the explicitly accepted advanced or lateral scope within this existing goal",
	}[j.Kind]
	if j.ObservationID != "" {
		c.Request = "Prepare support for the retained task after original direct failure " + j.ObservationID + ". Preserve uncertain component attribution; this does not establish failure of every linked unit. Reuse appropriate saved instruction or probes before creating new material."
	}
	if j.Kind == "expand" {
		proposal, err := suggestion(ctx, tx, j.SuggestionID)
		if err != nil {
			return c, err
		}
		c.Suggestion = &proposal
		c.Request += ". Accepted proposal " + proposal.ID + ": " + proposal.Reason
	}
	header, err := marshal(c)
	if err != nil {
		return c, err
	}
	bytes := len(header)
	fits := func(value any) bool {
		encoded, err := json.Marshal(value)
		if err != nil || bytes+len(encoded) > 63*1024 {
			return false
		}
		bytes += len(encoded)
		return true
	}
	if j.ObservationID != "" {
		ids, err := marshal([]string{j.ObservationID})
		if err != nil {
			return c, err
		}
		trigger, err := selectedKnowledgeEvidence(ctx, tx, now, "", ids)
		if err != nil {
			return c, err
		}
		if len(trigger) != 1 || trigger[0].ID != j.ObservationID || trigger[0].Kind != "review" || trigger[0].Rating != 1 || trigger[0].Outcome != "wrong" || trigger[0].Assisted || trigger[0].Disputed || len(trigger[0].CorrectionIDs) > 0 {
			return c, fmt.Errorf("%w: original direct failure is unavailable or corrected; no external work claimed", ErrConflict)
		}
		if !fits(trigger[0]) {
			return c, fmt.Errorf("%w: original observation exceeds the bounded context", ErrInvalid)
		}
		c.Evidence = append(c.Evidence, trigger[0])
	}
	seenUnits := map[string]bool{}
	addRequiredUnit := func(pin learning.UnitVersion) error {
		if seenUnits[pin.ID] {
			return nil
		}
		unit, err := knowledgeUnitVersion(ctx, tx, pin.ID, pin.Version)
		if err != nil {
			return err
		}
		if len(c.Units) >= 48 || !fits(unit) {
			return fmt.Errorf("%w: required scope exceeds bounded context; no external request claimed", ErrInvalid)
		}
		c.Units = append(c.Units, unit)
		seenUnits[unit.ID] = true
		return nil
	}
	targetMapped := false
	if j.TargetMaterialID != "" {
		target, err := materialVersion(ctx, tx, j.TargetMaterialID, j.TargetMaterialVersion)
		if err != nil {
			return c, err
		}
		targetMapped = len(target.Links) > 0
		if j.TargetPresentationID != "" {
			if err = tx.QueryRowContext(ctx, "SELECT goal_id,goal_revision FROM presentations WHERE id=?", j.TargetPresentationID).Scan(&target.GoalID, &target.GoalRevision); err != nil {
				return c, err
			}
		}
		if !fits(target) {
			return c, fmt.Errorf("%w: retained target exceeds bounded context", ErrInvalid)
		}
		c.Materials = append(c.Materials, target)
		for _, link := range target.Links {
			if err = addRequiredUnit(learning.UnitVersion{ID: link.UnitID, Version: link.UnitVersion}); err != nil {
				return c, err
			}
		}
	}
	if c.Suggestion != nil {
		for _, pin := range c.Suggestion.UnitVersions {
			if err = addRequiredUnit(pin); err != nil {
				return c, err
			}
		}
	}
	// Bound database work as well as output: omitted totals are SQL counts,
	// never a reason to hydrate the remainder of the library under a claim.
	var totalUnits, totalMaterials, totalRelations int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM knowledge_units WHERE archived=0").Scan(&totalUnits); err != nil {
		return c, err
	}
	ids, err := rowIDs(ctx, tx, `SELECT u.id FROM knowledge_units u WHERE u.archived=0
	 ORDER BY EXISTS(SELECT 1 FROM goal_units gu WHERE gu.unit_id=u.id AND gu.goal_id=?) DESC,u.created_at DESC,u.id LIMIT 48`, j.GoalID)
	if err != nil {
		return c, err
	}
	for _, id := range ids {
		if len(c.Units) >= 48 {
			break
		}
		if seenUnits[id] {
			continue
		}
		unit, err := knowledgeUnit(ctx, tx, id)
		if err != nil {
			return c, err
		}
		if !fits(unit) {
			continue
		}
		c.Units = append(c.Units, unit)
		seenUnits[id] = true
	}
	c.Omitted.Units = max(0, totalUnits-len(c.Units))
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM materials m JOIN sources s ON s.id=m.source_id WHERE m.archived=0 AND s.archived=0").Scan(&totalMaterials); err != nil {
		return c, err
	}
	ids, err = rowIDs(ctx, tx, `SELECT m.id FROM materials m JOIN sources s ON s.id=m.source_id WHERE m.archived=0 AND s.archived=0
	 ORDER BY (m.source_id=?) DESC,m.created_at DESC,m.id LIMIT 24`, j.SourceID)
	if err != nil {
		return c, err
	}
	for _, id := range ids {
		if len(c.Materials) >= 24 {
			break
		}
		if id == j.TargetMaterialID {
			continue
		}
		m, err := material(ctx, tx, id)
		if err != nil {
			return c, err
		}
		if fits(m) {
			c.Materials = append(c.Materials, m)
		}
	}
	c.Omitted.Materials = max(0, totalMaterials-len(c.Materials))
	refs := sourceReferences(j.SourceText)
	for _, m := range c.Materials {
		if m.ReferenceURL != "" {
			refs = append(refs, m.ReferenceURL)
		}
	}
	seenReferences := map[string]bool{}
	for _, ref := range refs {
		if seenReferences[ref] {
			continue
		}
		seenReferences[ref] = true
		if len(c.References) >= 16 || !fits(ref) {
			c.Omitted.References++
			continue
		}
		c.References = append(c.References, ref)
	}
	unitIDs := []string{}
	pins := []learning.UnitVersion{}
	for _, unit := range c.Units {
		unitIDs = append(unitIDs, unit.ID)
		pins = append(pins, learning.UnitVersion{ID: unit.ID, Version: unit.Version})
	}
	unitJSON, err := marshal(unitIDs)
	if err != nil {
		return c, err
	}
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM knowledge_relations WHERE archived=0").Scan(&totalRelations); err != nil {
		return c, err
	}
	ids, err = rowIDs(ctx, tx, `SELECT r.id FROM knowledge_relations r JOIN relation_versions v ON v.relation_id=r.id AND v.version=r.version
	 WHERE r.archived=0 AND v.from_id IN(SELECT value FROM json_each(?)) AND v.to_id IN(SELECT value FROM json_each(?))
	 ORDER BY r.created_at DESC,r.id LIMIT 64`, unitJSON, unitJSON)
	if err != nil {
		return c, err
	}
	for _, id := range ids {
		r, err := relation(ctx, tx, id)
		if err != nil {
			return c, err
		}
		if fits(r) {
			c.Relations = append(c.Relations, r)
		}
	}
	c.Omitted.Relations = max(0, totalRelations-len(c.Relations))
	evidence, omitted, err := knowledgeContextEvidence(ctx, tx, pins, now, 24)
	if err != nil {
		return c, err
	}
	c.Omitted.Evidence = omitted
	if j.ObservationID != "" && targetMapped {
		inRecent := false
		for _, event := range evidence {
			inRecent = inRecent || event.ID == j.ObservationID
		}
		if !inRecent && c.Omitted.Evidence > 0 {
			c.Omitted.Evidence--
		}
	}
	for _, event := range evidence {
		if event.ID == j.ObservationID {
			continue
		}
		if len(c.Evidence) >= 24 || !fits(event) {
			c.Omitted.Evidence++
			continue
		}
		c.Evidence = append(c.Evidence, event)
	}
	return c, nil
}

func validateReuse(ctx context.Context, tx *sql.Tx, j Job, result GenerationResult) error {
	src := Source{ID: j.SourceID, Text: j.SourceText, Kind: j.SourceKind, Revision: j.SourceRevision}
	for _, u := range result.Units {
		if u.ReuseID == "" {
			continue
		}
		found := false
		for _, old := range j.Context.Units {
			if old.ID != u.ReuseID {
				continue
			}
			current, err := knowledgeUnit(ctx, tx, old.ID)
			if err != nil {
				return err
			}
			if current.Archived || current.Version != old.Version {
				return fmt.Errorf("%w: reused knowledge changed", ErrConflict)
			}
			found = u.Statement == old.Statement && u.Kind == old.Kind
		}
		if !found {
			return fmt.Errorf("%w: reuse must name exact supplied knowledge", ErrInvalid)
		}
	}
	get := func(id string) (Material, error) {
		for _, m := range j.Context.Materials {
			if m.ID == id {
				current, err := material(ctx, tx, id)
				if err != nil {
					return Material{}, err
				}
				if current.Archived || current.Version != m.Version {
					return Material{}, fmt.Errorf("%w: reused material changed", ErrConflict)
				}
				return m, nil
			}
		}
		return Material{}, fmt.Errorf("%w: material reuse not in supplied context", ErrInvalid)
	}
	seen := map[string]bool{}
	for _, q := range result.Quizzes {
		if q.ReuseID == "" {
			continue
		}
		if seen[q.ReuseID] {
			return fmt.Errorf("%w: duplicate material reuse", ErrInvalid)
		}
		seen[q.ReuseID] = true
		m, err := get(q.ReuseID)
		if err != nil {
			return err
		}
		if m.Kind != "quiz" || m.Quiz == nil {
			return fmt.Errorf("%w: reuse is not assessment material", ErrInvalid)
		}
		old := m.Quiz
		if q.Kind != old.Kind || q.Prompt != old.Prompt || q.Answer != old.Answer || q.Explanation != old.Explanation || q.Basis != old.Basis || q.Evidence != old.Evidence || !slices.Equal(q.Choices, old.Choices) || !slices.Equal(q.Variants, old.Variants) || q.Level != m.Level || q.EstimatedSeconds != m.EstimatedSeconds {
			return fmt.Errorf("%w: reused assessment content must remain unchanged", ErrInvalid)
		}
	}
	for _, draft := range result.Materials {
		if draft.ReuseID == "" {
			if err := validateMaterial(draft, src, j.Context.References); err != nil {
				return err
			}
			if draft.StartSeconds != 0 || draft.EndSeconds != 0 {
				supported := false
				for _, m := range j.Context.Materials {
					supported = supported || (m.ReferenceURL == draft.ReferenceURL && m.StartSeconds == draft.StartSeconds && m.EndSeconds == draft.EndSeconds)
				}
				if !supported {
					return fmt.Errorf("%w: video segment was not supplied by a saved reference", ErrInvalid)
				}
			}
			continue
		}
		if seen[draft.ReuseID] {
			return fmt.Errorf("%w: duplicate material reuse", ErrInvalid)
		}
		seen[draft.ReuseID] = true
		m, err := get(draft.ReuseID)
		if err != nil {
			return err
		}
		if draft.Kind != m.Kind || draft.Title != m.Title || draft.Body != m.Body || draft.Basis != m.Basis || draft.Evidence != m.Evidence || draft.ReferenceURL != m.ReferenceURL || draft.StartSeconds != m.StartSeconds || draft.EndSeconds != m.EndSeconds || draft.EstimatedSeconds != m.EstimatedSeconds || !reflect.DeepEqual(draft.Diagram, m.Diagram) {
			return fmt.Errorf("%w: reused reference content must remain unchanged", ErrInvalid)
		}
	}
	return nil
}

func publishKnowledge(ctx context.Context, tx *sql.Tx, j Job, r GenerationResult, now int64) (int, int, int, error) {
	provenance := Provenance{SourceID: j.SourceID, SourceRevision: j.SourceRevision, JobID: j.ID, Model: r.Model, PromptVersion: r.PromptVersion, Basis: "background", Reason: "Authored generation bundle; modeled definitions are not observations", CreatedAt: now}
	units := map[string]KnowledgeUnit{}
	for _, draft := range r.Units {
		var unit KnowledgeUnit
		var err error
		if draft.ReuseID != "" {
			unit, err = knowledgeUnit(ctx, tx, draft.ReuseID)
			if err != nil {
				return 0, 0, 0, err
			}
		} else {
			unit = KnowledgeUnit{ID: newID(), Version: 1, Statement: draft.Statement, Kind: draft.Kind, Provenance: provenance, CreatedAt: now}
			encoded, err := marshal(provenance)
			if err != nil {
				return 0, 0, 0, err
			}
			if _, err = tx.ExecContext(ctx, "INSERT INTO knowledge_units(id,version,created_at) VALUES(?,1,?)", unit.ID, now); err != nil {
				return 0, 0, 0, err
			}
			if _, err = tx.ExecContext(ctx, "INSERT INTO unit_versions(unit_id,version,statement,kind,provenance_json,created_at) VALUES(?,1,?,?,?,?)", unit.ID, unit.Statement, unit.Kind, encoded, now); err != nil {
				return 0, 0, 0, err
			}
		}
		units[draft.Key] = unit
		if _, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO goal_units(goal_id,unit_id,unit_version,origin_job_id,created_at) VALUES(?,?,?,?,?)", j.GoalID, unit.ID, unit.Version, j.ID, now); err != nil {
			return 0, 0, 0, err
		}
	}
	for _, draft := range r.Relations {
		from, to := units[draft.From], units[draft.To]
		var exists bool
		err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM knowledge_relations r JOIN relation_versions v ON v.relation_id=r.id AND v.version=r.version WHERE r.archived=0 AND v.from_id=? AND v.from_version=? AND v.to_id=? AND v.to_version=? AND v.kind=?)`, from.ID, from.Version, to.ID, to.Version, draft.Kind).Scan(&exists)
		if err != nil {
			return 0, 0, 0, err
		}
		if exists {
			continue
		}
		id := newID()
		p := provenance
		p.Evidence = draft.Evidence
		encoded, err := marshal(p)
		if err != nil {
			return 0, 0, 0, err
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO knowledge_relations(id,version,created_at) VALUES(?,1,?)", id, now); err != nil {
			return 0, 0, 0, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO relation_versions(relation_id,version,from_id,from_version,to_id,to_version,kind,proposed,provenance_json,created_at) VALUES(?,1,?,?,?,?,?,1,?,?)`, id, from.ID, from.Version, to.ID, to.Version, draft.Kind, encoded, now); err != nil {
			return 0, 0, 0, err
		}
	}
	materialIDs := map[string]string{}
	newMaterials, reused, newQuizzes := 0, 0, 0
	publish := func(key, reuse string, m Material, generatedLinks []GeneratedLink, index int) error {
		links := make([]CoverageLink, 0, len(generatedLinks))
		for _, link := range generatedLinks {
			unit := units[link.UnitKey]
			links = append(links, CoverageLink{UnitID: unit.ID, UnitVersion: unit.Version, Role: link.Role, Provenance: provenance})
		}
		if reuse != "" {
			old, err := material(ctx, tx, reuse)
			if err != nil {
				return err
			}
			m = old
			merged := append([]CoverageLink{}, old.Links...)
			for _, link := range links {
				found := false
				for _, prior := range old.Links {
					if prior.UnitID == link.UnitID && prior.UnitVersion != link.UnitVersion {
						return fmt.Errorf("%w: reuse cannot reinterpret an existing unit definition; use explicit coverage correction", ErrConflict)
					}
					found = found || (prior.UnitID == link.UnitID && prior.UnitVersion == link.UnitVersion && prior.Role == link.Role)
				}
				if !found {
					merged = append(merged, link)
				}
			}
			if len(merged) > MaxMaterialLinks {
				return fmt.Errorf("%w: reused coverage union exceeds the explicit link bound", ErrInvalid)
			}
			if len(merged) != len(old.Links) {
				before := m.Version
				m.Version++
				m.Provenance.Reason = "New authored coverage added to reused content; prior coverage and observations remain pinned"
				m.Provenance.CreatedAt = now
				if err = saveMaterialVersion(ctx, tx, m, merged, now); err != nil {
					return err
				}
				if err = appendCorrection(ctx, tx, "coverage", m.ID, before, m.Version, m.Provenance.Reason, now); err != nil {
					return err
				}
				// Existing occurrences retain their immutable earlier coverage.
				// No content changed, so no cold target is silently replaced.
			}
			reused++
		} else {
			m.ID = newID()
			if m.Quiz != nil {
				m.ID = m.Quiz.ID
			}
			m.SourceID, m.Version, m.CreatedAt = j.SourceID, 1, now
			m.Provenance = provenance
			m.Provenance.Basis, m.Provenance.Evidence = m.Basis, m.Evidence
			if _, err := tx.ExecContext(ctx, `INSERT INTO materials(id,source_id,version,kind,created_at,origin_job_id,origin_index) VALUES(?,?,1,?,?,?,?)`, m.ID, m.SourceID, m.Kind, now, j.ID, index); err != nil {
				return err
			}
			if err := saveMaterialVersion(ctx, tx, m, links, now); err != nil {
				return err
			}
			newMaterials++
		}
		materialIDs[key] = m.ID
		_, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO goal_materials(goal_id,material_id,material_version,origin_job_id,created_at) VALUES(?,?,?,?,?)", j.GoalID, m.ID, m.Version, j.ID, now)
		return err
	}
	for index, draft := range r.Materials {
		m := Material{Kind: draft.Kind, Level: "foundation", Title: draft.Title, Body: draft.Body, Basis: draft.Basis, Evidence: draft.Evidence, ReferenceURL: draft.ReferenceURL, StartSeconds: draft.StartSeconds, EndSeconds: draft.EndSeconds, EstimatedSeconds: draft.EstimatedSeconds, Diagram: draft.Diagram}
		if err := publish(draft.Key, draft.ReuseID, m, draft.Links, index); err != nil {
			return 0, 0, 0, err
		}
	}
	for index, draft := range r.Quizzes {
		m := Material{Kind: "quiz", Level: draft.Level, Title: draft.Prompt, Basis: draft.Basis, Evidence: draft.Evidence, EstimatedSeconds: draft.EstimatedSeconds}
		if draft.ReuseID == "" {
			id := newID()
			encoded, err := marshal(draft)
			if err != nil {
				return 0, 0, 0, err
			}
			card, err := marshal(learning.NewCard(time.UnixMilli(now)))
			if err != nil {
				return 0, 0, 0, err
			}
			if _, err = tx.ExecContext(ctx, "INSERT INTO quizzes(id,source_id,version,created_at,origin_job_id,origin_index) VALUES(?,?,1,?,?,?)", id, j.SourceID, now, j.ID, index); err != nil {
				return 0, 0, 0, err
			}
			if _, err = tx.ExecContext(ctx, "INSERT INTO quiz_versions(quiz_id,version,content,model,prompt_version,created_at) VALUES(?,1,?,?,?,?)", id, encoded, r.Model, r.PromptVersion, now); err != nil {
				return 0, 0, 0, err
			}
			if _, err = tx.ExecContext(ctx, "INSERT INTO schedules(quiz_id,version,card,due_at,algorithm) VALUES(?,1,?,?,?)", id, card, now, learning.Algorithm); err != nil {
				return 0, 0, 0, err
			}
			q, err := quiz(ctx, tx, id)
			if err != nil {
				return 0, 0, 0, err
			}
			m.Quiz = &q
			newQuizzes++
		}
		if err := publish(draft.Key, draft.ReuseID, m, draft.Links, index); err != nil {
			return 0, 0, 0, err
		}
	}
	for _, draft := range r.Suggestions {
		uids, mids := []string{}, []string{}
		unitVersions, materialVersions := []learning.UnitVersion{}, []learning.UnitVersion{}
		for _, key := range draft.UnitKeys {
			unit := units[key]
			uids = append(uids, unit.ID)
			unitVersions = append(unitVersions, learning.UnitVersion{ID: unit.ID, Version: unit.Version})
		}
		for _, key := range draft.MaterialKeys {
			id := materialIDs[key]
			var version int
			if err := tx.QueryRowContext(ctx, "SELECT version FROM materials WHERE id=?", id).Scan(&version); err != nil {
				return 0, 0, 0, err
			}
			mids = append(mids, id)
			materialVersions = append(materialVersions, learning.UnitVersion{ID: id, Version: version})
		}
		a, err := marshal(uids)
		if err != nil {
			return 0, 0, 0, err
		}
		b, err := marshal(mids)
		if err != nil {
			return 0, 0, 0, err
		}
		u, err := marshal(unitVersions)
		if err != nil {
			return 0, 0, 0, err
		}
		m, err := marshal(materialVersions)
		if err != nil {
			return 0, 0, 0, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO suggestions(id,goal_id,goal_revision,kind,title,reason,unit_ids_json,material_ids_json,unit_versions_json,material_versions_json,origin_job_id,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, newID(), j.GoalID, j.GoalRevision, draft.Kind, draft.Title, draft.Reason, a, b, u, m, j.ID, now); err != nil {
			return 0, 0, 0, err
		}
	}
	coverage, err := marshal(r.Coverage)
	if err != nil {
		return 0, 0, 0, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE goals SET coverage_json=?,updated_at=? WHERE id=? AND revision=?", coverage, now, j.GoalID, j.GoalRevision); err != nil {
		return 0, 0, 0, err
	}
	if err = populatePendingBridge(ctx, tx, "", now); err != nil {
		return 0, 0, 0, err
	}
	return newMaterials, reused, newQuizzes, nil
}
