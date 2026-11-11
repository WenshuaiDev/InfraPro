import {execFileSync} from 'node:child_process';
import {randomBytes} from 'node:crypto';
import {chmodSync,existsSync,mkdirSync,readFileSync,writeFileSync} from 'node:fs';
import {resolve} from 'node:path';
import {ensureTestDependencies,testEnv} from '../test-environment.mjs';
export const root=resolve(import.meta.dirname,'../..');
export const directory=resolve(root,'.artifacts/project-resource-acceptance');
export const origin='http://localhost:35173';
const databaseName='infrapro_project_resource_acceptance_20261111';
export function savePrivate(name,value){mkdirSync(directory,{recursive:true,mode:0o700});const path=resolve(directory,name);writeFileSync(path,JSON.stringify(value,null,2)+'\n',{mode:0o600});chmodSync(path,0o600);}
export function readPrivate(name){return JSON.parse(readFileSync(resolve(directory,name),'utf8'));}
export function environment(){const own=readPrivate('environment.local.json');if(new URL(own.INFRAPRO_DATABASE_URL).pathname!==`/${databaseName}`||own.INFRAPRO_PUBLIC_CONSOLE_URL!==origin+'/')throw new Error('Acceptance environment mismatch');const env={...process.env};for(const key of Object.keys(env))if(key.startsWith('INFRAPRO_')||key.startsWith('VITE_'))delete env[key];return {...env,...own,CONSOLE_PORT:'35173',API_PROXY_TARGET:'http://127.0.0.1:38080',VITE_API_BASE_URL:'/api/v1',VITE_APP_BASE:'/'};}
export function provision(){if(existsSync(resolve(directory,'environment.local.json')))return environment();ensureTestDependencies();const run=args=>execFileSync('docker',['compose','-f','deploy/compose/test.yaml','exec','-T','postgres',...args],{cwd:root,encoding:'utf8'});if(run(['psql','-U','infrapro_test','-d','infrapro_test','-Atc',`SELECT 1 FROM pg_database WHERE datname='${databaseName}'`]).trim())throw new Error('Database exists without this checkout config; refusing to overwrite it');run(['createdb','-U','infrapro_test','-T','template0',databaseName]);const url=new URL(testEnv.INFRAPRO_TEST_DATABASE_URL);url.pathname='/'+databaseName;savePrivate('environment.local.json',{INFRAPRO_DATABASE_URL:url.href,INFRAPRO_REDIS_URL:testEnv.INFRAPRO_TEST_REDIS_URL,INFRAPRO_REDIS_KEY_PREFIX:'infrapro:project-resource-acceptance20261111:identity:v1:',INFRAPRO_CSRF_AUTH_KEY:randomBytes(32).toString('hex'),INFRAPRO_HTTP_ADDR:'127.0.0.1:38080',INFRAPRO_PUBLIC_CONSOLE_URL:origin+'/',INFRAPRO_TRUSTED_ORIGINS:origin,INFRAPRO_COOKIE_SECURE:'false',INFRAPRO_SUPPORT_CONTACT:'项目统一资源重构专用人工验收环境（虚构资料）'});return environment();}
