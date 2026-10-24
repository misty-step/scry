//! Synthetic, fully validated history for the Materials compatibility boundary.
//! This test-only constructor is not an authoring API or a production import.
use base64::{Engine as _, engine::general_purpose::STANDARD};
use scry::{
    api, engine, generation,
    materials::{Material, MaterialProvenance, MaterialRevision},
    model::*,
    owner_token, persistence, study,
};
use serde_json::{Value, json};
use std::collections::BTreeMap;

pub const NOW: i64 = 1_791_410_400_000;
pub const AUTHOR_CONCEPT_ID: &str = "synthetic-owner-concept";
pub const FIRST_MATERIAL_ID: &str = "synthetic-owner-material-first";
pub const SECOND_MATERIAL_ID: &str = "synthetic-owner-material-second";
pub const FIRST_REVISION_ID: &str = "synthetic-owner-revision-first";
pub const SECOND_REVISION_ID: &str = "synthetic-owner-revision-second";
pub const THIRD_REVISION_ID: &str = "synthetic-owner-revision-third";
pub const LEGACY_MATERIAL_ID: &str = "synthetic-legacy-concept-material";
pub const LEGACY_REVISION_ID: &str = "synthetic-legacy-concept-revision";
pub const REVIVAL_QUESTION_ID: &str = "legacy-import-quiz-synthetic-preserved";
pub const FIRST_TEXT: &str =
    "  Synthetic café notes\r\n\tKeep spacing, naïve accents and e\u{301}.\n";
pub const SECOND_TEXT: &str =
    "  Synthetic café notes, revised\r\n\tOld wording remains immutable.\n\n";
pub const THIRD_TEXT: &str = "A separate reference, not another version of the first.\n";

pub struct MaterialFixture {
    pub baseline: App,
    pub app: App,
    pub baseline_rows: BTreeMap<String, String>,
    pub material_rows: BTreeMap<String, String>,
    pub revival_bytes: Vec<u8>,
    pub photos: Vec<persistence::ArchivedPhoto>,
    pub owner_token_secret: String,
}

struct History {
    app: App,
    sequence: u64,
}

impl History {
    fn mutate(&mut self, path: &str, extra: &[(&str, &str)]) -> AppResult<MutationResult> {
        self.sequence += 1;
        let operation = format!("materials-history-{:04}", self.sequence);
        let mut fields = BTreeMap::from([
            ("csrf".into(), self.app.csrf.clone()),
            ("operation_id".into(), operation.clone()),
        ]);
        if let Some(occurrence) = &self.app.occurrence {
            fields.insert("occurrence_id".into(), occurrence.id.clone());
        }
        for (key, value) in extra {
            fields.insert((*key).into(), (*value).into());
        }
        let result = engine::mutate(
            &mut self.app,
            path,
            &fields,
            NOW + self.sequence as i64,
            &operation,
        )?;
        engine::validate_app(&self.app)?;
        Ok(result)
    }

    fn exact(&mut self, path: &str, mut data: Value) -> AppResult<Value> {
        self.sequence += 1;
        let operation = format!("materials-exact-{:04}", self.sequence);
        data["operation_id"] = json!(operation);
        let mut fields = api::library_fields(data)?;
        fields.insert("csrf".into(), self.app.csrf.clone());
        let result = engine::mutate(
            &mut self.app,
            api::mutation_path(path).expect("known test API path"),
            &fields,
            NOW + self.sequence as i64,
            &operation,
        )?;
        engine::validate_app(&self.app)?;
        Ok(study::wire_response(&self.app, &result)
            .unwrap_or_else(|| json!({"id":result.entity_id})))
    }

    fn next(&mut self, card: Option<&str>) -> AppResult<()> {
        let mut data = json!({});
        if let Some(current) = &self.app.practice.current
            && current.attempt_id.is_some()
        {
            data["after"] = json!(current.prompt.id);
        }
        if let Some(card) = card {
            data["card_id"] = json!(card);
        }
        self.exact("/api/v1/next", data)?;
        Ok(())
    }

