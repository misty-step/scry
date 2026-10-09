//! Portable storage rows and complete, self-contained recovery archives.
//!
//! SQLite stores records separately so immutable review history does not share
//! a single size-limited Durable Object SQL row. The runtime commits the row
//! delta and revision together with `storage.transactionSync`.
use crate::{learning::ALGORITHM, model::*};
use serde::{Deserialize, Serialize};
use serde_json::Value;
use sha2::{Digest, Sha256};
use std::{
    collections::{BTreeMap, BTreeSet},
    io::{self, Write},
};

pub const MAX_RECORD_BYTES: usize = 1024 * 1024;
pub const MAX_ARCHIVE_BYTES: usize = 16 * 1024 * 1024;
pub const ARCHIVE_FORMAT: &str = "scry-rust-durable-sqlite-v1";
const MAPS: &[&str] = &[
    "goals",
    "concepts",
    "questions",
    "operations",
    "jobs",
    "assessments",
];
const LISTS: &[&str] = &["events", "overrides", "spend", "feedback", "resets"];
const JOB_PAYLOADS: &[&str] = &[
    "raw_response",
    "candidate_json",
    "critic_json",
    "critic_cache_json",
];
const PAYLOAD_CHUNK_BYTES: usize = 64 * 1024;

fn payload_rows(
    result: &mut BTreeMap<String, String>,
    prefix: &str,
    id: &str,
    field: &str,
    value: &mut Value,
) -> AppResult<()> {
    if value.is_null() {
        return Ok(());
    }
    let Value::String(text) = std::mem::replace(value, Value::Null) else {
        return Err(AppError::new(500, "Invalid stored payload."));
    };
    let mut position = 0;
    let mut index = 0;
    loop {
        let mut end = (position + PAYLOAD_CHUNK_BYTES).min(text.len());
        while !text.is_char_boundary(end) {
            end -= 1;
        }
        result.insert(
            format!("{prefix}/{id}/{field}/{index:016}"),
            bounded_json(&Value::String(text[position..end].into()))?,
        );
        if end == text.len() {
            break;
        }
        position = end;
        index += 1;
    }
    Ok(())
}

pub fn rows(app: &App) -> AppResult<BTreeMap<String, String>> {
    let mut value = serde_json::to_value(app).map_err(serialization)?;
    let object = value
        .as_object_mut()
        .ok_or_else(|| AppError::new(500, "Invalid state."))?;
    let mut result = BTreeMap::new();
    for field in MAPS {
        let entries = object
            .get_mut(*field)
            .and_then(Value::as_object_mut)
            .ok_or_else(|| AppError::new(500, "Invalid state map."))?;
        for (id, mut record) in std::mem::take(entries) {
            if *field == "jobs" {
                let job = record
                    .as_object_mut()
                    .ok_or_else(|| AppError::new(500, "Invalid job record."))?;
                // Independently bounded responses must not combine into one
                // oversized SQL value that refuses a paid outcome receipt.
                for payload in JOB_PAYLOADS {
                    if let Some(value) = job.get_mut(*payload) {
                        payload_rows(&mut result, "job_payload", &id, payload, value)?;
                    }
                }
            }
            result.insert(format!("{field}/{id}"), bounded_json(&record)?);
        }
    }
    for field in LISTS {
        let entries = object
            .get_mut(*field)
            .and_then(Value::as_array_mut)
            .ok_or_else(|| AppError::new(500, "Invalid state history."))?;
        for (index, mut record) in std::mem::take(entries).into_iter().enumerate() {
            if *field == "spend"
                && let Some(payload) = record.get_mut("raw_response")
            {
                payload_rows(
                    &mut result,
                    "spend_payload",
                    &format!("{index:016}"),
                    "raw_response",
                    payload,
                )?;
            }
            result.insert(format!("{field}/{index:016}"), bounded_json(&record)?);
        }
    }
    result.insert("meta".into(), bounded_json(&value)?);
    Ok(result)
}

