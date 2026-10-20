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
        if args.as_slice() == ["token", "list"] {
            return request(&client, &login, "/api/v1/tokens", None);
        }
        let operation = option(&mut args, "--operation-id")?
            .map(Ok)
            .unwrap_or_else(random_id)?;
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
        Err("Usage: scry login|whoami|token create|list|revoke [--json] [--config PATH]")
    }
}
