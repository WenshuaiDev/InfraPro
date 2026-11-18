"""Streaming SigV4 S3 archive, verified TLS; no credentials in argv or output."""
import datetime,hashlib,hmac,http.client,json,pathlib,ssl,urllib.parse,xml.etree.ElementTree as ET
class S3:
 def __init__(self,endpoint,key,secret,ca):
  self.url=urllib.parse.urlsplit(endpoint);self.key=key;self.secret=secret;self.ctx=ssl.create_default_context(cafile=str(ca))
 def request(self,method,key='',query=None,file=None,destination=None):
  path='/'+urllib.parse.quote(key,safe='/~');query=urllib.parse.urlencode(sorted((query or {}).items()),quote_via=urllib.parse.quote,safe='~');stamp=datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ');date=stamp[:8]
  size=pathlib.Path(file).stat().st_size if file else 0; digest=hashlib.file_digest(open(file,'rb'),'sha256').hexdigest() if file else hashlib.sha256(b'').hexdigest();headers={'host':self.url.netloc,'x-amz-content-sha256':digest,'x-amz-date':stamp};names=';'.join(sorted(headers));canonical='\n'.join([method,path,query,''.join(k+':'+headers[k]+'\n' for k in sorted(headers)),names,digest]);scope=date+'/us-east-1/s3/aws4_request';signing=('AWS4'+self.secret).encode()
  for v in [date,'us-east-1','s3','aws4_request']:signing=hmac.new(signing,v.encode(),hashlib.sha256).digest()
  signature=hmac.new(signing,('AWS4-HMAC-SHA256\n'+stamp+'\n'+scope+'\n'+hashlib.sha256(canonical.encode()).hexdigest()).encode(),hashlib.sha256).hexdigest();headers['Authorization']='AWS4-HMAC-SHA256 Credential='+self.key+'/'+scope+', SignedHeaders='+names+', Signature='+signature;headers['Content-Length']=str(size)
  c=http.client.HTTPSConnection(self.url.hostname,self.url.port,context=self.ctx,timeout=180);c.putrequest(method,path+('?' +query if query else ''),skip_host=True)
  for k,v in headers.items():c.putheader(k,v)
  c.endheaders()
  if file:
   with open(file,'rb') as f:
    while data:=f.read(1<<20):c.send(data)
  r=c.getresponse()
  if not 200<=r.status<300:
   detail=r.read(300);c.close();raise RuntimeError('S3 '+method+' HTTP '+str(r.status)+' '+detail.decode(errors='replace'))
  if destination:
   h=hashlib.sha256();n=0
   with open(destination,'wb') as f:
    while data:=r.read(1<<20):f.write(data);h.update(data);n+=len(data)
   result={'size':n,'sha256':h.hexdigest()}
  else:result=r.read()
  c.close();return result
 def list(self,bucket):
  token='';ns={'s':'http://s3.amazonaws.com/doc/2006-03-01/'}
  while True:
   q={'list-type':'2','max-keys':'1000'}
   if token:q['continuation-token']=token
   root=ET.fromstring(self.request('GET',bucket,q))
   for row in root.findall('s:Contents',ns):yield row.findtext('s:Key',namespaces=ns),int(row.findtext('s:Size',namespaces=ns))
   token=root.findtext('s:NextContinuationToken',default='',namespaces=ns)
   if not token:break
 def archive(self,bucket,directory):
  d=pathlib.Path(directory);d.mkdir(parents=True);entries=[]
  for i,(key,size) in enumerate(self.list(bucket)):
   path=d/f'{i:08d}.object';result=self.request('GET',bucket+'/'+key,destination=path)
   if result['size']!=size:raise RuntimeError('S3 listing size mismatch')
   entries.append({'bucket':bucket,'key':key,'path':path.name,**result})
  (d/'manifest.json').write_text(json.dumps(entries,ensure_ascii=False,indent=2));return len(entries)
 def restore(self,bucket,directory):
  d=pathlib.Path(directory);entries=json.loads((d/'manifest.json').read_text())
  for entry in entries:
   p=d/entry['path']
   with open(p,'rb') as f:actual=hashlib.file_digest(f,'sha256').hexdigest()
   if actual!=entry['sha256'] or p.stat().st_size!=entry['size']:raise RuntimeError('Archive hash mismatch')
   self.request('PUT',bucket+'/'+entry['key'],file=p)
   tmp=d/'.readback';actual=self.request('GET',bucket+'/'+entry['key'],destination=tmp);tmp.unlink()
   if actual!={k:entry[k] for k in ['size','sha256']}:raise RuntimeError('Restore readback mismatch')
  return len(entries)
