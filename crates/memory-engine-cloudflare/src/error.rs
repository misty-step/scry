use std::{error::Error, fmt};

use memory_engine_generation::BetaGenerationError;
use memory_engine_persistence::BetaStoreError;
use memory_engine_service::{ContentFeedbackError, ServiceError};
use memory_engine_study::BetaStudyError;

pub type AppResult<T> = Result<T, Failure>;

/// A safe boundary failure. Provider, SQL and credential payloads are never messages.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct Failure {
    pub status: u16,
    pub message: String,
}

impl Failure {
    #[must_use]
    pub fn new(status: u16, message: impl Into<String>) -> Self {
        Self {
            status,
            message: message.into(),
        }
    }

    #[must_use]
    pub fn bad_request(message: impl Into<String>) -> Self {
        Self::new(400, message)
    }
    #[must_use]
    pub fn forbidden(message: impl Into<String>) -> Self {
        Self::new(403, message)
    }
    #[must_use]
    pub fn not_found(message: impl Into<String>) -> Self {
        Self::new(404, message)
    }
    #[must_use]
    pub fn conflict(message: impl Into<String>) -> Self {
        Self::new(409, message)
    }
    #[must_use]
    pub fn internal(message: impl Into<String>) -> Self {
        Self::new(500, message)
    }
}

impl fmt::Display for Failure {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        formatter.write_str(&self.message)
    }
}

impl Error for Failure {}

impl From<worker::Error> for Failure {
    fn from(_: worker::Error) -> Self {
        Self::internal("A Cloudflare runtime operation failed.")
    }
}

impl From<serde_json::Error> for Failure {
    fn from(_: serde_json::Error) -> Self {
        Self::bad_request("The JSON data does not match the required contract.")
    }
}

impl From<BetaStoreError> for Failure {
    fn from(error: BetaStoreError) -> Self {
        match error {
            BetaStoreError::Io(_)
            | BetaStoreError::Json(_)
            | BetaStoreError::UnsupportedVersion(_)
            | BetaStoreError::InjectedCommitFailure => {
                Self::internal("Stored study data could not be read or committed.")
            }
            BetaStoreError::UnknownSourceDocument(_)
            | BetaStoreError::UnknownReferenceSpan(_)
            | BetaStoreError::UnknownConceptReferenceNote(_)
            | BetaStoreError::UnknownReviewUnit(_)
            | BetaStoreError::UnknownGeneratedPromptDraft(_) => Self::not_found(error.to_string()),
            BetaStoreError::SourceDocumentArchived(_)
            | BetaStoreError::ReviewUnitArchived(_)
            | BetaStoreError::LearnerDraftDecisionAlreadyRecorded(_)
            | BetaStoreError::DuplicateAppliedReview(_)
            | BetaStoreError::DuplicateContentFeedback(_)
            | BetaStoreError::FeedbackSupersedesStale { .. }
            | BetaStoreError::StaleScheduleWrite(_) => Self::conflict(error.to_string()),
            _ => Self::bad_request(error.to_string()),
        }
    }
}

impl From<ServiceError<Failure>> for Failure {
    fn from(error: ServiceError<Failure>) -> Self {
        match error {
            ServiceError::Store(error) => error,
            ServiceError::Scheduler(_) => {
                Self::internal("The review schedule could not be calculated.")
            }
        }
    }
}

impl From<BetaGenerationError<Failure>> for Failure {
    fn from(error: BetaGenerationError<Failure>) -> Self {
        match error {
            BetaGenerationError::Store(error) => error,
            BetaGenerationError::UnknownSourceDocument(_)
            | BetaGenerationError::UnknownReviewUnit(_) => Self::not_found(error.to_string()),
            BetaGenerationError::ArchivedSourceDocument(_) => Self::conflict(error.to_string()),
            BetaGenerationError::LocalOnlySource(_) => Self::forbidden(error.to_string()),
            BetaGenerationError::SourceDocumentHasNoTextBody(_) => {
                Self::bad_request(error.to_string())
            }
            BetaGenerationError::ProviderFailure(_) => {
                Self::new(502, "Content generation did not complete.")
            }
        }
    }
}

impl From<BetaStudyError<Failure>> for Failure {
    fn from(error: BetaStudyError<Failure>) -> Self {
        match error {
            BetaStudyError::Store(error) => error,
            BetaStudyError::Generation(error) => error.into(),
            BetaStudyError::Service(error) => error.into(),
            BetaStudyError::UnknownReferenceSpan(_) => {
                Self::not_found("Study reference not found.")
            }
            BetaStudyError::NoActiveReviewUnit => {
                Self::conflict("This review is no longer active. Open the next quiz.")
            }
            BetaStudyError::NoConceptKey => {
                Self::bad_request("This quiz has no concept reference.")
            }
        }
    }
}

impl From<ContentFeedbackError<Failure>> for Failure {
    fn from(error: ContentFeedbackError<Failure>) -> Self {
        match error {
            ContentFeedbackError::Store(error) => error,
            ContentFeedbackError::BlankFeedbackId | ContentFeedbackError::BlankAccountId => {
                Self::bad_request("Feedback identity is required.")
            }
        }
    }
}
