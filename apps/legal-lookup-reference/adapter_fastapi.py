from fastapi import APIRouter,HTTPException
from fastapi.responses import FileResponse
from backend import LegalLookupService
def create_router(service=None):
    svc=service or LegalLookupService();r=APIRouter(prefix='/api/modules/legal_lookup')
    @r.get('/health')
    def health():return {'ok':True,'version':'2.0.1'}
    @r.get('/stats')
    def stats():return svc.stats()
    @r.get('/search')
    def search(q:str='',category:str='',status:str='',forms:str=''):return {'items':svc.search(q,category,status,forms)}
    @r.get('/document')
    def detail(id:str):
        d=svc.detail(id)
        if not d:raise HTTPException(404,'not_found')
        return d
    @r.get('/file')
    def file(id:str):
        p=svc.resolve_file(id)
        if not p:raise HTTPException(404,'not_found')
        return FileResponse(str(p),filename=p.name)
    return r
