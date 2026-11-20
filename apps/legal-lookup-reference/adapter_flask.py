from flask import Blueprint,jsonify,request,send_file
from backend import LegalLookupService
def create_blueprint(service=None,name='legal_lookup'):
    svc=service or LegalLookupService();bp=Blueprint(name,__name__,url_prefix='/api/modules/legal_lookup')
    @bp.get('/health')
    def health():return jsonify(ok=True,version='2.0.1')
    @bp.get('/stats')
    def stats():return jsonify(svc.stats())
    @bp.get('/search')
    def search():return jsonify(items=svc.search(request.args.get('q',''),request.args.get('category',''),request.args.get('status',''),request.args.get('forms','')))
    @bp.get('/document')
    def detail():
        d=svc.detail(request.args.get('id',''));return (jsonify(d),200) if d else (jsonify(error='not_found'),404)
    @bp.get('/file')
    def file():
        p=svc.resolve_file(request.args.get('id',''));return send_file(p,as_attachment=False) if p else (jsonify(error='not_found'),404)
    return bp
