from __future__ import annotations
import argparse,json
from pathlib import Path
from backend import LegalLookupService

def main():
    ap=argparse.ArgumentParser(description='Nạp gói cập nhật dữ liệu pháp lý đã được rà soát')
    ap.add_argument('bundle');ap.add_argument('--db');a=ap.parse_args()
    data=json.loads(Path(a.bundle).read_text(encoding='utf-8'))
    svc=LegalLookupService(Path(a.db).resolve() if a.db else None)
    counts={'documents':0,'relations':0,'forms':0}
    with svc.connect() as con:
        for d in data.get('documents',[]): svc.upsert_document(d,con=con);counts['documents']+=1
        for r in data.get('relations',[]): svc.add_relation(r,con=con);counts['relations']+=1
        for f in data.get('forms',[]): svc.upsert_form(f,con=con);counts['forms']+=1
        con.commit()
    print(json.dumps({'ok':True,**counts},ensure_ascii=False))
if __name__=='__main__':main()
