fn main() {
    // The version of a release build; a local build has none and does not update itself.
    println!("cargo:rerun-if-env-changed=POROS_VERSION");
    tauri_build::build()
}
