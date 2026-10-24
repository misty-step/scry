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
        if args.first().is_some_and(|arg| arg == "spend")
            && args.get(1).is_some_and(|arg| arg == "reconcile")
        {
            let cost = option(&mut args, "--cost-micros")?
                .ok_or("Use --cost-micros with an authoritative USD charge.")?;
            let cost: u64 = cost
                .parse()
                .map_err(|_| "Use an integer microdollar amount.")?;
            let reference = option(&mut args, "--reference")?
                .ok_or("Use --reference with a non-secret provider billing receipt reference.")?;
            let verified = flag(&mut args, "--verified-provider-billing");
            let operation = option(&mut args, "--operation-id")?
                .map(Ok)
                .unwrap_or_else(random_id)?;
            if args.len() != 3 || !verified {
                return Err(
                    "Usage: scry spend reconcile SPEND --cost-micros N --reference RECEIPT --verified-provider-billing [--operation-id ID]",
                );
            }
            return request(
                &client,
                &login,
                "/api/v1/spend/reconcile",
                Some(&json!({"operation_id":operation,
                "spend_id":args[2],"cost_micros":cost,"reference":reference,"verified":"provider_billing"})),
            );
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
            "Usage: scry login|whoami|token|search|work|version|learn|study|next|answer|reveal|correct|due|history|export [--library]|spend --by day|purpose|model [--json] [--config PATH]",
        )
    }
}
