import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import ts from "typescript";
const code = ts.transpileModule(
  readFileSync(
    new URL(
      "../../apps/console/src/features/spatial/facility-model.ts",
      import.meta.url,
    ),
    "utf8",
  ),
  {
    compilerOptions: {
      module: ts.ModuleKind.ESNext,
      target: ts.ScriptTarget.ES2022,
    },
  },
).outputText;
const { facilityAssets, directLocations, allSpatialPages } = await import(
  "data:text/javascript;base64," + Buffer.from(code).toString("base64")
);
test("switching facilities excludes prior facility assets and only exposes shared GIS explicitly", () => {
  const a = [
    { id: "a", facility_id: "A", kind: "model", created_at: "2026-01-01" },
    { id: "b", facility_id: "B", kind: "layout", created_at: "2026-02-01" },
    { id: "g", kind: "geolayer", created_at: "2026-03-01" },
    { id: "invalid", kind: "model", created_at: "2026-04-01" },
    { id: "new", facility_id: "A", kind: "layout", created_at: "2026-05-01" },
  ];
  assert.deepEqual(
    facilityAssets(a, "A").map((x) => x.id),
    ["new", "a"],
  );
  assert.deepEqual(
    facilityAssets(a, "B").map((x) => x.id),
    ["b"],
  );
  assert.deepEqual(facilityAssets(a, undefined), []);
  assert.deepEqual(
    facilityAssets(a, "A", true).map((x) => x.id),
    ["g"],
  );
});
test("inherited assets, historical versions and another object never count as direct positioning", () => {
  const owner = { kind: "point", id: "p" },
    own = { owner, version_id: "current" },
    history = { owner, version_id: "old" },
    ancestor = { owner: { kind: "facility", id: "f" }, version_id: "current" },
    other = { owner: { kind: "point", id: "q" }, version_id: "current" };
  assert.deepEqual(
    directLocations([history, ancestor, other, own], owner, "current"),
    [own],
  );
  assert.deepEqual(directLocations([ancestor], ancestor.owner, "current"), []);
  assert.deepEqual(directLocations([history], owner, "current"), []);
});
test("a component with more than one page of entities is fully resolved and repeated cursors fail", async () => {
  const cursors = [];
  const result = await allSpatialPages(async (cursor) => {
    cursors.push(cursor);
    return cursor
      ? { items: ["part-101"], next_cursor: "" }
      : {
          items: Array.from({ length: 100 }, (_, i) => "part-" + (i + 1)),
          next_cursor: "next",
        };
  });
  assert.equal(result.items.length, 101);
  assert.deepEqual(cursors, [undefined, "next"]);
  await assert.rejects(
    allSpatialPages(async () => ({ items: [], next_cursor: "repeated" })),
    /分页重复/,
  );
});

test("resource-map deep links survive parsing without being treated as saved project views", async () => {
  const source = ts.transpileModule(
    readFileSync(
      new URL(
        "../../apps/console/src/features/spatial/state.ts",
        import.meta.url,
      ),
      "utf8",
    ),
    {
      compilerOptions: {
        module: ts.ModuleKind.ESNext,
        target: ts.ScriptTarget.ES2022,
      },
    },
  ).outputText;
  const { spatialSearch } = await import(
    "data:text/javascript;base64," + Buffer.from(source).toString("base64")
  );
  const facility = "cbaef1bb-b6ad-406d-91aa-e1cd3a8d5559";
  assert.deepEqual(
    spatialSearch({ section: "gis", facility, map: "resources" }).map,
    "resources",
  );
  assert.equal(
    spatialSearch({ map: "unknown", view: "resources" }).map,
    undefined,
  );
  assert.equal(spatialSearch({ view: "resources" }).view, undefined);
  assert.equal(spatialSearch({ view: facility }).view, facility);
});
