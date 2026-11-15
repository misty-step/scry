#[cfg(target_arch = "wasm32")]
fn main() {}

#[cfg(not(target_arch = "wasm32"))]
fn main() {
    use std::io::Write;
    let json = std::env::args().any(|arg| arg == "--json");
    match cli::run() {
        Ok(value) => {
            let output = if json {
                serde_json::to_string(&value)
            } else {
                serde_json::to_string_pretty(&value)
            };
            if let Ok(output) = output
                && writeln!(std::io::stdout().lock(), "{output}").is_ok()
            {
                if !json && value.get("existing").and_then(|v| v.as_bool()) == Some(true) {
                    let _ = writeln!(
                        std::io::stderr().lock(),
                        "That text was already in the Library. Nothing was added."
                    );
                }
                return;
            }
            std::process::exit(1);
        }
        Err(message) => {
            if json {
                let _ = writeln!(
                    std::io::stdout().lock(),
                    "{}",
                    serde_json::json!({"error":message})
                );
            } else {
                let _ = writeln!(std::io::stderr().lock(), "{message}");
            }
            std::process::exit(1);
        }
    }
}

#[cfg(not(target_arch = "wasm32"))]
mod cli {
    use reqwest::Url;
    use scry::{
        api::Identity,
        owner_client::{self, Login, config_path, origin, request},
        owner_token::token_id,
    };
    use serde_json::{Value, json};
    use std::{
        borrow::Cow,
        fs::{self, OpenOptions},
        io::{Read, Write},
        os::unix::fs::{OpenOptionsExt, PermissionsExt},
        path::{Path, PathBuf},
    };
    type Result<T> = std::result::Result<T, Cow<'static, str>>;
    fn option(args: &mut Vec<String>, name: &str) -> Result<Option<String>> {
        if let Some(i) = args.iter().position(|arg| arg == name) {
            if i + 1 == args.len() {
                return Err("Missing option value.".into());
            }
            args.remove(i);
            Ok(Some(args.remove(i)))
        } else {
            Ok(None)
        }
    }
    fn flag(args: &mut Vec<String>, name: &str) -> bool {
        if let Some(i) = args.iter().position(|arg| arg == name) {
            args.remove(i);
            true
        } else {
            false
        }
    }
    fn random_id() -> Result<String> {
        let mut bytes = [0; 24];
        fs::File::open("/dev/urandom")
            .and_then(|mut file| file.read_exact(&mut bytes))
            .map_err(|_| "Randomness unavailable.")?;
        Ok(hex::encode(bytes))
    }
    fn store(path: &Path, login: &Login) -> Result<()> {
        let parent = path
            .parent()
            .filter(|p| !p.as_os_str().is_empty())
            .unwrap_or(Path::new("."));
        if !parent.exists() {
            fs::create_dir_all(parent).map_err(|_| "Cannot create credential directory.")?;
            fs::set_permissions(parent, fs::Permissions::from_mode(0o700))
                .map_err(|_| "Cannot protect credential directory.")?;
        }
        let temporary = parent.join(format!(".scry-login-{}", random_id()?));
        let result = (|| {
            let mut file = OpenOptions::new()
                .write(true)
                .create_new(true)
                .mode(0o600)
                .open(&temporary)
                .map_err(|_| "Cannot save login.")?;
            let bytes = serde_json::to_vec(login).map_err(|_| "Cannot encode login.")?;
            file.write_all(&bytes)
                .and_then(|_| file.sync_all())
                .map_err(|_| "Cannot save login.")?;
            fs::rename(&temporary, path).map_err(|_| "Cannot save login.")
        })();
        if result.is_err() {
            let _ = fs::remove_file(&temporary);
        }
        Ok(result?)
    }
    fn draft(args: &mut Vec<String>) -> Result<scry::library::VersionInput> {
        use scry::library::{
            Kind, Provenance, RelationInput, RelationKind, Rights, SectionInput, VersionInput,
        };
        let title = option(args, "--title")?.ok_or("Use --title WORK_TITLE.")?;
        let kind = match option(args, "--kind")?.as_deref() {
            Some("text") => Kind::Text,
            Some("ordered_list") => Kind::OrderedList,
            Some("keyed_mapping") => Kind::KeyedMapping,
            _ => return Err("Use --kind text|ordered_list|keyed_mapping.".into()),
        };
        let text_path = option(args, "--text-file")?.ok_or("Use --text-file PATH.")?;
        let text = fs::read_to_string(text_path).map_err(|_| "Cannot read UTF-8 text file.")?;
        let mut aliases = Vec::new();
        while let Some(alias) = option(args, "--alias")? {
            aliases.push(alias);
        }
        let mut sources = Vec::new();
        while let Some(source) = option(args, "--source")? {
            sources.push(source);
        }
        let sections: Option<Vec<SectionInput>> = option(args, "--sections-file")?
            .map(|path| {
                let bytes = fs::read(path).map_err(|_| "Cannot read Sections file.")?;
                serde_json::from_slice(&bytes)
                    .map_err(|_| "Use a JSON array of Sections with heading and units.")
            })
            .transpose()?;
        let rights = match option(args, "--rights")?.as_deref() {
            None | Some("private_only") => Rights::PrivateOnly,
            Some("public_domain") => Rights::PublicDomain,
            Some("licensed") => Rights::Licensed,
            _ => return Err("Use --rights private_only|public_domain|licensed.".into()),
        };
        let provenance = match option(args, "--provenance")?.as_deref() {
            None | Some("supplied") => Provenance::Supplied,
            Some("model_from_memory") => Provenance::ModelFromMemory,
            Some("checked_against_sources") => Provenance::CheckedAgainstSources,
            _ => {
                return Err(
                    "Use --provenance supplied|model_from_memory|checked_against_sources.".into(),
                );
            }
        };
        let direction = match option(args, "--direction")?.as_deref() {
            None | Some("forward") => scry::library::MappingDirection::Forward,
            Some("both") => scry::library::MappingDirection::Both,
            _ => return Err("Use --direction forward|both.".into()),
        };
        let mut safety_bearing = std::collections::BTreeSet::new();
        while let Some(position) = option(args, "--safety-bearing")? {
            safety_bearing.insert(
                position
                    .parse::<usize>()
                    .map_err(|_| "Use --safety-bearing with a zero-based pair position.")?,
            );
        }
        let mut relations = Vec::new();
        for (name, kind) in [
            ("--part-of", RelationKind::PartOf),
            ("--related-to", RelationKind::RelatedTo),
        ] {
            while let Some(work) = option(args, name)? {
                relations.push(RelationInput { kind, work });
            }
        }
        Ok(VersionInput {
            provenance,
            relations,
            direction,
            safety_bearing,
            title,
            kind,
            text,
            aliases,
            sources,
            rights,
            sections,
            name: option(args, "--name")?.unwrap_or_default(),
            context: option(args, "--context")?.unwrap_or_default(),
            spelling_strict: flag(args, "--spelling-strict"),
            ordered_aliases: option(args, "--ordered-aliases-file")?
                .map(|path| {
                    let bytes = fs::read(path).map_err(|_| "Cannot read ordered aliases file.")?;
                    serde_json::from_slice(&bytes).map_err(
                        |_| "Use a JSON object mapping zero-based item positions to alias arrays.",
                    )
                })
                .transpose()?
                .unwrap_or_default(),
        })
    }
    fn revision(args: &mut Vec<String>) -> Result<u64> {
        option(args, "--revision")?
            .ok_or("Use --revision from Work show.")?
            .parse()
            .map_err(|_| "Use the Work revision from show.".into())
    }
    fn resource_path(resource: &str, id: &str) -> Result<String> {
        if id.len() != 32 || !id.bytes().all(|b| b.is_ascii_hexdigit()) {
            return Err("Use a Work or Version identifier from search or show.".into());
        }
        Ok(format!("/api/v1/library/{resource}/{id}"))
    }
    pub fn run() -> Result<Value> {
        let mut args: Vec<String> = std::env::args().skip(1).collect();
        flag(&mut args, "--json");
        let path = option(&mut args, "--config")?
            .map(PathBuf::from)
            .map(Ok)
            .unwrap_or_else(config_path)?;
        let url = option(&mut args, "--url")?
            .map(|url| origin(&url))
            .transpose()?;
        let client = owner_client::client()?;
        if args.first().is_some_and(|arg| arg == "login") {
            if !flag(&mut args, "--token-stdin") || args.len() != 1 {
                return Err("Usage: scry login --url ORIGIN --token-stdin [--json]".into());
            }
            let mut token = String::new();
            std::io::stdin()
                .take(256)
                .read_to_string(&mut token)
                .map_err(|_| "Cannot read token from stdin.")?;
            let token = token.trim().to_string();
            if token_id(&token).is_none() {
                return Err("Owner authorization required.".into());
            }
            let login = Login {
                origin: url.ok_or("Use --url when logging in.")?,
                token,
            };
            let value = request(&client, &login, "/api/v1/whoami", None)?;
            let identity: Identity =
                serde_json::from_value(value).map_err(|_| "Invalid owner identity.")?;
            if !identity.owner || identity.token.is_none() {
                return Err("Owner authorization required.".into());
            }
            store(&path, &login)?;
            return serde_json::to_value(identity).map_err(|_| "Invalid owner identity.".into());
        }
        let login = owner_client::load(&path)?;
        if url.is_some_and(|url| url != login.origin) {
            return Err("Log in separately for this origin.".into());
        }
        if args.as_slice() == ["whoami"] {
            return request(&client, &login, "/api/v1/whoami", None);
        }
        if args.first().is_some_and(|arg| arg == "spend") {
            let by = option(&mut args, "--by")?.ok_or("Use spend --by day|purpose|model.")?;
            if args.len() != 1 || scry::spend::SpendBy::parse(&by).is_err() {
                return Err("Use spend --by day|purpose|model.".into());
            }
            let value = request(&client, &login, &format!("/api/v1/spend?by={by}"), None)?;
            let report: scry::spend::SpendReport =
                serde_json::from_value(value).map_err(|_| "Invalid spend report.")?;
            return serde_json::to_value(report).map_err(|_| "Invalid spend report.".into());
        }
        if args.as_slice() == ["token", "list"] {
            return request(&client, &login, "/api/v1/tokens", None);
        }
        if args.len() == 2 && args[0] == "search" {
            let mut url = Url::parse(&format!("{}/api/v1/library/search", login.origin))
                .map_err(|_| "Invalid origin.")?;
            url.query_pairs_mut().append_pair("q", &args[1]);
            return request(
                &client,
                &login,
                &format!("{}?{}", url.path(), url.query().unwrap_or_default()),
                None,
            );
        }
        if args.as_slice() == ["export", "--library"] {
            return request(&client, &login, "/api/v1/library/export", None);
        }
        if args.len() == 3 && matches!(args[0].as_str(), "work" | "version") && args[1] == "show" {
            let resource = if args[0] == "work" {
                "works"
            } else {
                "versions"
            };
            return request(&client, &login, &resource_path(resource, &args[2])?, None);
        }
        if args.as_slice() == ["draft"] {
            return request(&client, &login, "/api/v1/draft", None);
        }
        if args.as_slice() == ["study", "list"] {
            return request(&client, &login, "/api/v1/studies", None);
        }
        if args.len() == 1 && matches!(args[0].as_str(), "due" | "history" | "export") {
            return request(&client, &login, &format!("/api/v1/{}", args[0]), None);
        }
        let supplied_operation = option(&mut args, "--operation-id")?;
        if args
            .first()
            .is_some_and(|a| matches!(a.as_str(), "save-draft" | "discard-draft"))
            && supplied_operation.is_none()
        {
            return Err(
                "Use --operation-id ID and retain it to reconcile an uncertain checkpoint operation.".into(),
            );
        }
        let operation = supplied_operation.map(Ok).unwrap_or_else(random_id)?;
        if args.first().is_some_and(|a| a == "lesson") {
            let mut payload = json!({"operation_id":operation});
            for (option_name, field) in [
                ("--starter", "starter"),
                ("--version", "version_id"),
                ("--after", "after"),
                ("--leave", "leave"),
                ("--mode", "mode"),
            ] {
                if let Some(value) = option(&mut args, option_name)? {
                    payload[field] = json!(value);
                }
            }
            if args.len() != 2 || !matches!(args[1].as_str(), "start" | "next") {
                return Err(
                    "Usage: scry lesson start --starter NAME|--version VERSION [--after PROMPT|--leave PROMPT] | lesson next --after PROMPT [--mode phrases|retry|join|whole|middle]".into(),
                );
            }
            let action = args[1].as_str();
            if action == "start"
                && (payload.get("starter").is_some() == payload.get("version_id").is_some()
                    || payload.get("mode").is_some()
                    || payload.get("after").is_some() && payload.get("leave").is_some())
                || action == "next"
                    && (payload.get("after").is_none()
                        || payload.get("starter").is_some()
                        || payload.get("version_id").is_some()
                        || payload.get("leave").is_some())
            {
                return Err(
                    "Choose one starter or Version for start; next requires the answered Prompt and an optional mode.".into(),
                );
            }
            return request(
                &client,
                &login,
                &format!("/api/v1/lessons/{action}"),
                Some(&payload),
            );
        }
        if args.first().is_some_and(|a| a == "learn") {
            let pace = option(&mut args, "--pace")?.unwrap_or_else(|| "steady".into());
            if args.len() != 2 {
                return Err("Usage: scry learn VERSION [--pace light|steady|intense]".into());
            }
            return request(
                &client,
                &login,
                "/api/v1/learn",
                Some(&json!({"operation_id":operation,"version_id":args[1],"pace":pace})),
            );
        }
        if args.starts_with(&["study".into(), "pace".into()]) {
            let pace = option(&mut args, "--pace")?;
            if args.len() != 3 || pace.is_none() {
                return Err("Usage: scry study pace STUDY --pace light|steady|intense".into());
            }
            return request(
                &client,
                &login,
                "/api/v1/studies/pace",
                Some(&json!({"operation_id":operation,"study_id":args[2],"pace":pace})),
            );
        }
        // Study's mix: what the app's Study and Next serve.
        if args.first().is_some_and(|a| a == "mix") {
            let mut payload = json!({"operation_id":operation});
            for (option_name, field) in [("--after", "after"), ("--leave", "leave")] {
                if let Some(value) = option(&mut args, option_name)? {
                    payload[field] = json!(value);
                }
            }
            if args.len() != 1 || payload.get("after").is_some() && payload.get("leave").is_some() {
                return Err("Usage: scry mix [--after PROMPT | --leave PROMPT]".into());
            }
            return request(&client, &login, "/api/v1/mix", Some(&payload));
        }
        if args.first().is_some_and(|a| a == "study")
            && args.len() == 3
            && matches!(args[1].as_str(), "pause" | "resume" | "drop")
        {
            return request(
                &client,
                &login,
                &format!("/api/v1/studies/{}", args[1]),
                Some(&json!({"operation_id":operation,"study_id":args[2]})),
            );
        }
        if args.first().is_some_and(|a| a == "next") {
            let mut payload = json!({"operation_id":operation});
            for (option_name, field) in [
                ("--after", "after"),
                ("--study", "study_id"),
                ("--card", "card_id"),
                ("--recite", "recite"),
                ("--section", "section_id"),
            ] {
                if let Some(value) = option(&mut args, option_name)? {
                    payload[field] = json!(value);
                }
            }
            if args.len() != 1 {
                return Err(
                    "Usage: scry next [--after PROMPT] [--study STUDY] [--card CARD | --recite whole|section --section SECTION]".into(),
                );
            }
            return request(&client, &login, "/api/v1/next", Some(&payload));
        }
        if args.first().is_some_and(|a| a == "discard-draft") {
            let generation = option(&mut args, "--generation")?
                .ok_or("Use --generation NUMBER.")?
                .parse::<u64>()
                .map_err(|_| "Use a numeric draft generation.")?;
            if args.len() != 2 {
                return Err(
                    "Usage: scry discard-draft PROMPT --generation N --operation-id ID".into(),
                );
            }
            return request(
                &client,
                &login,
                "/api/v1/draft/discard",
                Some(
                    &json!({"operation_id":operation,"prompt_id":args[1],"generation":generation}),
                ),
            );
        }
        if args.first().is_some_and(|a| a == "save-draft") {
            let generation = option(&mut args, "--generation")?
                .ok_or("Use --generation NUMBER.")?
                .parse::<u64>()
                .map_err(|_| "Use a numeric draft generation.")?;
            let file = option(&mut args, "--text-file")?;
            let stdin = flag(&mut args, "--text-stdin");
            if args.len() != 2 || file.is_some() == stdin {
                return Err(
                    "Usage: scry save-draft PROMPT --generation N --text-file PATH|--text-stdin --operation-id ID".into(),
                );
            }
            let answer = if let Some(path) = file {
                fs::read_to_string(path).map_err(|_| "Cannot read UTF-8 answer file.")?
            } else {
                let mut text = String::new();
                std::io::stdin()
                    .take(32769)
                    .read_to_string(&mut text)
                    .map_err(|_| "Cannot read answer.")?;
                text
            };
            if answer.len() > 32768 {
                return Err("Keep your answer under 32 KiB.".into());
            }
            return request(
                &client,
                &login,
                "/api/v1/draft",
                Some(
                    &json!({"operation_id":operation,"prompt_id":args[1],"generation":generation,"answer":answer}),
                ),
            );
        }
        if args.first().is_some_and(|a| a == "answer") {
            let file = option(&mut args, "--text-file")?;
            let stdin = flag(&mut args, "--text-stdin");
            if args.len() != 2 || file.is_some() == stdin {
                return Err("Usage: scry answer PROMPT --text-file PATH|--text-stdin".into());
            }
            let answer = if let Some(path) = file {
                fs::read_to_string(path).map_err(|_| "Cannot read UTF-8 answer file.")?
            } else {
                let mut text = String::new();
                std::io::stdin()
                    .take(scry::library::TEXT_BYTES as u64 + 1)
                    .read_to_string(&mut text)
                    .map_err(|_| "Cannot read answer.")?;
                text
            };
            return request(
                &client,
                &login,
                "/api/v1/answer",
                Some(&json!({"operation_id":operation,"prompt_id":args[1],"answer":answer})),
            );
        }
        // A new, deliberate paid check of a saved answer that has no Verdict.
        if args.first().is_some_and(|a| a == "check") {
            if args.len() != 2 {
                return Err("Usage: scry check ATTEMPT [--operation-id ID]".into());
            }
            return request(
                &client,
                &login,
                "/api/v1/check",
                Some(&json!({"operation_id":operation,"attempt_id":args[1]})),
            );
        }
        if args
            .first()
            .is_some_and(|a| matches!(a.as_str(), "reveal" | "quiz"))
            && args.len() == 2
        {
            return request(
                &client,
                &login,
                if args[0] == "quiz" {
                    "/api/v1/quiz"
                } else {
                    "/api/v1/reveal"
                },
                Some(&json!({"operation_id":operation,"prompt_id":args[1]})),
            );
        }
        if args.starts_with(&["mapping".into(), "add".into()]) {
            let file =
                option(&mut args, "--file")?.ok_or("Use mapping add --file MANIFEST.json.")?;
            if args.len() != 2 {
                return Err(
                    "Usage: scry mapping add --file MANIFEST.json [--operation-id ID]".into(),
                );
            }
            let mut bytes = Vec::new();
            fs::File::open(file)
                .map_err(|_| "Cannot read mapping manifest.")?
                .take(1024 * 1024 + 1)
                .read_to_end(&mut bytes)
                .map_err(|_| "Cannot read mapping manifest.")?;
            if bytes.len() > 1024 * 1024 {
                return Err("Mapping manifest exceeds 1 MiB.".into());
            }
            let data: scry::library::MappingInput = serde_json::from_slice(&bytes).map_err(
                |_| "Use a supplied mapping manifest with direction forward or both and key/value pairs.",
            )?;
            data.clone()
                .into_version()
                .map_err(|_| "Invalid mapping pairs or metadata.")?;
            return request(
                &client,
                &login,
                "/api/v1/library/mappings",
                Some(&json!({"operation_id":operation,"data":data})),
            );
        }
        if args.starts_with(&["version".into(), "add".into()])
            || args.starts_with(&["version".into(), "supersede".into()])
        {
            let supersede = args[1] == "supersede";
            let rev = if supersede {
                Some(revision(&mut args)?)
            } else {
                None
            };
            let data = draft(&mut args)?;
            if args.len() != if supersede { 3 } else { 2 } {
                return Err(
                    "Usage: scry version add|supersede [VERSION --revision N] --title TITLE --kind text|ordered_list|keyed_mapping --text-file PATH [--name EDITION] [--alias NAME] [--source CITATION] [--rights private_only|public_domain|licensed] [--provenance supplied|model_from_memory|checked_against_sources] [--part-of WORK] [--related-to WORK] [--sections-file PATH] [--ordered-aliases-file PATH] [--spelling-strict] [--direction forward|both] [--safety-bearing PAIR_POSITION]".into(),
                );
            }
            let mut payload = json!({"operation_id":operation,"data":data});
            if let Some(rev) = rev {
                resource_path("versions", &args[2])?;
                payload["version_id"] = json!(args[2]);
                payload["revision"] = json!(rev);
            }
            let route = if supersede {
                "/api/v1/library/versions/supersede"
            } else {
                "/api/v1/library/versions"
            };
            return request(&client, &login, route, Some(&payload));
        }
        if args.starts_with(&["work".into(), "edit".into()]) {
            let rev = revision(&mut args)?;
            let title = option(&mut args, "--title")?;
            let context = option(&mut args, "--context")?;
            let clear_aliases = flag(&mut args, "--clear-aliases");
            let mut aliases = Vec::new();
            while let Some(alias) = option(&mut args, "--alias")? {
                aliases.push(alias);
            }
            let aliases = if clear_aliases || !aliases.is_empty() {
                Some(aliases)
            } else {
                None
            };
            if args.len() != 3 {
                return Err(
                    "Usage: scry work edit ID --revision N [--title TITLE] [--alias ALIAS] [--context NOTE]".into(),
                );
            }
            resource_path("works", &args[2])?;
            return request(
                &client,
                &login,
                "/api/v1/library/works/edit",
                Some(&json!({"operation_id":operation,
                "data":{"work_id":args[2],"revision":rev,"title":title,"aliases":aliases,"context":context}})),
            );
        }
        if args.len() >= 2 && matches!(args[0].as_str(), "work" | "version") && args[1] == "delete"
        {
            let rev = revision(&mut args)?;
            if args.len() != 3 {
                return Err("Usage: scry work|version delete ID --revision N".into());
            }
            let work = args[0] == "work";
            let resource = if work { "works" } else { "versions" };
            resource_path(resource, &args[2])?;
            let mut payload = json!({"operation_id":operation,"revision":rev});
            payload[if work { "work_id" } else { "version_id" }] = json!(args[2]);
            return request(
                &client,
                &login,
                &format!("/api/v1/library/{resource}/delete"),
                Some(&payload),
            );
        }
        if args.starts_with(&["token".into(), "create".into()]) {
            let paid = flag(&mut args, "--paid");
            let expiry = option(&mut args, "--expires-ms")?;
            if args.len() != 3 {
                return Err(
                    "Usage: scry token create NAME [--paid] [--expires-ms TIME] [--operation-id ID]".into(),
                );
            }
            let mut payload = json!({"operation_id":operation,"name":args[2],"paid":paid});
            if let Some(expiry) = expiry {
                payload["expires_ms"] = json!(
                    expiry
                        .parse::<i64>()
                        .map_err(|_| "Use Unix milliseconds for expiry.")?
                );
            }
            return request(&client, &login, "/api/v1/tokens", Some(&payload));
        }
        if args.len() == 3 && args[0] == "token" && args[1] == "revoke" {
            return request(
                &client,
                &login,
                "/api/v1/tokens/revoke",
                Some(&json!({"operation_id":operation,"token_id":args[2]})),
            );
        }
        Err(
            "Usage: scry login|whoami|token|search|work|version|mapping|lesson|learn|study [list|pause|resume|drop|pace]|mix|next|answer|check|draft|save-draft|discard-draft|reveal|quiz|due|history|export [--library]|spend --by day|purpose|model [--json] [--config PATH]".into(),
        )
    }
}
