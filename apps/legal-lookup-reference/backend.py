from __future__ import annotations
import json, re, sqlite3, unicodedata
from pathlib import Path
from typing import Any, Dict, List, Optional

MODULE_DIR = Path(__file__).resolve().parent
DATA_DIR = MODULE_DIR / 'data'
STATUS_LABELS = {'EFFECTIVE':'Còn hiệu lực','PARTIAL':'Còn hiệu lực một phần','EXPIRED':'Hết hiệu lực','REPLACED':'Đã được thay thế','REPEALED':'Bị bãi bỏ','UNKNOWN':'Chưa xác minh','NEEDS_REVIEW':'Cần rà soát'}
FORM_STATUS_LABELS = {'VERIFIED_CURRENT':'Mẫu hiện hành đã xác minh','NEEDS_REVIEW':'Mẫu cần rà soát','REPLACED':'Mẫu đã được thay thế','REPEALED':'Mẫu bị bãi bỏ','NO_FORMS':'Không có mẫu biểu','NOT_REVIEWED':'Chưa rà soát mẫu biểu','UNKNOWN':'Chưa xác minh'}

def _norm(s: Any) -> str:
    s='' if s is None else str(s)
    s=unicodedata.normalize('NFD',s)
    s=''.join(ch for ch in s if unicodedata.category(ch)!='Mn')
    return re.sub(r'\s+',' ',s.lower()).strip()

def _load_json(path: Path, default):
    try: return json.loads(path.read_text(encoding='utf-8'))
    except Exception: return default

