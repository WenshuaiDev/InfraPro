import { execFileSync } from 'node:child_process';
import { createHash, randomBytes } from 'node:crypto';
import { chmodSync, copyFileSync, cpSync, existsSync, lstatSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, readlinkSync, realpathSync, renameSync, rmSync, rmdirSync, statSync, symlinkSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, relative, resolve } from 'node:path';
import { parseArgs } from 'node:util';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { applicationSourceDigest, ingestionProfile, inventory, managed, root } from './architecture-runtime.mjs';
import { clickhouseProfile } from './architecture-clickhouse.mjs';
import { replicaLayout, publicVars } from './architecture-replicas.mjs';
import { renderRelaySlots } from './architecture-machines.mjs';
import { writeResourceLimits } from './architecture-capacities.mjs';
import { processEnvironment, processProfiles, renderOriginalProcessLimits } from './architecture-process-resources.mjs';

const privateRoles = [...Object.keys(inventory.privatePorts), 'display-runtime', 'diagnostics-observer'];
const active = managed.filter(role => ['both','split','standby','compat'].includes(inventory.roles[role].mode));
const stringify = value => JSON.stringify(value,null,2)+'\n';
function save(path, data, mode=0o600) {
  mkdirSync(dirname(path),{recursive:true,mode:0o700});
  writeFileSync(path, typeof data === 'string' ? data : stringify(data),{mode, flag:'wx'});
}
function certs(path, people) {
  const pki = join(path,'issuer');
  mkdirSync(pki,{mode:0o700});
  function openssl(args) {execFileSync('openssl',args,{cwd:pki,stdio:['ignore','ignore','pipe']});}
  openssl(['req','-x509','-new','-newkey','ec','-pkeyopt','ec_paramgen_curve:P-256','-nodes','-days','365','-subj','/CN=InfraPro private environment','-addext','basicConstraints=critical,CA:TRUE,pathlen:0','-addext','keyUsage=critical,keyCertSign,cRLSign','-keyout','ca.key','-out','ca.pem']);
  for (const {role:name,id,directory} of people) {
    const dest = join(path,directory,'tls');
    mkdirSync(dest,{recursive:true,mode:0o700});
    openssl(['req','-new','-newkey','ec','-pkeyopt','ec_paramgen_curve:P-256','-nodes','-keyout',join(dest,name+'.key'),'-out','request.csr','-subj','/CN='+name]);
    const names=[...new Set([name,id])].map(n=>'DNS:'+n+'.infrapro.internal');
    const purposes=['display-runtime','diagnostics-observer'].includes(name)?'clientAuth':'serverAuth,clientAuth';
    writeFileSync(join(pki,'extensions'), 'subjectAltName='+names.join(',')+'\nextendedKeyUsage='+purposes+'\nkeyUsage=critical,digitalSignature\nbasicConstraints=critical,CA:FALSE\n',{mode:0o600});
    openssl(['x509','-req','-days','90','-CA','ca.pem','-CAkey','ca.key','-set_serial','0x'+randomBytes(16).toString('hex'),'-in','request.csr','-out',join(dest,name+'.pem'),'-extfile','extensions']);
    chmodSync(join(dest,name+'.key'),0o600);
    copyFileSync(join(pki,'ca.pem'),join(dest,'ca.pem'));
  }
  rmSync(join(pki,'request.csr')); rmSync(join(pki,'extensions'));
}
function objectPolicy(objects) {
  const actions={r:'s3:GetObject',w:'s3:PutObject',d:'s3:DeleteObject'};
  const Statement=[], buckets=new Set();
  for(const [scope,rights] of Object.entries(objects)) {
    const [bucket,prefix]=scope.split(':');
    const arn='arn:aws:s3:::REPLACE_'+bucket.toUpperCase()+'_BUCKET';
    buckets.add(arn);
    Statement.push({Effect:'Allow',Action:[...rights].map(k=>actions[k]),Resource:[arn+'/'+prefix+'*']});
    Statement.push({Effect:'Allow',Action:['s3:ListBucket'],Resource:[arn],Condition:{StringLike:{'s3:prefix':[prefix+'*']}}});
  }
  if(buckets.size)Statement.push({Effect:'Allow',Action:['s3:GetBucketLocation'],Resource:[...buckets]});
  return {Version:'2012-10-17',Statement};
}

