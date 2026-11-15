#!/usr/bin/env python3
"""Real fixture and malformed-content regression checks; no mocked converter."""
import copy
import json
from pathlib import Path
import tempfile
import unittest
import convert as c

SAMPLES=Path(__file__).resolve().parents[2]/'samples/spatial'
class ConverterTests(unittest.TestCase):
 def convert(self,name):
  self.temp=tempfile.TemporaryDirectory();self.addCleanup(self.temp.cleanup)
  p=SAMPLES/name;fmt={'jpg':'jpeg','json':'geojson'}.get(p.suffix[1:],p.suffix[1:])
  c.run(fmt,p,Path(self.temp.name));return json.loads((Path(self.temp.name)/'manifest.json').read_text())
 def test_pdf_rotation_and_pyramid(self):
  a=self.convert('bridge-layouts-v1.pdf');b=self.convert('bridge-layouts-v2.pdf')
  self.assertEqual(len(a['pages']),3);self.assertEqual(len(b['pages']),2)
  self.assertEqual(b['pages'][0]['rotation'],90)
  self.assertGreater(len(a['pages'][0]['resolutions']),1)
  self.assertNotEqual((a['pages'][0]['width'],a['pages'][0]['height']),(b['pages'][0]['width'],b['pages'][0]['height']))
 def test_ifc_schemas_geometry_instances_and_properties(self):
  for name in ['bridge-ifc2x3-v1.ifc','bridge-ifc4-v1.ifc','slope-ifc4-v1.ifc']:
   with self.subTest(name=name):
    m=self.convert(name);self.assertTrue(any(e['fragments'] for e in m['elements']))
    self.assertTrue(any(e['properties'].get('GlobalId')==e['key'] for e in m['elements']))
    if name.startswith('bridge'):self.assertTrue(any(e['type_key'] and e['type_properties'] for e in m['elements']))
 def test_glb_instances_and_transforms(self):
  m=self.convert('bridge-v1.glb');instances=[e for e in m['elements'] if '/instance:' in e['key']]
  self.assertGreaterEqual(len(instances),2);self.assertNotEqual(instances[0]['transform'],instances[1]['transform'])
  self.assertEqual(len({e['key'] for e in m['elements']}),len(m['elements']))
  self.convert('bridge-mm-z-up.glb')
 def test_malformed_files_are_rejected(self):
  for name in ['broken.pdf','broken.glb','required-extension.glb','external-resource.glb','invalid-coordinates.geojson','unsupported-schema.ifc','missing-mapping.glb','pixel-budget.png']:
   with self.subTest(name=name):
    self.assertTrue((SAMPLES/'invalid'/name).exists())
    with self.assertRaises(Exception):self.convert('invalid/'+name)
 def test_geometry_rejection_profiles(self):
  original,binary=c.glb_unpack((SAMPLES/'bridge-v1.glb').read_bytes())
  for patch,code in [(lambda d:d.update(animations=[{}]),'unsupported_deformation'),(lambda d:d['nodes'][0].update(scale=[0,1,1]),'invalid_transform'),(lambda d:d['nodes'][1].setdefault('extras',{}).update(source_id=d['nodes'][0]['extras']['source_id']),'identity_conflict')]:
   with self.subTest(code=code):
    doc=copy.deepcopy(original);patch(doc)
    with tempfile.TemporaryDirectory() as t:
     with self.assertRaises(c.ConversionError) as error:c.convert_glb(c.glb_pack(doc,binary),Path(t),c.manifest(c.glb_pack(doc,binary),'glb'))
     self.assertEqual(error.exception.code,code)
 def test_geojson_all_six_types_and_topology(self):
  m=self.convert('reference.geojson');self.assertEqual({f['geometry']['type'] for f in m['features']},{'Point','MultiPoint','LineString','MultiLineString','Polygon','MultiPolygon'})
  doc={'type':'FeatureCollection','features':[{'type':'Feature','id':'bowtie','properties':{},'geometry':{'type':'Polygon','coordinates':[[[0,0],[1,1],[1,0],[0,1],[0,0]]]}}]}
  with tempfile.TemporaryDirectory() as t:
   with self.assertRaises(c.ConversionError):c.convert_geojson(json.dumps(doc).encode(),Path(t),c.manifest(b'','geojson'))
 def test_expansion_budget_and_external_texture(self):
  original,binary=c.glb_unpack((SAMPLES/'bridge-v1.glb').read_bytes())
  doc=copy.deepcopy(original);doc['images']=[{'uri':'https://example.invalid/texture.png'}]
  with self.assertRaises(c.ConversionError) as error:c.glb_validate(doc,binary)
  self.assertEqual(error.exception.code,'external_resource')
  prior=c.MAX_TRIANGLES;c.MAX_TRIANGLES=1
  try:
   with self.assertRaises(c.ConversionError) as error:c.glb_validate(original,binary)
   self.assertEqual(error.exception.code,'geometry_budget')
  finally:c.MAX_TRIANGLES=prior
if __name__=='__main__':unittest.main(verbosity=2)
