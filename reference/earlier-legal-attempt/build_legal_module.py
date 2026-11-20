import json,re,zipfile,subprocess,html,unicodedata
from pathlib import Path
BASE=Path('/mnt/data/hspo_full201/HQTN_HSPO_v1.9.4.1_LAN_Server')
DOCS=BASE/'static'/'documents'
MOD=BASE/'static'/'modules'/'legal_lookup'
DATA=MOD/'data'
DATA.mkdir(parents=True,exist_ok=True)
( BASE/'static'/'legal_data').mkdir(parents=True,exist_ok=True)
reg=json.loads((DOCS/'registry.json').read_text(encoding='utf-8'))['documents']

def docx_text(path):
    try:
        with zipfile.ZipFile(path) as z:
            chunks=[]
            for name in ('word/document.xml','word/footnotes.xml','word/endnotes.xml'):
                if name in z.namelist():
                    s=z.read(name).decode('utf-8','ignore')
                    s=re.sub(r'</w:p[^>]*>', '\n', s)
                    s=re.sub(r'<w:tab[^>]*/>', '\t', s)
                    s=re.sub(r'<[^>]+>', '', s)
                    chunks.append(html.unescape(s))
            return '\n'.join(chunks)
    except Exception:
        return ''

def olddoc_text(path):
    try:
        p=subprocess.run(['antiword',str(path)],capture_output=True,text=True,encoding='utf-8',errors='ignore',timeout=25)
        return p.stdout if p.returncode==0 else ''
    except Exception:
        return ''

def get_text(path):
    if not path.exists(): return ''
    if path.suffix.lower()=='.docx': return docx_text(path)
    if path.suffix.lower()=='.doc': return olddoc_text(path)
    return ''

def norm(s):
    s=unicodedata.normalize('NFKC',s or '').upper()
    s=s.replace('–','-').replace('—','-')
    return re.sub(r'\s+',' ',s)

known=[d['document_no'] for d in reg if d.get('document_no')]
# searchable variants, longest first
variants=[]
for no in known:
    vs={no, no.replace('/',' / '), no.replace('-',' - ')}
    variants.append((no, [norm(v) for v in vs]))

rel_kw=re.compile(r'(SỬA ĐỔI|BỔ SUNG|THAY THẾ|BÃI BỎ|HẾT HIỆU LỰC|ĐƯỢC SỬA ĐỔI|ĐƯỢC BỔ SUNG)')
form_re=re.compile(r'\b(MẪU\s+SỐ|MẪU\s+BIỂU|BIỂU\s+MẪU|THEO\s+MẪU|MẪU\s+TẠI)\b')
appendix_re=re.compile(r'\bPHỤ\s+LỤC\b')
items=[]; candidates=[]
for idx,d in enumerate(reg,1):
    f=d.get('file') or ''
    text=get_text(DOCS/f) if f else ''
    nt=norm(text)
    fh=len(form_re.findall(nt))
    ah=len(appendix_re.findall(nt))
    # Candidate relation: known document number near a relation keyword.
    seen=set()
    for target,tvars in variants:
        if target==d.get('document_no'): continue
        poss=[]
        for tv in tvars:
            st=0
            while True:
                k=nt.find(tv,st)
                if k<0: break
                poss.append(k); st=k+len(tv)
        for k in poss[:6]:
            a=max(0,k-260); b=min(len(nt),k+len(target)+260)
            window=nt[a:b]
            m=rel_kw.search(window)
            if m:
                key=(d.get('document_no'),target,m.group(1))
                if key in seen: continue
                seen.add(key)
                raw=re.sub(r'\s+',' ', text[max(0,a):min(len(text),b)]).strip()
                candidates.append({
                    'source_document_no':d.get('document_no'),
                    'target_document_no':target,
                    'relation_hint':m.group(1).title(),
                    'status':'CANDIDATE_REVIEW',
                    'snippet':raw[:520]
                })
    title=d.get('title') or ''
    has_explicit_appendix=bool(d.get('appendix_file'))
    form_hint=bool(fh)
    appendix_hint=bool(ah or has_explicit_appendix)
    items.append({
        'id':idx,
        'category':d.get('category','other'),
        'document_no':d.get('document_no',''),
        'title':title,
        'issuing_body':d.get('issuing_body',''),
        'effective':d.get('effective',''),
        'effective_note':d.get('effective_note',''),
        'note':d.get('note',''),
        'file':f,
        'pdf_file':d.get('pdf_file',''),
        'appendix_file':d.get('appendix_file',''),
        'form_hint':form_hint,
        'form_hint_count':fh,
        'appendix_hint':appendix_hint,
        'appendix_hint_count':ah,
        'forms_review_status':'NEEDS_REVIEW' if (form_hint or appendix_hint) else 'NO_HINT',
        'legal_status':'NOT_VERIFIED',
    })

payload={
 'module_version':'2.0.1',
 'generated_from_registry':reg and 'static/documents/registry.json',
 'principle':'Mọi văn bản có mẫu biểu/phụ lục phải rà soát riêng tình trạng sửa đổi, bổ sung, thay thế hoặc bãi bỏ của chính mẫu biểu; không suy diễn chỉ từ hiệu lực văn bản chính.',
 'documents':items,
 'relation_candidates':candidates,
 'stats':{'documents':len(items),'form_or_appendix_hints':sum(1 for x in items if x['forms_review_status']=='NEEDS_REVIEW'),'relation_candidates':len(candidates)}
}
(DATA/'legal_index.json').write_text(json.dumps(payload,ensure_ascii=False,indent=2),encoding='utf-8')
# persistent overlay outside module, future module replacement won't overwrite it
ov=BASE/'static'/'legal_data'/'legal_overrides.json'
if not ov.exists():
    ov.write_text(json.dumps({'schema_version':1,'documents':{},'relations':[],'forms':[]},ensure_ascii=False,indent=2),encoding='utf-8')
(MOD/'module.json').write_text(json.dumps({
    'id':'legal_lookup','name':'Tra cứu văn bản pháp lý','version':'2.0.1',
    'entry':'/static/modules/legal_lookup/index.html',
    'persistent_data':'/static/legal_data/legal_overrides.json',
    'upgrade_rule':'Các bản sau chỉ thay thư mục /static/modules/legal_lookup; không ghi đè /static/legal_data.'
},ensure_ascii=False,indent=2),encoding='utf-8')
print(json.dumps(payload['stats'],ensure_ascii=False))
