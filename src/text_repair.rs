//! Disposable native candidate. Runtime admission explicitly refuses HTTP use.
use crate::{
    library::{Kind, Library, SectionInput, Version},
    model::*,
    persistence::sha256,
};
use serde::{Deserialize, Serialize};
pub const POLICY: &str = "text-boundary-repair-v1;source=e7d69bdae91e15d9d48958d6007e68ef118818e3;exact-v2=1;unicode=17.0.0;rust=1.98.1";
#[derive(Clone, Debug, Serialize, Deserialize, PartialEq, Eq)]
#[serde(deny_unknown_fields)]
pub struct Repair {
    pub original: String,
    pub copy: String,
    pub policy: String,
    pub fingerprint: String,
}
fn scalar(out: &mut Vec<u8>, s: &str) {
    out.extend_from_slice(&(s.len() as u64).to_be_bytes());
    out.extend_from_slice(s.as_bytes());
}
fn count(out: &mut Vec<u8>, n: usize) {
    out.extend_from_slice(&(n as u64).to_be_bytes());
}
fn source(v: &Version, p: &[SectionInput], revision: Option<u64>) -> String {
    let mut out = Vec::from(b"scry-text-repair-preview-v1".as_slice());
    for s in [
        POLICY,
        &v.work_id,
        &v.id,
        &v.normalized_hash,
        &v.text,
        &v.name,
        "text",
        "supplied",
    ] {
        scalar(&mut out, s);
    }
    scalar(
        &mut out,
        match v.rights {
            crate::library::Rights::PublicDomain => "public_domain",
            crate::library::Rights::Licensed => "licensed",
            crate::library::Rights::PrivateOnly => "private_only",
        },
    );
    count(&mut out, v.sources.len());
    for s in &v.sources {
        scalar(&mut out, s);
    }
    out.push(u8::from(v.spelling_strict));
    count(&mut out, v.ordered_aliases.len());
    count(&mut out, v.sections.len());
    for s in &v.sections {
        scalar(&mut out, &s.id);
        scalar(&mut out, &s.heading);
        count(&mut out, s.units.len());
        for u in &s.units {
            scalar(&mut out, &u.id);
            scalar(&mut out, &u.text);
        }
    }
    count(&mut out, p.len());
    for s in p {
        scalar(&mut out, &s.heading);
        count(&mut out, s.units.len());
        for u in &s.units {
            scalar(&mut out, u);
        }
    }
    out.push(u8::from(revision.is_some()));
    if let Some(r) = revision {
        out.extend_from_slice(&r.to_be_bytes());
    }
    sha256(&out)
}
pub(crate) fn partition(v: &Version) -> AppResult<Vec<SectionInput>> {
    if v.kind != Kind::Text
        || !v.ordered_aliases.is_empty()
        || crate::text_repair_policy_v1::check_units(
            v.sections
                .iter()
                .flat_map(|s| &s.units)
                .map(|u| u.text.as_str()),
        )
        .is_ok()
    {
        return Err(AppError::new(
            422,
            "This Version is not eligible for text repair.",
        ));
    }
    let p = crate::text_repair_policy_v1::segments(&v.text);
    crate::text_repair_policy_v1::check_units(p.iter().flat_map(|s| &s.units).map(String::as_str))?;
    if p.is_empty() || p.iter().flat_map(|s| &s.units).cloned().collect::<String>() != v.text {
        return Err(AppError::new(422, "No safe repair partition."));
    }
    Ok(p)
}
pub fn preview(l: &Library, id: &str) -> AppResult<String> {
    let v = l
        .versions
        .get(id)
        .ok_or_else(|| AppError::new(404, "Version not found."))?;
    Ok(source(
        v,
        &partition(v)?,
        Some(
            l.works
                .get(&v.work_id)
                .ok_or_else(|| AppError::new(422, "Repair source Work is missing."))?
                .revision,
        ),
    ))
}
pub(crate) fn validate(l: &Library) -> AppResult<()> {
    let mut copies = std::collections::BTreeSet::new();
    for (id, r) in &l.repairs {
        let fail = || AppError::new(422, "Invalid text repair lineage.");
        let v = l.versions.get(id).ok_or_else(fail)?;
        let c = l.versions.get(&r.copy).ok_or_else(fail)?;
        let p = partition(v)?;
        if id != &r.original
            || r.policy != POLICY
            || id == &r.copy
            || l.repairs.contains_key(&r.copy)
            || !copies.insert(&r.copy)
            || r.fingerprint != source(v, &p, None)
        {
            return Err(fail());
        }
        let mut expected = v.clone();
        expected.id = c.id.clone();
        expected.sections = c.sections.clone();
        if &expected != c
            || c.sections.len() != p.len()
            || c.sections.iter().zip(&p).any(|(s, p)| {
                s.heading != p.heading
                    || s.units.iter().map(|u| &u.text).collect::<Vec<_>>()
                        != p.units.iter().collect::<Vec<_>>()
            })
        {
            return Err(fail());
        }
    }
    Ok(())
}
pub(crate) fn repair(
    l: &mut Library,
    id: &str,
    revision: u64,
    fingerprint: &str,
    seed: &str,
) -> AppResult<String> {
    let old = l
        .versions
        .get(id)
        .ok_or_else(|| AppError::new(404, "Version not found."))?
        .clone();
    if l.repairs.values().any(|r| r.copy == id)
        || l.works
            .get(&old.work_id)
            .ok_or_else(|| AppError::new(422, "Repair source Work is missing."))?
            .revision
            != revision
        || preview(l, id)? != fingerprint
    {
        return Err(AppError::conflict());
    }
    if let Some(r) = l.repairs.get(id) {
        validate(l)?;
        return Ok(r.copy.clone());
    }
    let p = partition(&old)?;
    let mut scratch = Library::default();
    let input = crate::library::VersionInput {
        title: l
            .works
            .get(&old.work_id)
            .ok_or_else(|| AppError::new(422, "Repair source Work is missing."))?
            .title
            .clone(),
        kind: old.kind,
        text: old.text.clone(),
        aliases: vec![],
        context: String::new(),
        name: old.name.clone(),
        sources: old.sources.clone(),
        rights: old.rights,
        spelling_strict: old.spelling_strict,
        ordered_aliases: old.ordered_aliases.clone(),
        sections: Some(p.clone()),
    };
    let new_id = scratch.add(input, seed, None)?;
    if l.versions.contains_key(&new_id) {
        return Err(AppError::conflict());
    }
    let mut copy = scratch.versions.remove(&new_id).unwrap();
    copy.work_id = old.work_id.clone();
    l.versions.insert(new_id.clone(), copy);
    l.relations.insert(
        format!("version-of-{new_id}"),
        crate::library::Relation::VersionOf {
            version_id: new_id.clone(),
            work_id: old.work_id.clone(),
        },
    );
    l.repairs.insert(
        id.into(),
        Repair {
            original: id.into(),
            copy: new_id.clone(),
            policy: POLICY.into(),
            fingerprint: source(&old, &p, None),
        },
    );
    l.works.get_mut(&old.work_id).unwrap().revision += 1;
    l.validate()?;
    Ok(new_id)
}

