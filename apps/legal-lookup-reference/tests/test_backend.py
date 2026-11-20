import tempfile,sys
from pathlib import Path
sys.path.insert(0,str(Path(__file__).resolve().parents[1]))
from backend import LegalLookupService
with tempfile.TemporaryDirectory() as td:
    s=LegalLookupService(Path(td)/'x.db')
    assert s.stats()['total']>0
    rows=s.search('31'); assert rows
    d=s.detail(rows[0]['id']); assert d and 'forms_review_label' in d
    print('OK',s.stats())
