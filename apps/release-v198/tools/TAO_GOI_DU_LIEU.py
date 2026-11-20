"""Trình đóng gói dữ liệu HSPO. Phải xác minh pháp luật trước khi đánh dấu đã duyệt.
Ví dụ:
 python TAO_GOI_DU_LIEU.py ./du_lieu_thay_doi ./DATA.zip 2026.11.01.01 2026.11.01.01 "Thay TT..." --review-date 2026-11-01 --review-evidence "Trang VBPL ... đối chiếu Điều ..."
"""
import argparse,hashlib,json,pathlib,zipfile
p=argparse.ArgumentParser(description=__doc__)
for name in ('source','output','data_version','rule_version','description'):p.add_argument(name)
p.add_argument('--review-date',default='');p.add_argument('--review-evidence',default='')
a=p.parse_args()
review=bool(a.review_date and a.review_evidence)
if bool(a.review_date)!=bool(a.review_evidence):p.error('Yêu cầu đồng thời ngày và bằng chứng kiểm tra hiệu lực')
root=pathlib.Path(a.source);files=[]
for f in sorted(root.rglob('*')):
 if not f.is_file():continue
 r=f.relative_to(root).as_posix()
 if not (r.startswith('policy_data/') or r.startswith('static/')):p.error(f'Tệp không hợp lệ: {r}')
 b=f.read_bytes();files.append({'path':r,'sha256':hashlib.sha256(b).hexdigest(),'size':len(b)})
if not files:p.error('Không có tệp')
m={'format':'HQTN_HSPO_DATA_V1','pack_id':'HSPO_'+a.data_version.replace('.','_'),
 'data_version':a.data_version,'rule_version':a.rule_version,'min_app_version':'1.9.8',
 'description':a.description,'legal_reviewed':review,'legal_review_date':a.review_date,
 'legal_review_evidence':a.review_evidence,'files':files}
with zipfile.ZipFile(a.output,'w',zipfile.ZIP_DEFLATED,compresslevel=8) as z:
 z.writestr('manifest.json',json.dumps(m,ensure_ascii=False,indent=2))
 for f in files:z.write(root/f['path'],f['path'])
print('Đã tạo:',a.output,'| Số tệp:',len(files),'| Đã duyệt hiệu lực:',review)
