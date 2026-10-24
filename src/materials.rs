//! Concept-owned reference material and immutable owner-supplied wording.
//! Authoring adds only the records accepted by the retained compatibility floor.
use crate::{generation::text_valid, model::*};
use serde::{Deserialize, Serialize};
use std::collections::{BTreeMap, BTreeSet};

pub const ARCHIVE_FORMAT: &str = "scry-rust-durable-sqlite-materials-v1";
pub const CAPABILITY_ROW: &str = "material_capability/v1";

#[derive(Clone, Debug, Default, Serialize, Deserialize, PartialEq)]
#[serde(deny_unknown_fields)]
pub struct Materials {
    /// Sticky once this schema has been used, even with no remaining entries.
    /// An optional JSON field alone would let an old writer silently shed data.
    pub capability: bool,
    pub items: BTreeMap<String, Material>,
    pub revisions: BTreeMap<String, MaterialRevision>,
}

impl Materials {
    pub fn is_empty(&self) -> bool {
        !self.capability && self.items.is_empty() && self.revisions.is_empty()
    }
}

pub fn requires_capability(app: &App) -> bool {
    !app.materials.is_empty()
        || app
            .concepts
            .values()
            .any(|c| c.origin.is_some() || c.goal_id.is_none())
        || app.operations.values().any(is_authoring_receipt)
}

pub(crate) fn authoring_path(path: &str) -> bool {
    matches!(
        path,
        "/concepts/create" | "/materials/add" | "/materials/revise"
    )
}

fn is_authoring_receipt(receipt: &Receipt) -> bool {
    receipt.path.as_deref().is_some_and(authoring_path)
}

/// A rollback reader must retain and validate acknowledged operations as
/// well as content. No broad unknown-entity exemption is safe.
pub(crate) fn receipt_entity(app: &App, receipt: &Receipt, id: &str) -> bool {
    let (concept_id, created_ms) = match receipt.path.as_deref() {
        Some("/concepts/create") => match app.concepts.get(id) {
            Some(c) if c.origin == Some(ConceptOrigin::OwnerAuthored) => {
                let Some(first) = app
                    .materials
                    .items
                    .values()
                    .filter(|m| m.concept_id == c.id)
                    .min_by_key(|m| m.created_ms)
                else {
                    return false;
                };
                (c.id.as_str(), first.created_ms)
            }
            _ => return false,
        },
        Some("/materials/add") => match app.materials.items.get(id) {
            Some(m) => (m.concept_id.as_str(), m.created_ms),
            None => return false,
        },
        Some("/materials/revise") => match app.materials.revisions.get(id) {
            Some(r) if r.ordinal > 1 && r.predecessor_id.is_some() => {
                match app.materials.items.get(&r.material_id) {
                    Some(m) => (m.concept_id.as_str(), r.created_ms),
                    None => return false,
                }
            }
            _ => return false,
        },
        _ => return false,
    };
    app.materials.capability
        && receipt.created_ms.is_some_and(|at| at >= created_ms)
        && receipt.result.redirect == format!("/concepts/{concept_id}")
        && receipt.result.data.is_none()
}

#[derive(Clone, Debug, Serialize, Deserialize, PartialEq)]
#[serde(deny_unknown_fields)]
pub struct Material {
    pub id: String,
    pub concept_id: String,
    pub current_revision_id: String,
    pub created_ms: i64,
}

#[derive(Clone, Debug, Serialize, Deserialize, PartialEq)]
#[serde(deny_unknown_fields)]
pub struct MaterialRevision {
    pub id: String,
    pub material_id: String,
    pub predecessor_id: Option<String>,
    pub ordinal: u64,
    pub title: String,
    /// Exact supplied UTF-8, never trimmed, normalized or regenerated.
    pub text: String,
    pub provenance: MaterialProvenance,
    pub created_ms: i64,
}

#[derive(Clone, Debug, Serialize, Deserialize, PartialEq)]
#[serde(rename_all = "kebab-case")]
pub enum MaterialProvenance {
    OwnerSupplied,
}

pub fn is_authoring_path(path: &str) -> bool {
    authoring_path(path)
}

fn field<'a>(fields: &'a BTreeMap<String, String>, key: &str) -> &'a str {
    fields.get(key).map(String::as_str).unwrap_or("")
}

fn identity(seed: &str, label: &str) -> String {
    crate::persistence::sha256(format!("{seed}:{label}").as_bytes())[..32].into()
}

fn check_concept_access(app: &App, concept_id: Option<&str>) -> AppResult<()> {
    if app.practice.protected() || concept_id.is_some_and(|id| app.protected_concept(id)) {
        return Err(AppError::new(
            409,
            "Record assistance or answer before authoring reference material.",
        ));
    }
    Ok(())
}

