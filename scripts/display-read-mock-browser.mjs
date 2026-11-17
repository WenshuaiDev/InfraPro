import {chromium,expect} from '@playwright/test';
import {readFileSync,mkdirSync,writeFileSync} from 'node:fs';
import {startServer} from '../packages/ts/display-devtools/src/server.mjs';
const {directory}=JSON.parse(readFileSync('.artifacts/display/monitoring-read-external.json','utf8'));
const out='.artifacts/display/monitoring-read/browser';mkdirSync(out,{recursive:true});
const browser=await chromium.launch({headless:true});const results=[];
try{
 for(const scenario of ['normal','empty','pending','partial','disconnected','revoked','unavailable','overlap','scope-changed']){
  const server=await startServer({dir:directory+'/dist',port:15960,scenario}),page=await browser.newPage({viewport:{width:1440,height:1000}}),errors=[];
  page.on('pageerror',e=>errors.push(e.message));
  try{await page.goto(server.url);const app=page.frameLocator('iframe');if(['disconnected','revoked'].includes(scenario)){await expect(page.locator('#status')).toContainText('模拟数据 · 运行已停止');await expect(page.locator('#status')).toContainText(scenario==='revoked'?'session_revoked':'dependency_unavailable');await expect(page.locator('iframe')).toHaveCount(0);}else{await expect(page.locator('#status')).toHaveText('模拟数据 · 本地调试',{timeout:20000});await expect(app.locator('#simulation')).toBeVisible();}
   if(!['scope-changed','disconnected','revoked','unavailable'].includes(scenario)){await expect(app.getByText('原始累计正式观测',{exact:true})).toBeVisible();await expect(app.locator('.error')).toHaveCount(0);}
   if(scenario==='unavailable')await expect(app.locator('.error').first()).toContainText('capability_unavailable');
   if(scenario==='normal'){
    for(const label of ['设施树与地图','设备与来源','指定指标与时间','图纸与模型','状态与诊断']){await app.getByRole('button',{name:label,exact:true}).click();await expect(app.locator('section')).not.toHaveCount(0);if(label==='指定指标与时间'){await app.getByRole('button',{name:'读取所选指标'}).click();await expect(app.getByText('9007199254740993.000000001',{exact:true})).toBeVisible();}await expect(app.locator('.error')).toHaveCount(0);}
    await page.setViewportSize({width:390,height:844});await app.getByRole('button',{name:'项目统计',exact:true}).click();await expect(app.getByText('原始累计正式观测',{exact:true})).toBeVisible();const frame=page.frames().find(f=>f.parentFrame());if(await frame.evaluate(()=>document.documentElement.scrollWidth>innerWidth))throw Error('Mobile overflow');
   }
   await page.screenshot({path:`${out}/${scenario}.png`,fullPage:true});if(errors.length)throw Error(errors.join(';'));results.push({scenario,passed:true});
  }catch(e){await page.screenshot({path:`${out}/failure-${scenario}.png`,fullPage:true});throw e;}finally{await page.close();await server.close();}
 }
 writeFileSync(`${out}/mock-result.json`,JSON.stringify({passed:true,results},null,2));
}finally{await browser.close();}