    fn answer(&mut self, answer: &str) -> AppResult<Value> {
        let prompt = self
            .app
            .practice
            .current
            .as_ref()
            .unwrap()
            .prompt
            .id
            .clone();
        self.exact(
            "/api/v1/answer",
            json!({"prompt_id":prompt,"answer":answer}),
        )
    }
}

/// Produce old-format history before adding any new Concept or Material.
pub fn material_fixture() -> AppResult<MaterialFixture> {
    let mut history = History {
        app: App::new("materials-synthetic-csrf-at-least-32-characters".into()),
        sequence: 0,
    };
    engine::seed_fixture(&mut history.app, NOW)?;
    engine::validate_app(&history.app)?;
    let answer = history
        .app
        .occurrence
        .as_ref()
        .unwrap()
        .presentation
        .answer
        .clone();
    history.mutate("/review/answer", &[("answer", &answer)])?;
    history.mutate("/review/override", &[("correct", "false")])?;
    let question_id = history.app.events[0].question_id.clone();
    let question = history.app.questions[&question_id].clone();
    history.mutate(
        &format!("/questions/{question_id}/dispute"),
        &[
            ("text", "Synthetic wording issue: reset this question."),
            ("reset", "true"),
            ("content_version", &question.content().version.to_string()),
            ("schedule_version", &question.schedule_version.to_string()),
        ],
    )?;
    let content = question.content();
    history.mutate(
        &format!("/questions/{question_id}/edit"),
        &[
            ("content_version", &content.version.to_string()),
            (
                "prompt",
                &format!("Synthetic revised question: {}", content.prompt),
            ),
            ("kind", &content.kind),
            ("answer", &content.answer),
            ("variants", &content.variants.join("\n")),
            ("choices", &content.choices.join("\n")),
            ("explanation", &content.explanation),
            ("basis", &content.basis),
            ("quotes", &content.quotes.join("\n")),
        ],
    )?;

    // Exercise real generation settlement/publication, with synthetic response
    // bytes only. Existing Concept identities append Notes rather than migrate.
    let mut batch: generation::Batch = serde_json::from_str(
        history.app.jobs["fixture-job"]
            .candidate_json
            .as_deref()
            .unwrap(),
    )
    .unwrap();
    let mapping: BTreeMap<_, _> = batch
        .concepts
        .iter()
        .map(|candidate| {
            let saved = history
                .app
                .concepts
                .values()
                .find(|c| c.name == candidate.name)
                .unwrap();
            (candidate.key.clone(), saved.id.clone())
        })
        .collect();
    for candidate in &mut batch.concepts {
        candidate.key = mapping[&candidate.key].clone();
        candidate.prerequisites = candidate
            .prerequisites
            .iter()
            .map(|id| mapping[id].clone())
            .collect();
        candidate
            .note
            .push_str("\nSynthetic refinement retains the original note.");
    }
    history.mutate(
        "/goals/fixture-http/refine",
        &[(
            "feedback",
            "Add a concrete example while retaining the existing ideas.",
        )],
    )?;
    let config = engine::WorkConfig {
        generation: true,
        jev: true,
        critic: false,
        model: "synthetic-material-history-generator".into(),
        jev_model: "synthetic-material-history-critic".into(),
    };
    history.sequence += 1;
    let work = engine::claim_work(
        &mut history.app,
        NOW + history.sequence as i64,
        "materials-synthetic-generation-lease",
        &config,
    )?
    .expect("synthetic refinement is queued");
    history.sequence += 1;
    let raw = json!({"choices":[{"message":{"content":serde_json::to_string(&batch).unwrap()}}]})
        .to_string();
    engine::finish_work(
        &mut history.app,
        &work,
        raw,
        config.model.clone(),
        Some(12_345),
        NOW + history.sequence as i64,
    )?;
    history.sequence += 1;
    assert!(
        engine::claim_work(
            &mut history.app,
            NOW + history.sequence as i64,
            "materials-synthetic-publish-lease",
            &config,
        )?
        .is_none()
    );
    assert_eq!(history.app.jobs[&work.id].status, "ready");
    engine::validate_app(&history.app)?;

    let supplied = |text| json!({"title":"Synthetic patient garden","kind":"text","name":"Private synthetic wording","text":text});
    let version = history.exact(
        "/api/v1/library/versions",
        json!({"data":supplied("We conceived a patient garden.\n")}),
    )?["id"]
        .as_str()
        .unwrap()
        .to_owned();
    history.exact(
        "/api/v1/learn",
        json!({"version_id":version,"pace":"steady"}),
    )?;
    history.next(None)?;
    let card = history.app.practice.current.as_ref().unwrap().targets[0]
        .card_id
        .clone();
    for _ in 0..6 {
        history.answer("We conceived a patient garden.")?;
        history.next(Some(&card))?;
    }
    assert_eq!(
        history
            .app
            .practice
            .current
            .as_ref()
            .unwrap()
            .prompt
            .cue_level,
        study::Cue::PreviousLine
    );
    let answer = history.answer("We imagined a patient garden.")?;
    history.exact(
        "/api/v1/correct",
        json!({
            "prompt_id":answer["attempt"]["prompt"]["id"],
            "attempt_id":answer["attempt"]["id"],
            "verdict":"Good"
        }),
    )?;
    let work_id = history.app.library.versions[&version].work_id.clone();
    let revision = history.app.library.works[&work_id].revision;
    history.exact(
        "/api/v1/library/versions/supersede",
        json!({
            "version_id":version,"revision":revision,
            "data":supplied("We cultivated a patient garden.\n")
        }),
    )?;

    // Capture a real synthetic PNG through the existing image boundary. There
    // is deliberately no provider: failed preparation keeps the accepted image.
    let photo_bytes = STANDARD.decode("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+j56cAAAAASUVORK5CYII=").unwrap();
    let photo = Photo {
        key: "photos/synthetic-material-history.png".into(),
        mime: "image/png".into(),
        size: photo_bytes.len(),
        sha256: persistence::sha256(&photo_bytes),
    };
    history.mutate(
        "/create",
        &[
            (
                "intent",
                "Retain this synthetic image without provider work.",
            ),
            ("__photo", &serde_json::to_string(&photo).unwrap()),
        ],
    )?;
    let disabled = engine::WorkConfig {
        generation: false,
        jev: false,
        critic: false,
        ..config.clone()
    };
    history.sequence += 1;
    assert!(
        engine::claim_work(
            &mut history.app,
            NOW + history.sequence as i64,
            "materials-disabled-photo-lease",
            &disabled
        )?
        .is_none()
    );
    let photos = vec![persistence::ArchivedPhoto {
        photo,
        bytes: photo_bytes,
    }];

    // A synthetic transmitted operation with no known response retains its
    // actual reservation. No provider is contacted by claim/fail transitions.
    history.mutate(
        "/create",
        &[("intent", "Synthetic unknown generation outcome.")],
    )?;
    history.sequence += 1;
    let unknown = engine::claim_work(
        &mut history.app,
        NOW + history.sequence as i64,
        "materials-synthetic-unknown-lease",
        &config,
    )?
    .expect("synthetic generation is queued");
    history.sequence += 1;
    engine::fail_work(
        &mut history.app,
        &unknown,
        "Synthetic transmission outcome is unknown.",
        true,
        NOW + history.sequence as i64,
    )?;
    engine::validate_app(&history.app)?;

    // Synthetic loopback-only token, never an existing credential. Keep the
    // secret out of App/archive; the explicit exporter passes it to local CLI QA.
    let entropy = "0123456789abcdef0123456789abcdef0123456789abcdef";
    let owner_token_secret = owner_token::issuance_secret(entropy);
    let token =
        owner_token::OwnerToken::issue("Synthetic Materials CLI", None, false, NOW, entropy)?;
    history
        .app
        .owner_tokens
        .insert(token.info.id.clone(), token);

    // Opaque, non-UTF-8 original custody crosses multiple canonical chunks.
    // It is deliberately not projected into new learning or Material records.
    let revival_bytes = [
        b"SYNTHETIC_ORIGINAL_GO_CUSTODY\0".as_slice(),
        &vec![0xff; 100_000],
    ]
    .concat();
    let hash = persistence::sha256(&revival_bytes);
    for (index, chunk) in revival_bytes.chunks(48 * 1024).enumerate() {
        history
            .app
            .revival_go
            .insert(format!("{hash}/{index:08}"), STANDARD.encode(chunk));
    }
    // Model the existing revival reader's explicit epoch convention with an
    // unreviewed typed Question. No event or remembered scheduler state is made up.
    let mut original = history
        .app
        .questions
        .values()
        .find(|question| question.schedule_version == 0)
        .expect("fixture includes an unreviewed question")
        .clone();
    original.id = REVIVAL_QUESTION_ID.into();
    original.schedule_version = 1;
    history.app.questions.insert(original.id.clone(), original);
    history
        .app
        .revival_schedule_origins
        .insert(REVIVAL_QUESTION_ID.into(), 1);
    engine::validate_app(&history.app)?;
    let baseline = history.app;
    let baseline_rows = persistence::rows(&baseline)?;
    let mut app = baseline.clone();
    add_authored_materials(&mut app)?;
    let material_rows = persistence::rows(&app)?;
    Ok(MaterialFixture {
        baseline,
        app,
        baseline_rows,
        material_rows,
        revival_bytes,
        photos,
        owner_token_secret,
    })
}

