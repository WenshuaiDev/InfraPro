"""Engineering checks exercise authored geometry, IFC conversion and cross-view correspondence."""
import json, tempfile, unittest
from pathlib import Path
import numpy as np
import engineering as e
import convert as c
class EngineeringTests(unittest.TestCase):
 def test_structural_geometry_is_closed_outward_and_finite(self):
  for make in (e.bridge,e.tunnel,e.slope,e.road):
   s=make();self.assertEqual(len({p['id'] for p in s.parts}),len(s.parts))
   for p in s.parts:
    v=p['v'][p['f']];volume=np.einsum('ij,ij->i',v[:,0],np.cross(v[:,1],v[:,2])).sum()/6
    with self.subTest(part=p['key']):self.assertGreater(volume,1e-9)
   keys={p['key'] for p in s.parts}
   for p in s.points:self.assertIn(p['part'],keys);self.assertIn(p['component'],s.components)
 def test_ifc_preserves_material_colors_identity_and_model_coordinates(self):
  s=e.bridge()
  with tempfile.TemporaryDirectory() as temp:
   root=Path(temp);e.ifc(s,root/'bridge.ifc');c.run('ifc',root/'bridge.ifc',root/'converted');m=json.loads((root/'converted/manifest.json').read_text());doc,binary=c.glb_unpack((root/'converted/model.glb').read_bytes());colors={tuple(x['pbrMetallicRoughness']['baseColorFactor']) for x in doc['materials']};self.assertGreaterEqual(len(colors),8);self.assertTrue(all(p['id'] in {x['key'] for x in m['elements']} for p in s.parts))
   boxes=[a for a in doc['accessors'] if a.get('type')=='VEC3' and 'min' in a];actual=[np.min([a['min'] for a in boxes],axis=0),np.max([a['max'] for a in boxes],axis=0)];v=np.concatenate([p['v'] for p in s.parts]);np.testing.assert_allclose(actual,[v.min(0),v.max(0)],atol=1e-4)
 def test_drawings_place_only_visible_points_within_page(self):
  with tempfile.TemporaryDirectory() as temp:
   s=e.bridge();pages=e.drawings(s,Path(temp)/'drawing.pdf');self.assertEqual(len(pages),3);self.assertEqual(len(pages[0]['points']),len(s.points));self.assertLess(len(pages[2]['points']),len(s.points))
   for page in pages:
    for p in page['points']:self.assertTrue(all(0<v<1 for v in p['anchor']))
if __name__=='__main__':unittest.main(verbosity=2)
