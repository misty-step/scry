//! Private content, independent of the Owner's Studies, credentials and receipts.
use crate::{grading, model::*, persistence::sha256};
use serde::{Deserialize, Serialize};
use std::collections::{BTreeMap, BTreeSet};

pub const TEXT_BYTES: usize = 256 * 1024;
#[derive(Clone, Copy, Debug, Deserialize, Serialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum Kind {
    Text,
    OrderedList,
    KeyedMapping,
}
#[derive(Clone, Copy, Debug, Default, Deserialize, Serialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum Rights {
    PublicDomain,
    Licensed,
    #[default]
    PrivateOnly,
}
#[derive(Clone, Copy, Debug, Deserialize, Serialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum Provenance {
    Supplied,
}
#[derive(Clone, Debug, Deserialize, Serialize, PartialEq, Eq)]
pub struct Work {
    pub id: String,
    pub revision: u64,
    pub title: String,
    pub aliases: Vec<String>,
    pub kind: Kind,
    pub context: String,
}
#[derive(Clone, Debug, Deserialize, Serialize, PartialEq, Eq)]
pub struct Unit {
    pub id: String,
    pub text: String,
}
#[derive(Clone, Debug, Deserialize, Serialize, PartialEq, Eq)]
pub struct Section {
    pub id: String,
    pub heading: String,
    pub units: Vec<Unit>,
}
#[derive(Clone, Debug, Deserialize, Serialize, PartialEq, Eq)]
pub struct Version {
    pub id: String,
    pub work_id: String,
    pub name: String,
    pub kind: Kind,
    pub text: String,
    pub normalized_hash: String,
    pub provenance: Provenance,
    pub rights: Rights,
    pub sources: Vec<String>,
    pub spelling_strict: bool,
    pub sections: Vec<Section>,
}
#[derive(Clone, Debug, Deserialize, Serialize, PartialEq, Eq)]
#[serde(tag = "kind", rename_all = "snake_case")]
pub enum Relation {
    VersionOf {
        version_id: String,
        work_id: String,
    },
    Supersedes {
        version_id: String,
        previous_id: String,
    },
    PartOf {
        work_id: String,
        parent_work_id: String,
    },
    RelatedTo {
        work_id: String,
        related_work_id: String,
    },
}
#[derive(Clone, Debug, Default, Deserialize, Serialize, PartialEq, Eq)]
pub struct Library {
    pub works: BTreeMap<String, Work>,
    pub versions: BTreeMap<String, Version>,
    pub relations: BTreeMap<String, Relation>,
}
#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
pub struct SectionInput {
    pub heading: String,
    pub units: Vec<String>,
}
#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
pub struct VersionInput {
    pub title: String,
    pub kind: Kind,
    pub text: String,
    #[serde(default)]
    pub aliases: Vec<String>,
    #[serde(default)]
    pub context: String,
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub sources: Vec<String>,
    #[serde(default)]
    pub rights: Rights,
    #[serde(default)]
    pub spelling_strict: bool,
    /// Omit for deterministic line/clause segmentation. Supplied boundaries are checked verbatim.
    pub sections: Option<Vec<SectionInput>>,
}
#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
pub struct WorkEdit {
    pub work_id: String,
    pub revision: u64,
    pub title: Option<String>,
    pub aliases: Option<Vec<String>>,
    pub context: Option<String>,
}
#[derive(Serialize)]
pub struct WorkView<'a> {
    pub work: &'a Work,
    pub versions: Vec<&'a Version>,
    pub relations: Vec<&'a Relation>,
}
#[derive(Serialize)]
pub struct VersionView<'a> {
    pub work: &'a Work,
    pub version: &'a Version,
    pub relations: Vec<&'a Relation>,
}
#[derive(Serialize)]
pub struct Search<'a> {
    pub matches: Vec<WorkView<'a>>,
    pub empty: bool,
    pub message: &'static str,
}