function unit(instance) {
  const {role,id,label,systemUser}=instance;
  const info=inventory.roles[role];
  const args=role==='files-checker' ? ' --directory /var/lib/infrapro-files --socket /var/lib/infrapro-files/checker.sock' : '';
  const dir='/etc/infrapro/'+instance.directory;
  const fileShared=['files-worker','files-checker','transfer-api','control-api'].includes(role);
  const envs=[
    'Environment=INFRAPRO_CONFIG='+dir+'/runtime.yaml',
    'Environment=INFRAPRO_ARCHITECTURE_FILE='+dir+'/architecture.json',
    'Environment=INFRAPRO_DIAGNOSTICS_REPORT_SHARED_GROUP=true',
    'Environment=INFRAPRO_DIAGNOSTICS_REPORT_FILE=/run/infrapro-'+label+'/process.json',
    'Environment=INFRAPRO_DIAGNOSTICS_REPLICA_ID='+id,
    'Environment=INFRAPRO_REPLICA_ID='+id,
    'Environment=INFRAPRO_DIAGNOSTICS_INSTANCE_ID=infrapro-architecture',
    'Environment=INFRAPRO_DIAGNOSTICS_NODE_ID='+instance.diagnostic_node,
    ...Object.entries(processEnvironment(instance,{},true)).map(([k,v])=>'Environment='+k+'='+v),
    'EnvironmentFile=-'+dir+'/runtime.env',
    'EnvironmentFile=-'+dir+'/database.env',
    ...(instance.goMemoryMiB ? ['EnvironmentFile='+dir+'/process-limits.env'] : []),
  ];
  if(instance.public_address)envs.push('Environment='+publicVars[role]+'='+instance.public_address);
  if(role==='files-checker') envs.splice(0,2);
  return `[Unit]
Description=InfraPro ${role}
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=${systemUser}
Group=${fileShared?'infrapro-files':systemUser}
${envs.join('\n')}
ExecStart=/opt/infrapro/current/bin/${info.binary}${args}
RuntimeDirectory=infrapro-${label}
RuntimeDirectoryMode=0750
StateDirectory=infrapro-${fileShared?'files':label}
StateDirectoryMode=0770
TemporaryFileSystem=/tmp:rw,size=${instance.tempMiB}M,mode=1777
ProtectSystem=strict
ProtectHome=true
NoNewPrivileges=true
CapabilityBoundingSet=
PrivateDevices=true
UMask=0007
CPUQuota=${(instance.cpu*100).toFixed(2)}%
MemoryMax=${instance.memoryMiB}M
TasksMax=128
TimeoutStopSec=${info.stopSeconds}
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
`;
}

