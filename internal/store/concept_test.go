package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

// TestSchema2To3MigrationPreservesFoundationAndPopulatesConceptsUS001 tests US-001 Criterion 1:
// WHEN the database upgrades from schema version 2, THE SYSTEM SHALL preserve all existing foundation rows
// and map units to concepts, materials to references, and foundation links to concept relations.
func TestSchema2To3MigrationPreservesFoundationAndPopulatesConceptsUS001(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "migration.sqlite")
	ctx := context.Background()

	// 1. Initialize SQLite with Schema 1 and Schema 2
	rawDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}

	if _, err = rawDB.ExecContext(ctx, schemaV1); err != nil {
		t.Fatal(err)
	}
	if _, err = rawDB.ExecContext(ctx, schemaV2); err != nil {
		t.Fatal(err)
	}

	// 2. Insert sample foundation data into Schema 2 database
	const insertData = `
INSERT INTO sources (id, text, kind, revision, created_at)
VALUES ('src_calvin', 'Calvin cycle text', 'topic', 1, 1000);
INSERT INTO source_revisions (source_id, revision, text, kind, created_at)
VALUES ('src_calvin', 1, 'Calvin cycle text', 'topic', 1000);
INSERT INTO jobs (id, source_id, source_revision, status, created_at, updated_at, available_at)
VALUES ('job_calvin', 'src_calvin', 1, 'complete', 1000, 1000, 1000);
INSERT INTO quizzes (id, source_id, version, created_at, origin_job_id, origin_index)
VALUES ('quiz_calvin', 'src_calvin', 1, 1000, 'job_calvin', 0);
INSERT INTO quiz_versions (quiz_id, version, content, model, prompt_version, created_at)
VALUES ('quiz_calvin', 1, '{"prompt":"Where does Calvin cycle occur?","answer":"Stroma"}', 'gpt', 'v1', 1000);
INSERT INTO foundation_requests (job_id, quiz_id, quiz_version)
VALUES ('job_calvin', 'quiz_calvin', 1);
INSERT INTO foundation_bundles (id, job_id, source_id, source_revision, quiz_id, quiz_version, model, prompt_version, note, created_at)
VALUES ('bundle_calvin', 'job_calvin', 'src_calvin', 1, 'quiz_calvin', 1, 'gemini', 'v1', 'calvin bundle', 1050);
INSERT INTO foundation_units (id, version, definition, kind, provenance)
VALUES ('unit_stroma', 1, 'The fluid-filled space surrounding the grana in chloroplasts.', 'foundation', 'model');
INSERT INTO foundation_materials (id, version, bundle_id, position, content, provenance)
VALUES ('mat_stroma_exp', 1, 'bundle_calvin', 0, '{"title":"Chloroplast Stroma Structure","kind":"explanation","body":"The stroma contains enzymes..."}', 'model');
INSERT INTO foundation_links (material_id, material_version, unit_id, unit_version, role, provenance)
VALUES ('mat_stroma_exp', 1, 'unit_stroma', 1, 'teaches', 'model');
`
	if _, err = rawDB.ExecContext(ctx, insertData); err != nil {
		t.Fatal(err)
	}
	rawDB.Close()

	// 3. Open via Open() — this must migrate from Schema 2 to Schema 3
	st, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer st.Close()

	// 4. Verify all foundation rows are preserved
	var unitCount, matCount, linkCount, bundleCount int
	rawDB2, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer rawDB2.Close()

	if err = rawDB2.QueryRowContext(ctx, "SELECT count(*) FROM foundation_units").Scan(&unitCount); err != nil || unitCount != 1 {
		t.Fatalf("expected 1 foundation_unit preserved, got %d, err: %v", unitCount, err)
	}
	if err = rawDB2.QueryRowContext(ctx, "SELECT count(*) FROM foundation_materials").Scan(&matCount); err != nil || matCount != 1 {
		t.Fatalf("expected 1 foundation_material preserved, got %d, err: %v", matCount, err)
	}
	if err = rawDB2.QueryRowContext(ctx, "SELECT count(*) FROM foundation_links").Scan(&linkCount); err != nil || linkCount != 1 {
		t.Fatalf("expected 1 foundation_link preserved, got %d, err: %v", linkCount, err)
	}
	if err = rawDB2.QueryRowContext(ctx, "SELECT count(*) FROM foundation_bundles").Scan(&bundleCount); err != nil || bundleCount != 1 {
		t.Fatalf("expected 1 foundation_bundle preserved, got %d, err: %v", bundleCount, err)
	}

	// 5. Verify concepts, references, concept_references, and concept_quizzes are populated
	concept, err := st.GetConcept(ctx, "unit_stroma")
	if err != nil {
		t.Fatalf("GetConcept failed: %v", err)
	}
	if concept.Name != "unit_stroma" {
		t.Errorf("expected concept name unit_stroma, got %q", concept.Name)
	}
	if concept.Description != "The fluid-filled space surrounding the grana in chloroplasts." {
		t.Errorf("expected concept description from definition, got %q", concept.Description)
	}
	if len(concept.References) != 1 {
		t.Fatalf("expected 1 linked reference, got %d", len(concept.References))
	}
	if concept.References[0].ID != "mat_stroma_exp" {
		t.Errorf("expected reference id mat_stroma_exp, got %q", concept.References[0].ID)
	}
	if concept.References[0].Title != "Chloroplast Stroma Structure" {
		t.Errorf("expected reference title 'Chloroplast Stroma Structure', got %q", concept.References[0].Title)
	}
	if len(concept.Quizzes) != 1 {
		t.Fatalf("expected 1 linked quiz, got %d", len(concept.Quizzes))
	}
	if concept.Quizzes[0].ID != "quiz_calvin" {
		t.Errorf("expected quiz id quiz_calvin, got %q", concept.Quizzes[0].ID)
	}
}

