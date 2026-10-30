use scry::private_learner::{matches, owner_only_path, space_name};

#[test]
fn absent_or_invalid_configuration_never_admits_a_learner() {
    for subject in [
        "", " owner", "owner ", "a/b", "a@b.com", "*", "\n", "é", "owner",
    ] {
        assert!(space_name(subject, "owner", "existing-owner-space").is_none());
        assert!(!matches(subject, subject, "owner", "existing-owner-space"));
    }
    assert!(space_name(&"a".repeat(129), "owner", "space").is_none());
    assert!(!matches("", "member", "owner", "space"));
    assert!(!matches("Member", "member", "owner", "space"));
}

#[test]
fn stable_subject_maps_to_a_separate_opaque_space_not_an_email() {
    let name = space_name("synthetic-member-1", "owner", "space").unwrap();
    assert_eq!(
        name,
        space_name("synthetic-member-1", "owner", "space").unwrap()
    );
    assert!(name.starts_with("scry-learner-"));
    assert!(!name.contains("synthetic-member"));
    assert_ne!(
        name,
        space_name("synthetic-member-2", "owner", "space").unwrap()
    );
    assert!(space_name("synthetic-member-1", "owner", &name).is_none());
    assert!(matches(
        "synthetic-member-1",
        "synthetic-member-1",
        "owner",
        "space"
    ));
}

#[test]
fn owner_capabilities_are_not_available_to_learner_routes() {
    for path in [
        "/operator",
        "/operator/restore",
        "/operator/resume-work",
        "/tokens",
        "/tokens/create",
        "/tokens/revoke",
        "/internal/backup",
        "/__internal/daily",
        "/__internal/owner-token",
        "/api/v1/works",
    ] {
        assert!(owner_only_path(path), "{path}");
    }
    for path in [
        "/",
        "/recall",
        "/library",
        "/library/add",
        "/practice/answer",
        "/practice/nato/start",
        "/history",
        "/export",
        "/settings",
    ] {
        assert!(!owner_only_path(path), "{path}");
    }
}
