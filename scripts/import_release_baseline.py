#!/usr/bin/env python3
"""Import only public/static assets of a later release into imports/ for DIFF, never overwrite source."""
import argparse,zipfile,pathlib,re,hashlib,json,sys
root=pathlib.Path(__file__).resolve().parents[1]
p=argparse.ArgumentParser();p.add_argument('--zip',required=True);p.add_argument('--label',required=True);p.add_argument('--dry-run',action='store_true');a=p.parse_args()
if not re.fullmatch(r'[A-Za-z0-9_.-]{1,40}',a.label):sys.exit('Bad label')
with zipfile.ZipFile(a.zip) as z:
 out=root/'imports'/a.label
 if out.exists() and not a.dry_run:sys.exit('Import exists; never overwrite')
 results=[]
 for ent in z.infolist():
  if ent.is_dir():continue
  parts=pathlib.PurePosixPath(ent.filename.replace('\\','/')).parts
  if '..' in parts or ent.filename.startswith('/'):sys.exit('Unsafe ZIP path')
  ix=next((i for i,c in enumerate(parts) if c in ('static','policy_data')),None)
  if ix is None:continue
  rel=pathlib.Path(*parts[ix:])
  if len(parts[ix:])<2:continue
  # Prevent importing personal data in any static/policy dir
  if any(c.lower() in ('users.json','search_history.jsonl','logs','diagnostics','data') for c in rel.parts):continue
  if ent.file_size>100*1024*1024:sys.exit('Oversized entry')
  b=z.read(ent);h=hashlib.sha256(b).hexdigest();results.append({'path':str(rel).replace('\\','/'),'size':len(b),'sha256':h})
  if not a.dry_run:
   dst=out/rel;dst.parent.mkdir(parents=True,exist_ok=True);dst.write_bytes(b)
 if not a.dry_run:
  (out/'IMPORT_MANIFEST.json').write_text(json.dumps({'label':a.label,'source_zip':pathlib.Path(a.zip).name,'files':results},ensure_ascii=False,indent=2),encoding='utf8')
 print(f'Import {a.label}: {len(results)} static/policy assets. dry_run={a.dry_run}; NEVER promoted automatically.')
