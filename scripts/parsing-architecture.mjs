import {execFileSync} from 'node:child_process';
import {readFileSync,writeFileSync,mkdirSync} from 'node:fs';
import {createHash} from 'node:crypto';
const graph=execFileSync('go',['list','-deps','./services/parser/cmd/parser','./services/parser/cmd/parser-runner'],{encoding:'utf8'});
if(graph.split('\n').some(v=>/InfraPro\/(packages\/go\/platform|services\/api)|pgx|redis|clickhouse|minio/.test(v)))throw Error('Standalone parser acquired a business/storage dependency');
const fixture=JSON.parse(readFileSync('samples/parsing/manifest.json','utf8'));
for(const sample of fixture.samples){const raw=readFileSync('samples/parsing/'+sample.file),hex=readFileSync('samples/parsing/'+sample.file.replace('.bin','.hex'),'utf8').replaceAll(/\s/g,'');if(raw.length!==sample.byte_length||createHash('sha256').update(raw).digest('hex')!==sample.sha256||!Buffer.from(hex,'hex').equals(raw))throw Error('Original parsing sample drift: '+sample.file);}
const measurement=readFileSync('samples/parsing/measurement.bin');if(measurement.subarray(...fixture.expected['measurement.bin'].value_range).toString()!=='"12.340"')throw Error('Handwritten byte expectation drift');
mkdirSync('.artifacts/parsing',{recursive:true});writeFileSync('.artifacts/parsing/architecture.json',JSON.stringify({passed:true,standalone:true,samples:fixture.samples.length,input_hashes_verified:true,handwritten_range_verified:true},null,2));console.log('Standalone dependency graph and original parsing samples verified.');