// The grader owns Unicode normalization. Library equality additionally ignores
// apostrophes and all spacing; no similarity or spelling tolerance enters dedupe.
fn key(text: &str) -> String {
    grading::normalized_words(text).join("").replace('\'', "")
}
fn identity(seed: &str, label: &str) -> String {
    sha256(format!("{seed}:{label}").as_bytes())[..32].into()
}
fn invalid() -> AppError {
    AppError::new(422, "Invalid Library metadata or segmentation.")
}
fn missing() -> AppError {
    AppError::new(404, "That Work or Version does not exist.")
}
fn label(text: &str, max: usize) -> bool {
    !text.trim().is_empty() && text.len() <= max && !text.chars().any(char::is_control)
}
fn names(work: &Work) -> impl Iterator<Item = String> + '_ {
    std::iter::once(&work.title)
        .chain(&work.aliases)
        .map(|name| key(name))
}
fn valid_work(work: &Work) -> bool {
    label(&work.title, 240)
        && !key(&work.title).is_empty()
        && work.aliases.len() <= 32
        && work
            .aliases
            .iter()
            .all(|a| label(a, 240) && !key(a).is_empty())
        && work.context.len() <= 4096
}
fn segment(text: &str) -> Vec<SectionInput> {
    let mut sections: Vec<SectionInput> = Vec::new();
    for line in text.split_inclusive('\n') {
        if line.trim().is_empty()
            && let Some(last) = sections.last_mut()
        {
            last.units.last_mut().unwrap().push_str(line);
            continue;
        }
        let mut units: Vec<String> = Vec::new();
        for clause in line.split_inclusive([',', ';', ':', '.', '!', '?', '\n']) {
            if clause.trim().is_empty()
                && let Some(last) = units.last_mut()
            {
                last.push_str(clause);
            } else {
                units.push(clause.into());
            }
        }
        sections.push(SectionInput {
            heading: format!("Section {}", sections.len() + 1),
            units,
        });
    }
    sections
}
fn checked_sections(input: &VersionInput) -> AppResult<Vec<SectionInput>> {
    if input.text.trim().is_empty()
        || input.text.len() > TEXT_BYTES
        || key(&input.text).is_empty()
        || input.name.len() > 240
        || input.sources.len() > 32
        || input.sources.iter().any(|s| !label(s, 2048))
    {
        return Err(invalid());
    }
    let sections = input
        .sections
        .clone()
        .unwrap_or_else(|| segment(&input.text));
    if sections.is_empty()
        || sections.len() > 4096
        || sections.iter().any(|s| {
            s.heading.len() > 240 || s.units.is_empty() || s.units.iter().any(String::is_empty)
        })
        || sections.iter().map(|s| s.units.len()).sum::<usize>() > 8192
        || sections
            .iter()
            .flat_map(|s| &s.units)
            .map(String::as_str)
            .collect::<String>()
            != input.text
    {
        return Err(invalid());
    }
    Ok(sections)
}
impl Library {
    pub fn is_empty(&self) -> bool {
        self.works.is_empty() && self.versions.is_empty() && self.relations.is_empty()
    }
    pub fn show_work(&self, id: &str) -> AppResult<WorkView<'_>> {
        let work = self.works.get(id).ok_or_else(missing)?;
        let versions: Vec<_> = self.versions.values().filter(|v| v.work_id == id).collect();
        let relations = self
            .relations
            .values()
            .filter(|r| match r {
                Relation::VersionOf { work_id, .. } => work_id == id,
                Relation::Supersedes { version_id, .. } => {
                    versions.iter().any(|v| &v.id == version_id)
                }
                Relation::PartOf {
                    work_id,
                    parent_work_id,
                } => work_id == id || parent_work_id == id,
                Relation::RelatedTo {
                    work_id,
                    related_work_id,
                } => work_id == id || related_work_id == id,
            })
            .collect();
        Ok(WorkView {
            work,
            versions,
            relations,
        })
    }
    pub fn show_version(&self, id: &str) -> AppResult<VersionView<'_>> {
        let version = self.versions.get(id).ok_or_else(missing)?;
        let work = self.works.get(&version.work_id).ok_or_else(missing)?;
        let relations = self
            .relations
            .values()
            .filter(|r| match r {
                Relation::VersionOf { version_id, .. } => version_id == id,
                Relation::Supersedes {
                    version_id,
                    previous_id,
                } => version_id == id || previous_id == id,
                _ => false,
            })
            .collect();
        Ok(VersionView {
            work,
            version,
            relations,
        })
    }
    pub fn search(&self, query: &str) -> Search<'_> {
        let query = key(query);
        let matches: Vec<_> = self
            .works
            .values()
            .filter(|w| {
                query.is_empty()
                    || names(w).any(|n| n.contains(&query))
                    || self
                        .versions
                        .values()
                        .filter(|v| v.work_id == w.id)
                        .any(|v| {
                            let opening = v.text.chars().take(240).collect::<String>();
                            key(&opening).contains(&query)
                        })
            })
            .filter_map(|w| self.show_work(&w.id).ok())
            .collect();
        let empty = matches.is_empty();
        Search {
            matches,
            empty,
            message: if empty {
                "Nothing matches."
            } else {
                "Matching Works."
            },
        }
    }
    fn unique_names(&self, proposed: &Work) -> AppResult<()> {
        let wanted: BTreeSet<_> = names(proposed).collect();
        if self
            .works
            .values()
            .any(|w| w.id != proposed.id && names(w).any(|n| wanted.contains(&n)))
        {
            return Err(AppError::new(
                409,
                "That title or alias already belongs to another Work.",
            ));
        }
        Ok(())
    }
    /// Validates everything before writing; the outer engine commits with its shared receipt.
    pub fn add(
        &mut self,
        input: VersionInput,
        seed: &str,
        previous: Option<&str>,
    ) -> AppResult<String> {
        let sections = checked_sections(&input)?;
        let mut proposed = Work {
            id: identity(seed, "work"),
            revision: 1,
            title: input.title.clone(),
            aliases: input.aliases.clone(),
            kind: input.kind,
            context: input.context.clone(),
        };
        if !valid_work(&proposed) {
            return Err(invalid());
        }
        let matching: Vec<_> = self
            .works
            .values()
            .filter(|w| names(w).any(|n| names(&proposed).any(|p| p == n)))
            .collect();
        if matching.len() > 1 {
            return Err(AppError::new(
                409,
                "The supplied names match different Works.",
            ));
        }
        let existing = matching.first().copied();
        if let Some(w) = existing {
            if w.kind != input.kind {
                return Err(AppError::new(409, "The Work has a different Kind."));
            }
            proposed = w.clone();
        }
        let hash = sha256(key(&input.text).as_bytes());
        if let Some(v) = self.versions.values().find(|v| v.normalized_hash == hash) {
            if existing.is_some_and(|w| w.id != v.work_id) || previous.is_some() {
                return Err(AppError::new(
                    409,
                    "That text already belongs to an existing Version. Show it instead.",
                ));
            }
            return Ok(v.id.clone());
        }
        if let Some(old) = previous
            && (self
                .versions
                .get(old)
                .is_none_or(|v| v.work_id != proposed.id)
                || self.relations.values().any(
                    |r| matches!(r,Relation::Supersedes{previous_id,..} if previous_id == old),
                ))
        {
            return Err(AppError::conflict());
        }
        let version_id = identity(seed, "version");
        let version = Version {
            id: version_id.clone(),
            work_id: proposed.id.clone(),
            name: input.name,
            kind: input.kind,
            text: input.text,
            normalized_hash: hash,
            provenance: Provenance::Supplied,
            rights: input.rights,
            sources: input.sources,
            spelling_strict: input.spelling_strict,
            sections: sections
                .into_iter()
                .enumerate()
                .map(|(s, section)| Section {
                    id: identity(&version_id, &format!("section-{s}")),
                    heading: section.heading,
                    units: section
                        .units
                        .into_iter()
                        .enumerate()
                        .map(|(u, text)| Unit {
                            id: identity(&version_id, &format!("unit-{s}-{u}")),
                            text,
                        })
                        .collect(),
                })
                .collect(),
        };
        if existing.is_some() {
            proposed.revision += 1;
        }
        self.works.insert(proposed.id.clone(), proposed.clone());
        self.versions.entry(version_id.clone()).or_insert(version);
        self.relations.insert(
            format!("version-of-{version_id}"),
            Relation::VersionOf {
                version_id: version_id.clone(),
                work_id: proposed.id,
            },
        );
        if let Some(old) = previous {
            self.relations.insert(
                format!("supersedes-{old}"),
                Relation::Supersedes {
                    version_id: version_id.clone(),
                    previous_id: old.into(),
                },
            );
        }
        Ok(version_id)
    }
    pub fn edit(&mut self, edit: WorkEdit) -> AppResult<String> {
        let mut next = self.works.get(&edit.work_id).ok_or_else(missing)?.clone();
        if next.revision != edit.revision {
            return Err(AppError::conflict());
        }
        if let Some(title) = edit.title {
            next.title = title;
        }
        if let Some(aliases) = edit.aliases {
            next.aliases = aliases;
        }
        if let Some(context) = edit.context {
            next.context = context;
        }
        if !valid_work(&next) {
            return Err(invalid());
        }
        self.unique_names(&next)?;
        next.revision += 1;
        self.works.insert(next.id.clone(), next);
        Ok(edit.work_id)
    }
    pub fn delete(
        &mut self,
        id: &str,
        work: bool,
        revision: u64,
        study_versions: &BTreeMap<String, String>,
    ) -> AppResult<String> {
        let work_id = if work {
            id
        } else {
            &self.versions.get(id).ok_or_else(missing)?.work_id
        };
        if self.works.get(work_id).ok_or_else(missing)?.revision != revision {
            return Err(AppError::conflict());
        }
        let removed: BTreeSet<String> = self
            .versions
            .values()
            .filter(|v| if work { v.work_id == id } else { v.id == id })
            .map(|v| v.id.clone())
            .collect();
        if study_versions.values().any(|v| removed.contains(v)) {
            return Err(AppError::new(
                409,
                "A Study uses this Version. Keep its wording and history.",
            ));
        }
        if work {
            self.works.remove(id);
        } else {
            self.works.get_mut(work_id).unwrap().revision += 1;
        }
        self.versions.retain(|v, _| !removed.contains(v));
        self.relations.retain(|_, r| match r {
            Relation::VersionOf { version_id, .. } => !removed.contains(version_id),
            Relation::Supersedes {
                version_id,
                previous_id,
            } => !removed.contains(version_id) && !removed.contains(previous_id),
            Relation::PartOf {
                work_id,
                parent_work_id,
            } => !work || (work_id != id && parent_work_id != id),
            Relation::RelatedTo {
                work_id,
                related_work_id,
            } => !work || (work_id != id && related_work_id != id),
        });
        Ok(id.into())
    }
    pub fn validate(&self) -> AppResult<()> {
        let mut all_names = BTreeSet::new();
        let mut hashes = BTreeSet::new();
        let mut ids = BTreeSet::new();
        for (id, w) in &self.works {
            if id != &w.id || !valid_id(id) || w.revision == 0 || !valid_work(w) {
                return Err(invalid());
            }
            for name in names(w).collect::<BTreeSet<_>>() {
                if !all_names.insert(name) {
                    return Err(invalid());
                }
            }
        }
        for (id, v) in &self.versions {
            if id != &v.id
                || !valid_id(id)
                || self.works.get(&v.work_id).is_none_or(|w| w.kind != v.kind)
                || !hashes.insert(&v.normalized_hash)
                || v.normalized_hash != sha256(key(&v.text).as_bytes())
            {
                return Err(invalid());
            }
            let input = VersionInput {
                title: String::new(),
                kind: v.kind,
                text: v.text.clone(),
                aliases: vec![],
                context: String::new(),
                name: v.name.clone(),
                sources: v.sources.clone(),
                rights: v.rights,
                spelling_strict: v.spelling_strict,
                sections: Some(
                    v.sections
                        .iter()
                        .map(|s| SectionInput {
                            heading: s.heading.clone(),
                            units: s.units.iter().map(|u| u.text.clone()).collect(),
                        })
                        .collect(),
                ),
            };
            checked_sections(&input)?;
            for s in &v.sections {
                if !valid_id(&s.id) || !ids.insert(&s.id) {
                    return Err(invalid());
                }
                for u in &s.units {
                    if !valid_id(&u.id) || !ids.insert(&u.id) {
                        return Err(invalid());
                    }
                }
            }
            if self.relations.get(&format!("version-of-{id}"))
                != Some(&Relation::VersionOf {
                    version_id: id.clone(),
                    work_id: v.work_id.clone(),
                })
            {
                return Err(invalid());
            }
        }
        for relation in self.relations.values() {
            let valid = match relation {
                Relation::VersionOf {
                    version_id,
                    work_id,
                } => self
                    .versions
                    .get(version_id)
                    .is_some_and(|v| &v.work_id == work_id),
                Relation::Supersedes {
                    version_id,
                    previous_id,
                } => {
                    version_id != previous_id
                        && self
                            .versions
                            .get(version_id)
                            .zip(self.versions.get(previous_id))
                            .is_some_and(|(a, b)| a.work_id == b.work_id)
                }
                Relation::PartOf {
                    work_id,
                    parent_work_id,
                } => {
                    work_id != parent_work_id
                        && self.works.contains_key(work_id)
                        && self.works.contains_key(parent_work_id)
                }
                Relation::RelatedTo {
                    work_id,
                    related_work_id,
                } => {
                    work_id != related_work_id
                        && self.works.contains_key(work_id)
                        && self.works.contains_key(related_work_id)
                }
            };
            if !valid {
                return Err(invalid());
            }
        }
        Ok(())
    }
}

