#!/usr/bin/env python3
"""Self-authored synthetic engineering fixtures. No surveyed or safety data."""
import hashlib
import json
import math
from pathlib import Path
import struct
import sys
import uuid

from PIL import Image, ImageDraw, ImageFont
import ifcopenshell
import ifcopenshell.guid
from reportlab.pdfgen import canvas

ROOT = Path(__file__).resolve().parents[2]
OUTPUT = ROOT / 'samples/spatial'
SEED = 'infrapro-spatial-synthetic-v1'


def uid(key):
    return ifcopenshell.guid.compress(uuid.uuid5(uuid.NAMESPACE_URL, SEED + '/' + key).hex)


def dump(path, value):
    path.write_text(json.dumps(value, indent=2, ensure_ascii=False, allow_nan=False) + '\n')


def font(size):
    for path in ['/System/Library/Fonts/Supplemental/Arial.ttf', '/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf']:
        if Path(path).exists():
            return ImageFont.truetype(path, size)
    return ImageFont.load_default(size=size)


def drawing(kind, view, v2=False):
    image = Image.new('RGB', (1600, 1000), '#f7fafc')
    d = ImageDraw.Draw(image)
    d.rectangle((36, 36, 1564, 964), outline='#64748b', width=2)
    d.text((72, 65), f'{kind.upper()} / {view.upper()} / V{2 if v2 else 1}', font=font(36), fill='#0f172a')
    d.text((72, 114), 'SYNTHETIC EXAMPLE - NOT A SURVEY OR STRUCTURAL ASSESSMENT', font=font(21), fill='#64748b')
    for x in range(100, 1501, 100):
        d.line((x, 210, x, 820), fill='#e2e8f0', width=1)
    for y in range(220, 821, 100):
        d.line((100, y, 1500, y), fill='#e2e8f0', width=1)
    if kind == 'bridge':
        if view == 'plan':
            d.rectangle((200, 350, 1400, 650), fill='#e2e8f0', outline='#334155', width=4)
            for y in (390, 610):
                d.line((200, y, 1400, y), fill='#64748b', width=6)
            for x in (400, 800, 1200):
                d.rectangle((x - 20, 340, x + 20, 660), outline='#64748b', width=3)
            d.line((200, 500, 1400, 500), fill='#94a3b8', width=2)
            d.text((690, 720), '60 m / schematic', font=font(25), fill='#334155')
        elif view == 'elevation':
            d.rectangle((200, 300, 1400, 360), fill='#cbd5e1', outline='#334155', width=4)
            for x in (400, 800, 1200):
                d.rectangle((x-28, 360, x+28, 710), fill='#e2e8f0', outline='#475569', width=3)
                d.rectangle((x-65, 710, x+65, 760), fill='#cbd5e1', outline='#475569', width=3)
            d.line((100, 760, 1500, 760), fill='#64748b', width=3)
        else:
            d.rectangle((420, 300, 1180, 340), fill='#cbd5e1', outline='#334155', width=3)
            for x in (540, 800, 1060):
                d.rectangle((x-35, 340, x+35, 500), fill='#cbd5e1', outline='#475569', width=3)
            d.text((670, 620), '12 m width', font=font(25), fill='#334155')
    else:
        if view == 'plan':
            d.polygon([(200,350),(1300,250),(1400,680),(300,750)], fill='#dbe7dd', outline='#486150', width=4)
            for y in range(380, 651, 60):
                d.line((300,y,1300,y-90), fill='#90a796', width=3)
        else:
            d.polygon([(240,740),(1350,740),(1350,280)], fill='#dbe7dd', outline='#486150', width=4)
            for x in (700,900,1100):
                top=740-(x-240)*460/1110
                d.line((x,top,x-80,top+170),fill='#64748b',width=6)
            d.text((600,800),'Illustrative slope / no stability claim',font=font(24),fill='#486150')
    if v2:
        d.rectangle((200, 350, 1400, 420), fill='#f7fafc')
        d.text((200, 350), 'V2: drawing crop/order changed - review anchors', font=font(23), fill='#a16207')
    d.text((72, 880), 'Anchor reference: P-01 at (0.500, 0.500) on plan; label is independent.', font=font(22), fill='#334155')
    d.text((72, 920), 'InfraPro / self-authored / CC0-1.0', font=font(20), fill='#64748b')
    return image


