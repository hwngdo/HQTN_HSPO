#!/usr/bin/env python3
"""Verify source snapshot and content hashes. Never launch services or alter data."""
import json, hashlib, pathlib, sys
root=pathlib.Path(__file__).resolve().parents[1]
manifest=json.loads((root/'SOURCE_SHA256.json').read_text(encoding='utf-8'))
fail=[]
for item in manifest['files']:
    p=root/item['path']
    if not p.is_file():fail.append('MISSING '+item['path']);continue
    h=hashlib.sha256(p.read_bytes()).hexdigest()
    if h!=item['sha256']:fail.append('SHA MISMATCH '+item['path'])
for unsafe in ['users.json','search_history.jsonl','.env','private.key']:
    if list(root.rglob(unsafe)):fail.append('FORBIDDEN '+unsafe)
print(f"Checked {len(manifest['files'])} files. Failures: {len(fail)}")
for f in fail:print(f)
sys.exit(1 if fail else 0)
