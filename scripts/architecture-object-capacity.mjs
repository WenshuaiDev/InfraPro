import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';

const image = 'chrislusf/seaweedfs:4.53@sha256:ccad23e18639c8b98cee17e7100b95830a06077b56d8e5b006d6dcf8ec5e94a0';
function docker(args) {
  return execFileSync('docker', args, {encoding:'utf8', stdio:['ignore','pipe','pipe']}).trim();
}

// SeaweedFS reserves several volumes for each bucket/collection. Give the
// isolated architecture suite additional writable capacity without recycling
// the existing contract lab or evicting another test's accepted objects.
export function ensureObjectCapacity(id) {
  assert.match(id,/^[a-f0-9]{12}$/);
  const master = JSON.parse(docker(['inspect','infrapro-monitoring-contract-s3-1']))[0];
  assert.equal(master.Config.Labels['com.docker.compose.project'],'infrapro-monitoring-contract');
  const network = Object.keys(master.NetworkSettings.Networks).find(n=>n==='infrapro-monitoring-contract_default');
  assert.ok(network,'isolated monitoring contract network required');
  const name = 'infrapro-arc-volume-'+id;
  const known = docker(['ps','-a','--filter','name=^/'+name+'$','--format','{{.Names}}']);
  if(known){
    const volume=JSON.parse(docker(['inspect',name]))[0];
    assert.equal(volume.Config.Labels['infrapro.architecture'],id);
    if(!volume.State.Running) docker(['start',name]);
  } else {
    docker(['run','-d','--name',name,'--label','infrapro.architecture='+id,'--network',network,'--memory','256m','-e','GOMEMLIMIT=192MiB',image,'volume','-ip='+name,'-dir=/data','-master=s3:9333','-max=256']);
  }
  return name;
}
