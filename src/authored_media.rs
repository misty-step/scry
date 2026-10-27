//! Supplied review media. No fetching, generation, transcription or grading.
use crate::{
    library::{MappingInput, Rights, Version},
    model::*,
    persistence::sha256,
};
use base64::{Engine as _, engine::general_purpose::STANDARD};
use serde::{Deserialize, Serialize};
use std::collections::{BTreeMap, BTreeSet};

pub const ASSET_BYTES: usize = 1024 * 1024;
/// Complete JSON request, including base64 and metadata, not decoded bytes.
pub const IMPORT_BYTES: usize = 4 * 1024 * 1024;
pub const MAX_ATTACHMENTS: usize = 64;
pub const FORMAT: &str = "scry-rust-durable-sqlite-authored-media-v1";

#[derive(Clone, Copy, Debug, Deserialize, Serialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum Side {
    Prompt,
    Answer,
}
#[derive(Clone, Debug, Deserialize, Serialize, PartialEq, Eq)]
#[serde(deny_unknown_fields)]
pub struct Asset {
    pub mime: String,
    pub size: usize,
    pub sha256: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub width: Option<u32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub height: Option<u32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub duration_ms: Option<u32>,
}
#[derive(Clone, Debug, Deserialize, Serialize, PartialEq, Eq)]
#[serde(deny_unknown_fields)]
pub struct Attachment {
    pub unit_index: usize,
    pub side: Side,
    pub file_name: String,
    pub alt: String,
    pub transcript: String,
    pub source: String,
    pub rights: Rights,
    pub license: String,
    pub asset: Asset,
}
#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
pub struct Upload {
    pub unit_index: usize,
    pub side: Side,
    pub file_name: String,
    pub alt: String,
    pub transcript: String,
    pub source: String,
    pub rights: Rights,
    pub license: String,
    pub mime: String,
    pub sha256: String,
    pub bytes_base64: String,
}
#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
pub struct Input {
    pub mapping: MappingInput,
    pub attachments: Vec<Upload>,
}
#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct ArchivedAsset {
    pub asset: Asset,
    #[serde(with = "asset_data")]
    pub bytes: Vec<u8>,
}
mod asset_data {
    use super::*;
    pub fn serialize<S: serde::Serializer>(bytes: &[u8], s: S) -> Result<S::Ok, S::Error> {
        s.serialize_str(&STANDARD.encode(bytes))
    }
    pub fn deserialize<'de, D: serde::Deserializer<'de>>(d: D) -> Result<Vec<u8>, D::Error> {
        let text = String::deserialize(d)?;
        decode(&text).map_err(|_| serde::de::Error::custom("Invalid bounded media bytes."))
    }
}
fn invalid() -> AppError {
    AppError::new(
        422,
        "Supply bounded PNG or PCM WAV media with matching hash, accessible text, source and rights metadata.",
    )
}
fn decode(text: &str) -> AppResult<Vec<u8>> {
    if text.len() > ASSET_BYTES.div_ceil(3) * 4 {
        return Err(invalid());
    }
    let bytes = STANDARD.decode(text).map_err(|_| invalid())?;
    if bytes.is_empty() || bytes.len() > ASSET_BYTES || STANDARD.encode(&bytes) != text {
        return Err(invalid());
    }
    Ok(bytes)
}
fn label(text: &str, max: usize) -> bool {
    !text.trim().is_empty() && text.len() <= max && !text.chars().any(char::is_control)
}
fn hash(text: &str) -> bool {
    text.len() == 64
        && text
            .bytes()
            .all(|b| b.is_ascii_digit() || (b'a'..=b'f').contains(&b))
}
fn metadata(a: &Attachment, units: usize) -> bool {
    a.unit_index < units
        && label(&a.file_name, 128)
        && !a.file_name.contains(['/', '\\'])
        && label(&a.source, 2048)
        && label(&a.license, 2048)
        && a.alt.len() <= 1024
        && a.transcript.len() <= 8192
        && a.asset.size > 0
        && a.asset.size <= ASSET_BYTES
        && hash(&a.asset.sha256)
        && match a.asset.mime.as_str() {
            "image/png" => {
                label(&a.alt, 1024)
                    && a.transcript.is_empty()
                    && a.asset.width.is_some_and(|n| (1..=2048).contains(&n))
                    && a.asset.height.is_some_and(|n| (1..=2048).contains(&n))
                    && a.asset.duration_ms.is_none()
            }
            "audio/wav" => {
                label(&a.transcript, 8192)
                    && a.alt.is_empty()
                    && a.asset.width.is_none()
                    && a.asset.height.is_none()
                    && a.asset
                        .duration_ms
                        .is_some_and(|n| (1..=30_000).contains(&n))
            }
            _ => false,
        }
}
fn crc32(bytes: &[u8]) -> u32 {
    let mut crc = !0u32;
    for byte in bytes {
        crc ^= u32::from(*byte);
        for _ in 0..8 {
            crc = (crc >> 1) ^ (0xedb88320 & 0u32.wrapping_sub(crc & 1));
        }
    }
    !crc
}
fn be32(bytes: &[u8]) -> u32 {
    u32::from_be_bytes(bytes.try_into().unwrap())
}
fn le32(bytes: &[u8]) -> u32 {
    u32::from_le_bytes(bytes.try_into().unwrap())
}
/// Strict bounded PNG chunk envelope. Browser remains the image decoder; no
/// SVG, ancillary metadata, animation or unknown chunks are admitted.
fn png(bytes: &[u8]) -> Option<(u32, u32)> {
    if !bytes.starts_with(b"\x89PNG\r\n\x1a\n") {
        return None;
    }
    let mut pos = 8usize;
    let mut dimensions = None;
    let mut data = false;
    while pos.checked_add(12)? <= bytes.len() {
        let n = be32(&bytes[pos..pos + 4]) as usize;
        let end = pos.checked_add(12)?.checked_add(n)?;
        if end > bytes.len() || crc32(&bytes[pos + 4..end - 4]) != be32(&bytes[end - 4..end]) {
            return None;
        }
        let kind = &bytes[pos + 4..pos + 8];
        let body = &bytes[pos + 8..end - 4];
        match kind {
            b"IHDR" if pos == 8 && n == 13 => {
                let w = be32(&body[..4]);
                let h = be32(&body[4..8]);
                if !(1..=2048).contains(&w)
                    || !(1..=2048).contains(&h)
                    || body[8] != 8
                    || !matches!(body[9], 0 | 2 | 4 | 6)
                    || body[10..] != [0, 0, 0]
                {
                    return None;
                }
                dimensions = Some((w, h));
            }
            b"IDAT" if dimensions.is_some() && n > 0 => data = true,
            b"IEND" if dimensions.is_some() && data && n == 0 && end == bytes.len() => {
                return dimensions;
            }
            _ => return None,
        }
        pos = end;
    }
    None
}
/// Canonical little-endian mono/stereo signed 16-bit PCM, 8–48 kHz, <=30s.
/// Exactly fmt(16) + data chunks, with no embedded names or metadata.
fn wav(bytes: &[u8]) -> Option<u32> {
    if bytes.len() < 46
        || &bytes[..4] != b"RIFF"
        || le32(&bytes[4..8]) as usize != bytes.len() - 8
        || &bytes[8..16] != b"WAVEfmt "
        || le32(&bytes[16..20]) != 16
        || bytes[20..22] != [1, 0]
        || bytes[34..36] != [16, 0]
        || &bytes[36..40] != b"data"
    {
        return None;
    }
    let channels = u16::from_le_bytes(bytes[22..24].try_into().ok()?) as u32;
    let rate = le32(&bytes[24..28]);
    let byte_rate = le32(&bytes[28..32]);
    let align = u16::from_le_bytes(bytes[32..34].try_into().ok()?) as u32;
    let size = le32(&bytes[40..44]);
    if !matches!(channels, 1 | 2)
        || !(8000..=48000).contains(&rate)
        || align != channels * 2
        || byte_rate != rate * align
        || size == 0
        || size as usize != bytes.len() - 44
        || !size.is_multiple_of(align)
        || size > byte_rate * 30
    {
        return None;
    }
    Some((u64::from(size) * 1000).div_ceil(u64::from(byte_rate)) as u32)
}
pub fn inspect(bytes: &[u8], mime: &str, expected_hash: &str) -> AppResult<Asset> {
    if bytes.is_empty()
        || bytes.len() > ASSET_BYTES
        || !hash(expected_hash)
        || sha256(bytes) != expected_hash
    {
        return Err(invalid());
    }
    let (width, height, duration_ms) = match mime {
        "image/png" => {
            let (w, h) = png(bytes).ok_or_else(invalid)?;
            (Some(w), Some(h), None)
        }
        "audio/wav" => (None, None, Some(wav(bytes).ok_or_else(invalid)?)),
        _ => return Err(invalid()),
    };
    Ok(Asset {
        mime: mime.into(),
        size: bytes.len(),
        sha256: expected_hash.into(),
        width,
        height,
        duration_ms,
    })
}
impl Input {
    pub fn checked(&self) -> AppResult<(Vec<Attachment>, Vec<ArchivedAsset>)> {
        self.mapping.clone().into_version()?;
        if self.attachments.is_empty() || self.attachments.len() > MAX_ATTACHMENTS {
            return Err(invalid());
        }
        let mut attachments = Vec::new();
        let mut assets = BTreeMap::new();
        for u in &self.attachments {
            let bytes = decode(&u.bytes_base64)?;
            let asset = inspect(&bytes, &u.mime, &u.sha256)?;
            let attachment = Attachment {
                unit_index: u.unit_index,
                side: u.side,
                file_name: u.file_name.clone(),
                alt: u.alt.clone(),
                transcript: u.transcript.clone(),
                source: u.source.clone(),
                rights: u.rights,
                license: u.license.clone(),
                asset: asset.clone(),
            };
            if !metadata(&attachment, self.mapping.pairs.len()) {
                return Err(invalid());
            }
            attachments.push(attachment);
            assets.insert(asset.sha256.clone(), ArchivedAsset { asset, bytes });
        }
        validate_attachments(&attachments, self.mapping.pairs.len())?;
        Ok((attachments, assets.into_values().collect()))
    }
}
pub fn parse(text: &str) -> AppResult<Input> {
    if text.len() > IMPORT_BYTES {
        return Err(AppError::new(413, "Media import exceeds 4 MiB of JSON."));
    }
    serde_json::from_str(text).map_err(|_| invalid())
}
pub fn identity(media: &[Attachment]) -> Option<String> {
    (!media.is_empty()).then(|| sha256(&serde_json::to_vec(media).expect("media serializes")))
}
pub fn validate_attachments(media: &[Attachment], units: usize) -> AppResult<()> {
    let mut slots = BTreeSet::new();
    if media.len() > MAX_ATTACHMENTS
        || media.iter().any(|a| {
            !metadata(a, units)
                || !slots.insert((a.unit_index, a.side as u8, a.asset.mime.as_str()))
        })
    {
        return Err(invalid());
    }
    Ok(())
}
pub fn validate_version(version: &Version) -> AppResult<()> {
    validate_attachments(
        &version.media,
        version.sections.iter().map(|s| s.units.len()).sum(),
    )?;
    if version.media_hash != identity(&version.media)
        || (!version.media.is_empty() && version.kind != crate::library::Kind::KeyedMapping)
    {
        return Err(invalid());
    }
    Ok(())
}
pub fn used(app: &App) -> bool {
    app.library.versions.values().any(|v| !v.media.is_empty())
        || app.operations.values().any(|r| {
            r.path
                .as_deref()
                .is_some_and(|p| p.starts_with("/library/media/"))
        })
}
pub fn required(app: &App) -> AppResult<BTreeMap<&str, &Asset>> {
    let mut assets = BTreeMap::new();
    for version in app.library.versions.values() {
        validate_version(version)?;
        for a in &version.media {
            if assets
                .insert(a.asset.sha256.as_str(), &a.asset)
                .is_some_and(|old| old != &a.asset)
            {
                return Err(invalid());
            }
        }
    }
    Ok(assets)
}
/// Under a protected Prompt, only this Prompt's declared prompt-side attachment
/// is readable. The entire answer attachment (including names and metadata) is absent.
pub fn allowed(app: &App, version: &Version, attachment: &Attachment) -> bool {
    if app.protected_goal().is_some() {
        return false;
    }
    if !app.practice.protected() {
        return true;
    }
    let p = &app.practice.current.as_ref().unwrap().prompt;
    attachment.side == Side::Prompt
        && p.version_id == version.id
        && version
            .sections
            .iter()
            .flat_map(|s| &s.units)
            .nth(attachment.unit_index)
            .is_some_and(|u| p.unit_ids.contains(&u.id))
}
pub fn lookup<'a>(app: &'a App, version_id: &str, index: usize) -> AppResult<&'a Attachment> {
    let version = app
        .library
        .versions
        .get(version_id)
        .ok_or_else(|| AppError::new(404, "Media unavailable."))?;
    let attachment = version
        .media
        .get(index)
        .ok_or_else(|| AppError::new(404, "Media unavailable."))?;
    if !allowed(app, version, attachment) {
        return Err(AppError::new(
            409,
            "Record assistance before reading answer media.",
        ));
    }
    Ok(attachment)
}
pub fn for_prompt(app: &App, prompt: &crate::study::Prompt) -> Vec<serde_json::Value> {
    let Some(version) = app.library.versions.get(&prompt.version_id) else {
        return vec![];
    };
    version
        .media
        .iter()
        .enumerate()
        .filter(|(_, a)| {
            allowed(app, version, a)
                && version
                    .sections
                    .iter()
                    .flat_map(|s| &s.units)
                    .nth(a.unit_index)
                    .is_some_and(|u| prompt.unit_ids.contains(&u.id))
        })
        .map(|(index, a)| {
            let mut value = serde_json::to_value(a).expect("media serializes");
            value["url"] = serde_json::json!(format!("/api/v1/media/{}/{index}", version.id));
            value
        })
        .collect()
}
pub fn wire_response(app: &App, value: &mut serde_json::Value) {
    let prompt = value
        .get("prompt")
        .cloned()
        .or_else(|| value.get("attempt").and_then(|a| a.get("prompt")).cloned())
        .or_else(|| {
            value
                .get("prompt_id")
                .and_then(|v| v.as_str())
                .and_then(|id| app.practice.current.as_ref().filter(|p| p.prompt.id == id))
                .map(|p| serde_json::json!(p.prompt))
        });
    if let Some(prompt) =
        prompt.and_then(|p| serde_json::from_value::<crate::study::Prompt>(p).ok())
    {
        let media = for_prompt(app, &prompt);
        if !media.is_empty() {
            value["media"] = serde_json::json!(media);
        }
    }
}
