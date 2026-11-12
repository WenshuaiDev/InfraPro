import {readFileSync} from 'node:fs';
import {test,expect} from '@playwright/test';
import {deviceSemanticDifferences,normalizeDeviceText} from '../../packages/ts/api-client/src/device-semantics';
import type {components} from '../../packages/ts/api-client/src/schema';
type Semantics=components['schemas']['DeviceSemantics'];
const vectors=JSON.parse(readFileSync(new URL('../../contracts/fixtures/device-semantics.json',import.meta.url),'utf8')) as {baseline:Semantics;cases:{name:string;output:Semantics;paths:string[]}[];normalization:{input:string;expected:string}[]};
test('shared seven-field semantics and Unicode normalization match Go and SQL',()=>{for(const v of vectors.cases)expect(deviceSemanticDifferences(v.output,vectors.baseline),v.name).toEqual(v.paths);for(const v of vectors.normalization)expect(normalizeDeviceText(v.input)).toBe(v.expected);});