def make_drawings():
    for kind in ('bridge','slope'):
        for view in ('plan','elevation','section'):
            img=drawing(kind,view)
            img.save(OUTPUT/f'{kind}-{view}.png')
            img.save(OUTPUT/f'{kind}-{view}.jpg',quality=90)
        pdf=canvas.Canvas(str(OUTPUT/f'{kind}-layouts-v1.pdf'),pagesize=(800,500),invariant=1)
        for view in ('plan','elevation','section'):
            pdf.drawImage(str(OUTPUT/f'{kind}-{view}.png'),0,0,width=800,height=500)
            pdf.showPage()
        pdf.save()
        v2=drawing(kind,'plan',True)
        v2.save(OUTPUT/f'{kind}-plan-v2.png')
        pdf=canvas.Canvas(str(OUTPUT/f'{kind}-layouts-v2.pdf'),pagesize=(1000,600),invariant=1)
        pdf.setPageRotation(90)
        pdf.drawImage(str(OUTPUT/f'{kind}-section.png'),60,30,width=800,height=500)
        pdf.showPage()
        pdf.setPageRotation(0)
        pdf.drawImage(str(OUTPUT/f'{kind}-plan-v2.png'),30,45,width=900,height=510)
        pdf.showPage()
        pdf.save()


def pack_glb(doc,binary):
    encoded=json.dumps(doc,separators=(',',':'),allow_nan=False).encode()
    encoded+=b' '*(-len(encoded)%4)
    binary+=b'\0'*(-len(binary)%4)
    return struct.pack('<4sII',b'glTF',2,28+len(encoded)+len(binary))+struct.pack('<II',len(encoded),0x4e4f534a)+encoded+struct.pack('<II',len(binary),0x004e4942)+binary


def make_glb(kind,v2=False,mm=False):
    import numpy as np
    doc=dict(asset=dict(version='2.0',generator=SEED),scene=0,scenes=[dict(nodes=[])],nodes=[],meshes=[],buffers=[dict(byteLength=0)],bufferViews=[],accessors=[],materials=[dict(name='Concrete',pbrMetallicRoughness=dict(baseColorFactor=[.64,.72,.78,1],metallicFactor=0,roughnessFactor=.86)),dict(name='Slope',pbrMetallicRoughness=dict(baseColorFactor=[.43,.60,.46,1],metallicFactor=0,roughnessFactor=1))])
    binary=bytearray()
    def array(values,dtype,component,typ):
        data=np.asarray(values,dtype=dtype)
        while len(binary)%4:binary.append(0)
        offset=len(binary);binary.extend(data.tobytes());index=len(doc['bufferViews']);doc['bufferViews'].append(dict(buffer=0,byteOffset=offset,byteLength=data.nbytes))
        accessor=dict(bufferView=index,componentType=component,count=len(data),type=typ)
        if typ=='VEC3':accessor.update(min=data.min(axis=0).tolist(),max=data.max(axis=0).tolist())
        doc['accessors'].append(accessor);return len(doc['accessors'])-1
    faces=[0,2,1,0,3,2,4,5,6,4,6,7,0,1,5,0,5,4,3,7,6,3,6,2,1,2,6,1,6,5,0,4,7,0,7,3]
    def box(key,name,size,translation,instances=None):
        x,y,z=[v/2 for v in size];verts=[[-x,-y,-z],[x,-y,-z],[x,y,-z],[-x,y,-z],[-x,-y,z],[x,-y,z],[x,y,z],[-x,y,z]]
        position=array(verts,'<f4',5126,'VEC3');indices=array(faces,'<u4',5125,'SCALAR');mesh=len(doc['meshes']);doc['meshes'].append(dict(primitives=[dict(attributes=dict(POSITION=position),indices=indices,material=0)]))
        node=dict(name=name,mesh=mesh,translation=translation,extras=dict(source_id=key,internal_unpublished_note='must be removed from browser GLB'))
        if instances is not None:
            accessor=array(instances,'<f4',5126,'VEC3');node['extensions']={'EXT_mesh_gpu_instancing':{'attributes':{'TRANSLATION':accessor}}};doc['extensionsUsed']=['EXT_mesh_gpu_instancing'];doc['extensionsRequired']=['EXT_mesh_gpu_instancing']
        doc['scenes'][0]['nodes'].append(len(doc['nodes']));doc['nodes'].append(node)
    if kind=='bridge':
        box('bridge-main-beam','Main beam',(60,2,10),(0,9+(1 if v2 else 0),0))
        box('bridge-deck','Deck',(60,.4,12),(0,10.2+(1 if v2 else 0),0))
        box('bridge-pier','Pier',(2.4,8,1.6),(0,4,0),[[-20,0,0],[0,0,0]] if v2 else [[-20,0,0],[0,0,0],[20,0,0]])
        box('west-foundation-new' if v2 else 'west-foundation','West foundation',(6,1,5),(-20,-.5,0))
        box('middle-foundation','Middle foundation',(6,1,5),(0,-.5,0))
        box('east-foundation','East foundation',(6,1,5),(20,-.5,0))
    else:
        verts=[[-30,0,-10],[30,0,-10],[-30,0,10],[30,0,10],[-30,12,-10],[30,12,-10]]
        face=[0,2,1,1,2,3,0,1,4,1,5,4,0,4,2,1,3,5,2,4,5,2,5,3]
        position=array(verts,'<f4',5126,'VEC3');indices=array(face,'<u4',5125,'SCALAR');doc['meshes'].append(dict(primitives=[dict(attributes=dict(POSITION=position),indices=indices,material=1)]));doc['nodes'].append(dict(name='Synthetic slope body',mesh=0,extras=dict(source_id='slope-body')));doc['scenes'][0]['nodes']=[0]
        box('slope-anchor','Anchor beam',(50,.4,.5),(0,6,0))
    if mm:
        # Child authoring coordinates in millimetres/Z-up; glTF root restores m/Y-up.
        root=dict(name='Authoring millimetres Z-up',scale=[.001,.001,.001],rotation=[-.7071067811865475,0,0,.7071067811865476],children=doc['scenes'][0]['nodes'],extras=dict(source_id='author-frame'))
        for node in doc['nodes']:
            node['scale']=[1000,1000,1000]
            if 'translation' in node:
                x,y,z=node['translation'];node['translation']=[x*1000,-z*1000,y*1000]
            node['rotation']=[.7071067811865475,0,0,.7071067811865476]
        doc['scenes'][0]['nodes']=[len(doc['nodes'])];doc['nodes'].append(root)
    doc['buffers'][0]['byteLength']=len(binary)
    name=f'{kind}-'+('mm-z-up' if mm else f'v{2 if v2 else 1}')+'.glb'
    (OUTPUT/name).write_bytes(pack_glb(doc,bytes(binary)))
    return doc,bytes(binary)