function compose(instances,mode) {
  const services={};
  for(const instance of instances) {
    const {role,label,id}=instance;
    const spec=inventory.roles[role];
    const configDir='/run/infrapro';
    const report='/run/infrapro-report';
    const vars={INFRAPRO_REPLICA_ID:id,INFRAPRO_DIAGNOSTICS_PID_NAMESPACE:'container',INFRAPRO_DIAGNOSTICS_REPORT_SHARED_GROUP:'true',INFRAPRO_CONFIG:configDir+'/runtime.yaml',INFRAPRO_ARCHITECTURE_FILE:configDir+'/architecture.json',INFRAPRO_DIAGNOSTICS_INSTANCE_ID:'${INFRAPRO_INSTANCE:-infrapro-architecture}',INFRAPRO_DIAGNOSTICS_NODE_ID:'${INFRAPRO_NODE:-'+instance.diagnostic_node+'}',INFRAPRO_DIAGNOSTICS_REPLICA_ID:id,INFRAPRO_DIAGNOSTICS_REPORT_FILE:report+'/process.json',...processEnvironment(instance,{},true)};
    const volumes=[{type:'bind',source:'./'+instance.directory,target:configDir,read_only:true,bind:{create_host_path:false}}, {type:'bind',source:'./reports/'+label,target:report,bind:{create_host_path:false}}];
    let image='${INFRAPRO_API_IMAGE:-infrapro-api:architecture}';
    if(role==='parser')image='${INFRAPRO_PARSER_IMAGE:-infrapro-parser:architecture}';
    if(role.startsWith('display'))image='${INFRAPRO_DISPLAY_IMAGE:-infrapro-display:architecture}';
    if(role==='spatial-worker')image='${INFRAPRO_SPATIAL_IMAGE:-infrapro-spatial:architecture}';
    const service={image,user:'${INFRAPRO_SERVICE_UID:-10001}:${INFRAPRO_SERVICE_GID:-10001}',entrypoint:['/usr/local/bin/'+spec.binary],environment:vars,env_file:['runtime.env','database.env'].map(f=>({path:'./'+instance.directory+'/'+f,required:false})),volumes,read_only:true,tmpfs:['/tmp:size='+instance.tempMiB+'m,mode=1777'],cap_drop:['ALL'],security_opt:['no-new-privileges:true'],pids_limit:128,cpus:Number(instance.cpu.toFixed(6)),mem_limit:instance.memoryMiB+'m',stop_grace_period:spec.stopSeconds+'s',restart:'unless-stopped',labels:{'io.infrapro.instance':'${INFRAPRO_INSTANCE:-infrapro-architecture}','io.infrapro.replica':id}};
    service.container_name='${COMPOSE_PROJECT_NAME:-infrapro-architecture}-'+id;
    if (role==='compat-api' || spec.mode==='compat') service.profiles=['rollback'];
    else if (!['api','identity-api','control-api','monitoring-query-api','transfer-api'].includes(role)) service.profiles=[role,'workers'];
    if (role==='api') vars.INFRAPRO_HTTP_ADDR='0.0.0.0:8080';
    if (role==='parser'){
      service.entrypoint=['/parser'];delete vars.INFRAPRO_CONFIG;delete vars.INFRAPRO_ARCHITECTURE_FILE;
    }
    if(instance.public_address)vars[publicVars[role]]=instance.public_address;
    if (role==='files-checker') {service.network_mode='none'; service.command=['--directory','/staging','--socket','/staging/checker.sock'];delete vars.INFRAPRO_CONFIG;delete vars.INFRAPRO_ARCHITECTURE_FILE;}
    if (['files-worker','files-checker','control-api','transfer-api'].includes(role)) volumes.push({type:'bind',source:'./staging',target:'/staging',bind:{create_host_path:false}});
    if (mode==='mixed' && !service.network_mode) service.network_mode='host';
    if(mode==='compose'&&role!==label)service.networks={default:{aliases:[role]}};
    services[label]=service;
  }
  if (mode==='compose') services.web={volumes:[{type:'bind',source:'./nginx.conf',target:'/etc/nginx/conf.d/default.conf',read_only:true,bind:{create_host_path:false}}],image:'${INFRAPRO_WEB_IMAGE:-infrapro-web:architecture}',ports:['${HTTP_BIND:-127.0.0.1}:${HTTP_PORT:-8088}:80'],restart:'unless-stopped',depends_on:instances.filter(i=>i.role==='api').map(i=>i.label)};
  if(mode==='compose'){
    for(let n=1;n<=3;n++){
      const name='broker-'+n,info=inventory.roles['nats-server'];
      services[name]={
        image:'${INFRAPRO_BROKER_IMAGE:-infrapro-broker:architecture}',container_name:'${COMPOSE_PROJECT_NAME:-infrapro-architecture}-'+name,profiles:['broker'],
        user:'${INFRAPRO_SERVICE_UID:-10001}:${INFRAPRO_SERVICE_GID:-10001}',entrypoint:['/usr/local/bin/nats-server','-c','/etc/infrapro/broker/nats.conf'],
        volumes:[{type:'bind',source:'./roles/'+name,target:'/etc/infrapro/broker',read_only:true,bind:{create_host_path:false}},
          {type:'bind',source:'./jetstream/'+name,target:'/var/lib/infrapro/jetstream',bind:{create_host_path:false}}],
        labels:{'io.infrapro.instance':'${INFRAPRO_INSTANCE:-infrapro-architecture}','io.infrapro.replica':'nats-server-'+n},
        networks:{default:{aliases:[name+'.internal']}},read_only:true,cap_drop:['ALL'],security_opt:['no-new-privileges:true'],
        healthcheck:{test:['CMD','/usr/local/bin/event-relay','broker-health'],interval:'10s',timeout:'4s',retries:3},
        cpus:info.cpu,mem_limit:info.memoryMiB+'m',pids_limit:128,stop_grace_period:info.stopSeconds+'s',restart:'unless-stopped',
      };
    }
  }
  return {name:'infrapro-architecture',services};
}

export function prepare(options) {
  const {mode='local',directory}=options;
  if (!directory || !['local','native','compose','mixed'].includes(mode)) throw Error('prepare requires --directory and --mode local|native|compose|mixed');
  const dest=resolve(directory);
  if(existsSync(dest)&&readdirSync(dest).length)throw Error('Use an empty destination to retain existing certificates and credentials.');
  mkdirSync(dirname(dest),{recursive:true,mode:0o700});
  const stage=mkdtempSync(join(dirname(dest),'.infrapro-layout-'));
  try{
    render(options,mode,stage);
    if(existsSync(dest))rmdirSync(dest);
    renameSync(stage,dest);return dest;
  }finally{rmSync(stage,{recursive:true,force:true});}
}