/// Typed fixture-only insertion. Validate the complete candidate before commit;
/// never manufacture raw rows or use an invalid partial App as restore input.
pub fn add_authored_materials(app: &mut App) -> AppResult<()> {
    engine::validate_app(app)?;
    let mut next = app.clone();
    next.materials.capability = true;
    let concept = Concept {
        id: AUTHOR_CONCEPT_ID.into(),
        goal_id: None,
        origin: Some(ConceptOrigin::OwnerAuthored),
        name: "Synthetic authored idea".into(),
        summary: String::new(),
        notes: vec![],
        prerequisites: vec![],
        introduced_ms: None,
        exposure_ms: None,
        already_known: false,
        archived: false,
    };
    if next.concepts.insert(concept.id.clone(), concept).is_some() {
        return Err(AppError::new(
            409,
            "Synthetic authored Concept already exists.",
        ));
    }
    for (id, current) in [
        (FIRST_MATERIAL_ID, SECOND_REVISION_ID),
        (SECOND_MATERIAL_ID, THIRD_REVISION_ID),
    ] {
        let material = Material {
            id: id.into(),
            concept_id: AUTHOR_CONCEPT_ID.into(),
            current_revision_id: current.into(),
            created_ms: NOW + 1_000,
        };
        if next.materials.items.insert(id.into(), material).is_some() {
            return Err(AppError::new(409, "Synthetic Material already exists."));
        }
    }
    for (id, material, predecessor, ordinal, title, text, created_ms) in [
        (
            FIRST_REVISION_ID,
            FIRST_MATERIAL_ID,
            None,
            1,
            "Original notes",
            FIRST_TEXT,
            NOW + 1_000,
        ),
        (
            SECOND_REVISION_ID,
            FIRST_MATERIAL_ID,
            Some(FIRST_REVISION_ID),
            2,
            "Revised notes",
            SECOND_TEXT,
            NOW + 1_001,
        ),
        (
            THIRD_REVISION_ID,
            SECOND_MATERIAL_ID,
            None,
            1,
            "Separate reference",
            THIRD_TEXT,
            NOW + 1_000,
        ),
    ] {
        let revision = MaterialRevision {
            id: id.into(),
            material_id: material.into(),
            predecessor_id: predecessor.map(str::to_owned),
            ordinal,
            title: title.into(),
            text: text.into(),
            provenance: MaterialProvenance::OwnerSupplied,
            created_ms,
        };
        if next
            .materials
            .revisions
            .insert(id.into(), revision)
            .is_some()
        {
            return Err(AppError::new(
                409,
                "Synthetic Material revision already exists.",
            ));
        }
    }
    for (operation, path, entity, created_ms) in [
        (
            "synthetic-future-concept-create",
            "/concepts/create",
            AUTHOR_CONCEPT_ID,
            NOW + 1_000,
        ),
        (
            "synthetic-future-material-add",
            "/materials/add",
            SECOND_MATERIAL_ID,
            NOW + 1_000,
        ),
        (
            "synthetic-future-material-revise",
            "/materials/revise",
            SECOND_REVISION_ID,
            NOW + 1_001,
        ),
    ] {
        future_receipt(
            &mut next,
            operation,
            path,
            entity,
            AUTHOR_CONCEPT_ID,
            created_ms,
        )?;
    }
    next.revision += 3;
    engine::validate_app(&next)?;
    *app = next;
    Ok(())
}