def make_ifc(schema,v2=False,slope=False):
    f=ifcopenshell.file(schema=schema)
    person=f.create_entity('IfcPerson',FamilyName='Synthetic')
    org=f.create_entity('IfcOrganization',Name='InfraPro synthetic samples')
    user=f.create_entity('IfcPersonAndOrganization',ThePerson=person,TheOrganization=org)
    app=f.create_entity('IfcApplication',ApplicationDeveloper=org,Version='1',ApplicationFullName='InfraPro sample generator',ApplicationIdentifier='InfraPro')
    history=f.create_entity('IfcOwnerHistory',OwningUser=user,OwningApplication=app,ChangeAction='ADDED',CreationDate=1794528000)
    def point(values):return f.create_entity('IfcCartesianPoint',Coordinates=tuple(float(v) for v in values))
    def axis(location=(0,0,0)):return f.create_entity('IfcAxis2Placement3D',Location=point(location))
    def root(cls,key,**fields):return f.create_entity(cls,GlobalId=uid(key),OwnerHistory=history,**fields)
    context=f.create_entity('IfcGeometricRepresentationContext',ContextIdentifier='Model',ContextType='Model',CoordinateSpaceDimension=3,Precision=1e-5,WorldCoordinateSystem=axis())
    unit=f.create_entity('IfcSIUnit',UnitType='LENGTHUNIT',Name='METRE')
    units=f.create_entity('IfcUnitAssignment',Units=[unit])
    project=root('IfcProject','project',Name='Synthetic example - not a real engineering model',RepresentationContexts=[context],UnitsInContext=units)
    site=root('IfcSite','site',Name='Illustrative site',CompositionType='ELEMENT')
    building=root('IfcBuilding','bridge',Name='Synthetic bridge',CompositionType='ELEMENT')
    storey=root('IfcBuildingStorey','structure',Name='Structure',Elevation=0.,CompositionType='ELEMENT')
    for i,(parent,child) in enumerate([(project,site),(site,building),(building,storey)]):root('IfcRelAggregates',f'aggregate-{i}',RelatingObject=parent,RelatedObjects=[child])
    def solid(size):
        x,y,height=size
        profile=f.create_entity('IfcRectangleProfileDef',ProfileType='AREA',Position=f.create_entity('IfcAxis2Placement2D',Location=point((0,0))),XDim=float(x),YDim=float(y))
        return f.create_entity('IfcExtrudedAreaSolid',SweptArea=profile,Position=axis(),ExtrudedDirection=f.create_entity('IfcDirection',DirectionRatios=(0.,0.,1.)),Depth=float(height))
    def shape(item,typ='SweptSolid'):return f.create_entity('IfcShapeRepresentation',ContextOfItems=context,RepresentationIdentifier='Body',RepresentationType=typ,Items=[item])
    mapped_shape=shape(solid((2.4,1.6,8)))
    mapping=f.create_entity('IfcRepresentationMap',MappingOrigin=axis(),MappedRepresentation=mapped_shape)
    products=[]
    def product(cls,key,name,position,size=None,mapped=False,item=None):
        placement=f.create_entity('IfcLocalPlacement',RelativePlacement=axis(position))
        if mapped:
            transform=f.create_entity('IfcCartesianTransformationOperator3D',LocalOrigin=point((0,0,0)),Scale=1.)
            rep=shape(f.create_entity('IfcMappedItem',MappingSource=mapping,MappingTarget=transform),'MappedRepresentation')
        else:rep=shape(item or solid(size),'Tessellation' if item else 'SweptSolid')
        definition=f.create_entity('IfcProductDefinitionShape',Representations=[rep])
        obj=root(cls,key,Name=name,ObjectPlacement=placement,Representation=definition,**({'PredefinedType':'PAD_FOOTING'} if cls=='IfcFooting' else {}))
        pset=root('IfcPropertySet',key+'/properties',Name='Pset_SyntheticExample',HasProperties=[f.create_entity('IfcPropertySingleValue',Name='Source',NominalValue=f.create_entity('IfcLabel','Self-authored synthetic example')),f.create_entity('IfcPropertySingleValue',Name='InspectionCode',NominalValue=f.create_entity('IfcIdentifier',key)),f.create_entity('IfcPropertySingleValue',Name='DesignValue',NominalValue=f.create_entity('IfcReal',12.5))])
        root('IfcRelDefinesByProperties',key+'/defines',RelatedObjects=[obj],RelatingPropertyDefinition=pset)
        products.append(obj);return obj
    if slope:
        coordinates=f.create_entity('IfcCartesianPointList3D',CoordList=((-30.,-10.,0.),(30.,-10.,0.),(-30.,10.,0.),(30.,10.,0.),(-30.,-10.,12.),(30.,-10.,12.)))
        item=f.create_entity('IfcTriangulatedFaceSet',Coordinates=coordinates,Closed=True,CoordIndex=((1,3,2),(2,3,4),(1,2,5),(2,6,5),(1,5,3),(2,4,6),(3,5,6),(3,6,4)))
        product('IfcBuildingElementProxy','slope-body','Synthetic slope body',(0,0,0),item=item)
    else:
        beam=product('IfcBeam','bridge-main-beam','Main beam',(0,0,8+(1 if v2 else 0)),(60,10,2))
        product('IfcSlab','bridge-deck','Deck',(0,0,10+(1 if v2 else 0)),(60,12,.4))
        piers=[]
        for i,x in enumerate((-20,0) if v2 else (-20,0,20)):
            piers.append(product('IfcColumn',f'bridge-pier-{i}','Pier '+str(i), (x,0,0),mapped=True))
        product('IfcFooting','west-foundation-new' if v2 else 'west-foundation','West foundation',(-20,0,-1),(6,5,1))
        typ=root('IfcBeamType','main-beam-type',Name='Synthetic beam type',PredefinedType='BEAM')
        pset=root('IfcPropertySet','beam-type-properties',Name='Pset_TypeExample',HasProperties=[f.create_entity('IfcPropertySingleValue',Name='Material',NominalValue=f.create_entity('IfcLabel','Illustrative concrete'))])
        typ.HasPropertySets=[pset]
        root('IfcRelDefinesByType','beam-type-relation',RelatedObjects=[beam],RelatingType=typ)
        material=f.create_entity('IfcMaterial',Name='Illustrative concrete')
        root('IfcRelAssociatesMaterial','beam-material',RelatedObjects=[beam],RelatingMaterial=material)
    root('IfcRelContainedInSpatialStructure','contained',RelatingStructure=storey,RelatedElements=products)
    name=('slope' if slope else 'bridge')+'-'+schema.lower()+f'-v{2 if v2 else 1}.ifc'
    f.write(str(OUTPUT/name))


