"""Preview, stage and restore same-version HQTN_HSPO userdata offline."""

import argparse
import hashlib
import json
import shutil
import stat
import tempfile
from pathlib import Path


FILE_NAMES = (
    "users.json",
    "search_history.jsonl",
    "cas_history.json",
    "Lich_su_tra_cuu_CAS.xlsx",
    "admin_audit.jsonl",
    "verification.json",
)
MANIFEST_NAME = "migration_manifest.json"


def plain_path(path, kind):
    info = path.lstat()
    if getattr(info, "st_file_attributes", 0) & getattr(stat, "FILE_ATTRIBUTE_REPARSE_POINT", 0):
        raise ValueError(f"{path.name}: reparse point is not supported")
    if not kind(info.st_mode):
        raise ValueError(f"{path.name}: unexpected file type")
    return path


def fingerprint(path):
    with plain_path(path, stat.S_ISREG).open("rb") as handle:
        digest = hashlib.file_digest(handle, "sha256").hexdigest()
        return {"size": handle.tell(), "sha256": digest}


def runtime_identity(root):
    version = plain_path(root / "VERSION.txt", stat.S_ISREG).read_text(encoding="utf-8-sig").strip()
    if version != "1.9.8":
        raise ValueError("This migration supports only the original v1.9.8 userdata format.")
    binaries = {
        name: fingerprint(root / name)
        for name in ("HQTN_HSPO_Core.exe", "HQTN_HSPO_TEST_Runtime_Server.exe")
    }
    # Use the actual shipped data files, even if the version file was copied
    # from a different server or the version label was left unchanged.
    policy = plain_path(root / "policy_data", stat.S_ISDIR)
    static = plain_path(root / "static", stat.S_ISDIR)
    data = {
        f"policy_data/{file.name}": fingerprint(file)
        for file in sorted(policy.glob("*.json"))
    }
    data.update({
        f"static/{file.name}": fingerprint(file)
        for file in sorted([*static.glob("*.json.gz"), *static.glob("*.json")])
    })
    if not data:
        raise ValueError("Missing policy and tariff data in the runtime.")
    return {"app_version": version, "runtime_binaries": binaries, "datasets": data}


def manifest_for(runtime):
    userdata = plain_path(runtime / "userdata", stat.S_ISDIR)
    files = {
        name: fingerprint(userdata / name)
        for name in FILE_NAMES if (userdata / name).exists()
    }
    if "users.json" not in files:
        raise ValueError("Source userdata is missing users.json.")
    return {"schema": 1, **runtime_identity(runtime), "files": files}


def copy_verified(source, destination, files):
    destination.mkdir(mode=0o700)
    for name, info in files.items():
        if name not in FILE_NAMES:
            raise ValueError("Migration manifest contains an unsupported file.")
        file = plain_path(source / name, stat.S_ISREG)
        shutil.copyfile(file, destination / name)
        if fingerprint(destination / name) != info:
            raise ValueError(f"{name}: SHA-256 changed during transfer.")


def require_separate(first, second):
    if first == second or first in second.parents or second in first.parents:
        raise ValueError("Source and destination must use independent directories.")


def export(server, bundle, apply):
    server, bundle = server.resolve(strict=True), bundle.resolve()
    require_separate(server, bundle)
    if bundle.exists():
        raise FileExistsError("Migration destination already exists.")
    metadata = manifest_for(server)
    if apply:
        with tempfile.TemporaryDirectory(prefix="hspo-migrate-", dir=bundle.parent) as name:
            staged = Path(name) / "bundle"
            staged.mkdir(mode=0o700)
            copy_verified(server / "userdata", staged / "userdata", metadata["files"])
            if manifest_for(server) != metadata:
                raise ValueError("Source changed during transfer; stop it and retry.")
            (staged / MANIFEST_NAME).write_text(json.dumps(metadata, indent=2) + "\n", encoding="utf-8")
            if bundle.exists():
                raise FileExistsError("Migration destination already exists.")
            staged.rename(bundle)
    return len(metadata["files"])


def restore(bundle, server, apply):
    bundle, server = bundle.resolve(strict=True), server.resolve(strict=True)
    require_separate(bundle, server)
    target = server / "userdata"
    if target.exists():
        raise FileExistsError("Destination already contains userdata.")
    metadata = json.loads(plain_path(bundle / MANIFEST_NAME, stat.S_ISREG).read_bytes())
    if not isinstance(metadata, dict) or metadata.get("schema") != 1 or not isinstance(metadata.get("files"), dict):
        raise ValueError("Migration format is not supported.")
    identity = runtime_identity(server)
    if any(metadata.get(key) != value for key, value in identity.items()):
        raise ValueError("Destination app version, binaries, or datasets differ from the source.")
    lists = metadata["files"]
    if "users.json" not in lists or not set(lists) <= set(FILE_NAMES):
        raise ValueError("Migration manifest has missing or unsupported files.")
    source = plain_path(bundle / "userdata", stat.S_ISDIR)
    for name, expected in lists.items():
        if fingerprint(source / name) != expected:
            raise ValueError(f"{name}: invalid size or SHA-256.")
    if apply:
        with tempfile.TemporaryDirectory(prefix="hspo-restore-", dir=server) as name:
            staged = Path(name) / "userdata"
            copy_verified(source, staged, lists)
            if target.exists():
                raise FileExistsError("Destination already contains userdata.")
            staged.rename(target)
    return len(lists)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("export", "restore"))
    parser.add_argument("source", type=Path)
    parser.add_argument("destination", type=Path)
    parser.add_argument("--apply", action="store_true", help="Write the verified copy. Stop both servers first.")
    args = parser.parse_args()
    operation = export if args.action == "export" else restore
    try:
        count = operation(args.source, args.destination, args.apply)
    except (OSError, ValueError, TypeError) as error:
        parser.exit(1, f"{error}\n")
    status = "completed" if args.apply else "dry run passed"
    print(f"Migration {args.action} {status}: {count} userdata files.")


if __name__ == "__main__":
    main()
