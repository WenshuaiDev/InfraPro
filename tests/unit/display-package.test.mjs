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
