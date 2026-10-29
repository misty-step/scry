//! Small curated, externally authored starter shelf. Grading lives in native policy.
use crate::{
    library::{self, VersionInput},
    model::*,
};
use serde::Deserialize;
#[derive(Clone, Debug, Deserialize)]
pub struct Lesson {
    pub id: String,
    pub description: String,
    pub input: VersionInput,
}
pub fn catalog() -> Vec<Lesson> {
    serde_json::from_str(include_str!("../assets/lessons.json"))
        .expect("bundled authored lessons are valid")
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