function render(options,mode,path){
  const host=options.host;
  const port=Number(options['port-base'] ?? 18441);
  if (!Number.isInteger(port)||port<1||port>65535-Object.keys(inventory.privatePorts).length) throw Error('Invalid private port base');
  const mixed=new Set((options['container-roles']??'monitoring-history-worker,monitoring-read-worker').split(','));
  if (mode==='mixed' && [...mixed].some(k=>!active.includes(k)&&k!=='parser')) throw Error('Unknown mixed container role');
  const loadOption=key=>options[key]?JSON.parse(readFileSync(resolve(options[key]))):{};
  const maxInflight=loadOption('max-inflight');
  const {endpoints,instances,slots,proxies,publicEndpoints}=replicaLayout({mode,host,port,replicas:loadOption('replicas'),maxInflight,publicListeners:loadOption('public-listeners'),containerRoles:mixed});
  const processResources=processProfiles(instances,loadOption('process-budgets'));
  renderOriginalProcessLimits(path,processResources,save);
  if(options.network&&!/^[a-zA-Z0-9][a-zA-Z0-9_.-]*$/.test(options.network))throw Error('Invalid external Docker network');
  const pg_connections={},max_inflight={};
  for(const [name,v] of Object.entries(inventory.roles)){if(v.pgConnections)pg_connections[name]=v.pgConnections;max_inflight[name]=maxInflight[name]??v.maxInflight;}
  const base={endpoints,pg_connections,max_inflight,replica_slots:slots,split_display:true};
  const ingestion=options.ingestion?JSON.parse(readFileSync(resolve(options.ingestion))):undefined;
  mkdirSync(path,{recursive:true,mode:0o700});
  if(mode==='local' || options['local-ca']) certs(path,privateRoles.flatMap(role=>instances[role]??[{role,id:role,directory:'roles/'+role}]));
  const processes=[...active,'parser'].flatMap(role=>instances[role]);
  renderRelaySlots(path,instances['ingest-relay'],options['relay-configs'],save);
  for(const instance of processes){
    const {role,id,label}=instance;
    const mount=join(path,instance.directory),container=mode==='compose'||(mode==='mixed'&&mixed.has(role));
    if(instance.goMemoryMiB)
      save(join(mount,'process-limits.env'),Object.entries(processEnvironment(instance)).map(([k,v])=>k+'='+v).join('\n')+'\n',0o644);
    if(role==='parser'){
      const addr=instance.public_address??(container?'0.0.0.0:8091':'127.0.0.1:8091');
      save(join(mount,'runtime.env'),['INFRAPRO_PARSER_ADDR='+addr,'INFRAPRO_PARSER_CONCURRENCY='+instance.maxInflight,'INFRAPRO_PARSER_PACKAGE='+(container?'/opt/parser/release':'/opt/infrapro/current/parser-package'),'INFRAPRO_PARSER_TOKEN_FILE='+(container?'/run/infrapro':'/etc/infrapro/'+instance.directory)+'/parser-token',''].join('\n'));
    }else save(join(mount,'architecture.json'),{...base,replica_id:id,listen_address:instance.listen_address,cert_directory:'tls'});
    const relayProfile=ingestion&&ingestionProfile(role,ingestion);
    if(relayProfile)save(join(mount,'ingestion.json'),relayProfile);
    const analytics=clickhouseProfile(role,options['clickhouse-database']??'REPLACE_MONITORING_DATABASE');
    if(analytics)save(join(mount,'clickhouse.sql'),analytics.sql);
    if(Object.keys(inventory.roles[role].objects).length) save(join(mount,'objects.iam.json'),objectPolicy(inventory.roles[role].objects));
    if(mode!=='local'){
      // Deployment supplies this role's credentials. The example only contains safe defaults.
      const config={shutdown_timeout:inventory.roles[role].stopSeconds-5+'s'};
      if(relayProfile)config.ingestion_file=(container?'/run/infrapro':'/etc/infrapro/'+instance.directory)+'/ingestion.json';
      if(role==='api')config.http_addr=container?'0.0.0.0:8080':'127.0.0.1:8080';
      if(['files-worker','control-api','transfer-api'].includes(role))config.files={temp_directory:container?'/staging':'/var/lib/infrapro-files',checker_socket:container?'/staging/checker.sock':'/var/lib/infrapro-files/checker.sock'};
      save(join(mount,'runtime.yaml'),config);
    }
    if(!container)save(join(path,'units','infrapro-'+label+'.service'),unit(instance),0o644);
    if(container)mkdirSync(join(path,'reports',label),{recursive:true,mode:0o750});
  }
  if(proxies.length){
    save(join(path,'replica-proxy.json'),proxies,0o644);
    save(join(path,'units/infrapro-replica-proxy.service'),readFileSync(join(root,'deploy/architecture/infrapro-replica-proxy.service'),'utf8'),0o644);
  }
  if(mode==='compose'||mode==='mixed'){
    mkdirSync(join(path,'staging'),{recursive:true,mode:0o770});
    const manifest=compose(processes.filter(i=>mode==='compose'||mixed.has(i.role)),mode);
    if(options.network)manifest.networks={default:{external:true,name:options.network}};
    save(join(path,'compose.json'),manifest);
    if(mode==='compose'){
      let proxy=readFileSync(join(root,'deploy/display/nginx.conf'),'utf8').replace('/opt/infrapro/web','/usr/share/nginx/html');
      const upstreams=[['api',8080],['display-runtime',5180],['display-assets',5181]];
      const zones=[];
      for(const [name,upstreamPort] of upstreams){
        const zone='ip_'+name.replaceAll('-','_');
        zones.push('upstream '+zone+' { zone '+zone+' 64k; server '+name+':'+publicEndpoints[name].port+' resolve; }');
        proxy=proxy.replaceAll('http://127.0.0.1:'+upstreamPort,'http://'+zone);
      }
      save(join(path,'nginx.conf'),'resolver 127.0.0.11 valid=10s ipv6=off;\n'+zones.join('\n')+'\n'+proxy,0o644);
      for(let n=1;n<=3;n++){
        const name='broker-'+n;
        const profile=JSON.parse(readFileSync(join(root,'deploy/events/broker.example.json')));
        profile.name=name;
        save(join(path,'roles',name,'broker.json'),profile);
        mkdirSync(join(path,'jetstream',name),{recursive:true,mode:0o700});
      }
    }
  }
  if(proxies.length)instances['replica-proxy']=[{id:'replica-proxy-1',role:'replica-proxy',label:'replica-proxy',entrypoint:inventory.roles['replica-proxy'].entrypoint,pgConnections:0,maxInflight:proxies.reduce((sum,p)=>sum+p.max_connections,0)}];
  save(join(path,'layout.json'),{version:inventory.version,mode,resource_limits:'resource-limits.json',process_resources:processResources,private_roles:privateRoles,roles:active,replica_slots:slots,instances,tcp_proxies:proxies,replica_pg_connections:pg_connections,split_connections:Object.entries(inventory.roles).filter(([,v])=>['both','split'].includes(v.mode)&&v.placement==='application').reduce((sum,[,v])=>sum+v.pgConnections,0)});
  writeResourceLimits(path,options);
  return path;
}