pub const LIBRARY_FORMAT: &str = "scry-library-text-repair-v1";
#[derive(Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
struct LibraryEnvelope {
    format: String,
    policy: String,
    library: Library,
}
/// Historical ordinary export bytes retain their shape. Repaired exports cannot
/// deserialize as an old Library because its required root fields are absent.
pub fn export_library(library: &Library) -> AppResult<serde_json::Value> {
    library.validate()?;
    let encoded = if library.repairs.is_empty() {
        serde_json::to_value(library)
    } else {
        serde_json::to_value(LibraryEnvelope {
            format: LIBRARY_FORMAT.into(),
            policy: POLICY.into(),
            library: library.clone(),
        })
    };
    encoded.map_err(|_| AppError::new(500, "Cannot encode Library."))
}
/// Native codec only, not a new import HTTP endpoint or restore authorization.
pub fn decode_library_export(bytes: &[u8]) -> AppResult<Library> {
    let invalid = || AppError::new(422, "Unsupported or invalid Library export.");
    if bytes.len() > crate::persistence::MAX_ARCHIVE_BYTES {
        return Err(invalid());
    }
    let value: serde_json::Value = serde_json::from_slice(bytes).map_err(|_| invalid())?;
    let library = if value.get("format").is_some() {
        let e: LibraryEnvelope = serde_json::from_value(value).map_err(|_| invalid())?;
        if e.format != LIBRARY_FORMAT || e.policy != POLICY || e.library.repairs.is_empty() {
            return Err(invalid());
        }
        e.library
    } else {
        let library: Library = serde_json::from_value(value).map_err(|_| invalid())?;
        if !library.repairs.is_empty() {
            return Err(invalid());
        }
        library
    };
    library.validate()?;
    Ok(library)
}
