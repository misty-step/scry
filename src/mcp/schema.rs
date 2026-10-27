use serde_json::{Value, json};

pub(super) const NAMES: &[&str] = &[
    "scry_capabilities",
    "scry_schema",
    "scry_search",
    "scry_show",
    "scry_validate",
    "scry_import",
    "scry_learn",
    "scry_due",
    "scry_next",
    "scry_answer",
    "scry_reveal",
];

fn object(properties: Value, required: &[&str]) -> Value {
    json!({"type":"object","properties":properties,"required":required,"additionalProperties":false})
}
fn choices(values: &[&str]) -> Value {
    json!({"type":"string","enum":values})
}
fn id() -> Value {
    json!({"type":"string","pattern":"^[0-9a-fA-F]{32}$"})
}
fn operation() -> Value {
    json!({"type":"string","pattern":"^[A-Za-z0-9_-]{16,128}$","description":"Caller-created and retained before sending. Repeat exactly the same ID and payload to reconcile an unknown response; never generate a replacement implicitly."})
}
fn common_metadata() -> Value {
    json!({
        "title":{"type":"string","minLength":1,"maxLength":240,"description":"At most 240 UTF-8 bytes."},
        "name":{"type":"string","maxLength":240},
        "context":{"type":"string","maxLength":4096},
        "aliases":{"type":"array","maxItems":32,"items":{"type":"string","minLength":1,"maxLength":240}},
        "sources":{"type":"array","maxItems":32,"items":{"type":"string","minLength":1,"maxLength":2048},"description":"Caller-supplied citations; never fetched."},
        "rights":{"type":"string","enum":["private_only","public_domain","licensed"],"default":"private_only"},
        "spelling_strict":{"type":"boolean"}
    })
}
pub(super) fn manifests() -> Value {
    let mut version = common_metadata();
    version["kind"] = choices(&["text", "ordered_list", "keyed_mapping"]);
    version["text"] = json!({"type":"string","minLength":1,"maxLength":262144,"description":"At most 256 KiB of UTF-8; supplied Units must join to these bytes exactly."});
    version["sections"] = json!({"type":["array","null"],"maxItems":4096,"description":"At most 8192 Units total. Omit for native segmentation.","items":object(json!({"heading":{"type":"string","maxLength":240},"units":{"type":"array","minItems":1,"maxItems":8192,"items":{"type":"string","minLength":1}}}),&["heading","units"])});
    let mut mapping = common_metadata();
    mapping["direction"] = choices(&["forward"]);
    mapping["spelling_strict"]["default"] = json!(true);
    mapping["pairs"] = json!({"type":"array","minItems":1,"maxItems":8192,"description":"Unique normalized keys, nonempty key/value, no control characters or surrounding whitespace. Combined stored text must fit 256 KiB.","items":object(json!({"key":{"type":"string","minLength":1,"maxLength":240},"value":{"type":"string","minLength":1,"maxLength":4096}}),&["key","value"])});
    json!({"schema_dialect":"https://json-schema.org/draft/2020-12/schema",
        "version":object(version,&["title","kind","text"]),
        "mapping":object(mapping,&["title","direction","pairs"]),
        "validation_scope":"Local shape/domain validation only; remote duplicates, policy, capacity, study admission and hosted access are not checked.",
        "limits":"All domain text limits are UTF-8 bytes, not characters. JSON Schema bounds are supplemented by the native Library validator.",
        "multimedia":"Not implemented in this slice; no media URL is fetched.",
        "study_support":"Text and forward keyed mappings; ordered lists can be stored, not practised by this slice."})
}

pub(super) fn tools() -> Vec<Value> {
    let empty = object(json!({}), &[]);
    let schemas = manifests();
    let manifest = object(
        json!({"format":choices(&["version","mapping"]),"data":{"type":"object","description":"Use scry_schema to select the version or mapping schema."}}),
        &["format", "data"],
    );
    let mut import = manifest.clone();
    import["properties"]["operation_id"] = operation();
    import["required"] = json!(["operation_id", "format", "data"]);
    // Branch-specific data schemas are machine-readable as well as discoverable.
    import["allOf"] = json!([
        {"if":{"properties":{"format":{"const":"version"}}},"then":{"properties":{"data":schemas["version"]}}},
        {"if":{"properties":{"format":{"const":"mapping"}}},"then":{"properties":{"data":schemas["mapping"]}}}
    ]);
    let mut validate = import.clone();
    validate["properties"]
        .as_object_mut()
        .unwrap()
        .remove("operation_id");
    validate["required"] = json!(["format", "data"]);
    let definitions = [
        (
            "scry_capabilities",
            "Inspect this adapter's supported surface, limits and retry rules. This does not probe hosted availability.",
            empty.clone(),
            true,
        ),
        (
            "scry_schema",
            "Discover exact externally authored Version and forward mapping input schemas. No native generation or multimedia import.",
            empty.clone(),
            true,
        ),
        (
            "scry_search",
            "Search private Library titles, aliases and opening words before import. Empty query lists content; protected unaided practice may refuse reads.",
            object(
                json!({"query":{"type":"string","maxLength":4096}}),
                &["query"],
            ),
            true,
        ),
        (
            "scry_show",
            "Show one Work or Version through the existing Owner API assistance fence. Does not start practice.",
            object(
                json!({"kind":choices(&["work","version"]),"id":id()}),
                &["kind", "id"],
            ),
            true,
        ),
        (
            "scry_validate",
            "Validate supplied material locally with Scry's native Library validator; writes nothing. Does not establish remote acceptance, deduplication or capacity.",
            validate,
            true,
        ),
        (
            "scry_import",
            "Import supplied material through the Owner API; no generation and no automatic Study. Retain operation_id and show the returned entity identity.",
            import,
            false,
        ),
        (
            "scry_learn",
            "Start or return the retained Study for a Version. Retain operation_id; native Scry owns grading and scheduling.",
            object(
                json!({"operation_id":operation(),"version_id":id(),"pace":choices(&["light","steady","intense"])}),
                &["operation_id", "version_id"],
            ),
            false,
        ),
        (
            "scry_due",
            "Read due and upcoming Cards without presenting or consuming a Prompt.",
            empty,
            true,
        ),
        (
            "scry_next",
            "Present/resume the current Prompt or held result. Only after deliberately releases an answered Prompt. Reconcile every unknown answer/reveal response BEFORE Next.",
            object(
                json!({"operation_id":operation(),"after":id(),"study_id":id(),"card_id":id(),"recite":choices(&["whole","section"]),"section_id":id()}),
                &["operation_id"],
            ),
            false,
        ),
        (
            "scry_answer",
            "Submit the learner's raw answer; native Scry grades and schedules. Empty answer is a miss. No verdict, rating, correct flag or self-grade is accepted.",
            object(
                json!({"operation_id":operation(),"prompt_id":id(),"answer":{"type":"string","maxLength":262144,"description":"At most 256 KiB UTF-8, unchanged raw learner answer."}}),
                &["operation_id", "prompt_id", "answer"],
            ),
            false,
        ),
        (
            "scry_reveal",
            "Deliberately reveal a Prompt's target. Scry durably records assistance before disclosure; this is not a correct answer or unaided success.",
            object(
                json!({"operation_id":operation(),"prompt_id":id()}),
                &["operation_id", "prompt_id"],
            ),
            false,
        ),
    ];
    definitions.into_iter().map(|(name,description,input,read_only)| json!({
        "name":name,"description":description,"inputSchema":input,
        "annotations":{"readOnlyHint":read_only,"destructiveHint":false,"idempotentHint":true,"openWorldHint":false}
    })).collect()
}