class LegalLookupService:
    def __init__(self, db_path: Optional[Path]=None, module_dir: Optional[Path]=None):
        self.module_dir=Path(module_dir or MODULE_DIR)
        self.data_dir=self.module_dir/'data'
        self.config=_load_json(self.data_dir/'config.json',{})
        self.db_path=Path(db_path or (self.data_dir/self.config.get('db_file','legal_lookup.db')))
        self.db_path.parent.mkdir(parents=True,exist_ok=True)
        self._init_db(); self._seed_if_empty()
    def connect(self):
        con=sqlite3.connect(self.db_path); con.row_factory=sqlite3.Row; con.execute('PRAGMA foreign_keys=ON'); return con
    def _init_db(self):
        sql='''
        CREATE TABLE IF NOT EXISTS documents(id TEXT PRIMARY KEY,number TEXT,year INTEGER,doc_type TEXT,issuer TEXT,title TEXT,category TEXT,filename TEXT,relative_path TEXT,status TEXT DEFAULT 'UNKNOWN',effective_date TEXT,expiry_date TEXT,consolidated_ref TEXT,source_verified INTEGER DEFAULT 0,has_forms INTEGER,forms_review_status TEXT DEFAULT 'NOT_REVIEWED',forms_hint INTEGER DEFAULT 0,notes TEXT,search_blob_norm TEXT,indexed_at TEXT DEFAULT CURRENT_TIMESTAMP,updated_at TEXT DEFAULT CURRENT_TIMESTAMP);
        CREATE TABLE IF NOT EXISTS relations(id INTEGER PRIMARY KEY AUTOINCREMENT,from_doc_id TEXT NOT NULL,to_doc_id TEXT NOT NULL,relation_type TEXT NOT NULL,scope TEXT,effective_date TEXT,verified INTEGER DEFAULT 0,notes TEXT,UNIQUE(from_doc_id,to_doc_id,relation_type,scope),FOREIGN KEY(from_doc_id) REFERENCES documents(id) ON DELETE CASCADE,FOREIGN KEY(to_doc_id) REFERENCES documents(id) ON DELETE CASCADE);
        CREATE TABLE IF NOT EXISTS forms(id TEXT PRIMARY KEY,doc_id TEXT NOT NULL,code TEXT,name TEXT,appendix TEXT,status TEXT DEFAULT 'NOT_REVIEWED',current_form_ref TEXT,replacement_doc_id TEXT,effective_date TEXT,verified INTEGER DEFAULT 0,notes TEXT,FOREIGN KEY(doc_id) REFERENCES documents(id) ON DELETE CASCADE);
        CREATE TABLE IF NOT EXISTS relation_candidates(id INTEGER PRIMARY KEY AUTOINCREMENT,doc_id TEXT NOT NULL,candidate_text TEXT NOT NULL,candidate_kind TEXT,verified INTEGER DEFAULT 0,UNIQUE(doc_id,candidate_text));
        CREATE INDEX IF NOT EXISTS idx_docs_number ON documents(number,year); CREATE INDEX IF NOT EXISTS idx_docs_category ON documents(category); CREATE INDEX IF NOT EXISTS idx_docs_status ON documents(status); CREATE INDEX IF NOT EXISTS idx_docs_search ON documents(search_blob_norm);
        '''
        with self.connect() as con: con.executescript(sql)
    def _seed_if_empty(self):
        with self.connect() as con:
            if con.execute('SELECT COUNT(*) FROM documents').fetchone()[0]: return
            for d in _load_json(self.data_dir/'seed_documents.json',[]): self.upsert_document(d,con=con)
            for r in _load_json(self.data_dir/'seed_relations.json',[]): self.add_relation(r,con=con)
            for f in _load_json(self.data_dir/'seed_forms.json',[]): self.upsert_form(f,con=con)
    def upsert_document(self,d:Dict[str,Any],con=None):
        own=con is None; con=con or self.connect()
        try:
            blob=' '.join(str(d.get(k) or '') for k in ('number','year','doc_type','issuer','title','category','filename','notes'))
            vals={'id':d['id'],'number':d.get('number'),'year':d.get('year'),'doc_type':d.get('doc_type'),'issuer':d.get('issuer'),'title':d.get('title') or '','category':d.get('category'),'filename':d.get('filename'),'relative_path':d.get('relative_path'),'status':d.get('status','UNKNOWN'),'effective_date':d.get('effective_date'),'expiry_date':d.get('expiry_date'),'consolidated_ref':d.get('consolidated_ref'),'source_verified':int(bool(d.get('source_verified'))),'has_forms':None if d.get('has_forms') is None else int(bool(d.get('has_forms'))),'forms_review_status':d.get('forms_review_status','NOT_REVIEWED'),'forms_hint':int(bool(d.get('forms_hint'))),'notes':d.get('notes'),'search_blob_norm':_norm(blob)}
            con.execute('''INSERT INTO documents(id,number,year,doc_type,issuer,title,category,filename,relative_path,status,effective_date,expiry_date,consolidated_ref,source_verified,has_forms,forms_review_status,forms_hint,notes,search_blob_norm) VALUES(:id,:number,:year,:doc_type,:issuer,:title,:category,:filename,:relative_path,:status,:effective_date,:expiry_date,:consolidated_ref,:source_verified,:has_forms,:forms_review_status,:forms_hint,:notes,:search_blob_norm) ON CONFLICT(id) DO UPDATE SET number=excluded.number,year=excluded.year,doc_type=excluded.doc_type,issuer=excluded.issuer,title=CASE WHEN excluded.title<>'' THEN excluded.title ELSE documents.title END,category=excluded.category,filename=excluded.filename,relative_path=excluded.relative_path,forms_hint=MAX(documents.forms_hint,excluded.forms_hint),search_blob_norm=excluded.search_blob_norm,indexed_at=CURRENT_TIMESTAMP''',vals)
            if own: con.commit()
        finally:
            if own: con.close()
    def add_relation(self,r:Dict[str,Any],con=None):
        own=con is None; con=con or self.connect()
        try:
            con.execute('INSERT OR IGNORE INTO relations(from_doc_id,to_doc_id,relation_type,scope,effective_date,verified,notes) VALUES(?,?,?,?,?,?,?)',(r['from_doc_id'],r['to_doc_id'],r['relation_type'],r.get('scope'),r.get('effective_date'),int(bool(r.get('verified'))),r.get('notes')))
            if own: con.commit()
        finally:
            if own: con.close()
    def upsert_form(self,f:Dict[str,Any],con=None):
        own=con is None; con=con or self.connect()
        try:
            con.execute('''INSERT INTO forms(id,doc_id,code,name,appendix,status,current_form_ref,replacement_doc_id,effective_date,verified,notes) VALUES(?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET code=excluded.code,name=excluded.name,appendix=excluded.appendix,status=excluded.status,current_form_ref=excluded.current_form_ref,replacement_doc_id=excluded.replacement_doc_id,effective_date=excluded.effective_date,verified=excluded.verified,notes=excluded.notes''',(f['id'],f['doc_id'],f.get('code'),f.get('name'),f.get('appendix'),f.get('status','NOT_REVIEWED'),f.get('current_form_ref'),f.get('replacement_doc_id'),f.get('effective_date'),int(bool(f.get('verified'))),f.get('notes')))
            if own: con.commit()
        finally:
            if own: con.close()
    def search(self,q:str='',category:str='',status:str='',forms:str='',limit:int=200)->List[Dict[str,Any]]:
        clauses=[]; args=[]
        if q: clauses.append('search_blob_norm LIKE ?'); args.append('%'+_norm(q)+'%')
        if category: clauses.append('category=?'); args.append(category)
        if status: clauses.append('status=?'); args.append(status)
        if forms=='needs_review': clauses.append("forms_review_status IN ('NOT_REVIEWED','NEEDS_REVIEW','UNKNOWN')")
        elif forms=='has_forms': clauses.append('has_forms=1')
        elif forms=='verified': clauses.append("forms_review_status='VERIFIED_CURRENT'")
        where=(' WHERE '+' AND '.join(clauses)) if clauses else ''
        sql=f'''SELECT d.*,(SELECT COUNT(*) FROM relations r WHERE r.from_doc_id=d.id OR r.to_doc_id=d.id) relation_count,(SELECT COUNT(*) FROM forms f WHERE f.doc_id=d.id) form_count FROM documents d {where} ORDER BY year DESC,CAST(number AS INTEGER) DESC,number DESC LIMIT ?'''; args.append(max(1,min(int(limit),1000)))
        with self.connect() as con: rows=[dict(r) for r in con.execute(sql,args)]
        for r in rows:
            r['status_label']=STATUS_LABELS.get(r['status'],r['status']); r['forms_review_label']=FORM_STATUS_LABELS.get(r['forms_review_status'],r['forms_review_status']); r['warning_forms']=r['forms_review_status'] in ('NOT_REVIEWED','NEEDS_REVIEW','UNKNOWN') or bool(r.get('forms_hint') and not r.get('form_count')); r['has_forms']=None if r['has_forms'] is None else bool(r['has_forms']); r['source_verified']=bool(r['source_verified']); r['forms_hint']=bool(r['forms_hint'])
        return rows
    def detail(self,doc_id:str)->Optional[Dict[str,Any]]:
        with self.connect() as con:
            row=con.execute('SELECT * FROM documents WHERE id=?',(doc_id,)).fetchone()
            if not row: return None
            d=dict(row)
            rel=[dict(x) for x in con.execute('''SELECT r.*,f.number from_number,f.year from_year,t.number to_number,t.year to_year,f.title from_title,t.title to_title FROM relations r JOIN documents f ON f.id=r.from_doc_id JOIN documents t ON t.id=r.to_doc_id WHERE r.from_doc_id=? OR r.to_doc_id=? ORDER BY r.id''',(doc_id,doc_id))]
            forms=[dict(x) for x in con.execute('SELECT * FROM forms WHERE doc_id=? ORDER BY appendix,code,name',(doc_id,))]
            cands=[dict(x) for x in con.execute('SELECT * FROM relation_candidates WHERE doc_id=? ORDER BY id',(doc_id,))]
        d['relations']=rel; d['forms']=forms; d['relation_candidates']=cands; d['status_label']=STATUS_LABELS.get(d['status'],d['status']); d['forms_review_label']=FORM_STATUS_LABELS.get(d['forms_review_status'],d['forms_review_status']); d['has_forms']=None if d['has_forms'] is None else bool(d['has_forms']); d['source_verified']=bool(d['source_verified']); d['forms_hint']=bool(d['forms_hint']); return d
    def stats(self):
        with self.connect() as con:
            total=con.execute('SELECT COUNT(*) FROM documents').fetchone()[0]; needs=con.execute("SELECT COUNT(*) FROM documents WHERE status IN ('UNKNOWN','NEEDS_REVIEW')").fetchone()[0]; forms=con.execute("SELECT COUNT(*) FROM documents WHERE forms_review_status IN ('NOT_REVIEWED','NEEDS_REVIEW','UNKNOWN')").fetchone()[0]; rel=con.execute('SELECT COUNT(*) FROM relations WHERE verified=1').fetchone()[0]; cats=[dict(r) for r in con.execute('SELECT category,COUNT(*) count FROM documents GROUP BY category ORDER BY count DESC')]
        return {'total':total,'needs_review':needs,'forms_needs_review':forms,'verified_relations':rel,'categories':cats,'module_version':self.config.get('module_version','2.0.1')}
    def resolve_file(self,doc_id:str)->Optional[Path]:
        d=self.detail(doc_id)
        if not d or not d.get('relative_path'): return None
        hint=self.config.get('documents_root','../../VAN_BAN_PHAP_LUAT'); root=(self.module_dir/hint).resolve() if not Path(hint).is_absolute() else Path(hint).resolve(); p=(root/d['relative_path']).resolve()
        try: p.relative_to(root)
        except ValueError: return None
        return p if p.is_file() else None
service=LegalLookupService()