pub fn app_from_rows(rows: &BTreeMap<String, String>) -> AppResult<App> {
    let meta = rows
        .get("meta")
        .ok_or_else(|| AppError::new(500, "State metadata is missing."))?;
    let mut value: Value = serde_json::from_str(meta).map_err(serialization)?;
    for (key, data) in rows {
        if key == "meta" {
            continue;
        }
        let (field, id) = key
            .split_once('/')
            .ok_or_else(|| AppError::new(500, "Invalid storage record."))?;
        if matches!(field, "job_payload" | "spend_payload") {
            continue;
        }
        let record: Value = serde_json::from_str(data).map_err(serialization)?;
        if MAPS.contains(&field) {
            value
                .get_mut(field)
                .and_then(Value::as_object_mut)
                .ok_or_else(|| AppError::new(500, "Invalid state map."))?
                .insert(id.into(), record);
        } else if LISTS.contains(&field) {
            value
                .get_mut(field)
                .and_then(Value::as_array_mut)
                .ok_or_else(|| AppError::new(500, "Invalid state history."))?
                .push(record);
        } else {
            return Err(AppError::new(500, "Unknown storage record."));
        }
    }
    let mut payloads = BTreeMap::<(String, String, String), BTreeMap<usize, String>>::new();
    for (key, data) in rows {
        let Some((prefix, payload)) = key.split_once('/') else {
            continue;
        };
        if !matches!(prefix, "job_payload" | "spend_payload") {
            continue;
        }
        let (head, tail) = payload
            .rsplit_once('/')
            .ok_or_else(|| AppError::new(500, "Invalid stored payload."))?;
        let (id, field, index) = if let Ok(index) = tail.parse::<usize>() {
            let (id, field) = head
                .rsplit_once('/')
                .ok_or_else(|| AppError::new(500, "Invalid stored payload chunk."))?;
            (id, field, index)
        } else {
            // Compatible with the earlier single-row payload codec.
            (head, tail, 0)
        };
        if (prefix == "job_payload" && !JOB_PAYLOADS.contains(&field))
            || (prefix == "spend_payload" && field != "raw_response")
        {
            return Err(AppError::new(500, "Unknown stored payload field."));
        }
        let text: String = serde_json::from_str(data).map_err(serialization)?;
        if payloads
            .entry((prefix.into(), id.into(), field.into()))
            .or_default()
            .insert(index, text)
            .is_some()
        {
            return Err(AppError::new(500, "Stored payload chunk was duplicated."));
        }
    }
    for ((prefix, id, field), chunks) in payloads {
        let record = if prefix == "job_payload" {
            value.get_mut("jobs").and_then(|jobs| jobs.get_mut(&id))
        } else {
            id.parse::<usize>().ok().and_then(|index| {
                value
                    .get_mut("spend")
                    .and_then(|spend| spend.get_mut(index))
            })
        }
        .and_then(Value::as_object_mut)
        .ok_or_else(|| AppError::new(500, "Stored payload has no record."))?;
        if record
            .get(&field)
            .is_some_and(|existing| !existing.is_null())
        {
            return Err(AppError::new(500, "Stored payload was duplicated."));
        }
        let mut text = String::new();
        for (expected, (index, chunk)) in chunks.into_iter().enumerate() {
            if index != expected {
                return Err(AppError::new(500, "Stored payload chunk is missing."));
            }
            text.push_str(&chunk);
        }
        record.insert(field, Value::String(text));
    }
    let app: App = serde_json::from_value(value).map_err(serialization)?;
    if app.schema != SCHEMA {
        return Err(AppError::new(500, "A compatible Scry build is required."));
    }
    Ok(app)
}
fn bounded_json(value: &Value) -> AppResult<String> {
    let text = serde_json::to_string(value).map_err(serialization)?;
    if text.len() > MAX_RECORD_BYTES {
        return Err(AppError::new(413, "This record exceeds the storage limit."));
    }
    Ok(text)
}
fn serialization(_: impl std::fmt::Display) -> AppError {
    AppError::new(500, "Stored data could not be decoded safely.")
}
pub fn sha256(bytes: &[u8]) -> String {
    hex::encode(Sha256::digest(bytes))
}

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct ArchivedPhoto {
    pub photo: Photo,
    #[serde(with = "photo_data")]
    pub bytes: Vec<u8>,
}
mod photo_data {
    use base64::{Engine as _, engine::general_purpose::STANDARD};
    use serde::{Deserialize, Deserializer, Serializer};
    pub fn serialize<S: Serializer>(bytes: &[u8], serializer: S) -> Result<S::Ok, S::Error> {
        serializer.serialize_str(&STANDARD.encode(bytes))
    }
    pub fn deserialize<'de, D: Deserializer<'de>>(deserializer: D) -> Result<Vec<u8>, D::Error> {
        let text = String::deserialize(deserializer)?;
        if text.len() > crate::model::PHOTO_LIMIT.div_ceil(3) * 4 {
            return Err(serde::de::Error::custom(
                "Photo archive exceeds its size limit.",
            ));
        }
        STANDARD.decode(text).map_err(serde::de::Error::custom)
    }
}
#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct Archive {
    pub format: String,
    pub schema: u32,
    pub algorithm: String,
    pub created_ms: i64,
    pub app: App,
    pub photos: Vec<ArchivedPhoto>,
}
pub fn required_photos(app: &App) -> AppResult<BTreeMap<&str, &Photo>> {
    let mut required = BTreeMap::new();
    for photo in app.goals.values().filter_map(|g| g.photo.as_ref()) {
        if required
            .insert(photo.key.as_str(), photo)
            .is_some_and(|prior| prior != photo)
        {
            return Err(AppError::new(
                422,
                "The same photo reference contains conflicting metadata.",
            ));
        }
    }
    Ok(required)
}
struct BoundedSize {
    bytes: usize,
}
impl BoundedSize {
    fn add(&mut self, bytes: usize) -> io::Result<()> {
        self.bytes = self
            .bytes
            .checked_add(bytes)
            .filter(|n| *n <= MAX_ARCHIVE_BYTES)
            .ok_or_else(|| io::Error::other("Archive size limit exceeded."))?;
        Ok(())
    }
}
impl Write for BoundedSize {
    fn write(&mut self, chunk: &[u8]) -> io::Result<usize> {
        self.add(chunk.len())?;
        Ok(chunk.len())
    }
    fn flush(&mut self) -> io::Result<()> {
        Ok(())
    }
}