// TestSearchConceptsAndReferencesUS001 tests US-001 Criteria 2 & 4:
// WHEN I search concepts or references by text, THE SYSTEM SHALL return matching concepts and references with their linked counterparts.
// IF a search query matches no concepts or references, THE SYSTEM SHALL return an empty result without error.
func TestSearchConceptsAndReferencesUS001(t *testing.T) {
	t.Parallel()
	st, _ := newTestStore(t)
	ctx := context.Background()

	// 1. Save two concepts
	c1 := Concept{
		ID:          "mitochondria",
		Name:        "Mitochondria",
		Description: "The powerhouse of the cell responsible for ATP synthesis through cellular respiration.",
		CreatedAt:   1000,
	}
	c2 := Concept{
		ID:          "chloroplast",
		Name:        "Chloroplast",
		Description: "The organelle in plant cells where photosynthesis occurs.",
		CreatedAt:   1001,
	}
	if err := st.SaveConcept(ctx, c1); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveConcept(ctx, c2); err != nil {
		t.Fatal(err)
	}

	// 2. Save references linked to concepts
	ref1 := Reference{
		ID:        "ref_atp_synth",
		Title:     "Mechanism of ATP Synthase",
		Content:   "Detailed breakdown of electrochemical proton gradient driving ATP generation in mitochondrial cristae.",
		Format:    "explanation",
		CreatedAt: 1002,
	}
	if err := st.SaveReference(ctx, ref1, []string{"mitochondria"}); err != nil {
		t.Fatal(err)
	}

	ref2 := Reference{
		ID:        "ref_thylakoid",
		Title:     "Thylakoid Membrane Diagram",
		Content:   "Visual schematics of light-dependent reaction complexes inside chloroplast thylakoids.",
		Format:    "diagram",
		CreatedAt: 1003,
	}
	if err := st.SaveReference(ctx, ref2, []string{"chloroplast"}); err != nil {
		t.Fatal(err)
	}

	// Criterion 2: Search matching "ATP" finds concept (via description) and reference (via title and content)
	results, err := st.SearchConceptsAndReferences(ctx, "ATP")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results.Concepts) != 1 || results.Concepts[0].ID != "mitochondria" {
		t.Errorf("expected 1 concept 'mitochondria', got %d", len(results.Concepts))
	}
	if len(results.References) != 1 || results.References[0].ID != "ref_atp_synth" {
		t.Errorf("expected 1 reference 'ref_atp_synth', got %d", len(results.References))
	}
	// Verify linked counterpart
	if len(results.References[0].Concepts) != 1 || results.References[0].Concepts[0].ID != "mitochondria" {
		t.Errorf("expected reference to have linked counterpart concept 'mitochondria'")
	}

	// Search matching "chloroplast" finds concept and reference
	results2, err := st.SearchConceptsAndReferences(ctx, "chloroplast")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results2.Concepts) != 1 || results2.Concepts[0].ID != "chloroplast" {
		t.Errorf("expected 1 concept 'chloroplast', got %d", len(results2.Concepts))
	}
	if len(results2.References) != 1 || results2.References[0].ID != "ref_thylakoid" {
		t.Errorf("expected 1 reference 'ref_thylakoid', got %d", len(results2.References))
	}

	// Criterion 4: Query matching nothing returns empty result without error
	resultsEmpty, err := st.SearchConceptsAndReferences(ctx, "quantum_electrodynamics_absent_query")
	if err != nil {
		t.Fatalf("expected no error on unmatched query, got: %v", err)
	}
	if len(resultsEmpty.Concepts) != 0 || len(resultsEmpty.References) != 0 {
		t.Errorf("expected 0 concepts and 0 references, got %d concepts, %d references", len(resultsEmpty.Concepts), len(resultsEmpty.References))
	}

	// Query empty string returns empty result without error
	resultsBlank, err := st.SearchConceptsAndReferences(ctx, "   ")
	if err != nil {
		t.Fatalf("expected no error on blank query, got: %v", err)
	}
	if len(resultsBlank.Concepts) != 0 || len(resultsBlank.References) != 0 {
		t.Errorf("expected 0 concepts and 0 references on blank query")
	}
}