/// Authoring is never an implicit Reveal. The same fence applies to token and
/// browser writes, and receipt replays recheck the *current* protected state.
pub fn check_access(app: &App, path: &str, fields: &BTreeMap<String, String>) -> AppResult<()> {
    let concept_id = match path {
        "/concepts/create" => None,
        "/materials/add" => Some(field(fields, "concept_id")),
        "/materials/revise" => app
            .materials
            .items
            .get(field(fields, "material_id"))
            .map(|m| m.concept_id.as_str()),
        _ => return Ok(()),
    };
    check_concept_access(app, concept_id)
}

pub(crate) fn check_replay(app: &App, receipt: &Receipt) -> AppResult<()> {
    let Some(path) = receipt
        .path
        .as_deref()
        .filter(|path| is_authoring_path(path))
    else {
        return Ok(());
    };
    let id = receipt
        .result
        .entity_id
        .as_deref()
        .ok_or_else(AppError::conflict)?;
    let concept_id = match path {
        "/concepts/create" => app.concepts.get(id).map(|c| c.id.as_str()),
        "/materials/add" => app.materials.items.get(id).map(|m| m.concept_id.as_str()),
        "/materials/revise" => app
            .materials
            .revisions
            .get(id)
            .and_then(|r| app.materials.items.get(&r.material_id))
            .map(|m| m.concept_id.as_str()),
        _ => unreachable!(),
    };
    check_concept_access(app, Some(concept_id.ok_or_else(AppError::conflict)?))
}

/// Only the engine calls this against its uncommitted clone. The engine adds
/// the receipt and validates the complete state and persistence bounds before
/// committing any of these changes.
pub(crate) fn mutate(
    app: &mut App,
    path: &str,
    fields: &BTreeMap<String, String>,
    now: i64,
    seed: &str,
) -> AppResult<MutationResult> {
    let allowed: &[&str] = match path {
        "/concepts/create" => &["name", "summary", "title", "text"],
        "/materials/add" => &["concept_id", "title", "text"],
        "/materials/revise" => &["material_id", "expected_revision_id", "title", "text"],
        _ => return Err(AppError::new(404, "That authoring action does not exist.")),
    };
    if fields.keys().any(|key| {
        !matches!(key.as_str(), "operation_id" | "csrf") && !allowed.contains(&key.as_str())
    }) {
        return Err(AppError::new(422, "Unknown material authoring field."));
    }
    check_access(app, path, fields)?;
    let title = field(fields, "title");
    let text = field(fields, "text");
    if !text_valid(title, 200) || !text_valid(text, TEXT_LIMIT) {
        return Err(AppError::new(
            422,
            "Supply a title up to 200 bytes and nonempty text up to 32 KiB.",
        ));
    }

    let revision_id = identity(seed, "material-revision");
    if app.materials.revisions.contains_key(&revision_id) {
        return Err(AppError::conflict());
    }
    let (concept_id, material_id, predecessor_id, ordinal, entity_id) = match path {
        "/concepts/create" => {
            let name = field(fields, "name");
            let summary = field(fields, "summary");
            if !text_valid(name, 200) || (!summary.is_empty() && !text_valid(summary, 2000)) {
                return Err(AppError::new(
                    422,
                    "Supply a Concept name up to 200 bytes and an optional summary up to 2000 bytes.",
                ));
            }
            let concept_id = identity(seed, "material-concept");
            if app.concepts.contains_key(&concept_id) {
                return Err(AppError::conflict());
            }
            app.concepts.insert(
                concept_id.clone(),
                Concept {
                    id: concept_id.clone(),
                    goal_id: None,
                    origin: Some(ConceptOrigin::OwnerAuthored),
                    name: name.into(),
                    summary: summary.into(),
                    notes: vec![],
                    prerequisites: vec![],
                    introduced_ms: None,
                    exposure_ms: None,
                    already_known: false,
                    archived: false,
                },
            );
            let entity_id = concept_id.clone();
            (concept_id, identity(seed, "material"), None, 1, entity_id)
        }
        "/materials/add" => {
            let concept_id = field(fields, "concept_id");
            if !app.concepts.contains_key(concept_id) {
                return Err(AppError::new(404, "That Concept does not exist."));
            }
            let material_id = identity(seed, "material");
            (concept_id.into(), material_id.clone(), None, 1, material_id)
        }
        "/materials/revise" => {
            let material = app
                .materials
                .items
                .get(field(fields, "material_id"))
                .ok_or_else(|| AppError::new(404, "That Material does not exist."))?;
            if material.current_revision_id != field(fields, "expected_revision_id") {
                return Err(AppError::conflict());
            }
            let previous = app
                .materials
                .revisions
                .get(&material.current_revision_id)
                .ok_or_else(AppError::conflict)?;
            if now < previous.created_ms {
                return Err(AppError::conflict());
            }
            (
                material.concept_id.clone(),
                material.id.clone(),
                Some(previous.id.clone()),
                previous
                    .ordinal
                    .checked_add(1)
                    .ok_or_else(AppError::conflict)?,
                revision_id.clone(),
            )
        }
        _ => unreachable!(),
    };
    if predecessor_id.is_none() {
        if app.materials.items.contains_key(&material_id) {
            return Err(AppError::conflict());
        }
        app.materials.items.insert(
            material_id.clone(),
            Material {
                id: material_id.clone(),
                concept_id: concept_id.clone(),
                current_revision_id: revision_id.clone(),
                created_ms: now,
            },
        );
    } else {
        app.materials
            .items
            .get_mut(&material_id)
            .unwrap()
            .current_revision_id = revision_id.clone();
    }
    app.materials.revisions.insert(
        revision_id.clone(),
        MaterialRevision {
            id: revision_id,
            material_id,
            predecessor_id,
            ordinal,
            title: title.into(),
            text: text.into(),
            provenance: MaterialProvenance::OwnerSupplied,
            created_ms: now,
        },
    );
    app.materials.capability = true;
    Ok(MutationResult {
        redirect: format!("/concepts/{concept_id}"),
        entity_id: Some(entity_id),
        data: None,
        message: None,
    })
}

