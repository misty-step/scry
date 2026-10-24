//! Concept-owned reference material and immutable wording. Phase A deliberately
//! has no authoring route or public mutation: this is the compatible reader and
//! writer floor for complete recovery and later authoring releases.
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

/// Reserved receipt shapes for the later writer, not executable routes in this
/// build. A rollback reader must retain and validate acknowledged operations as
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
