use std::path::PathBuf;

fn main() -> Result<(), Box<dyn std::error::Error>> {
    // Use vendored protoc so `cargo build` needs no system protoc.
    unsafe {
        std::env::set_var("PROTOC", protoc_bin_vendored::protoc_bin_path()?);
    }

    let manifest = PathBuf::from(env!("CARGO_MANIFEST_DIR"));
    let proto_root = manifest
        .join("../../../..")
        .join("proto");
    let protos = [
        proto_root.join("console/v1/common.proto"),
        proto_root.join("console/v1/favorites.proto"),
        proto_root.join("console/v1/ports.proto"),
        proto_root.join("console/v1/project.proto"),
        proto_root.join("console/v1/scripts.proto"),
        proto_root.join("console/v1/settings.proto"),
        proto_root.join("console/v1/usage.proto"),
    ];

    println!("cargo:rerun-if-changed=build.rs");
    for proto in &protos {
        println!("cargo:rerun-if-changed={}", proto.display());
    }
    println!("cargo:rerun-if-changed={}", proto_root.join("console/v1").display());

    let out = PathBuf::from(std::env::var("OUT_DIR")?);
    let descriptor_file = out.join("descriptors.bin");

    let mut prost_config = prost_build::Config::new();
    prost_config.file_descriptor_set_path(&descriptor_file);
    prost_config.compile_protos(&protos, &[&proto_root])?;

    let descriptors = std::fs::read(&descriptor_file)?;
    pbjson_build::Builder::new()
        .ignore_unknown_fields()
        .register_descriptors(&descriptors)?
        .build(&[".console.v1"])?;

    Ok(())
}
