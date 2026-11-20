"""Prepare an isolated QA runtime from the v1.9.8 account and history formats."""

import argparse
import json
from pathlib import Path
import shutil
import tempfile


def read_records(path, *, lines=False):
    with path.open(encoding="utf-8-sig") as source:
        records = [json.loads(line) for line in source if line.strip()] if lines else json.load(source)
    if not isinstance(records, list) or not all(isinstance(row, dict) for row in records):
        raise ValueError(f"{path.name}: expected a list of objects")
    return records


def write_records(path, records):
    with path.open("w", encoding="utf8") as destination:
        json.dump(records, destination, ensure_ascii=False, indent=2)
        destination.write("\n")


def stage_legacy_userdata(source, destination):
    source, destination = source.resolve(), destination.resolve()
    if source == destination or source in destination.parents or destination in source.parents:
        raise ValueError("Use separate directories for source and QA runtime")
    if destination.exists():
        raise FileExistsError("QA runtime directory already exists")

    users = read_records(source / "users.json")
    names = set()
    for user in users:
        name, password = user.get("username"), user.get("password")
        if not isinstance(name, str) or not name.strip() or not isinstance(password, str) or not password:
            raise ValueError("users.json contains an invalid account shape")
        if name.casefold() in names:
            raise ValueError("users.json contains duplicate usernames")
        names.add(name.casefold())
        if "display_name" not in user and isinstance(user.get("displayName"), str):
            user["display_name"] = user["displayName"]
    if not users:
        raise ValueError("users.json contains no accounts")

    history_path = source / "search_history.jsonl"
    history = read_records(history_path, lines=True) if history_path.is_file() else []
    with tempfile.TemporaryDirectory(prefix="hspo-legacy-", dir=destination.parent) as temporary:
        runtime = Path(temporary) / "runtime"
        data = runtime / "data"
        data.mkdir(parents=True)
        write_records(data / "users.json", users)
        write_records(data / "search_history.json", history)
        verification = source / "verification.json"
        if verification.is_file():
            write_records(data / verification.name, read_records(verification))
        for name in ("admin_audit.jsonl", "Lich_su_tra_cuu_CAS.xlsx"):
            old_file = source / name
            if old_file.is_file():
                archive = runtime / "legacy_archive"
                archive.mkdir(exist_ok=True)
                shutil.copyfile(old_file, archive / name)
        runtime.rename(destination)
    return len(users), len(history)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("source", type=Path)
    parser.add_argument("destination", type=Path)
    args = parser.parse_args()
    accounts, searches = stage_legacy_userdata(args.source, args.destination)
    print(f"QA runtime staged: {accounts} accounts, {searches} history entries.")