pub fn mutate(
    app: &mut App,
    path: &str,
    fields: &BTreeMap<String, String>,
    seed: &str,
) -> AppResult<MutationResult> {
    let field = |key| fields.get(key).map(String::as_str).unwrap_or("");
    let revision = || {
        field("revision")
            .parse::<u64>()
            .map_err(|_| AppError::conflict())
    };
    let id = match path {
        "/library/version/add" => app.library.add(
            serde_json::from_str(field("data")).map_err(|_| invalid())?,
            seed,
            None,
        )?,
        "/library/version/supersede" => {
            let old = app
                .library
                .versions
                .get(field("version_id"))
                .ok_or_else(missing)?;
            let work = app.library.works.get(&old.work_id).ok_or_else(missing)?;
            if work.revision != revision()? {
                return Err(AppError::conflict());
            }
            app.library.add(
                serde_json::from_str(field("data")).map_err(|_| invalid())?,
                seed,
                Some(field("version_id")),
            )?
        }
        "/library/work/edit" => app
            .library
            .edit(serde_json::from_str(field("data")).map_err(|_| invalid())?)?,
        "/library/work/delete" => {
            app.library
                .delete(field("work_id"), true, revision()?, &app.study_version_refs)?
        }
        "/library/version/delete" => app.library.delete(
            field("version_id"),
            false,
            revision()?,
            &app.study_version_refs,
        )?,
        _ => return Err(AppError::new(404, "Library action not found.")),
    };
    let resource = if path.contains("/work/") {
        "works"
    } else {
        "versions"
    };
    Ok(MutationResult {
        redirect: format!("/api/v1/library/{resource}/{id}"),
        entity_id: Some(id),
        message: None,
    })
}

fn valid_id(id: &str) -> bool {
    id.len() == 32
        && id
            .bytes()
            .all(|b| b.is_ascii_digit() || (b'a'..=b'f').contains(&b))
}
/// Deletion retains its immutable receipt even when its Library entity is gone.
pub(crate) fn receipt_entity(receipt: &Receipt, id: &str) -> bool {
    let resource = match receipt.path.as_deref() {
        Some("/library/version/add" | "/library/version/supersede" | "/library/version/delete") => {
            "versions"
        }
        Some("/library/work/edit" | "/library/work/delete") => "works",
        _ => return false,
    };
    valid_id(id) && receipt.result.redirect == format!("/api/v1/library/{resource}/{id}")
}