/// Simulate the later writer's acknowledged receipt, without enabling or
/// invoking a production authoring path. The ordinary full validator remains
/// the authority for its entity, ownership, timestamp and redirect shape.
fn future_receipt(
    app: &mut App,
    operation: &str,
    path: &str,
    entity: &str,
    concept: &str,
    created_ms: i64,
) -> AppResult<()> {
    let payload = BTreeMap::from([
        ("operation_id", operation),
        ("synthetic_fixture", "future-writer"),
        ("entity_id", entity),
    ]);
    let receipt = Receipt {
        acting_token: app.owner_tokens.keys().next().cloned(),
        path: Some(path.into()),
        created_ms: Some(created_ms),
        payload_hash: persistence::sha256(&serde_json::to_vec(&(path, payload)).unwrap()),
        result: MutationResult {
            data: None,
            redirect: format!("/concepts/{concept}"),
            entity_id: Some(entity.into()),
            message: None,
        },
    };
    if app.operations.insert(operation.into(), receipt).is_some() {
        return Err(AppError::new(
            409,
            "Synthetic future-writer operation already exists.",
        ));
    }
    Ok(())
}

/// A separate forward fixture isolates Material compatibility from the authored
/// Concept's optional Goal: every Concept here still has its original Goal.
pub fn add_material_to_legacy_concept(app: &mut App) -> AppResult<()> {
    engine::validate_app(app)?;
    let mut next = app.clone();
    let concept_id = next
        .concepts
        .values()
        .find(|concept| concept.goal_id.is_some())
        .ok_or_else(|| AppError::new(422, "Synthetic legacy Concept is missing."))?
        .id
        .clone();
    next.materials.capability = true;
    let material = Material {
        id: LEGACY_MATERIAL_ID.into(),
        concept_id: concept_id.clone(),
        current_revision_id: LEGACY_REVISION_ID.into(),
        created_ms: NOW + 1_000,
    };
    let revision = MaterialRevision {
        id: LEGACY_REVISION_ID.into(),
        material_id: LEGACY_MATERIAL_ID.into(),
        predecessor_id: None,
        ordinal: 1,
        title: "Synthetic supplied legacy reference".into(),
        text: THIRD_TEXT.into(),
        provenance: MaterialProvenance::OwnerSupplied,
        created_ms: NOW + 1_000,
    };
    if next
        .materials
        .items
        .insert(material.id.clone(), material)
        .is_some()
        || next
            .materials
            .revisions
            .insert(revision.id.clone(), revision)
            .is_some()
    {
        return Err(AppError::new(
            409,
            "Synthetic legacy Material already exists.",
        ));
    }
    future_receipt(
        &mut next,
        "synthetic-future-legacy-material-add",
        "/materials/add",
        LEGACY_MATERIAL_ID,
        &concept_id,
        NOW + 1_000,
    )?;
    next.revision += 1;
    engine::validate_app(&next)?;
    *app = next;
    Ok(())
}

pub fn unrelated_settings_mutation(app: &mut App) -> AppResult<()> {
    let fields = BTreeMap::from([
        ("csrf".into(), app.csrf.clone()),
        ("operation_id".into(), "materials-unrelated-settings".into()),
        ("pace".into(), "light".into()),
    ]);
    engine::mutate(
        app,
        "/settings",
        &fields,
        NOW + 2_000,
        "materials-unrelated-settings",
    )?;
    engine::validate_app(app)
}