function hashes(dir) {
  const files=[];
  for(const entry of readdirSync(dir,{recursive:true,withFileTypes:true})){
    if(entry.isDirectory())continue;
    if(!entry.isFile())throw Error('Unsupported release member type: '+entry.name);
    files.push(join(entry.parentPath,entry.name));
  }
  return files.sort().map(p=>({path:relative(dir,p),bytes:statSync(p).size,sha256:createHash('sha256').update(readFileSync(p)).digest('hex')}));
}

function projectFiles(){
  return execFileSync('git',['ls-files','-z','--cached','--others','--exclude-standard'],{cwd:root,encoding:'utf8'}).split('\0').filter(Boolean).sort();
}
function sourceDigest(excluded=[],files=projectFiles()) {
  const tree=createHash('sha256');
  for(const name of files){
    const file=join(root,name);
    if(excluded.some(dir=>file===dir||file.startsWith(dir+'/')))continue;
    const entry=lstatSync(file,{throwIfNoEntry:false});
    const content=entry?.isSymbolicLink()?readlinkSync(file):entry&&readFileSync(file);
    const kind=!entry?'deleted':entry.isSymbolicLink()?'link':entry.mode&0o111?'executable':'file';
    tree.update(name).update('\0').update(kind).update('\0').update(content===undefined?'':createHash('sha256').update(content).digest('hex')).update('\0');
  }
  return tree.digest('hex');
}

function provenance() {
  const git=args=>execFileSync('git',args,{cwd:root,encoding:'utf8'}).trim();
  return {commit:git(['rev-parse','HEAD']),working_tree_modified:git(['status','--porcelain']).length>0};
}

function freezeSources(directory) {
  const files=projectFiles(),original={...provenance(),files,sha256:sourceDigest([],files)};
  for(const name of files){
    const from=join(root,name),to=join(directory,name),info=lstatSync(from,{throwIfNoEntry:false});
    if(!info)continue; // A retained deletion contributes to the source digest.
    mkdirSync(dirname(to),{recursive:true});
    if(info.isFile())copyFileSync(from,to);
    else if(info.isSymbolicLink()){
      const link=readlinkSync(from);
      if(!resolve(dirname(to),link).startsWith(directory+'/'))throw Error('Source symlink escapes the build directory: '+name);
      symlinkSync(link,to);
    }else throw Error('Unsupported build source: '+name);
  }
  return original;
}