/// One legacy Note chain is one read-only Material projection. It borrows the
/// original notes and their identity, without copying them into new row families
/// or inventing separate Materials for revisions of the same note.
#[derive(Debug, Serialize)]
#[serde(tag = "kind", rename_all = "kebab-case")]
pub enum MaterialView<'a> {
    LegacyNote {
        concept_id: &'a str,
        revisions: &'a [Note],
    },
    Supplied {
        material: &'a Material,
        revisions: Vec<&'a MaterialRevision>,
    },
}

pub fn for_concept<'a>(app: &'a App, concept: &'a Concept) -> Vec<MaterialView<'a>> {
    let mut result = Vec::new();
    let mut by_material = BTreeMap::<&str, Vec<&MaterialRevision>>::new();
    for revision in app.materials.revisions.values() {
        by_material
            .entry(&revision.material_id)
            .or_default()
            .push(revision);
    }
    if !concept.notes.is_empty() {
        result.push(MaterialView::LegacyNote {
            concept_id: &concept.id,
            revisions: &concept.notes,
        });
    }
    for material in app
        .materials
        .items
        .values()
        .filter(|m| m.concept_id == concept.id)
    {
        let mut revisions = by_material
            .remove(material.id.as_str())
            .expect("validated Material has revisions");
        revisions.sort_by_key(|r| r.ordinal);
        result.push(MaterialView::Supplied {
            material,
            revisions,
        });
    }
    result
}

pub(crate) fn validate(app: &App, ids: &mut BTreeSet<String>) -> AppResult<()> {
    let invalid = || AppError::new(422, "The archive contains invalid Concept Materials.");
    let store = &app.materials;
    let mut revision_counts = BTreeMap::<&str, usize>::new();
    for revision in store.revisions.values() {
        *revision_counts.entry(&revision.material_id).or_default() += 1;
    }
    if !store.capability && requires_capability(app) {
        return Err(invalid());
    }
    for receipt in app.operations.values().filter(|r| is_authoring_receipt(r)) {
        if !receipt
            .result
            .entity_id
            .as_ref()
            .is_some_and(|id| receipt_entity(app, receipt, id))
        {
            return Err(invalid());
        }
    }
    for (key, material) in &store.items {
        crate::engine::archive_id(key, &material.id, ids)?;
        if !app.concepts.contains_key(&material.concept_id) {
            return Err(invalid());
        }
        let mut chain = BTreeSet::new();
        let mut current = store
            .revisions
            .get(&material.current_revision_id)
            .ok_or_else(invalid)?;
        loop {
            if current.material_id != material.id || !chain.insert(current.id.as_str()) {
                return Err(invalid());
            }
            match &current.predecessor_id {
                Some(id) => {
                    let previous = store.revisions.get(id).ok_or_else(invalid)?;
                    if previous.ordinal.checked_add(1) != Some(current.ordinal)
                        || previous.created_ms > current.created_ms
                    {
                        return Err(invalid());
                    }
                    current = previous;
                }
                None => {
                    if current.ordinal != 1 || current.created_ms != material.created_ms {
                        return Err(invalid());
                    }
                    break;
                }
            }
        }
        if revision_counts.get(material.id.as_str()).copied() != Some(chain.len()) {
            return Err(invalid());
        }
    }
    for (key, revision) in &store.revisions {
        crate::engine::archive_id(key, &revision.id, ids)?;
        if !store.items.contains_key(&revision.material_id)
            || !text_valid(&revision.title, 200)
            || !text_valid(&revision.text, TEXT_LIMIT)
        {
            return Err(invalid());
        }
    }
    Ok(())
}