def make_geo():
    features=[dict(type='Feature',id='bridge-site',properties=dict(name='Synthetic bridge',category='bridge'),geometry=dict(type='Point',coordinates=[116.38,39.90])),dict(type='Feature',id='reference-road',properties=dict(name='Illustrative road'),geometry=dict(type='LineString',coordinates=[[116.37,39.90],[116.39,39.90]])),dict(type='Feature',id='slope-area',properties=dict(name='Synthetic slope'),geometry=dict(type='Polygon',coordinates=[[[116.395,39.895],[116.405,39.895],[116.405,39.905],[116.395,39.905],[116.395,39.895]]]))]
    features += [dict(type='Feature',id='bridge-reference-points',properties=dict(name='Synthetic multiple points'),geometry=dict(type='MultiPoint',coordinates=[[116.382,39.900],[116.383,39.901]])),dict(type='Feature',id='reference-multiple-lines',properties=dict(name='Synthetic multiple lines'),geometry=dict(type='MultiLineString',coordinates=[[[116.37,39.897],[116.38,39.897]],[[116.37,39.899],[116.38,39.899]]])),dict(type='Feature',id='slope-multiple-areas',properties=dict(name='Synthetic multiple areas'),geometry=dict(type='MultiPolygon',coordinates=[[[[116.385,39.900],[116.386,39.900],[116.386,39.901],[116.385,39.900]]],[[[116.387,39.902],[116.388,39.902],[116.388,39.903],[116.387,39.902]]]]))]
    dump(OUTPUT/'reference.geojson',dict(type='FeatureCollection',features=features))
    features[0]['geometry']['coordinates']=[116.381,39.901]
    dump(OUTPUT/'reference-v2.geojson',dict(type='FeatureCollection',features=features))
    tile=Image.new('RGB',(256,256),'#edf3ee');d=ImageDraw.Draw(tile)
    for x in range(0,257,32):d.line((x,0,x,256),fill='#d3dfd8');d.line((0,x,256,x),fill='#d3dfd8')
    d.line((0,180,256,90),fill='#b9cedb',width=24);d.line((0,130,256,130),fill='#ffffff',width=7);d.text((12,12),'SYNTHETIC / XYZ',font=font(16),fill='#486150')
    target=OUTPUT/'tiles/12/3372';target.mkdir(parents=True,exist_ok=True);tile.save(target/'1553.png')