/// Check the complete archive footprint without loading any R2 photo bodies.
/// Base64 needs exactly four ASCII bytes per three input bytes; it requires no
/// JSON escaping. The timestamp bound also covers future compatible exports.
pub fn archive_size(app: &App) -> AppResult<usize> {
    #[derive(Serialize)]
    struct PhotoSize<'a> {
        photo: &'a Photo,
        bytes: &'static str,
    }
    #[derive(Serialize)]
    struct ArchiveSize<'a> {
        format: &'static str,
        schema: u32,
        algorithm: &'static str,
        created_ms: i64,
        app: &'a App,
        photos: Vec<PhotoSize<'a>>,
    }
    let mut size = BoundedSize { bytes: 0 };
    let mut photos = Vec::new();
    for photo in required_photos(app)?.into_values() {
        if photo.size > PHOTO_LIMIT {
            return Err(AppError::new(
                413,
                "A photo exceeds the archive size limit.",
            ));
        }
        size.add(photo.size.div_ceil(3) * 4).map_err(|_| {
            AppError::new(413, "The complete archive exceeds its bounded size limit.")
        })?;
        photos.push(PhotoSize { photo, bytes: "" });
    }
    serde_json::to_writer(
        &mut size,
        &ArchiveSize {
            format: ARCHIVE_FORMAT,
            schema: SCHEMA,
            algorithm: ALGORITHM,
            created_ms: i64::MIN,
            app,
            photos,
        },
    )
    .map_err(|_| AppError::new(413, "The complete archive exceeds its bounded size limit."))?;
    Ok(size.bytes)
}
impl Archive {
    pub fn new(app: App, photos: Vec<ArchivedPhoto>, now: i64) -> Self {
        Self {
            format: ARCHIVE_FORMAT.into(),
            schema: SCHEMA,
            algorithm: ALGORITHM.into(),
            created_ms: now,
            app,
            photos,
        }
    }
    pub fn encode(&self) -> AppResult<Vec<u8>> {
        archive_size(&self.app)?;
        self.validate()?;
        // Count through a bounded writer first: never allocate an oversized
        // complete JSON archive and only then discover its memory limit.
        let mut size = BoundedSize { bytes: 0 };
        serde_json::to_writer(&mut size, self).map_err(|_| {
            AppError::new(413, "The complete archive exceeds its bounded size limit.")
        })?;
        let mut bytes = Vec::with_capacity(size.bytes);
        serde_json::to_writer(&mut bytes, self).map_err(serialization)?;
        if bytes.len() > MAX_ARCHIVE_BYTES {
            return Err(AppError::new(
                413,
                "The complete archive exceeds its bounded size limit.",
            ));
        }
        Ok(bytes)
    }
    pub fn decode(bytes: &[u8], expected_hash: &str) -> AppResult<Self> {
        if bytes.len() > MAX_ARCHIVE_BYTES
            || expected_hash.len() != 64
            || sha256(bytes) != expected_hash
        {
            return Err(AppError::new(
                422,
                "The recovery archive checksum or size is invalid.",
            ));
        }
        let archive: Self = serde_json::from_slice(bytes)
            .map_err(|_| AppError::new(422, "The recovery archive is invalid."))?;
        archive.validate()?;
        Ok(archive)
    }
    pub fn validate(&self) -> AppResult<()> {
        if self.format != ARCHIVE_FORMAT
            || self.schema != SCHEMA
            || self.app.schema != SCHEMA
            || self.algorithm != ALGORITHM
        {
            return Err(AppError::new(
                422,
                "This archive requires its compatible schema and scheduler build.",
            ));
        }
        crate::engine::validate_app(&self.app)?;
        let required = required_photos(&self.app)?;
        let mut seen = BTreeSet::new();
        for archived in &self.photos {
            let p = &archived.photo;
            if !seen.insert(p.key.as_str())
                || required.get(p.key.as_str()).copied() != Some(p)
                || !p.key.starts_with("photos/")
                || p.size != archived.bytes.len()
                || p.size > PHOTO_LIMIT
                || sha256(&archived.bytes) != p.sha256
                || image_mime(&archived.bytes) != Some(p.mime.as_str())
            {
                return Err(AppError::new(
                    422,
                    "The archive contains invalid or unexpected photo data.",
                ));
            }
        }
        if seen.len() != required.len() {
            return Err(AppError::new(
                422,
                "The archive is missing required photo data.",
            ));
        }
        rows(&self.app)?;
        Ok(())
    }
}

