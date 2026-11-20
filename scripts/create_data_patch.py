#!/usr/bin/env python3
"""Create offline candidate data ZIP. NOT a production update installer."""
import argparse,json,hashlib,pathlib,datetime,zipfile
root=pathlib.Path(__file__).resolve().parents[1]
p=argparse.ArgumentParser();p.add_argument('--output',required=True);p.add_argument('--target-version',required=True);p.add_argument('--include-legal-docs',action='store_true');a=p.parse_args()
files=list((root/'assets/policy_data').glob('*.json'))+list((root/'assets/static').glob('*.json.gz'))
if a.include_legal_docs:files.extend(p for p in (root/'assets/static/documents').rglob('*') if p.is_file())
entries=[]
with zipfile.ZipFile(a.output,'w',compression=zipfile.ZIP_DEFLATED,compresslevel=6) as z:
 for f in sorted(files):
  rel=f.relative_to(root/'assets').as_posix();b=f.read_bytes()
  z.writestr(rel,b);entries.append({'path':rel,'size':len(b),'sha256':hashlib.sha256(b).hexdigest()})
 manifest={'schema_version':1,'kind':'CANDIDATE_DATA_PATCH_UNVERIFIED','target_version':a.target_version,'source_snapshot':'v1.9.7-RC2.1','created_at':datetime.datetime.now(datetime.timezone.utc).isoformat(),'legal_effect_review':'REQUIRED_BEFORE_APPLY','files':entries}
 z.writestr('manifest.json',json.dumps(manifest,ensure_ascii=False,indent=2))
print('Created',a.output,'with',len(entries),'files. NOT APPLIED. LEGAL EFFECT NOT VERIFIED.')
