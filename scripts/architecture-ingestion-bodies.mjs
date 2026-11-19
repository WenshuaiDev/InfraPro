// Offline verification uses only the returned synthetic DTOs and the two
// simulator manifests. Cookie and CSRF values are excluded at capture time.
import assert from 'node:assert/strict';
import { existsSync,readFileSync,realpathSync,statSync } from 'node:fs';
import { join,resolve } from 'node:path';
import { pathToFileURL } from 'node:url';
import { digest,read,write } from './architecture-lab.mjs';
import { PairedHTTP } from './architecture-workflow-http.mjs';

const uuid=/^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/;
const sha=/^[a-f0-9]{64}$/;
const timestamp=/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/;
const chronological=new Set(['sampled_at','applied_at','received_at','stored_at','last_received_at','last_parsed_at','last_measurement_at','first_received_at','verified_at','assigned_at','claimed_at','acknowledged_at']);
const token=/^[A-Za-z0-9_-]{20,80}$/;
const canonical=v=>v&&typeof v==='object'?(Array.isArray(v)?v.map(canonical):Object.fromEntries(Object.keys(v).sort().map(k=>[k,canonical(v[k])]))):v;
const parserResponses = new Set(['Parsing:platform:runs:response', 'Parsing:tenant:runs:response', 'Ingestion:tenant:messages:response']);

// The parser contract returns the hash of the executable that served the
// request, independently of the immutable method manifest and its runner.
export function compareParserAttestation(operation, value, expected) {
  if (!expected) return value; // Historical runs used one identical parser on both sides.
  function envelope(response) {
    assert.match(expected, sha);
    assert.equal(response.contract_version, 1);
    assert.equal(response.service_build_digest, expected, operation + ' serving parser digest');
    return { ...response, service_build_digest: '[verified parser executable]' };
  }
  if (parserResponses.has(operation))
    return envelope(value);
  if (['Parsing:platform:runs:result', 'Parsing:tenant:runs:result'].includes(operation))
    return { ...value, response: envelope(value.response) };
  if (operation === 'Ingestion:tenant:messages:results')
    return value.map(item => ({ ...item, response: envelope(item.response) }));
  return value;
}

