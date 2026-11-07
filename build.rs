// Pin standard-library Unicode classification used by historical repair policy.
use std::{env, process::Command};
fn main() {
    println!("cargo:rustc-check-cfg=cfg(scry_synthetic_repair_http)");
    println!("cargo:rerun-if-env-changed=RUSTC");
    println!("cargo:rerun-if-changed=build.rs");
    let rustc = env::var_os("RUSTC").expect("Cargo must identify rustc");
    let output = Command::new(rustc)
        .arg("--version")
        .output()
        .expect("inspect selected rustc");
    assert!(
        output.status.success(),
        "Cannot identify repair-policy compiler"
    );
    let version = String::from_utf8(output.stdout).expect("rustc version UTF-8");
    assert_eq!(
        version.split_whitespace().nth(1),
        Some("1.98.1"),
        "text-boundary-repair-v1 requires Rust 1.98.1; toolchain upgrade needs an explicit reviewed classification policy, not relabeling old repairs"
    );
    println!("cargo:rustc-env=SCRY_REPAIR_POLICY_RUSTC=1.98.1");
}