def main():
    OUTPUT.mkdir(parents=True,exist_ok=True)
    make_drawings()
    doc,binary=make_glb('bridge');make_glb('bridge',True);make_glb('bridge',mm=True);make_glb('slope')
    make_ifc('IFC2X3');make_ifc('IFC4');make_ifc('IFC4',True);make_ifc('IFC4',slope=True)
    make_geo()
    abnormal=OUTPUT/'invalid';abnormal.mkdir(exist_ok=True)
    (abnormal/'broken.pdf').write_bytes(b'%PDF-1.7\ntruncated')
    (abnormal/'broken.glb').write_bytes(b'glTF\x00')
    doc['extensionsRequired']=['UNKNOWN_required'];(abnormal/'required-extension.glb').write_bytes(pack_glb(doc,binary))
    doc.pop('extensionsRequired');doc['buffers'][0]['uri']='https://example.invalid/blocked.bin';(abnormal/'external-resource.glb').write_bytes(pack_glb(doc,binary))
    doc['buffers'][0].pop('uri');doc['nodes'][0]['mesh']=999;(abnormal/'missing-mapping.glb').write_bytes(pack_glb(doc,binary));doc['nodes'][0].pop('mesh')
    Image.new('RGB',(6000,6000),'white').save(abnormal/'pixel-budget.png')
    dump(abnormal/'invalid-coordinates.geojson',dict(type='FeatureCollection',features=[dict(type='Feature',geometry=dict(type='Point',coordinates=[190,95]),properties={})]))
    (abnormal/'unsupported-schema.ifc').write_text("ISO-10303-21;\nHEADER;\nFILE_SCHEMA(('IFC4X3_ADD2'));\nENDSEC;\nDATA;\nENDSEC;\nEND-ISO-10303-21;\n")
    paths=sorted(p for p in OUTPUT.rglob('*') if p.is_file() and p.name not in ('manifest.json','expected.json','README.md'))
    dump(OUTPUT/'manifest.json',dict(generator=SEED,license='CC0-1.0',synthetic=True,files=[dict(path=str(p.relative_to(OUTPUT)),sha256=hashlib.sha256(p.read_bytes()).hexdigest(),size=p.stat().st_size) for p in paths]))
    print(f'Generated {len(paths)} synthetic files at {OUTPUT}')

if __name__=='__main__':main()