export function checkIngestionBodies(directory,operations){
  const pairs=['a','b'].map(s=>read(join(directory,'private',s,'public-bodies.json')));
  assert.equal(pairs[0].length,pairs[1].length);assert.equal(operations.length,pairs[0].length);
  const sim=['a','b'].map(s=>read(join(directory,'private',s,'simulator.json')));
  const start=statSync(join(directory,'snapshot.json')).mtimeMs-2000;
  const end=Math.max(...['a','b'].map(s=>statSync(join(directory,'private',s,'public-bodies.json')).mtimeMs))+2000;
  const family=new PairedHTTP(directory,'ingestion-bodies');family.start=start;
  const programsFile = join(directory, 'parser-programs.json');
  const programs = existsSync(programsFile) ? read(programsFile) : {};
  const snapshot = read(join(directory, 'snapshot.json'));
  const builds = ['a','b'].map(side => {
    if (!programs[side]) return undefined;
    const record = programs[side], release = read(record.releaseManifest);
    assert.equal(side === 'a' ? release.commit : release.inputs.sourceSHA256,
      side === 'a' ? snapshot.baseline : snapshot.candidateSHA256);
    for (const [name, observed] of Object.entries(record.binaries)) {
      assert.equal(release.binaries[name], observed);
      assert.equal(digest(readFileSync(join(release.bin, name))), observed);
    }
    assert.match(record.binaries.parser, sha);
    return record.binaries.parser;
  });
  if (existsSync(programsFile)) assert.ok(builds.every(Boolean));
  const rows=[];
  const ordered=(arr,key)=>arr.every((row,i)=>i===0||arr[i-1][key]<row[key]);
  const map=(x,y)=>assert.ok(family.bind(x,y),'new public identities must have one-to-one correspondence');
  function normalized(l,r,op,loc='',key=''){
    if(l===r)return [l,r];
    if(typeof l==='string'&&typeof r==='string'){
      if(['epoch','spool_epoch'].includes(key)&&uuid.test(l)&&uuid.test(r))map(l,r);
      if(key==='request_id'&&op.method==='GET'&&token.test(l)&&token.test(r)&&l.length===r.length)map(l,r);
      if(chronological.has(key)&&timestamp.test(l)&&timestamp.test(r)&&[l,r].every(t=>Date.parse(t)>=start&&Date.parse(t)<=end))map(l,r);
      return [l,r];
    }
    if(!l||!r||typeof l!=='object'||typeof r!=='object')return [l,r];
    if(l.base64&&r.base64){
      const left=Buffer.from(l.base64,'base64'),right=Buffer.from(r.base64,'base64');
      if(left.equals(right) && !parserResponses.has(op.operationId))return [l,r];
      // Verify the actual serving executable even when the raw bytes agree.
      // Every other decoded field is compared recursively.
      const [x,y]=[left,right].map((bytes, side) => compareParserAttestation(op.operationId,JSON.parse(bytes),builds[side]));
      assert.ok(uuid.test(x.execution_id)&&uuid.test(y.execution_id));
      map(digest(left),digest(right));return normalized(x,y,op,loc,key);
    }
    if(Array.isArray(l)&&Array.isArray(r)){
      let sortKey;
      if(key==='items'&&op.operationId.startsWith('Parsing:')&&l.every(v=>v.entry)&&r.every(v=>v.entry))
        sortKey=row=>JSON.stringify(canonical(row.entry));
      if(key==='items'&&op.operationId.includes('configs:list'))sortKey=row=>row.number;
      if(key==='items'&&op.operationId==='Ingestion:tenant:messages:list')sortKey=row=>row.message_kind+':'+row.sha256;
      if(key==='items'&&op.operationId==='Ingestion:tenant:unmatched:list')sortKey=row=>row.byte_length;
      if(sortKey&&l.length>1){
        assert.ok(ordered(l,'id')&&ordered(r,'id'),op.operationId+' UUID cursor order');
        const sorted=arr=>arr.toSorted((a,b)=>String(sortKey(a)).localeCompare(String(sortKey(b))));
        l=sorted(l);r=sorted(r);
        assert.equal(new Set(l.map(sortKey)).size,l.length,'ambiguous logical page row');
        assert.deepEqual(l.map(sortKey),r.map(sortKey));
      }
      const left=[],right=[];for(let i=0;i<Math.max(l.length,r.length);i++){
        const [a,b]=normalized(l[i],r[i],op,loc+'/'+i,key);left.push(a);right.push(b);
      }return [left,right];
    }
    if(Array.isArray(l)||Array.isArray(r))return [l,r];
    if(l.snapshot?.source_revision_digest&&r.snapshot?.source_revision_digest){
      for(const v of [l,r])assert.equal(digest(JSON.stringify({...v.snapshot,source_revision_digest:''})),v.digest);
      map(l.digest,r.digest);
      assert.deepEqual([l.snapshot.listener_port,r.snapshot.listener_port],sim.map(s=>s.sources[0].port));
    }
    if(l.envelope?.envelope_digest&&r.envelope?.envelope_digest){
      for(const v of [l,r]){
        assert.equal(v.envelope.envelope_digest,v.envelope_digest);
        assert.equal(v.envelope.delivery_attempt,0);
        assert.equal(digest(JSON.stringify({...v.envelope,envelope_digest:'',payload_base64:''})),v.envelope_digest);
      }
      map(l.envelope_digest,r.envelope_digest);
    }
    if(l.snapshot&&r.snapshot&&sha.test(l.content_digest)&&sha.test(r.content_digest)){
      for(const v of [l,r])assert.equal(digest(JSON.stringify(v.snapshot)),v.content_digest);
      map(l.content_digest,r.content_digest);
    }
    const left={},right={};
    for(const k of new Set([...Object.keys(l),...Object.keys(r)])){
      let a=l[k],b=r[k];
      if(k==='listener_port'&&a===sim[0].sources[0].port&&b===sim[1].sources[0].port)a=b='paired-listener';
      [a,b]=normalized(a,b,op,loc+'/'+k,k);
      if(Object.hasOwn(l,k))left[k]=a;if(Object.hasOwn(r,k))right[k]=b;
    }
    return [left,right];
  }
  // Validate the parser response itself below and bind the digest of its exact
  // bytes before an earlier result metadata view refers to that digest.
  for(let i=0;i<operations.length;i++){
    const [l,r]=pairs.map(p=>p[i].value);
    if(!l?.base64||!r?.base64||l.base64===r.base64)continue;
    const [a,b]=[l,r].map(v=>Buffer.from(v.base64,'base64'));const x=JSON.parse(a),y=JSON.parse(b);
    assert.ok(uuid.test(x.execution_id)&&uuid.test(y.execution_id));map(digest(a),digest(b));map(x.execution_id,y.execution_id);
  }
  operations.forEach((op,i)=>{
    const [a,b]=pairs.map(s=>s[i]);assert.deepEqual([a.operationId,b.operationId],[op.operationId,op.operationId]);
    assert.deepEqual([a.command,b.command],[op.command,op.command]);
    const values = [a,b].map((row, side) => row.status >= 200 && row.status < 300 && !row.value?.base64
      ? compareParserAttestation(op.operationId,row.value,builds[side]) : row.value);
    const [l,r]=normalized(...values,op),out=family.verify(l,r,op,start);
    rows.push({operationId:op.operationId,bodyIndex:i,differences:out.differences,mapped:out.mapped.length});
  });
  return {status:rows.every(r=>r.differences.length===0)?'passed':'failed',...(existsSync(programsFile) && {parserBuilds:builds}),rows};
}
if(process.argv[1]&&existsSync(process.argv[1])&&pathToFileURL(realpathSync(process.argv[1])).href===import.meta.url){
  const scenario=read(resolve(process.argv[2])),dir=scenario.sandbox;
  assert.ok(existsSync(join(dir,'snapshot.json')));
  const proof=checkIngestionBodies(dir,scenario.records);write(join(dir,'body-comparison.json'),proof);
  console.log(proof.rows.filter(r=>r.differences.length>0).map(r=>[r.bodyIndex,r.operationId,r.differences.slice(0,4)]));
  if(proof.status!=='passed')process.exitCode=1;
}