// TestGetConceptWithLinkedReferencesQuizzesAndPrerequisitesUS001 tests US-001 Criterion 3:
// WHEN I inspect a concept, THE SYSTEM SHALL return its linked references, quizzes, and prerequisites.
func TestGetConceptWithLinkedReferencesQuizzesAndPrerequisitesUS001(t *testing.T) {
	t.Parallel()
	st, _ := newTestStore(t)
	ctx := context.Background()

	// 1. Create target concept and prerequisite concept
	prereq := Concept{
		ID:          "concept_cell_membrane",
		Name:        "Cell Membrane",
		Description: "Lipid bilayer providing selective permeability to cellular compartments.",
		CreatedAt:   100,
	}
	target := Concept{
		ID:          "concept_active_transport",
		Name:        "Active Transport",
		Description: "Movement of molecules across a cellular membrane against their concentration gradient.",
		CreatedAt:   101,
	}
	if err := st.SaveConcept(ctx, prereq); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveConcept(ctx, target); err != nil {
		t.Fatal(err)
	}

	// 2. Link prerequisite
	if err := st.AddConceptPrerequisite(ctx, "concept_active_transport", "concept_cell_membrane"); err != nil {
		t.Fatal(err)
	}

	// 3. Link reference
	ref := Reference{
		ID:        "ref_na_k_pump",
		Title:     "Sodium-Potassium Pump Mechanics",
		Content:   "ATP hydrolysis energizes the conformational change driving 3 Na+ out and 2 K+ in.",
		Format:    "explanation",
		CreatedAt: 102,
	}
	if err := st.SaveReference(ctx, ref, []string{"concept_active_transport"}); err != nil {
		t.Fatal(err)
	}

	// 4. Create source & quiz, then link quiz to concept
	src := publishFixture(t, st, authoredChoice("How many sodium ions are pumped out per ATP molecule?"))
	quizID := src.Quizzes[0].ID
	if err := st.LinkQuizToConcept(ctx, quizID, "concept_active_transport"); err != nil {
		t.Fatal(err)
	}

	// 5. Inspect concept
	detail, err := st.GetConcept(ctx, "concept_active_transport")
	if err != nil {
		t.Fatalf("GetConcept failed: %v", err)
	}

	// Verify concept core fields
	if detail.ID != "concept_active_transport" || detail.Name != "Active Transport" {
		t.Errorf("unexpected concept identity: %+v", detail.Concept)
	}

	// Verify linked reference
	if len(detail.References) != 1 || detail.References[0].ID != "ref_na_k_pump" {
		t.Errorf("expected linked reference 'ref_na_k_pump', got: %+v", detail.References)
	}

	// Verify linked quiz
	if len(detail.Quizzes) != 1 || detail.Quizzes[0].ID != quizID {
		t.Errorf("expected linked quiz '%s', got: %+v", quizID, detail.Quizzes)
	}

	// Verify prerequisite
	if len(detail.Prerequisites) != 1 || detail.Prerequisites[0].ID != "concept_cell_membrane" {
		t.Errorf("expected prerequisite 'concept_cell_membrane', got: %+v", detail.Prerequisites)
	}

	// Verify reverse lookup: quiz concepts
	quizConcepts, err := st.GetQuizConcepts(ctx, quizID)
	if err != nil {
		t.Fatalf("GetQuizConcepts failed: %v", err)
	}
	if len(quizConcepts) != 1 || quizConcepts[0].ID != "concept_active_transport" {
		t.Errorf("expected quiz to be linked to 'concept_active_transport', got: %+v", quizConcepts)
	}

	// Verify self-prerequisite is rejected
	if err := st.AddConceptPrerequisite(ctx, "concept_active_transport", "concept_active_transport"); err == nil {
		t.Errorf("expected error when adding concept as its own prerequisite, got nil")
	}
}