pub fn image_mime(bytes: &[u8]) -> Option<&'static str> {
    if bytes.starts_with(b"\x89PNG\r\n\x1a\n") {
        Some("image/png")
    } else if bytes.starts_with(&[0xff, 0xd8, 0xff]) {
        Some("image/jpeg")
    } else if bytes.len() >= 12 && &bytes[..4] == b"RIFF" && &bytes[8..12] == b"WEBP" {
        Some("image/webp")
    } else {
        None
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn row_roundtrip_and_archive_checksum() {
        let app = App::new("a-test-csrf-value-at-least-24-chars".into());
        assert_eq!(app_from_rows(&rows(&app).unwrap()).unwrap(), app);
        let archive = Archive::new(app.clone(), vec![], 1700000000000);
        let bytes = archive.encode().unwrap();
        assert_eq!(Archive::decode(&bytes, &sha256(&bytes)).unwrap().app, app);
        assert!(Archive::decode(&bytes, &"0".repeat(64)).is_err());
        let mut wrong = archive;
        wrong.algorithm = "future-scheduler".into();
        assert!(wrong.encode().is_err());
    }
    #[test]
    fn checks_image_bytes_instead_of_a_declared_mime() {
        assert_eq!(image_mime(b"<svg onload='alert(1)'>"), None);
        assert_eq!(image_mime(b"\x89PNG\r\n\x1a\nbytes"), Some("image/png"));
        assert_eq!(image_mime(b"RIFFabcdWEBPbytes"), Some("image/webp"));
    }
    #[test]
    fn checks_complete_asset_capacity_without_loading_photo_bodies() {
        let mut app = App::new("a-test-csrf-value-at-least-24-chars".into());
        let empty = Archive::new(app.clone(), vec![], i64::MIN);
        assert_eq!(archive_size(&app).unwrap(), empty.encode().unwrap().len());
        for index in 0..3 {
            let id = format!("photo-{index}");
            app.goals.insert(
                id.clone(),
                Goal {
                    id: id.clone(),
                    title: "Source photo".into(),
                    intent: "Learn from this photo".into(),
                    created_ms: 0,
                    revision: 1,
                    status: "queued".into(),
                    paused: false,
                    focused: false,
                    archived: false,
                    concept_ids: vec![],
                    job_id: id.clone(),
                    photo: Some(Photo {
                        key: format!("photos/test/{id}"),
                        mime: "image/png".into(),
                        size: PHOTO_LIMIT,
                        sha256: "0".repeat(64),
                    }),
                    transcript: None,
                },
            );
            if index < 2 {
                assert!(archive_size(&app).is_ok());
            }
        }
        assert_eq!(archive_size(&app).unwrap_err().status, 413);
        app.goals.get_mut("photo-2").unwrap().photo = app.goals["photo-0"].photo.clone();
        assert!(
            archive_size(&app).is_ok(),
            "Shared identical assets are stored once."
        );
        app.goals
            .get_mut("photo-2")
            .unwrap()
            .photo
            .as_mut()
            .unwrap()
            .sha256 = "1".repeat(64);
        assert_eq!(required_photos(&app).unwrap_err().status, 422);
    }
    #[test]
    fn paid_payloads_roundtrip_in_separate_bounded_sql_records() {
        let mut app = App::new("a-test-csrf-value-at-least-24-chars".into());
        crate::engine::seed_fixture(&mut app, 1_700_000_000_000).unwrap();
        let job = app.jobs.values_mut().next().unwrap();
        // Sixfold JSON control-character escaping plus multibyte boundaries.
        let escaped = format!("{}💭", "\u{0001}".repeat(PAYLOAD_CHUNK_BYTES - 1)).repeat(3);
        job.raw_response = Some(escaped.clone());
        job.candidate_json = Some(escaped.clone());
        job.critic_json = Some(escaped.clone());
        app.spend.push(Spend {
            id: "synthetic-paid-lease".into(),
            work_id: "fixture-job".into(),
            created_ms: 1_700_000_000_000,
            reserved_micros: RESERVATION,
            cost_micros: None,
            status: "unknown".into(),
            model: "synthetic-model".into(),
            response_hash: Some(sha256(escaped.as_bytes())),
            response_model: Some("synthetic-model".into()),
            response_outcome: Some("received".into()),
            response_bytes: Some(escaped.len()),
            raw_response: Some(escaped),
        });
        let stored = rows(&app).unwrap();
        assert!(stored.keys().any(|key| key.starts_with("job_payload/")));
        assert!(stored.keys().any(|key| key.starts_with("spend_payload/")));
        assert!(stored.values().all(|value| value.len() <= MAX_RECORD_BYTES));
        assert_eq!(app_from_rows(&stored).unwrap(), app);
        let mut incomplete = stored;
        let last = incomplete
            .keys()
            .filter(|key| key.starts_with("spend_payload/"))
            .nth(1)
            .unwrap()
            .clone();
        incomplete.remove(&last);
        assert!(app_from_rows(&incomplete).is_err());
    }
}
