import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtemp, writeFile, readFile, rm, symlink } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import {
  pack,
  validate,
  strictJSON,
} from "../../packages/ts/display-devtools/src/package.mjs";
test("standalone pack round trip, duplicate fields, traversal, links and tampering rejected", async () => {
  const dir = await mkdtemp(join(tmpdir(), "display-package-"));
  try {
    const manifest = JSON.parse(
      await readFile(
        new URL(
          "../../samples/display-sdk/display.manifest.json",
          import.meta.url,
        ),
        "utf8",
      ),
    );
    await writeFile(
      join(dir, "index.html"),
      "<!doctype html><title>Package contract</title>",
    );
    await writeFile(join(dir, "manifest.json"), JSON.stringify(manifest));
    const zip = join(tmpdir(), `display-${Date.now()}.zip`);
    try {
      await pack(dir, join(dir, "manifest.json"), zip);
      const result = await validate(zip);
      assert.equal(result.manifest.files.length, 1);
      const bytes = await readFile(zip);
      bytes[50] ^= 1;
      await writeFile(zip, bytes);
      await assert.rejects(validate(zip));
    } finally {
      await rm(zip, { force: true });
    }
    assert.throws(() => strictJSON('{"a":1,"a":2}'));
    await symlink("/etc/hosts", join(dir, "outside.txt"));
    await assert.rejects(validate(dir));
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
});

test('Manifest 2 named view validation and Manifest 1 format gate agree', async()=>{
  const {validateManifest}=await import('../../packages/ts/display-devtools/src/package.mjs');
  const m=JSON.parse(await readFile(new URL('../../samples/display-sdk/display.manifest.json',import.meta.url),'utf8'));
  m.files=[{path:'index.html',size:1,media_type:'text/html',sha256:'a'.repeat(64)}];m.manifest_version=2;
  m.required_capabilities={'views.read':1,'queries.read':1,'statistics.resources':1,'statistics.monitoring':1};
  m.read_view_schema={project:{domain:'resources',required:true,purpose:'统计',allow_dynamic:false}};
  validateManifest(m);
  for(const change of [v=>v.manifest_version=1,v=>v.read_view_schema.project.domain='tenant',v=>v.read_view_schema.project.ids=['unscoped'],v=>v.read_view_schema.project.allow_dynamic='true',v=>v.read_view_schema.project.purpose='a'.repeat(241),v=>v.read_view_schema=Array(2)]){
    const copy=structuredClone(m);change(copy);assert.throws(()=>validateManifest(copy),{code:'package_invalid'});
  }
  m.manifest_version=1;m.read_view_schema={};assert.throws(()=>validateManifest(m));
});

test('full structure manifests declare the model and reject future versions',async()=>{
  const {validateManifest}=await import('../../packages/ts/display-devtools/src/package.mjs');
  const manifest=JSON.parse(await readFile(new URL('../../examples/display-engineering-structure/display.manifest.json',import.meta.url),'utf8'));
  manifest.files=[{path:'index.html',size:1,media_type:'text/html',sha256:'a'.repeat(64)}];
  assert.doesNotThrow(()=>validateManifest(manifest));
  for(const version of [0,3,'2',null]){
    const invalid=structuredClone(manifest);invalid.read_view_schema.project_resources.structure_model=version;
    assert.throws(()=>validateManifest(invalid));
  }
});
