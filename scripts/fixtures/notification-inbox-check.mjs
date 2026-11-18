import {expect} from '@playwright/test';

// The popover and account inbox use the real isolated notification service. None of these checks mock
// counts, permission disclosure, read/archived state, or business navigation.
export async function checkNotificationInbox(page,call,state,origin,checks) {
 const bell=page.getByRole('button',{name:/^通知，/});
 await page.goto(`${origin}/t/${state.tenants[0]}/profile`);
 await expect(bell).toHaveAttribute('aria-label',/^通知，\d+ 条未读$/);
 const before=(await call('/me/notifications/summary')).unread;
 await bell.click();
 const panel=page.getByRole('dialog');
 await expect(panel).toBeVisible();
 const box=await panel.boundingBox();
 if(!box||box.width>460||box.x<700)throw Error('Notification popover is not anchored at the right of the desktop toolbar');
 await expect(page.locator('[data-slot="dialog-overlay"]')).toHaveCount(0);
 const scopedResponse=page.waitForResponse(r=>r.request().method()==='GET'&&new URL(r.url()).pathname==='/api/v1/me/notifications'&&new URL(r.url()).searchParams.get('tenant_id')===state.tenants[0]);
 await panel.getByRole('combobox',{name:'通知来源范围',exact:true}).click();await page.getByRole('option',{name:'当前工作空间',exact:true}).click();await scopedResponse;
 if((await call('/me/notifications/summary')).unread!==before)throw Error('Scope filtering marked notifications read');
 const selected=(await call('/me/notifications?tenant_id='+state.tenants[0]+'&limit=8')).items[0];
 await panel.locator(`[data-notification-id="${selected.id}"] button`).click();
 await expect(panel.getByRole('region',{name:'通知详情'})).toBeVisible();
 await expect.poll(async()=>(await call('/me/notifications/'+selected.id)).read_at).not.toBeNull();
 await expect.poll(async()=>(await call('/me/notifications/summary')).unread).toBe(before-(selected.read_at?0:1));
 await panel.getByRole('link',{name:'在通知中心查看'}).click();
 await page.waitForURL(v=>v.pathname==='/notifications/'+selected.id&&v.searchParams.get('tenant_id')===state.tenants[0]);
 const detail=page.getByRole('region',{name:'通知详情'});
 await expect(detail.getByRole('heading',{name:selected.object_name||selected.title,exact:true})).toBeVisible();
 await expect(page.getByRole('region',{name:'通知列表',exact:true})).toHaveCount(0);
 await expect(page.getByRole('navigation',{name:'账号设置'}).getByRole('link',{name:'通知中心'})).toHaveAttribute('aria-current','page');
 await detail.getByRole('link',{name:'返回通知列表',exact:true}).click();
 checks.push('Right-anchored nonmodal popover, scope filtering without read, inline detail reads only one, detail deep link retains source');

 await page.getByLabel('查找本页通知',{exact:true}).fill('definitely-no-notification-matches');
 await expect(page.getByText('本页没有匹配的通知',{exact:true})).toBeVisible();
 await expect(page.getByRole('button',{name:'当前筛选全部已读',exact:true})).toBeDisabled();
 await page.getByRole('button',{name:'清空本页查找',exact:true}).click();
 await expect(page.getByRole('list',{name:'通知条目'})).toBeVisible();
 await page.getByRole('region',{name:'通知列表',exact:true}).locator(`[data-notification-id="${selected.id}"] a`).click();
 await detail.getByRole('button',{name:'归档',exact:true}).click();
 await expect.poll(async()=>(await call('/me/notifications/'+selected.id)).archived_at).not.toBeNull();
 await detail.getByRole('link',{name:'返回通知列表',exact:true}).click();
 await page.getByRole('combobox',{name:'阅读状态',exact:true}).selectOption('archived');
 await page.getByRole('region',{name:'通知列表',exact:true}).locator(`[data-notification-id="${selected.id}"] a`).click();
 await detail.getByRole('button',{name:'取消归档',exact:true}).click();
 await expect.poll(async()=>(await call('/me/notifications/'+selected.id)).archived_at).toBeNull();
 if(!(await call('/me/notifications/'+selected.id)).read_at)throw Error('Unarchiving must remain read');
 await detail.getByRole('button',{name:'归档',exact:true}).click();
 await expect(detail.getByRole('button',{name:'取消归档',exact:true})).toBeEnabled();
 await detail.getByRole('button',{name:'恢复未读',exact:true}).click();
 await expect.poll(async()=>{const v=await call('/me/notifications/'+selected.id);return [v.read_at,v.archived_at];}).toEqual([null,null]);
 checks.push('Inbox local-page find is explicit and disables bulk-read; archive/unarchive/read restoration retain service semantics');

 await page.goto(`${origin}/notifications?category=tasks&tenant_id=${state.tenants[0]}`);
 const list=page.getByRole('region',{name:'通知列表',exact:true});
 await list.getByRole('link').first().click();
 const url=new URL(page.url());
 if(url.searchParams.get('category')!=='tasks'||url.searchParams.get('tenant_id')!==state.tenants[0])throw Error('Selection lost inbox filters');
 await page.reload();await expect(detail.getByRole('button',{name:'恢复未读',exact:true})).toBeVisible();
 await page.setViewportSize({width:390,height:844});
 await expect(list).toBeHidden();await expect(detail).toBeVisible();
 if(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth))throw Error('Narrow selected detail overflows horizontally');
 await detail.getByRole('link',{name:'返回通知列表',exact:true}).click();
 await expect(list).toBeVisible();
 if(new URL(page.url()).searchParams.get('category')!=='tasks')throw Error('Mobile back lost category filter');
 await page.setViewportSize({width:1440,height:1000});
 checks.push('Inbox selection/filter URLs survive reload; mobile detail/list back retains category with no horizontal overflow');

 await page.goto(origin+'/notifications');
 const last=list.getByRole('link').last();
 await expect(last).toBeVisible();await last.scrollIntoViewIfNeeded();
 const scrollBefore=await page.evaluate(()=>window.scrollY);
 if(scrollBefore<=0)throw Error('Scroll restoration fixture did not exercise a scrolled list');
 await last.click();await expect(detail.getByRole('button',{name:'恢复未读',exact:true})).toBeVisible();
 await expect(list).toHaveCount(0);
 await detail.getByRole('link',{name:'返回通知列表',exact:true}).click();
 await expect(last).toBeFocused();
 await expect.poll(()=>page.evaluate(()=>window.scrollY)).toBeCloseTo(scrollBefore,0);
 await page.getByLabel('查找本页通知').fill('通知');
 await list.getByRole('link').first().click();
 await expect(list).toHaveCount(0);
 await detail.getByRole('link',{name:'返回通知列表',exact:true}).click();
 await expect(page.getByLabel('查找本页通知')).toHaveValue('通知');
 await page.getByRole('button',{name:'清空本页查找',exact:true}).click();
 checks.push('Inbox list scroll position and local-page search survive list/detail route switches within the account authorization epoch');
}
