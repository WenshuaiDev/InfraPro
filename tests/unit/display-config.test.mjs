import {test} from 'node:test';
import assert from 'node:assert/strict';
import {upgradeConfig} from '../../apps/console/src/features/displays/config-upgrade.ts';
test('schema upgrades inherit only compatible values and report new requirements',()=>{
 const pack={manifest:{config_schema:{type:'object',required:['title','threshold','newField'],properties:{title:{type:'string'},threshold:{type:'integer',maximum:5,default:2},newField:{type:'string'}}},binding_schema:{site:{kind:'resources',required:true},newChannel:{kind:'channels',required:true}}}};
 const old={parameters:{title:'Saved title',threshold:10,obsolete:true},bindings:{site:{kind:'resources',ids:['site-id']}},mode:'follow'};
 const result=upgradeConfig(pack,old);
 assert.deepEqual(result.parameters,{title:'Saved title',threshold:2});
 assert.deepEqual(result.missing,['参数.newField','绑定.newChannel']);
 assert.deepEqual(result.replaced,['参数.threshold']);
 assert.deepEqual(result.removed,['参数.obsolete']);
 assert.deepEqual(result.bindings.site,old.bindings.site);
 result.bindings.site.ids.push('another');
 assert.deepEqual(old.bindings.site.ids,['site-id']);
 assert.equal(result.mode,'follow');
});
