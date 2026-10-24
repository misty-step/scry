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
    use reqwest::{Url, blocking::Client, redirect::Policy};
    use scry::{api::Identity, owner_token::token_id};
    use serde::{Deserialize, Serialize};
    use serde_json::{Value, json};
    use std::{
        fs::{self, OpenOptions},
        io::{Read, Write},
        os::unix::fs::{OpenOptionsExt, PermissionsExt},
        path::{Path, PathBuf},
        time::Duration,
    };
    type Result<T> = std::result::Result<T, &'static str>;
    #[derive(Serialize, Deserialize)]
    struct Login {
        origin: String,
        token: String,
    }
    fn option(args: &mut Vec<String>, name: &str) -> Result<Option<String>> {
        if let Some(i) = args.iter().position(|arg| arg == name) {
            if i + 1 == args.len() {
                return Err("Missing option value.");
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
    fn config_path() -> Result<PathBuf> {
        if let Some(dir) = std::env::var_os("XDG_CONFIG_HOME") {
            return Ok(PathBuf::from(dir).join("scry/login.json"));
        }
        Ok(
            PathBuf::from(std::env::var_os("HOME").ok_or("Set HOME or use --config.")?)
                .join(".config/scry/login.json"),
        )
    }
    fn origin(input: &str) -> Result<String> {
        let url = Url::parse(input).map_err(|_| "Use an HTTPS origin.")?;
        let local = url.scheme() == "http"
            && matches!(url.host_str(), Some("127.0.0.1" | "localhost" | "[::1]"));
        if !(url.scheme() == "https" || local)
            || url.path() != "/"
            || url.query().is_some()
            || url.fragment().is_some()
            || !url.username().is_empty()
            || url.password().is_some()
            || url.host_str().is_none()
        {
            return Err("Use an HTTPS origin (HTTP is allowed only on loopback).");
        }
        Ok(url.origin().ascii_serialization())
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
        result
    }
    fn request(
        client: &Client,
        login: &Login,
        path: &str,
        payload: Option<&Value>,
    ) -> Result<Value> {
        let url = format!("{}{path}", login.origin);
        let request = match payload {
            Some(payload) => client.post(url).json(payload),
            None => client.get(url),
        };
        let response = request
            .bearer_auth(&login.token)
            .send()
            .map_err(|_| "HTTP request failed; retry mutations with the same operation ID.")?;
        if !response.status().is_success() {
            // Never reflect a proxy body or diagnostic that might echo Authorization.
            return Err(match response.status().as_u16() {
                401 => "Owner authorization required.",
                403 => "Request refused.",
                409 => "Operation conflict.",
                _ => "API request failed.",
            });
        }
        let mut bytes = Vec::new();
        response
            .take(16 * 1024 * 1024 + 1)
            .read_to_end(&mut bytes)
            .map_err(|_| "Cannot read API response.")?;
        if bytes.len() > 16 * 1024 * 1024 {
            return Err("API response too large.");
        }
        serde_json::from_slice(&bytes).map_err(|_| "API returned invalid JSON.")
    }
    fn draft(args: &mut Vec<String>) -> Result<scry::library::VersionInput> {
        use scry::library::{Kind, Rights, SectionInput, VersionInput};
        let title = option(args, "--title")?.ok_or("Use --title WORK_TITLE.")?;
        let kind = match option(args, "--kind")?.as_deref() {
            Some("text") => Kind::Text,
            Some("ordered_list") => Kind::OrderedList,
            Some("keyed_mapping") => Kind::KeyedMapping,
            _ => return Err("Use --kind text|ordered_list|keyed_mapping."),
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
            _ => return Err("Use --rights private_only|public_domain|licensed."),
        };
        Ok(VersionInput {
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
        })
    }
    fn revision(args: &mut Vec<String>) -> Result<u64> {
        option(args, "--revision")?
            .ok_or("Use --revision from Work show.")?
            .parse()
            .map_err(|_| "Use the Work revision from show.")
    }
    fn resource_path(resource: &str, id: &str) -> Result<String> {
        if id.len() != 32 || !id.bytes().all(|b| b.is_ascii_hexdigit()) {
            return Err("Use a Work or Version identifier from search or show.");
        }
        Ok(format!("/api/v1/library/{resource}/{id}"))
    }
    fn material_resource_path(resource: &str, id: &str) -> Result<String> {
        // Concepts and Materials include retained opaque identities, not only
        // the Library's 32-hex IDs. Keep an ID within a single URL path segment.
        if id.is_empty()
            || !id
                .bytes()
                .all(|b| b.is_ascii_alphanumeric() || matches!(b, b'-' | b'_'))
        {
            return Err("Use a Concept, Material or revision identifier from list or show.");
        }
        Ok(format!("/api/v1/{resource}/{id}"))
    }
    fn material_read_path(args: &mut Vec<String>) -> Result<Option<String>> {
        let command = args.first().map(String::as_str);
        let action = args.get(1).map(String::as_str);
        match (command, action) {
            (Some("concept"), Some("list")) => {
                if args.len() != 2 {
                    return Err("Usage: scry concept list");
                }
                Ok(Some("/api/v1/concepts".into()))
            }
            (Some("concept"), Some("show")) => {
                if args.len() != 3 {
                    return Err("Usage: scry concept show CONCEPT");
                }
                Ok(Some(material_resource_path("concepts", &args[2])?))
            }
            (Some("material"), Some("list")) => {
                if args.len() != 3 {
                    return Err("Usage: scry material list CONCEPT");
                }
                Ok(Some(format!(
                    "{}/materials",
                    material_resource_path("concepts", &args[2])?
                )))
            }
            (Some("material"), Some("show")) => {
                let revision = option(args, "--revision-id")?;
                if args.len() != 3 {
                    return Err("Usage: scry material show MATERIAL [--revision-id REVISION]");
                }
                let path = material_resource_path("materials", &args[2])?;
                Ok(Some(match revision {
                    Some(revision) => {
                        material_resource_path("revisions", &revision)?;
                        format!("{path}/revisions/{revision}")
                    }
                    None => path,
                }))
            }
            (Some("material"), Some("history")) => {
                if args.len() != 3 {
                    return Err("Usage: scry material history MATERIAL");
                }
                Ok(Some(format!(
                    "{}/revisions",
                    material_resource_path("materials", &args[2])?
                )))
            }
            _ => Ok(None),
        }
    }
    fn material_text(reader: impl Read) -> Result<String> {
        // Bound transport memory without normalizing line endings or silently
        // truncating input. All content validation still belongs to the engine.
        let mut bytes = Vec::new();
        reader
            .take(scry::model::TEXT_LIMIT as u64 + 1)
            .read_to_end(&mut bytes)
            .map_err(|_| "Cannot read Material text.")?;
        if bytes.len() > scry::model::TEXT_LIMIT {
            return Err("Material text must be at most 32768 UTF-8 bytes.");
        }
        String::from_utf8(bytes).map_err(|_| "Material text must be UTF-8.")
    }
    fn material_mutation(
        args: &mut Vec<String>,
        operation: &str,
        stdin: impl Read,
    ) -> Result<Option<(&'static str, Value)>> {
        let command = args.first().map(String::as_str);
        let action = args.get(1).map(String::as_str);
        let (route, usage, create, revise) = match (command, action) {
            (Some("concept"), Some("create")) => (
                "/api/v1/concepts/create",
                "Usage: scry concept create --name NAME [--summary SUMMARY] --title TITLE --text-file PATH|--stdin",
                true,
                false,
            ),
            (Some("material"), Some("add")) => (
                "/api/v1/materials/add",
                "Usage: scry material add CONCEPT --title TITLE --text-file PATH|--stdin",
                false,
                false,
            ),
            (Some("material"), Some("revise")) => (
                "/api/v1/materials/revise",
                "Usage: scry material revise MATERIAL --expected-revision-id REVISION --title TITLE --text-file PATH|--stdin",
                false,
                true,
            ),
            _ => return Ok(None),
        };
        let mut payload = json!({"operation_id":operation});
        if create {
            payload["name"] = json!(option(args, "--name")?.ok_or("Use --name CONCEPT_NAME.")?);
            if let Some(summary) = option(args, "--summary")? {
                payload["summary"] = json!(summary);
            }
        }
        if revise {
            payload["expected_revision_id"] = json!(
                option(args, "--expected-revision-id")?
                    .ok_or("Use --expected-revision-id from Material show.")?
            );
        }
        payload["title"] = json!(option(args, "--title")?.ok_or("Use --title MATERIAL_TITLE.")?);
        let file = option(args, "--text-file")?;
        let use_stdin = flag(args, "--stdin");
        let positional_count = if create { 2 } else { 3 };
        if args.len() != positional_count || file.is_some() == use_stdin {
            return Err(usage);
        }
        if !create {
            payload[if revise { "material_id" } else { "concept_id" }] = json!(args[2]);
        }
        payload["text"] = json!(match file {
            Some(path) => {
                let file = fs::File::open(path).map_err(|_| "Cannot open Material text file.")?;
                material_text(file)?
            }
            None => material_text(stdin)?,
        });
        Ok(Some((route, payload)))
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
        let client = Client::builder()
            .redirect(Policy::none())
            .timeout(Duration::from_secs(30))
            .build()
            .map_err(|_| "HTTP client unavailable.")?;
        if args.first().is_some_and(|arg| arg == "login") {
            if !flag(&mut args, "--token-stdin") || args.len() != 1 {
                return Err("Usage: scry login --url ORIGIN --token-stdin [--json]");
            }
            let mut token = String::new();
            std::io::stdin()
                .take(256)
                .read_to_string(&mut token)
                .map_err(|_| "Cannot read token from stdin.")?;
            let token = token.trim().to_string();
            if token_id(&token).is_none() {
                return Err("Owner authorization required.");
            }
            let login = Login {
                origin: url.ok_or("Use --url when logging in.")?,
                token,
            };
            let value = request(&client, &login, "/api/v1/whoami", None)?;
            let identity: Identity =
                serde_json::from_value(value).map_err(|_| "Invalid owner identity.")?;
            if !identity.owner || identity.token.is_none() {
                return Err("Owner authorization required.");
            }
            store(&path, &login)?;
            return serde_json::to_value(identity).map_err(|_| "Invalid owner identity.");
        }
        let metadata = fs::symlink_metadata(&path).map_err(|_| "Log in with scry login first.")?;
        if !metadata.is_file() || metadata.permissions().mode() & 0o077 != 0 {
            return Err("Credential file must be private (chmod 600).");
        }
        let login: Login =
            serde_json::from_slice(&fs::read(path).map_err(|_| "Cannot read login.")?)
                .map_err(|_| "Invalid login file.")?;
        if origin(&login.origin)? != login.origin || token_id(&login.token).is_none() {
            return Err("Invalid login file.");
        }
        if url.is_some_and(|url| url != login.origin) {
            return Err("Log in separately for this origin.");
        }
        if args.as_slice() == ["whoami"] {
            return request(&client, &login, "/api/v1/whoami", None);
        }
        if let Some(path) = material_read_path(&mut args)? {
            return request(&client, &login, &path, None);
        }
        if args.first().is_some_and(|arg| arg == "spend") {
            let by = option(&mut args, "--by")?.ok_or("Use spend --by day|purpose|model.")?;
            if args.len() != 1 || scry::spend::SpendBy::parse(&by).is_err() {
                return Err("Use spend --by day|purpose|model.");
            }
            let value = request(&client, &login, &format!("/api/v1/spend?by={by}"), None)?;
            let report: scry::spend::SpendReport =
                serde_json::from_value(value).map_err(|_| "Invalid spend report.")?;
            return serde_json::to_value(report).map_err(|_| "Invalid spend report.");
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
        if args.as_slice() == ["study", "list"] {
            return request(&client, &login, "/api/v1/studies", None);
        }
        if args.len() == 1 && matches!(args[0].as_str(), "due" | "history" | "export") {
            return request(&client, &login, &format!("/api/v1/{}", args[0]), None);
        }
        let operation = option(&mut args, "--operation-id")?
            .map(Ok)
            .unwrap_or_else(random_id)?;
        if let Some((path, payload)) =
            material_mutation(&mut args, &operation, std::io::stdin().lock())?
        {
            return request(&client, &login, path, Some(&payload));
        }
        if args.first().is_some_and(|a| a == "learn") {
            let pace = option(&mut args, "--pace")?.unwrap_or_else(|| "steady".into());
            if args.len() != 2 {
                return Err("Usage: scry learn VERSION [--pace light|steady|intense]");
            }
            return request(
                &client,
                &login,
                "/api/v1/learn",
                Some(&json!({"operation_id":operation,"version_id":args[1],"pace":pace})),
            );
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
                    "Usage: scry next [--after PROMPT] [--study STUDY] [--card CARD | --recite whole|section --section SECTION]",
                );
            }
            return request(&client, &login, "/api/v1/next", Some(&payload));
        }
        if args.first().is_some_and(|a| a == "answer") {
            let file = option(&mut args, "--text-file")?;
            let stdin = flag(&mut args, "--text-stdin");
            if args.len() != 2 || file.is_some() == stdin {
                return Err("Usage: scry answer PROMPT --text-file PATH|--text-stdin");
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
        if args.first().is_some_and(|a| a == "reveal") && args.len() == 2 {
            return request(
                &client,
                &login,
                "/api/v1/reveal",
                Some(&json!({"operation_id":operation,"prompt_id":args[1]})),
            );
        }
        if args.first().is_some_and(|a| a == "correct") {
            let verdict = option(&mut args, "--verdict")?.ok_or("Use --verdict Good|Again.")?;
            let unit = option(&mut args, "--unit")?.unwrap_or_default();
            if args.len() != 3 {
                return Err(
                    "Usage: scry correct PROMPT ATTEMPT --verdict Good|Again [--unit UNIT]",
                );
            }
            return request(
                &client,
                &login,
                "/api/v1/correct",
                Some(
                    &json!({"operation_id":operation,"prompt_id":args[1],"attempt_id":args[2],"unit_id":unit,"verdict":verdict}),
                ),
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
                return Err("Unexpected Version arguments.");
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
                    "Usage: scry work edit ID --revision N [--title TITLE] [--alias ALIAS] [--context NOTE]",
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
                return Err("Usage: scry work|version delete ID --revision N");
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
                    "Usage: scry token create NAME [--paid] [--expires-ms TIME] [--operation-id ID]",
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
            "Usage: scry login|whoami|token|concept|material|search|work|version|learn|study|next|answer|reveal|correct|due|history|export [--library]|spend --by day|purpose|model [--json] [--config PATH]",
        )
    }

    #[cfg(test)]
    mod tests {
        use super::*;

        fn args(values: &[&str]) -> Vec<String> {
            values.iter().map(|value| (*value).into()).collect()
        }

        struct NoInput;

        impl Read for NoInput {
            fn read(&mut self, _: &mut [u8]) -> std::io::Result<usize> {
                panic!("invalid arguments must not consume stdin");
            }
        }

        #[test]
        fn material_reads_use_the_canonical_resource_routes() {
            for (arguments, expected) in [
                (vec!["concept", "list"], "/api/v1/concepts"),
                (
                    vec!["concept", "show", "concept-retained_1"],
                    "/api/v1/concepts/concept-retained_1",
                ),
                (
                    vec!["material", "list", "concept-1"],
                    "/api/v1/concepts/concept-1/materials",
                ),
                (
                    vec!["material", "show", "material-1"],
                    "/api/v1/materials/material-1",
                ),
                (
                    vec!["material", "history", "material-1"],
                    "/api/v1/materials/material-1/revisions",
                ),
                (
                    vec![
                        "material",
                        "show",
                        "material-1",
                        "--revision-id",
                        "revision-1",
                    ],
                    "/api/v1/materials/material-1/revisions/revision-1",
                ),
            ] {
                assert_eq!(
                    material_read_path(&mut args(&arguments)).unwrap(),
                    Some(expected.into())
                );
            }
            assert_eq!(material_read_path(&mut args(&["history"])).unwrap(), None);
        }

        #[test]
        fn material_read_arguments_cannot_change_the_route_or_query() {
            for id in [
                "",
                ".",
                "..",
                "../export",
                "a/b",
                "a?b",
                "a#b",
                "a%2fb",
                "é",
            ] {
                assert!(material_read_path(&mut args(&["concept", "show", id])).is_err());
                assert!(
                    material_read_path(&mut args(&[
                        "material",
                        "show",
                        "material-1",
                        "--revision-id",
                        id
                    ]))
                    .is_err()
                );
            }
            for arguments in [
                vec!["concept", "list", "extra"],
                vec!["concept", "show"],
                vec!["material", "list"],
                vec!["material", "show", "material-1", "--revision-id"],
                vec!["material", "history", "material-1", "extra"],
            ] {
                assert!(material_read_path(&mut args(&arguments)).is_err());
            }
        }

        #[test]
        fn concept_create_transports_exact_supplied_fields_and_operation() {
            let text = "  First\r\n\tCafé 💡\nhttps://example.invalid/reference\n";
            let (path, payload) = material_mutation(
                &mut args(&[
                    "concept",
                    "create",
                    "--name",
                    " Name ",
                    "--summary",
                    " Summary ",
                    "--title",
                    " Title ",
                    "--stdin",
                ]),
                "retained-operation",
                text.as_bytes(),
            )
            .unwrap()
            .unwrap();
            assert_eq!(path, "/api/v1/concepts/create");
            assert_eq!(
                payload,
                json!({"operation_id":"retained-operation","name":" Name ","summary":" Summary ","title":" Title ","text":text})
            );
        }

        #[test]
        fn concept_create_does_not_invent_optional_summary() {
            let (_, payload) = material_mutation(
                &mut args(&[
                    "concept", "create", "--name", "Name", "--title", "Title", "--stdin",
                ]),
                "operation",
                &b"Text"[..],
            )
            .unwrap()
            .unwrap();
            assert!(payload.get("summary").is_none());
        }

        #[test]
        fn identical_concept_commands_retain_optional_field_presence_for_replay() {
            let base = [
                "concept", "create", "--name", "Name", "--title", "Title", "--stdin",
            ];
            for summary in [None, Some("")] {
                let mut arguments = args(&base);
                if let Some(summary) = summary {
                    arguments.extend(args(&["--summary", summary]));
                }
                let first =
                    material_mutation(&mut arguments.clone(), "operation", &b"Text"[..]).unwrap();
                let second = material_mutation(&mut arguments, "operation", &b"Text"[..]).unwrap();
                assert_eq!(first, second);
                assert_eq!(
                    first.unwrap().1.get("summary").cloned(),
                    summary.map(|value| json!(value))
                );
            }
        }

        #[test]
        fn material_add_and_revise_transport_opaque_identities() {
            for (arguments, path, fields) in [
                (
                    vec![
                        "material",
                        "add",
                        "concept-1",
                        "--title",
                        "Title",
                        "--stdin",
                    ],
                    "/api/v1/materials/add",
                    json!({"concept_id":"concept-1"}),
                ),
                (
                    vec![
                        "material",
                        "revise",
                        "material-1",
                        "--expected-revision-id",
                        "revision-1",
                        "--title",
                        "Title",
                        "--stdin",
                    ],
                    "/api/v1/materials/revise",
                    json!({"material_id":"material-1","expected_revision_id":"revision-1"}),
                ),
            ] {
                let (actual_path, payload) =
                    material_mutation(&mut args(&arguments), "operation", &b"Text\r\n"[..])
                        .unwrap()
                        .unwrap();
                let mut expected =
                    json!({"operation_id":"operation","title":"Title","text":"Text\r\n"});
                expected
                    .as_object_mut()
                    .unwrap()
                    .extend(fields.as_object().unwrap().clone());
                assert_eq!(actual_path, path);
                assert_eq!(payload, expected);
            }
        }

        #[test]
        fn material_file_and_stdin_preserve_identical_utf8_bytes() {
            let path =
                std::env::temp_dir().join(format!("scry-cli-material-{}", random_id().unwrap()));
            let text = " \r\nCafé 💡\t\n ";
            fs::write(&path, text.as_bytes()).unwrap();
            let from_file = material_mutation(
                &mut args(&[
                    "material",
                    "add",
                    "concept-1",
                    "--title",
                    "Title",
                    "--text-file",
                    path.to_str().unwrap(),
                ]),
                "operation",
                NoInput,
            );
            fs::remove_file(path).unwrap();
            let from_stdin = material_mutation(
                &mut args(&[
                    "material",
                    "add",
                    "concept-1",
                    "--title",
                    "Title",
                    "--stdin",
                ]),
                "operation",
                text.as_bytes(),
            );
            assert_eq!(from_file, from_stdin);
            assert_eq!(from_file.unwrap().unwrap().1["text"], text);
        }

        #[test]
        fn material_transport_refuses_oversize_and_invalid_utf8_without_truncation() {
            let limit = scry::model::TEXT_LIMIT;
            let exact = "💡".repeat(limit / 4);
            assert_eq!(material_text(exact.as_bytes()).unwrap(), exact);
            let too_large = format!("{exact}x");
            assert_eq!(
                material_text(too_large.as_bytes()),
                Err("Material text must be at most 32768 UTF-8 bytes.")
            );
            assert_eq!(
                material_text(&[0xff][..]),
                Err("Material text must be UTF-8.")
            );
            // Empty or blank supplied content goes to the shared engine, which
            // owns content validity rather than this transport adapter.
            assert_eq!(material_text(&b" \r\n"[..]).unwrap(), " \r\n");
        }

        #[test]
        fn material_mutation_rejects_bad_arguments_before_reading_input() {
            for arguments in [
                vec!["concept", "create", "--title", "Title", "--stdin"],
                vec!["material", "add", "concept-1", "--stdin"],
                vec!["material", "add", "concept-1", "--title", "Title"],
                vec![
                    "material",
                    "add",
                    "concept-1",
                    "--title",
                    "Title",
                    "--stdin",
                    "--text-file",
                    "/missing",
                ],
                vec![
                    "material",
                    "add",
                    "concept-1",
                    "--title",
                    "Title",
                    "--stdin",
                    "--stdin",
                ],
                vec![
                    "material",
                    "add",
                    "concept-1",
                    "--title",
                    "Title",
                    "--stdin",
                    "--source",
                    "https://example.invalid",
                ],
                vec![
                    "material",
                    "add",
                    "concept-1",
                    "--title",
                    "Title",
                    "--stdin",
                    "--paid",
                ],
                vec![
                    "material",
                    "revise",
                    "material-1",
                    "--title",
                    "Title",
                    "--stdin",
                ],
                vec![
                    "material",
                    "revise",
                    "material-1",
                    "--expected-revision-id",
                    "revision-1",
                    "--title",
                    "Title",
                    "--stdin",
                    "extra",
                ],
            ] {
                assert!(material_mutation(&mut args(&arguments), "operation", NoInput).is_err());
            }
            assert_eq!(
                material_mutation(&mut args(&["next"]), "operation", NoInput).unwrap(),
                None
            );
        }
    }
}