function copyRetainedPackage(source,dest){
  const packageRoot=realpathSync(source);
  const names=new Set(['catalog.json']),catalog=JSON.parse(readFileSync(join(source,'catalog.json')));
  const hex=digest=>{if(!/^[0-9a-f]{64}$/.test(digest))throw Error('Invalid retained package digest');return digest;};
  for(const entry of catalog.entries){
    names.add('artifacts/'+hex(entry.artifact_digest));
    for(const member of ['manifest.json','outputs.json']){
      const name='manifests/'+hex(entry.manifest_digest)+'/'+member;
      if(existsSync(join(source,name)))names.add(name);
    }
  }
  if(existsSync(join(source,'ingestion-capabilities.json'))){
    names.add('ingestion-capabilities.json');
    const capabilities=JSON.parse(readFileSync(join(source,'ingestion-capabilities.json')));
    for(const entry of capabilities.identification_methods)names.add('artifacts/'+hex(entry.method.artifact_digest));
  }
  for(const name of names){
    const from=join(source,name),to=join(dest,name);
    if(!realpathSync(from).startsWith(packageRoot+'/')||!lstatSync(from).isFile())throw Error('Invalid retained package member '+name);
    mkdirSync(dirname(to),{recursive:true});copyFileSync(from,to);
  }
}

function parserTargets(directory,goos,goarch){
  const artifacts=join(directory,'parser-package/artifacts'),target=goos+'/'+goarch;
  return readdirSync(artifacts).sort().map(hash=>{
    const path=join(artifacts,hash),bytes=readFileSync(path);
    if(!lstatSync(path).isFile()||!(statSync(path).mode&0o111)||createHash('sha256').update(bytes).digest('hex')!==hash)throw Error('Unusable parser artifact '+hash);
    let compatible=false,format;
    if(bytes.subarray(0,4).toString('hex')==='7f454c46'){
      format='ELF';compatible=bytes[4]===2&&bytes[5]===1&&[0,3].includes(bytes[7])&&goos==='linux'&&bytes.readUInt16LE(18)===({amd64:62,arm64:183})[goarch];
    }else if(bytes.subarray(0,4).toString('hex')==='cffaedfe'){
      format='Mach-O';compatible=goos==='darwin'&&bytes.readUInt32LE(4)===({amd64:0x01000007,arm64:0x0100000c})[goarch];
    }else if(['cafebabe','cafebabf'].includes(bytes.subarray(0,4).toString('hex'))){
      format='Mach-O universal';
      const count=bytes.readUInt32BE(4),size=bytes[3]===0xbf?32:20;
      const cpu=({amd64:0x01000007,arm64:0x0100000c})[goarch];
      if(goos==='darwin'&&8+count*size<=bytes.length)for(let i=0;i<count;i++)compatible||=bytes.readUInt32BE(8+i*size)===cpu;
    }else if(bytes.subarray(0,2).toString()==='#!'){
      format='interpreter';compatible=true;
    }
    if(!compatible)throw Error('Parser artifact '+hash+' is incompatible with '+target);
    return {sha256:hash,format,target};
  });
}

function release(o, frozen) {
  if(!o.directory)throw Error('release requires --directory');
  const dest=resolve(o.directory);
  if(existsSync(dest)&&readdirSync(dest).length)throw Error('Release destination must be empty');
  mkdirSync(dirname(dest),{recursive:true});
  const stage=mkdtempSync(join(dirname(dest),'.infrapro-release-'));
  try {
    const manifest=assembleRelease(stage,o,frozen);
    if(frozen && manifest.source_tree_sha256!==frozen.sha256)throw Error('Sources changed while building the release');
    renameSync(stage,dest);
    return manifest;
  } finally {
    rmSync(stage,{recursive:true,force:true});
  }
}

