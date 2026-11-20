#!/usr/bin/env python3
"""Static discovery of API paths referred to by the distributed frontend."""
import argparse
import pathlib
import re

root=pathlib.Path(__file__).resolve().parents[1]
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("html", nargs="?", type=pathlib.Path, default=pathlib.Path("assets/static/index.html"))
args = parser.parse_args()
html=(root / args.html).read_text(encoding='utf8')
paths=sorted(set(re.findall(r'/(?:api|ocr|__hspo_admin)/[A-Za-z0-9_/.?=:+-]+|/health\b',html)))
print('# Frontend paths discovered (not a validated request/response contract)')
print(f'UI file: {args.html.as_posix()}. Paths found: {len(paths)}')
for p in paths:print('- `'+p.rstrip('.,;')+'`')
