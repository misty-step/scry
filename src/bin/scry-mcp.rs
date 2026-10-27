#[cfg(target_arch = "wasm32")]
fn main() {}

#[cfg(not(target_arch = "wasm32"))]
fn main() {
    use std::io::Write;
    let mut args = std::env::args().skip(1);
    let path = match (args.next().as_deref(), args.next(), args.next()) {
        (None, None, None) => scry::owner_client::config_path(),
        (Some("--config"), Some(path), None) => Ok(std::path::PathBuf::from(path)),
        _ => Err("Usage: scry-mcp [--config PATH]"),
    };
    let result = path.and_then(scry::mcp::serve);
    if let Err(message) = result {
        // Protocol output belongs exclusively on stdout. No untrusted error details.
        let _ = writeln!(std::io::stderr().lock(), "{message}");
        std::process::exit(1);
    }
}