function assembleRelease(dest,o,frozen) {
  const binaries=resolve(o.bin??join(root,'build/bin'));
  mkdirSync(join(dest,'bin'),{recursive:true});
  for(const name of new Set(Object.values(inventory.roles).filter(v=>!v.entrypoint).map(v=>v.binary).concat(['ingestion-operator','parsing-coverage','parsing-recovery','parser-package']))){
    const source=name==='nats-server'?o.broker:join(binaries,name);
    if(!source||!existsSync(source))throw Error('Missing release binary '+name);
    copyFileSync(source,join(dest,'bin',name));
  }
  for(const app of ['console','display-runtime','diagnostics'])cpSync(join(root,'apps',app,'dist'),join(dest,'web',app),{recursive:true});
  const files=frozen?.files??projectFiles();
  // Operators may store private recovery inputs next to the templates. Only
  // reviewed files explicitly listed in this release are eligible for export.
  const resources=JSON.parse(readFileSync(join(root,'deploy/architecture/offline-members.json')));
  if(resources.version!==1)throw Error('Unknown offline resource inventory version');
  for(const [directory,members] of Object.entries(resources.directories)){
    for(const member of members){
      const name=directory+'/'+member,from=join(root,name),to=join(dest,name);
      if(!files.includes(name)||realpathSync(from)!==join(realpathSync(root),name)||!lstatSync(from).isFile())
        throw Error('Invalid declared offline resource '+name);
      mkdirSync(dirname(to),{recursive:true});
      copyFileSync(from,to);
    }
  }
  if(o['parser-package'])copyRetainedPackage(resolve(o['parser-package']),join(dest,'parser-package'));
  // parser-package is a build-host utility. It reads the *target* runner bytes;
  // using go run lets a Darwin build host package a Linux runner offline.
  const hostEnv={...process.env,GOWORK:join(root,'go.work'),GOPROXY:'off',GOTOOLCHAIN:'local'};delete hostEnv.GOOS;delete hostEnv.GOARCH;
  execFileSync('go',['run','./services/parser/cmd/parser-package',join(dest,'bin/parser-runner'),join(root,'samples/parsing'),join(dest,'parser-package')],{cwd:root,env:hostEnv,stdio:['ignore','pipe','pipe']});
  for(const f of ['SHA256SUMS','broker.json']){
    const from=join(dirname(resolve(o.broker)),f);
    if(!existsSync(from))throw Error('Broker provenance missing: '+f);
    copyFileSync(from,join(dest,'deploy/events',f));
  }
  const priorDigest=readFileSync(join(dest,'deploy/events/SHA256SUMS'),'utf8').trim().split(/\s+/)[0];
  if(priorDigest!==createHash('sha256').update(readFileSync(join(dest,'bin/nats-server'))).digest('hex'))throw Error('Offline broker checksum mismatch');
  const buildInfo=execFileSync('go',['version','-m',join(dest,'bin','api')],{encoding:'utf8'});
  const platform=name=>buildInfo.match(new RegExp('build\\s+'+name+'=(\\w+)'))?.[1];
  const {commit,working_tree_modified}=frozen??provenance();
  const releaseInfo={version:1,commit,working_tree_modified,source_digest_version:2,source_tree_sha256:sourceDigest([dest],files),goos:platform('GOOS'),goarch:platform('GOARCH'),schema:Number(readFileSync(join(root,'packages/go/platform/migrations/migrations.go'),'utf8').match(/const Version int64 = (\d+)/)[1]),files:hashes(dest)};
  releaseInfo.application_source_sha256=applicationSourceDigest(root,files);
  if(releaseInfo.files.length===0||!releaseInfo.goos||!releaseInfo.goarch)throw Error('Unidentified build');
  releaseInfo.entrypoints=Object.fromEntries(Object.entries(inventory.roles).map(([role,v])=>[role,v.entrypoint??'bin/'+v.binary]));
  for(const path of Object.values(releaseInfo.entrypoints))if(!lstatSync(join(dest,path)).isFile())throw Error('Missing release entrypoint '+path);
  releaseInfo.parser_artifacts=parserTargets(dest,releaseInfo.goos,releaseInfo.goarch);
  for(const name of readdirSync(join(dest,'bin'))){
    const info=execFileSync('go',['version','-m',join(dest,'bin',name)],{encoding:'utf8'});
    if(name==='infrapro-diagnostics'&&!info.includes('-tags=diagnostics_release'))throw Error('Diagnostics release must embed the built UI');
    if(!info.includes('GOOS='+releaseInfo.goos)||!info.includes('GOARCH='+releaseInfo.goarch))throw Error('Incompatible binary '+name);
  }
  save(join(dest,'release.json'),releaseInfo,0o644);
  return releaseInfo;
}

async function buildRelease(options) {
  if(!options.directory || !/^(linux|darwin)\/(amd64|arm64)$/.test(options.target??''))throw Error('Choose --directory and --target linux/amd64, linux/arm64, darwin/amd64 or darwin/arm64');
  if(existsSync(options.directory)&&readdirSync(options.directory).length)throw Error('Release destination must be empty');
  const output={...options,directory:resolve(options.directory)};
  if(options['parser-package'])output['parser-package']=realpathSync(options['parser-package']);
  const checkout=mkdtempSync(join(tmpdir(),'infrapro-release-source-'));
  try{
    const frozen=freezeSources(checkout);
    const builder=await import(pathToFileURL(join(checkout,'scripts/architecture-deploy.mjs')));
    return builder.buildFrozenRelease(output,frozen);
  }finally{rmSync(checkout,{recursive:true,force:true});}
}

