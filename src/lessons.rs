//! Small curated, externally authored starter shelf. Grading lives in native policy.
use crate::{
    library::{self, MappingInput, VersionInput},
    model::*,
};
use serde::Deserialize;
#[derive(Clone, Debug, Deserialize)]
pub struct Lesson {
    pub id: String,
    pub description: String,
    pub input: VersionInput,
}
impl Lesson {
    /// Presentation identity requires complete authored content, not just a
    /// user-editable title or edition name. Never expose custom metadata as a
    /// trusted starter while a question is protected.
    pub fn matches(&self, version: &library::Version) -> bool {
        let input = &self.input;
        input.kind == version.kind
            && input.name == version.name
            && input.text == version.text
            && input.spelling_strict == version.spelling_strict
            && input.ordered_aliases == version.ordered_aliases
            && input.sections.as_ref().is_some_and(|sections| {
                sections.len() == version.sections.len()
                    && sections.iter().zip(&version.sections).all(|(a, b)| {
                        a.heading == b.heading
                            && a.units.len() == b.units.len()
                            && a.units.iter().zip(&b.units).all(|(a, b)| a == &b.text)
                    })
            })
    }
}
pub fn catalog() -> Vec<Lesson> {
    let mut lessons: Vec<Lesson> = serde_json::from_str(include_str!("../assets/lessons.json"))
        .expect("bundled authored lessons are valid");
    for (id, description, source) in [
        (
            "baby-soothing",
            "Safe soothing",
            include_str!("../examples/baby-soothing.json"),
        ),
        (
            "baby-everyday",
            "Everyday safety",
            include_str!("../examples/baby-everyday.json"),
        ),
        (
            "baby-montessori",
            "Montessori-inspired care",
            include_str!("../examples/baby-montessori.json"),
        ),
    ] {
        let mapping: MappingInput = serde_json::from_str(source).expect("bundled mapping is valid");
        lessons.push(Lesson {
            id: id.into(),
            description: description.into(),
            input: mapping.into_version().expect("bundled mapping imports"),
        });
    }
    lessons
}
pub fn input(id: &str) -> AppResult<VersionInput> {
    if id == "nato" {
        return library::nato_input();
    }
    catalog()
        .into_iter()
        .find(|s| s.id == id)
        .map(|s| s.input)
        .ok_or_else(|| AppError::new(404, "Lesson not found."))
}

/// Only exact bundled content receives a trusted focus route; editable titles
/// or supplied metadata cannot impersonate a starter.
pub fn mapping_route(version: &library::Version) -> Option<&'static str> {
    catalog()
        .iter()
        .find(|l| l.input.kind == library::Kind::KeyedMapping && l.matches(version))
        .and_then(|l| match l.id.as_str() {
            "baby-soothing" => Some("/?practice=baby-soothing"),
            "baby-everyday" => Some("/?practice=baby-everyday"),
            "baby-montessori" => Some("/?practice=baby-montessori"),
            _ => None,
        })
}
