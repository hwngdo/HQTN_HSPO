#!/usr/bin/env python3
"""Schema/sanity checks ONLY; never claims legal validity."""
import json,pathlib,sys,collections
root=pathlib.Path(__file__).resolve().parents[1]
p=root/'assets/policy_data';failed=[]
for f in p.glob('*.json'):
    try:json.loads(f.read_text(encoding='utf-8-sig'))
    except Exception as e:failed.append(f'{f.name}: {e}')
chem=json.loads((p/'chemical_policy.json').read_text(encoding='utf8'))
cas=[x.get('cas','') for x in chem]
if len(cas)!=len(set(cas)):failed.append('Duplicate CAS keys')
print('Chemical records:',len(chem),'unique CAS:',len(set(cas)))
print('Known checksum invalid:',[(x.get('cas')) for x in chem if x.get('cas_valid') in (0,False)])
print('LEGAL_EFFECT: UNVERIFIED. Snapshot is historic; do not infer current law.')
for f in failed:print('ERROR',f)
sys.exit(bool(failed))
