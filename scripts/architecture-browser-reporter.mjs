import { appendFileSync,writeFileSync } from 'node:fs';
import { join } from 'node:path';

// The existing browser suite uses one worker because it mutates platform-wide
// revisions. Its bounded test intervals identify which public requests were
// actually observed while a particular browser assertion ran.
export default class ArchitectureBrowserReporter {
  onBegin(_config,suite){
    const inventory=process.env.ARC_BROWSER_INVENTORY_FILE;
    if(inventory)writeFileSync(inventory,JSON.stringify(suite.allTests().map(test=>({id:test.id,title:test.titlePath().slice(1)}))),{mode:0o600});
  }
  onTestEnd(test,result){
    const dir=process.env.INFRAPRO_ARCH_TEST_RECORD;
    if(!dir)return;
    const record={id:test.id,title:test.titlePath().slice(1),file:test.location.file.split('/tests/e2e/').at(-1),
      status:result.status,expected:test.expectedStatus,start:result.startTime.getTime(),end:result.startTime.getTime()+result.duration};
    appendFileSync(join(dir,'browser-cases.jsonl'),JSON.stringify(record)+'\n',{mode:0o600});
    if(process.env.ARC_BROWSER_OUTCOME_FILE)appendFileSync(process.env.ARC_BROWSER_OUTCOME_FILE,JSON.stringify(record)+'\n',{mode:0o600});
  }
}
