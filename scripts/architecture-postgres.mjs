// PostgreSQL role provisioning for an intentionally selected database.
// Every password goes to psql over stdin or the child environment.
import { execFileSync } from 'node:child_process';
import { createHash, randomBytes } from 'node:crypto';
import { existsSync, readFileSync, renameSync, writeFileSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { inventory, root, serviceInstances } from './architecture-runtime.mjs';

export const postgresRoleSQLFiles = ['deploy/architecture/restore-base-grants.sql','deploy/display/package-worker-role.sql','deploy/architecture/postgres-roles.sql','deploy/events/postgres-grants.sql'];

export function provision({url, container, directory, localLogins=false, issueLogins=false, retireSlots=false, roles, sqlDirectory=root}) {
  if (!url) throw Error('INFRAPRO_DATABASE_URL required');
  const target=new URL(url);
  const connection=new URL(target);connection.password='';
  const env={...process.env, PGPASSWORD:decodeURIComponent(target.password)};
  function sql(input) {
    const args=['-X','--no-psqlrc','-v','ON_ERROR_STOP=1','-qAt'];
    try {
      if(container) return execFileSync('docker',['exec','-i','-e','PGPASSWORD',container,'psql','--host','127.0.0.1','--username',decodeURIComponent(target.username),'--dbname',decodeURIComponent(target.pathname.slice(1)),...args],{input,env,encoding:'utf8',stdio:['pipe','pipe','pipe']});
      return execFileSync('psql',['--dbname',connection.toString(),...args],{input,env,encoding:'utf8',stdio:['pipe','pipe','pipe']});
    } catch(e) {
      const reason=e.stderr?.toString().split('\n').find(line=>line.startsWith('ERROR:'))??'connection failed';
      throw Error('PostgreSQL role installation failed: '+reason.replaceAll(/PASSWORD\s+.*$/gi,'password not accepted'),{cause:e});
    }
  }
  const applied = {};
  for (const file of postgresRoleSQLFiles) {
    const body = readFileSync(join(sqlDirectory,file));
    sql(body);
    applied[file] = createHash('sha256').update(body).digest('hex');
  }
  if (!localLogins&&!issueLogins) return applied;
  const marker=join(directory,'layout.json');
  if (!existsSync(marker))throw Error('Role login issuance requires a prepared directory');
  const layout=JSON.parse(readFileSync(marker));
  if (localLogins&&layout.mode!=='local') throw Error('Local logins require an existing local role directory');
  const pending=[];
  const namespace=sql('SELECT current_database()||\'/\'||current_schema()').trim();
  const clusterTag='infrapro-slot-v1:'+createHash('sha256').update(namespace).digest('hex').slice(0,24)+':';
  for (const [role,info] of Object.entries(inventory.roles)) {
    if(!info.sqlRole.startsWith('infrapro_')||!existsSync(join(directory,'roles',role))) continue;
    if(roles&&!roles.includes(role))continue;
    const slots=serviceInstances(role,directory);
    if(slots.reduce((sum,s)=>sum+s.pgConnections,0)!==info.pgConnections)throw Error('Slot connection totals differ for '+role);
    for(const instance of slots){
      if(!/^[a-z][a-z0-9-]{0,62}$/.test(instance.id)||!['roles/'+role,'replicas/'+role+'/'+instance.id].includes(instance.directory))throw Error('Invalid slot identity or directory');
      const mount=join(directory,instance.directory),dest=join(mount,'environment.json');
      const limit=instance.pgConnections;
      if(!Number.isSafeInteger(limit)||limit<1||limit>info.pgConnections)throw Error('Invalid slot connection quota');
      const group=info.sqlRole;
      if (!/^[a-z][a-z0-9_]{0,48}$/.test(group)) throw Error('Invalid group in inventory');
      const existing=existsSync(dest);
      const runtime=existing?JSON.parse(readFileSync(dest)):{};
      const uri=new URL(existing?runtime.INFRAPRO_DATABASE_URL:target);
      if(existing && (uri.host!==target.host || uri.pathname!==target.pathname || uri.search!==target.search)) throw Error('Saved role login belongs to a different target');
      if(!existing){
        const id=createHash('sha256').update(namespace+'/'+role+'/'+instance.id).digest('hex').slice(0,12);
        uri.username='arc_'+role.replaceAll('-','_')+'_'+id;uri.password=randomBytes(32).toString('hex');runtime.INFRAPRO_DATABASE_URL=uri.toString();
      }
      const login=decodeURIComponent(uri.username),password=decodeURIComponent(uri.password);
      if(!/^arc_[a-z_]+_(?:[0-9a-f]{8}|[0-9a-f]{12})$/.test(login)||!/^[a-f0-9]{64}$/.test(password)) throw Error('Invalid issued role credential');
      pending.push({dest,login,role,group,password,runtime,existing,limit,mount,tag:clusterTag+role+':'+instance.id});
    }
  }
  if(!pending.length)return applied;
  const known=new Set(sql("SELECT rolname FROM pg_roles WHERE rolname LIKE 'arc_%'").trim().split('\n'));
  if(pending.some(record=>!record.existing&&known.has(record.login)))throw Error('Database already has a login for the requested slot. Retain its protected credentials.');
  const rolesToReconcile=[...new Set(pending.map(r=>r.role))];
  const reconciliations=rolesToReconcile.map(role=>{
    const tags=pending.filter(r=>r.role===role).map(r=>"'"+r.tag+"'");
    return `DO $$ DECLARE old record; BEGIN
     PERFORM pg_advisory_xact_lock(hashtextextended('${clusterTag}${role}',0));
     FOR old IN SELECT rolname,oid FROM pg_roles
       WHERE rolcanlogin AND shobj_description(oid,'pg_authid') LIKE '${clusterTag}${role}:%'
         AND shobj_description(oid,'pg_authid') NOT IN (${tags.join(',')}) LOOP
       IF NOT ${retireSlots} OR EXISTS(SELECT 1 FROM pg_stat_activity WHERE usesysid=old.oid) THEN
         RAISE EXCEPTION 'Superseded slot still available. Drain its users and use --retire-slots during the cutover';
       END IF;
       EXECUTE format('ALTER ROLE %I NOLOGIN CONNECTION LIMIT 0',old.rolname);
     END LOOP; END $$;`;
  });
  // Write generated credentials with owner-only permissions before committing the
  // database creation transaction. A retry can validate and reuse these logins.
  const installed=[];
  try {
    for (const {dest,runtime,existing} of pending) {if(existing)continue;writeFileSync(dest,JSON.stringify(runtime)+'\n',{mode:0o600,flag:'wx'});installed.push(dest);}
    const statements=pending.map(({login,password,group,limit,existing,tag})=>`DO $$
BEGIN
 IF NOT EXISTS(SELECT FROM pg_roles WHERE rolname='${login}') THEN
  CREATE ROLE ${login} LOGIN CONNECTION LIMIT ${limit} PASSWORD '${password}' IN ROLE ${group};
 ELSIF NOT ${existing} THEN
  RAISE EXCEPTION 'Login already issued for this slot; retain the existing slot credentials';
 ELSIF NOT EXISTS (
  SELECT FROM pg_roles r WHERE r.rolname='${login}' AND (r.rolcanlogin OR shobj_description(r.oid,'pg_authid')='${tag}') AND r.rolinherit
   AND NOT (r.rolsuper OR r.rolcreatedb OR r.rolcreaterole OR r.rolreplication OR r.rolbypassrls)
   AND pg_has_role(r.oid,'${group}','USAGE')
   AND NOT EXISTS(SELECT FROM pg_auth_members m WHERE m.member=r.oid AND (m.roleid<>'${group}'::regrole OR m.admin_option))
 ) THEN
  RAISE EXCEPTION 'Saved login has unexpected role privileges';
 ELSE
  ALTER ROLE ${login} LOGIN CONNECTION LIMIT ${limit};
 END IF;
 COMMENT ON ROLE ${login} IS '${tag}';
END $$;`);
    sql(['BEGIN;',...reconciliations,...statements,'COMMIT;'].join('\n'));
    if(layout.mode!=='local')for(const {mount,runtime} of pending){
      const target=join(mount,'database.env'),tmp=target+'.'+randomBytes(6).toString('hex');
      writeFileSync(tmp,'INFRAPRO_DATABASE_URL='+JSON.stringify(runtime.INFRAPRO_DATABASE_URL)+'\n',{mode:0o600,flag:'wx'});renameSync(tmp,target);
    }
  } catch(e) {
    // Keep files for uncertain commits, allowing the operator to check pg_roles.
    throw Error(e.message+'; protected credentials retained for reconciliation ('+installed.length+' files)',{cause:e});
  }
  return applied;
}
if (process.argv[1]&&resolve(process.argv[1])===fileURLToPath(import.meta.url)){
  const local=process.argv.indexOf('--local-logins'),issue=process.argv.indexOf('--issue-logins'),arg=Math.max(local,issue);
  provision({url:process.env.INFRAPRO_DATABASE_URL,container:process.env.INFRAPRO_PG_TOOLS_CONTAINER,localLogins:local>=0,issueLogins:issue>=0,retireSlots:process.argv.includes('--retire-slots'),directory:arg>=0?resolve(process.argv[arg+1]):''});
  console.log('Database role privileges installed.');
}
