import {expect} from '@playwright/test';

// Delay the first notification's state response until the second detail has
// loaded. The old mutation must update only its original cache entry.
export async function checkNotificationDetailSwitch(page,call,first,second) {
 let release,started,finished;
 const gate=new Promise(resolve=>{release=resolve;}),sent=new Promise(resolve=>{started=resolve;}),settled=new Promise(resolve=>{finished=resolve;});
 const pattern='**/api/v1/me/notifications/'+first.id;
 await expect(page.getByRole('button',{name:'恢复未读',exact:true})).toBeEnabled();
 await page.route(pattern,async route=>{if(route.request().method()!=='PATCH')return route.continue();try{const response=await route.fetch();started();await gate;await route.fulfill({response});}finally{finished();}});
 try{
  await page.getByRole('button',{name:'恢复未读',exact:true}).click();await sent;
  await page.getByRole('button',{name:/^通知，/}).click();await page.getByRole('dialog').locator(`[data-notification-id="${second.id}"] button`).click();await page.getByRole('dialog').getByRole('link',{name:'在通知中心查看'}).click();
  await page.waitForURL(v=>v.pathname.endsWith('/notifications/'+second.id));
  await expect.poll(async()=>(await call('/me/notifications/'+second.id)).read_at).not.toBeNull();
  release();await settled;await expect(page.getByRole('heading',{name:second.object_name||second.title,exact:true})).toBeVisible();
  await expect(page.getByRole('button',{name:'恢复未读',exact:true})).toBeEnabled();
 }finally{release();await page.unroute(pattern);}
}

export async function checkNotificationTargetSwitch(page,first,second,origin) {
 await page.goto(origin+'/notifications/'+first.id+'?category=tasks');
 const detail=page.getByRole('region',{name:'通知详情'});
 await expect(detail.getByRole('button',{name:'打开业务页面',exact:true})).toBeEnabled();
 let release,started=false,settled=false;
 const gate=new Promise(resolve=>{release=resolve;});
 const pattern='**/api/v1/me/notifications/'+first.id+'/target';
 await page.route(pattern,async route=>{const response=await route.fetch();started=true;await gate;try{await route.fulfill({response});}finally{settled=true;}});
 try {
  await detail.getByRole('button',{name:'打开业务页面',exact:true}).click();
  await expect.poll(()=>started).toBe(true);
  await page.getByRole('region',{name:'通知列表',exact:true}).locator(`[data-notification-id="${second.id}"] a`).click();
  await page.waitForURL(v=>v.pathname==='/notifications/'+second.id);
  release();await expect.poll(()=>settled).toBe(true);
  await expect(detail.getByRole('heading',{name:second.object_name||second.title,exact:true})).toBeVisible();
  await expect(page).toHaveURL(new RegExp('/notifications/'+second.id+'\\?category=tasks$'));
 }finally{release();await page.unroute(pattern);}
}
