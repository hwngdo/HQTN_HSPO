from __future__ import annotations
import json,mimetypes,urllib.parse
from http.server import ThreadingHTTPServer,BaseHTTPRequestHandler
from pathlib import Path
from backend import service,MODULE_DIR
WEB=MODULE_DIR/'web';PREFIX='/api/modules/legal_lookup'
class H(BaseHTTPRequestHandler):
    server_version='HQTN-LegalLookup/2.0.1'
    def log_message(self,fmt,*args):pass
    def js(self,obj,code=200):
        b=json.dumps(obj,ensure_ascii=False).encode();self.send_response(code);self.send_header('Content-Type','application/json; charset=utf-8');self.send_header('Content-Length',str(len(b)));self.end_headers();self.wfile.write(b)
    def file(self,p,download=False):
        p=Path(p)
        if not p.is_file():return self.send_error(404)
        data=p.read_bytes();self.send_response(200);self.send_header('Content-Type',mimetypes.guess_type(p.name)[0] or 'application/octet-stream');
        if download:self.send_header('Content-Disposition',f'inline; filename="{p.name}"')
        self.send_header('Content-Length',str(len(data)));self.end_headers();self.wfile.write(data)
    def do_GET(self):
        u=urllib.parse.urlparse(self.path);qs=urllib.parse.parse_qs(u.query);path=u.path
        if path in ('/','/modules/legal_lookup','/modules/legal_lookup/'):return self.file(WEB/'index.html')
        if path.startswith('/modules/legal_lookup/'):
            p=(WEB/path[len('/modules/legal_lookup/'):]).resolve()
            try:p.relative_to(WEB.resolve())
            except ValueError:return self.send_error(403)
            return self.file(p)
        if path==PREFIX+'/health':return self.js({'ok':True,'version':'2.0.1'})
        if path==PREFIX+'/stats':return self.js(service.stats())
        if path==PREFIX+'/search':return self.js({'items':service.search(q=qs.get('q',[''])[0],category=qs.get('category',[''])[0],status=qs.get('status',[''])[0],forms=qs.get('forms',[''])[0])})
        if path==PREFIX+'/document':
            d=service.detail(qs.get('id',[''])[0]);return self.js(d or {'error':'not_found'},200 if d else 404)
        if path==PREFIX+'/file':
            p=service.resolve_file(qs.get('id',[''])[0]);return self.file(p,True) if p else self.send_error(404)
        return self.send_error(404)
def main():
    import argparse,webbrowser
    ap=argparse.ArgumentParser();ap.add_argument('--host',default='127.0.0.1');ap.add_argument('--port',type=int,default=8781);ap.add_argument('--open',action='store_true');a=ap.parse_args();url=f'http://{a.host}:{a.port}/modules/legal_lookup/';print(url); webbrowser.open(url) if a.open else None;ThreadingHTTPServer((a.host,a.port),H).serve_forever()
if __name__=='__main__':main()