// Executed from the captured plain source directory, so pnpm-generated assets
// and the Go workspace refer to the same bytes even during concurrent edits.
export function buildFrozenRelease(options,frozen) {
  if(sourceDigest([],frozen.files)!==frozen.sha256)throw Error('Sources changed while capturing the offline build');
  const buildEnv={...process.env,GOWORK:join(root,'go.work'),GOPROXY:'off',GOTOOLCHAIN:'local'};
  const [goos,goarch]=options.target.split('/'),env={...buildEnv,GOOS:goos,GOARCH:goarch,CGO_ENABLED:'0'};
  const run=(program,args,environment=process.env)=>execFileSync(program,args,{cwd:root,env:environment,stdio:'inherit'});
  const temp=mkdtempSync(join(tmpdir(),'infrapro-release-build-'));
  try{
    run('pnpm',['install','--offline','--frozen-lockfile']);
    run('pnpm',['build:apps']);
    run(process.execPath,['scripts/diagnostics-build.mjs','--skip-ui','--embed-only']);
    if(sourceDigest([],frozen.files)!==frozen.sha256)throw Error('Sources changed while building the three applications');
    const commands=new Set(Object.values(inventory.roles).filter(r=>!r.entrypoint&&!['external','services/diagnostics'].includes(r.module)).map(r=>'./'+r.module+'/cmd/'+r.binary));
    for(const cli of ['ingestion-operator','parsing-coverage','parsing-recovery'])commands.add('./tools/infrapro/cmd/'+cli);
    commands.add('./services/parser/cmd/parser-package');
    run('go',['build','-trimpath','-ldflags','-s -w','-o',temp+'/',...commands],env);
    const embed=JSON.parse(readFileSync(join(root,'services/diagnostics/internal/web/assets/build.json'))).build_id;
    run('go',['build','-trimpath','-tags','diagnostics_release','-ldflags','-s -w -X github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/web.BuildID='+embed,'-o',join(temp,'infrapro-diagnostics'),'./services/diagnostics/cmd/infrapro-diagnostics'],env);
    run(process.execPath,['scripts/architecture-broker-build.mjs',goos,goarch],buildEnv);
    return release({...options,bin:temp,broker:join(root,'build/events',goos+'-'+goarch,'nats-server')},frozen);
  } finally {rmSync(temp,{recursive:true,force:true});}
}
function verifyRelease(options){
  if(!options.directory || !/^[a-f0-9]{64}$/.test(options['manifest-sha256']??''))throw Error('Supply release directory and its trusted manifest SHA-256');
  const directory=resolve(options.directory),raw=readFileSync(join(directory,'release.json'));
  if(createHash('sha256').update(raw).digest('hex')!==options['manifest-sha256'])throw Error('Release manifest checksum mismatch');
  const manifest=JSON.parse(raw),files=hashes(directory).filter(f=>f.path!=='release.json');
  if(manifest.version!==1||JSON.stringify(files)!==JSON.stringify(manifest.files))throw Error('Release members or checksums differ');
  for(const file of readdirSync(join(directory,'bin')))if(!(statSync(join(directory,'bin',file)).mode&0o111))throw Error('Release binary is not executable: '+file);
  const targets=parserTargets(directory,manifest.goos,manifest.goarch);
  if(manifest.parser_artifacts&&JSON.stringify(targets)!==JSON.stringify(manifest.parser_artifacts))throw Error('Parser artifact target differs from the manifest');
  return {files:files.length,goos:manifest.goos,goarch:manifest.goarch,schema:manifest.schema,source_tree_sha256:manifest.source_tree_sha256};
}
if(process.argv[1] && resolve(process.argv[1])===fileURLToPath(import.meta.url)){
  const command=process.argv[2],{values}=parseArgs({args:process.argv.slice(3),options:{mode:{type:'string'},directory:{type:'string'},'port-base':{type:'string'},host:{type:'string'},'local-ca':{type:'boolean'},'container-roles':{type:'string'},replicas:{type:'string'},'max-inflight':{type:'string'},'process-budgets':{type:'string'},'runtime-config':{type:'string'},'reports-directory':{type:'string'},'report-map':{type:'string'},'relay-configs':{type:'string'},'public-listeners':{type:'string'},bin:{type:'string'},ingestion:{type:'string'},'clickhouse-database':{type:'string'},broker:{type:'string'},network:{type:'string'},target:{type:'string'},'manifest-sha256':{type:'string'},'parser-package':{type:'string'}}});
  if(command==='prepare')console.log(prepare(values));
  else if(command==='resource-limits'){
    if(!values.directory)throw Error('resource-limits requires --directory');
    const result=writeResourceLimits(resolve(values.directory),{...values,environment:process.env});
    console.log(JSON.stringify({slots:result.slots.length,checked_reports:result.checked_reports,totals:result.totals}));
  }
  else if(command==='release'||command==='build-release'){
    const manifest=command==='release'?release(values):await buildRelease(values);
    console.log('Verified '+manifest.files.length+' offline files for '+manifest.goos+'/'+manifest.goarch);
    console.log('Manifest SHA-256 '+createHash('sha256').update(readFileSync(join(resolve(values.directory),'release.json'))).digest('hex'));
  }
  else if(command==='verify-release')console.log(JSON.stringify(verifyRelease(values)));
  else throw Error('Choose prepare, resource-limits, release, build-release or verify-release');
}
