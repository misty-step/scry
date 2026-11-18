//! Stored bytes of the Owner's real keyed mappings. NATO and the three
//! baby-care starters, practised forward with only what existed before keyed
//! mappings gained directions, safety-bearing pairs and the year rule, must
//! produce exactly the rows and archive that source `6ace75e` produced for the
//! same steps. The hashes below were recorded by running this scenario, built
//! against `6ace75e`, before the change.
use scry::{engine, model::*, owner_token::MutationActor, persistence};
use std::collections::BTreeMap;

const ROWS_SHA256: &str = "dd1b65b8894dad0e433a9ecdbeb4bf1e0935b4f9a770d05b0fa7707866a1f966";
const ARCHIVE_SHA256: &str = "d7426eb26ce4a74cf11662abb3d7c29cc62ee034c35c2a7bda8986e29fb8ca15";
const FORMAT: &str = "scry-rust-durable-sqlite-study-phase-v3";

const NOW: i64 = 1_800_000_000_000;

struct O {
    app: App,
    n: u64,
    now: i64,
}
impl O {
    fn call(&mut self, path: &str, pairs: &[(&str, &str)]) -> AppResult<MutationResult> {
        self.n += 1;
        let op = format!("differential-operation-{:06}", self.n);
        let mut f = BTreeMap::from([
            ("csrf".to_string(), self.app.csrf.clone()),
            ("operation_id".to_string(), op.clone()),
        ]);
        f.extend(pairs.iter().map(|(k, v)| (k.to_string(), v.to_string())));
        let r = engine::mutate_judged(
            &mut self.app,
            path,
            &f,
            self.now,
            &format!("seed-{op}"),
            MutationActor::Browser,
            false,
        );
        self.now += 61_000;
        if r.is_ok() {
            engine::validate_app(&self.app).unwrap();
        }
        r
    }
    fn held(&self) -> Option<&scry::study::Presentation> {
        self.app.practice.current.as_ref()
    }
    /// Answer the held Prompt: exact, a miss, or after a reveal.
    fn answer(&mut self, how: u64) {
        let Some(p) = self.held().cloned() else {
            return;
        };
        if p.attempt_id.is_some() {
            return;
        }
        if how % 5 == 3 {
            let _ = self.call("/practice/reveal", &[("prompt_id", &p.prompt.id)]);
        }
        let p = self.held().cloned().unwrap();
        if p.source_visible() {
            self.call("/practice/quiz", &[("prompt_id", &p.prompt.id)])
                .unwrap();
        }
        let p = self.held().cloned().unwrap();
        let v = &self.app.library.versions[&p.prompt.version_id];
        let units: Vec<String> = v
            .sections
            .iter()
            .flat_map(|s| &s.units)
            .map(|u| u.text.clone())
            .collect();
        let exact = p
            .targets
            .iter()
            .map(|t| {
                let u = &units[t.unit_index];
                scry::library::mapping_pair(u).map_or(u.trim().to_string(), |(k, v)| {
                    if p.prompt.cue_level == scry::study::Cue::MappingKey {
                        v.to_string()
                    } else {
                        k.to_string()
                    }
                })
            })
            .collect::<Vec<_>>()
            .join("\n");
        let text = match how % 4 {
            1 => "not the answer".to_string(),
            2 => exact.to_uppercase(),
            _ => exact,
        };
        self.call(
            "/practice/answer",
            &[("prompt_id", &p.prompt.id), ("answer", &text)],
        )
        .unwrap();
    }
    fn after(&self) -> String {
        self.held().map(|p| p.prompt.id.clone()).unwrap_or_default()
    }
}

fn forward() -> O {
    let mut o = O {
        app: App::new("differential-synthetic-csrf-0123456789".into()),
        n: 0,
        now: NOW,
    };
    o.call("/practice/nato/start", &[]).unwrap();
    for i in 0..12 {
        o.answer(i);
        let sid = o.held().unwrap().prompt.study_id.clone();
        let after = o.after();
        o.call("/practice/lesson/next", &[("after", &after)])
            .unwrap_or_else(|_| {
                o.call("/practice/next", &[("after", &after), ("study_id", &sid)])
                    .unwrap()
            });
    }
    for starter in ["baby-soothing", "baby-everyday", "baby-montessori"] {
        let after = o.after();
        let mut fields = vec![("starter", starter)];
        let leave;
        if o.held().is_some_and(|p| p.attempt_id.is_some()) {
            fields.push(("after", after.as_str()));
        } else if o.held().is_some() {
            leave = after.clone();
            fields.push(("leave", leave.as_str()));
        }
        o.call("/practice/lesson/start", &fields).unwrap();
        for i in 0..7 {
            o.answer(i + 1);
            let after = o.after();
            if o.call("/practice/lesson/next", &[("after", &after)])
                .is_err()
                || o.held().is_none()
            {
                break;
            }
        }
    }
    // Study's mix across everything, a day later.
    o.now += 86_400_000;
    let after = o.after();
    if o.held().is_some_and(|p| p.attempt_id.is_some()) {
        o.call("/practice/review/next", &[("after", &after)])
            .unwrap();
    } else {
        o.call("/practice/review/start", &[]).unwrap();
    }
    for i in 0..30 {
        if o.held().is_none() {
            break;
        }
        o.answer(i);
        let after = o.after();
        o.call("/practice/review/next", &[("after", &after)])
            .unwrap();
    }
    // An agent's forward mapping with a year value, answered as before.
    let data = r#"{"title":"Differential dates","direction":"forward","pairs":[{"key":"Differential battle","value":"1066"}]}"#;
    let vid = o
        .call("/library/mapping/add", &[("data", data)])
        .unwrap()
        .entity_id
        .unwrap();
    o.call("/practice/learn", &[("version_id", &vid)]).unwrap();
    o
}

#[test]
fn nato_and_baby_care_practised_forward_keep_the_bytes_of_the_earlier_source() {
    let o = forward();
    let rows = persistence::rows(&o.app).unwrap();
    assert!(!rows.contains_key("mapping_compatibility/v1"));
    assert_eq!(
        persistence::sha256(&serde_json::to_vec_pretty(&rows).unwrap()),
        ROWS_SHA256
    );
    let archive = persistence::Archive::new(o.app.clone(), vec![], o.now);
    assert_eq!(archive.format, FORMAT);
    assert_eq!(
        persistence::sha256(&archive.encode().unwrap()),
        ARCHIVE_SHA256
    );
    assert_eq!(
        (o.app.practice.attempts.len(), o.app.practice.cards.len()),
        (74, 28)
    );
}
