from __future__ import annotations
import argparse,json,re,zipfile
from pathlib import Path
from xml.etree import ElementTree as ET
from backend import LegalLookupService
TYPE_MAP={'ND-CP':'Nghị định','TT-BTC':'Thông tư','TT-BNNMT':'Thông tư','TT-BKHCN':'Thông tư','QD-TTg':'Quyết định','QH15':'Luật/Nghị quyết QH','QH16':'Luật/Nghị quyết QH'}
ISSUER_MAP={'BTC':'Bộ Tài chính','BNNMT':'Bộ Nông nghiệp và Môi trường','BKHCN':'Bộ Khoa học và Công nghệ','CP':'Chính phủ','TTg':'Thủ tướng Chính phủ','QH15':'Quốc hội','QH16':'Quốc hội'}
def docx_text(path):
    try:
        with zipfile.ZipFile(path) as z: xml=z.read('word/document.xml')
        root=ET.fromstring(xml); out=[]
        for e in root.iter():
            if e.tag.endswith('}t') and e.text: out.append(e.text)
            elif e.tag.endswith('}p'): out.append('\n')
        return ' '.join(out).replace(' \n ','\n')
    except Exception:return ''
def pdf_text(path):
    try:
        from pypdf import PdfReader
        return '\\n'.join((p.extract_text() or '') for p in PdfReader(str(path)).pages)
    except Exception:return ''
def first_title(text):
    lines=[re.sub(r'\s+',' ',x).strip() for x in text.splitlines() if x.strip()]
    for x in lines[:80]:
        if any(k in x.lower() for k in ('nghị định','thông tư','quyết định','luật','nghị quyết')) and len(x)>12:return x[:500]
    return lines[0][:500] if lines else ''
def parse_filename(path,root):
    stem=re.sub(r'\(\d+\)$','',path.stem); m=re.match(r'^(\d+)_([12]\d{3})_([^_]+)',stem); rel=path.relative_to(root).as_posix(); category=path.relative_to(root).parts[0] if len(path.relative_to(root).parts)>1 else 'KHAC'
    if not m:return {'id':re.sub(r'[^a-z0-9]+','-',stem.lower()),'number':stem,'year':None,'doc_type':'','issuer':'','category':category,'filename':path.name,'relative_path':rel}
    number,year,kind=m.groups(); code=kind.split('-',1)[1] if '-' in kind else kind; return {'id':f'{number}-{year}-{kind}'.lower(),'number':number,'year':int(year),'doc_type':TYPE_MAP.get(kind,kind),'issuer':ISSUER_MAP.get(code,code),'category':category,'filename':path.name,'relative_path':rel}
def form_hint(text): return bool(re.search(r'\b(mẫu\s+số|biểu\s+mẫu|phụ\s+lục|mẫu\s+tờ\s+khai|mẫu\s+báo\s+cáo)\b',text.lower()))
def relation_candidates(text):
    out=[]; compact=re.sub(r'\s+',' ',text)
    for kind,kw in [('AMEND','sửa đổi'),('SUPPLEMENT','bổ sung'),('REPLACE','thay thế'),('REPEAL','bãi bỏ'),('CONSOLIDATE','hợp nhất')]:
        for m in re.finditer(r'.{0,90}\b'+re.escape(kw)+r'\b.{0,160}',compact,re.I):
            s=m.group(0).strip()
            if re.search(r'\d{1,4}/\d{4}/',s):out.append((kind,s[:320]))
            if len(out)>=50:return out
    return out
def main():
    ap=argparse.ArgumentParser();ap.add_argument('--root',required=True);ap.add_argument('--db');a=ap.parse_args();root=Path(a.root).resolve();svc=LegalLookupService(Path(a.db).resolve() if a.db else None); files=[p for p in root.rglob('*') if p.suffix.lower() in ('.docx','.pdf','.doc','.txt') and not p.name.startswith('~$')];count=0;cands=0
    with svc.connect() as con:
        for p in files:
            meta=parse_filename(p,root); text=docx_text(p) if p.suffix.lower()=='.docx' else pdf_text(p) if p.suffix.lower()=='.pdf' else ''; meta.update({'title':first_title(text),'status':'UNKNOWN','source_verified':False,'has_forms':None,'forms_review_status':'NOT_REVIEWED','forms_hint':form_hint(text),'notes':'Lập chỉ mục tự động từ tệp nguồn; hiệu lực và mẫu biểu phải được xác minh nghiệp vụ.'}); svc.upsert_document(meta,con=con); count+=1; con.execute('DELETE FROM relation_candidates WHERE doc_id=?',(meta['id'],))
            for kind,s in relation_candidates(text):con.execute('INSERT OR IGNORE INTO relation_candidates(doc_id,candidate_text,candidate_kind) VALUES(?,?,?)',(meta['id'],s,kind));cands+=1
        con.commit()
    print(json.dumps({'ok':True,'files':count,'relation_candidates':cands,'root':str(root)},ensure_ascii=False))
if __name__=='__main__':main()
