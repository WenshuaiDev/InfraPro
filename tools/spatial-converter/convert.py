#!/usr/bin/env python3
"""Deterministic, bounded conversion. No database, credentials, or network inputs.

The only interface is: convert.py FORMAT SOURCE OUTPUT_DIRECTORY.
The parent owns cancellation, wall-clock time, hashes and immutable publication.
"""
import hashlib
import json
import math
import os
from pathlib import Path
import resource
import struct
import sys

CONTRACT = 'infrapro.spatial/1'
CONVERTER = 'infrapro-spatial-python/1'
MAX_PIXELS = 32_000_000
MAX_TOTAL_PIXELS = 256_000_000
MAX_ELEMENTS = 100_000
MAX_TRIANGLES = 5_000_000
MAX_OUTPUT = 1024 * 1024 * 1024
IDENTITY = [1., 0., 0., 0., 0., 1., 0., 0., 0., 0., 1., 0., 0., 0., 0., 1.]


class ConversionError(Exception):
    def __init__(self, code, message):
        self.code = code
        super().__init__(message)


def require(ok, code, message):
    if not ok:
        raise ConversionError(code, message)


def digest(data):
    return hashlib.sha256(data).hexdigest()


def canonical(value):
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(',', ':'), allow_nan=False).encode()


def decode_json(data):
    def pairs(items):
        result = {}
        for k, v in items:
            require(k not in result, 'duplicate_key', 'Duplicate JSON key')
            result[k] = v
        return result
    def invalid_number(value):
        raise ConversionError('invalid_number', 'Non-finite numeric value: ' + value)
    return json.loads(data, object_pairs_hook=pairs, parse_constant=invalid_number)


def finite_tree(value, depth=0):
    require(depth < 96, 'depth_limit', 'Document nesting exceeds 96 levels')
    if isinstance(value, float):
        require(math.isfinite(value), 'invalid_number', 'Non-finite numeric value')
    elif isinstance(value, dict):
        for v in value.values():
            finite_tree(v, depth + 1)
    elif isinstance(value, list):
        for v in value:
            finite_tree(v, depth + 1)


def manifest(source, fmt):
    return dict(contract=CONTRACT, converter=CONVERTER, source_sha256=digest(source), format=fmt,
                files=[], pages=[], elements=[], features=[], warnings=[],
                coordinates=dict(unit='m', up_axis='Y', source_unit='m', source_to_view=IDENTITY, view_to_source=IDENTITY))


def add_file(out, result, name, data, media_type):
    require('/' not in name and '\\' not in name, 'output_name', 'Invalid output filename')
    require(sum(f['size'] for f in result['files']) + len(data) <= MAX_OUTPUT, 'output_limit', 'Expanded output exceeds 1 GiB')
    (out / name).write_bytes(data)
    result['files'].append(dict(id=name, name=name, sha256=digest(data), size=len(data), media_type=media_type))
    return name


def image_page(image, out, result, page, source_size=None, rotation=0, crop=None):
    from PIL import Image
    import io
    width, height = image.size
    require(width > 0 and height > 0 and width * height <= MAX_PIXELS, 'pixel_limit', 'A page exceeds 32 million pixels')
    require(sum(p['width'] * p['height'] for p in result['pages']) + width * height <= MAX_TOTAL_PIXELS,
            'pixel_limit', 'Document exceeds 256 million pixels')
    source_size = source_size or (width, height)
    entry = dict(id=f'page-{page}', number=page, width=width, height=height,
                 source_width=source_size[0], source_height=source_size[1], rotation=rotation,
                 crop_box=crop or [0, 0, *source_size], resolutions=[])
    sizes = sorted(set([min(512, max(width, height)), min(2048, max(width, height)), max(width, height)]))
    for size in sizes:
        scaled = image.convert('RGB')
        scaled.thumbnail((size, size), Image.Resampling.LANCZOS)
        buffer = io.BytesIO()
        scaled.save(buffer, format='PNG', optimize=False)
        file_id = add_file(out, result, f'page-{page}-{size}.png', buffer.getvalue(), 'image/png')
        entry['resolutions'].append(dict(file_id=file_id, width=scaled.width, height=scaled.height))
        entry['file_id'] = file_id
    result['pages'].append(entry)
    result['coordinates'] = dict(unit='normalized', up_axis='image-top', source_unit='pixels', source_to_view=IDENTITY, view_to_source=IDENTITY)


def convert_image(source, out, result):
    from PIL import Image, ImageOps
    import io
    Image.MAX_IMAGE_PIXELS = MAX_PIXELS
    with Image.open(io.BytesIO(source)) as image:
        require(image.format in ('PNG', 'JPEG') and image.format.lower() == result['format'], 'format_mismatch', 'Image content and declared format differ')
        require(getattr(image, 'n_frames', 1) == 1, 'animated_image', 'Animated/multiple-frame images are not supported')
        size = image.size
        image_page(ImageOps.exif_transpose(image), out, result, 1, size)


def convert_pdf(source, out, result):
    import pypdfium2 as pdfium
    import pypdfium2.raw as raw
    require(source.startswith(b'%PDF-'), 'format_mismatch', 'Not a PDF document')
    with pdfium.PdfDocument(source) as pdf:
        require(raw.FPDF_GetSecurityHandlerRevision(pdf) == -1, 'encrypted_pdf', 'Encrypted PDF is not supported')
        require(0 < len(pdf) <= 128, 'page_limit', 'PDF must contain 1–128 pages')
        for i in range(len(pdf)):
            page = pdf[i]
            try:
                width, height = page.get_size()
                require(width > 0 and height > 0, 'page_size', 'Invalid page dimensions')
                scale = min(2., 8192 / max(width, height))
                require(width * height * scale * scale <= MAX_PIXELS, 'pixel_limit', 'PDF page exceeds pixel budget')
                bitmap = page.render(scale=scale, draw_annots=False, may_draw_forms=False)
                try:
                    image_page(bitmap.to_pil(), out, result, i + 1, (width, height), page.get_rotation(), list(page.get_cropbox()))
                finally:
                    bitmap.close()
            finally:
                page.close()


def glb_pack(doc, binary):
    text = canonical(doc)
    text += b' ' * (-len(text) % 4)
    binary += b'\x00' * (-len(binary) % 4)
    chunks = struct.pack('<II', len(text), 0x4E4F534A) + text
    if binary:
        chunks += struct.pack('<II', len(binary), 0x004E4942) + binary
    return struct.pack('<4sII', b'glTF', 2, 12 + len(chunks)) + chunks


def glb_unpack(source):
    require(len(source) >= 20, 'invalid_glb', 'GLB header is truncated')
    magic, version, length = struct.unpack_from('<4sII', source)
    require(magic == b'glTF' and version == 2 and length == len(source), 'invalid_glb', 'Invalid glTF 2.0 binary header')
    chunks, offset = [], 12
    while offset < len(source):
        require(offset + 8 <= len(source), 'invalid_glb', 'Truncated chunk header')
        size, kind = struct.unpack_from('<II', source, offset)
        offset += 8
        require(size % 4 == 0 and offset + size <= len(source), 'invalid_glb', 'Invalid chunk bounds')
        chunks.append((kind, source[offset:offset + size]))
        offset += size
    require(chunks and chunks[0][0] == 0x4E4F534A and len(chunks) <= 2 and (len(chunks) == 1 or chunks[1][0] == 0x004E4942), 'invalid_glb', 'Expected JSON and optional BIN chunks')
    doc = decode_json(chunks[0][1])
    finite_tree(doc)
    return doc, chunks[1][1] if len(chunks) == 2 else b''


def glb_validate(doc, binary):
    require(not doc.get('animations') and not doc.get('skins'), 'unsupported_deformation', 'Animated and skinned geometry is outside the static model profile')
    require(doc.get('asset', {}).get('version') == '2.0', 'unsupported_gltf', 'Only glTF 2.0 is supported')
    supported = {'KHR_materials_unlit', 'KHR_texture_transform', 'KHR_mesh_quantization', 'EXT_mesh_gpu_instancing'}
    require(set(doc.get('extensionsRequired', [])) <= supported, 'required_extension', 'Unsupported required glTF extension')
    # Optional extensions with data/geometry effects cannot be silently discarded either.
    require(set(doc.get('extensionsUsed', [])) <= supported, 'unsupported_extension', 'Extension is outside this converter compatibility list')
    for buffer in doc.get('buffers', []):
        require('uri' not in buffer, 'external_resource', 'GLB buffers must be embedded')
        require(0 <= buffer.get('byteLength', -1) <= len(binary), 'invalid_buffer', 'Buffer length exceeds BIN chunk')
    require(len(doc.get('buffers', [])) <= 1, 'invalid_buffer', 'GLB supports one embedded buffer')
    for image in doc.get('images', []):
        require('uri' not in image and 'bufferView' in image, 'external_resource', 'Textures must be embedded in a bufferView')
        require(image.get('mimeType') in ('image/png', 'image/jpeg'), 'unsupported_texture', 'Only embedded PNG/JPEG textures are supported')
    views = doc.get('bufferViews', [])
    from PIL import Image
    import io
    for texture in doc.get('images', []):
        index = texture['bufferView']
        require(type(index) is int and 0 <= index < len(views), 'invalid_texture', 'Texture references invalid bufferView')
        view = views[index]
        image = Image.open(io.BytesIO(binary[view.get('byteOffset', 0):view.get('byteOffset', 0)+view.get('byteLength', 0)]))
        require(image.width*image.height <= MAX_PIXELS, 'texture_budget', 'Embedded texture exceeds pixel budget')
        image.verify()
    for view in views:
        require(view.get('buffer') == 0 and 0 <= view.get('byteOffset', 0) and 0 < view.get('byteLength', 0)
                and view.get('byteOffset', 0) + view['byteLength'] <= len(binary), 'invalid_buffer_view', 'Invalid bufferView bounds')
    components = {5120: 1, 5121: 1, 5122: 2, 5123: 2, 5125: 4, 5126: 4}
    dimensions = {'SCALAR': 1, 'VEC2': 2, 'VEC3': 3, 'VEC4': 4, 'MAT2': 4, 'MAT3': 9, 'MAT4': 16}
    for accessor in doc.get('accessors', []):
        require('sparse' not in accessor, 'unsupported_accessor', 'Sparse accessors are not in the first compatibility profile')
        index = accessor.get('bufferView', -1)
        require(isinstance(index, int) and 0 <= index < len(views), 'invalid_accessor', 'Missing accessor bufferView')
        size = components.get(accessor.get('componentType'), 0) * dimensions.get(accessor.get('type'), 0)
        count = accessor.get('count', 0)
        view = views[index]
        stride = view.get('byteStride', size)
        offset = accessor.get('byteOffset', 0)
        require(size > 0 and isinstance(count, int) and 0 < count <= 20_000_000 and size <= stride <= 252
                and offset >= 0 and offset + (count - 1) * stride + size <= view['byteLength'], 'invalid_accessor', 'Invalid accessor dimensions/bounds')
        if accessor['componentType'] == 5126:
            import numpy as np
            array = np.ndarray((count, size // 4), dtype='<f4', buffer=binary, offset=view.get('byteOffset', 0) + offset, strides=(stride, 4))
            require(bool(np.isfinite(array).all()), 'invalid_number', 'Geometry contains non-finite coordinates')
    triangles = 0
    accessors = doc.get('accessors', [])
    for mesh in doc.get('meshes', []):
        for primitive in mesh.get('primitives', []):
            require(not primitive.get('targets'), 'unsupported_deformation', 'Morph targets are outside the static model profile')
            require(primitive.get('mode', 4) == 4 and 'POSITION' in primitive.get('attributes', {}), 'unsupported_primitive', 'Only triangle mesh primitives are supported')
            indices = [*primitive['attributes'].values()]
            if 'indices' in primitive:
                indices.append(primitive['indices'])
            require(all(isinstance(i, int) and 0 <= i < len(accessors) for i in indices), 'invalid_accessor', 'Primitive references invalid accessor')
            count = accessors[primitive.get('indices', primitive['attributes']['POSITION'])]['count']
            require(count % 3 == 0, 'invalid_primitive', 'Triangle index count must be divisible by three')
            position=accessors[primitive['attributes']['POSITION']]
            require(position['type']=='VEC3', 'invalid_position', 'Positions require VEC3 accessors')
            if 'indices' in primitive:
                import numpy as np
                a=accessors[primitive['indices']];v=views[a['bufferView']]
                require(a['type']=='SCALAR' and a['componentType'] in (5121,5123,5125), 'invalid_indices', 'Triangle indices require unsigned scalar accessors')
                data=np.ndarray((a['count'],),dtype={5121:'u1',5123:'<u2',5125:'<u4'}[a['componentType']],buffer=binary,offset=v.get('byteOffset',0)+a.get('byteOffset',0),strides=(v.get('byteStride',components[a['componentType']]),))
                require(int(data.max())<position['count'], 'invalid_indices', 'Triangle index exceeds position buffer')
            triangles += count // 3
    require(0 < triangles <= MAX_TRIANGLES, 'geometry_budget', 'Model must contain 1–5 million triangles')
    nodes = doc.get('nodes', [])
    require(0 < len(nodes) <= MAX_ELEMENTS, 'element_limit', 'Model exceeds element budget')
    parents = {}
    for i, node in enumerate(nodes):
        if 'mesh' in node:
            require(isinstance(node['mesh'], int) and 0 <= node['mesh'] < len(doc.get('meshes', [])), 'invalid_node', 'Invalid node mesh')
        for child in node.get('children', []):
            require(isinstance(child, int) and 0 <= child < len(nodes) and child not in parents, 'invalid_hierarchy', 'Invalid/shared node parent')
            parents[child] = i
    for i in range(len(nodes)):
        chain, current = set(), i
        while current in parents:
            require(current not in chain and len(chain) < 96, 'invalid_hierarchy', 'Cyclic or too-deep model tree')
            chain.add(current)
            current = parents[current]
    require(doc.get('scenes') and 0 <= doc.get('scene', 0) < len(doc['scenes']), 'invalid_scene', 'A valid default scene is required')
    for scene in doc.get('scenes', []):
        require(all(isinstance(i, int) and 0 <= i < len(nodes) and i not in parents for i in scene.get('nodes', [])), 'invalid_scene', 'Invalid scene root')
    for node in nodes:
        node_matrix(node)
    return parents


def node_matrix(node):
    import numpy as np
    if 'matrix' in node:
        require(len(node['matrix'])==16 and not any(k in node for k in ('translation','rotation','scale')), 'invalid_transform', 'Matrix and TRS cannot be combined')
        matrix=np.asarray(node['matrix'],dtype=float).reshape(4,4,order='F')
    else:
        t=node.get('translation',[0,0,0]);q=node.get('rotation',[0,0,0,1]);scale=node.get('scale',[1,1,1])
        require(len(t)==3 and len(q)==4 and len(scale)==3, 'invalid_transform', 'Invalid TRS dimensions')
        x,y,z,w=q;norm=x*x+y*y+z*z+w*w
        require(abs(norm-1)<1e-4, 'invalid_transform', 'Rotation quaternion must be normalized')
        matrix=np.eye(4);matrix[:3,:3]=np.array([[1-2*y*y-2*z*z,2*x*y-2*z*w,2*x*z+2*y*w],[2*x*y+2*z*w,1-2*x*x-2*z*z,2*y*z-2*x*w],[2*x*z-2*y*w,2*y*z+2*x*w,1-2*x*x-2*y*y]])@np.diag(scale);matrix[:3,3]=t
    require(bool(np.isfinite(matrix).all()) and abs(np.linalg.det(matrix))>1e-15, 'invalid_transform', 'Transform must be finite and invertible')
    return matrix


def strip_extras(value):
    if isinstance(value, dict):
        value.pop('extras', None)
        for item in value.values():
            strip_extras(item)
    elif isinstance(value, list):
        for item in value:
            strip_extras(item)


def convert_glb(source, out, result):
    doc, binary = glb_unpack(source)
    parents = glb_validate(doc, binary)
    nodes = doc['nodes']
    keys = [str(node.get('extras', {}).get('source_id', f'node:{i}')) for i, node in enumerate(nodes)]
    require(len(set(keys)) == len(keys), 'identity_conflict', 'Duplicate source element identity')
    require(all(0 < len(k) <= 200 for k in keys), 'identity_limit', 'Source element key exceeds 200 characters')
    source_keys = [bool(node.get('extras', {}).get('source_id')) for node in nodes]
    import numpy as np
    matrices={}
    def world(index):
        if index not in matrices:
            matrices[index]=(world(parents[index]) if index in parents else np.eye(4))@node_matrix(nodes[index])
        return matrices[index]
    def instance_attribute(index):
        a=doc['accessors'][index];v=doc['bufferViews'][a['bufferView']];width={'VEC3':3,'VEC4':4}.get(a['type'],0)
        require(a['componentType']==5126 and width, 'instance_mapping', 'Instance transforms require float VEC3/VEC4')
        return np.ndarray((a['count'],width),dtype='<f4',buffer=binary,offset=v.get('byteOffset',0)+a.get('byteOffset',0),strides=(v.get('byteStride',width*4),4))
    rendered_triangles=0
    for i, node in enumerate(nodes):
        instances = node.get('extensions', {}).get('EXT_mesh_gpu_instancing', {}).get('attributes', {})
        count = 1
        if instances:
            accessors = doc.get('accessors', [])
            require(all(name in ('TRANSLATION', 'ROTATION', 'SCALE') and isinstance(index, int) and 0 <= index < len(accessors) for name, index in instances.items()), 'instance_mapping', 'Invalid instance transform attributes')
            counts = {accessors[index]['count'] for index in instances.values()}
            require(len(counts) == 1, 'instance_mapping', 'Instance attributes have different counts')
            count = counts.pop()
        require(len(result['elements']) + count <= MAX_ELEMENTS, 'element_limit', 'Instance count exceeds element budget')
        if 'mesh' in node:
            rendered_triangles+=count*sum(doc['accessors'][primitive['indices'] if 'indices' in primitive else primitive['attributes']['POSITION']]['count']//3 for primitive in doc['meshes'][node['mesh']]['primitives'])
            require(rendered_triangles<=MAX_TRIANGLES, 'geometry_budget', 'Expanded instances exceed 5 million rendered triangles')
        attributes={name:instance_attribute(index) for name,index in instances.items()}
        geometry = dict(node=node, mesh=doc.get('meshes', [])[node['mesh']] if 'mesh' in node else None,
                        binary_hash=digest(binary), ancestry=[nodes[parents[i]]] if i in parents else [])
        for instance in range(count):
            key = keys[i] + (f'/instance:{instance}' if instances else '')
            fragment = dict(node=i)
            if instances:
                fragment['instance'] = instance
            transform=world(i)
            if instances:
                trs={name.lower():data[instance].tolist() for name,data in attributes.items()}
                transform=transform@node_matrix(trs)
            geometry['world_transform']=transform.flatten(order='F').tolist()
            result['elements'].append(dict(key=key, name=str(node.get('name', key))[:500], type='glTF node',
                parent=keys[parents[i]] if i in parents else '', type_key='',
                fragments=[fragment] if 'mesh' in node else [], geometry_hash=digest(canonical(geometry)),
                properties={'source_identity': source_keys[i]}, type_properties={}, transform=transform.flatten(order='F').tolist()))
    strip_extras(doc)
    for i, node in enumerate(nodes):
        node['extras'] = {'spatial_key': keys[i]}
    add_file(out, result, 'model.glb', glb_pack(doc, binary), 'model/gltf-binary')


def convert_geojson(source, out, result):
    doc = decode_json(source)
    finite_tree(doc)
    require(doc.get('type') == 'FeatureCollection' and isinstance(doc.get('features'), list), 'invalid_geojson', 'Expected RFC 7946 FeatureCollection')
    require('crs' not in doc and len(doc['features']) <= 100_000, 'invalid_geojson', 'Only declared WGS84 GeoJSON is supported; at most 100,000 features')
    seen = set()
    depths = {'Point': 0, 'MultiPoint': 1, 'LineString': 1, 'MultiLineString': 2, 'Polygon': 2, 'MultiPolygon': 3}
    total = 0
    for i, feature in enumerate(doc['features']):
        require(feature.get('type') == 'Feature' and 'crs' not in feature, 'invalid_geojson', 'Invalid feature')
        geometry = feature.get('geometry') or {}
        kind = geometry.get('type')
        require(kind in depths and 'crs' not in geometry, 'unsupported_geometry', 'Only Point/LineString/Polygon and Multi forms are supported')
        points = []
        def walk(value, depth):
            require(isinstance(value, list) and len(value) > 0, 'invalid_coordinates', 'Empty or invalid coordinate array')
            if depth == 0:
                require(2 <= len(value) <= 3 and all(type(x) in (int, float) and math.isfinite(x) for x in value)
                        and -180 <= value[0] <= 180 and -90 <= value[1] <= 90, 'invalid_coordinates', 'Invalid WGS84 coordinate')
                points.append(value)
            else:
                for child in value:
                    walk(child, depth - 1)
        coords = geometry.get('coordinates')
        walk(coords, depths[kind])
        lines = [coords] if kind == 'LineString' else coords if kind == 'MultiLineString' else []
        require(all(len(line) >= 2 for line in lines), 'invalid_linestring', 'LineString needs at least two positions')
        polygons = [coords] if kind == 'Polygon' else coords if kind == 'MultiPolygon' else []
        for polygon in polygons:
            for ring in polygon:
                require(len(ring) >= 4 and ring[0] == ring[-1], 'invalid_polygon', 'Polygon rings must be closed and contain at least four positions')
                require(abs(sum(a[0] * b[1] - b[0] * a[1] for a, b in zip(ring, ring[1:]))) > 1e-14, 'invalid_polygon', 'Polygon ring has no area')
        if polygons:
            from shapely.geometry import shape
            require(shape(geometry).is_valid, 'invalid_polygon', 'Polygon contains invalid rings, holes or self-intersections')
        total += len(points)
        require(total <= 2_000_000, 'geometry_budget', 'GeoJSON exceeds 2 million positions')
        key = str(feature.get('id', f'feature-{i + 1}'))
        require(0 < len(key) <= 200 and key not in seen, 'identity_conflict', 'Feature IDs must be unique')
        seen.add(key)
        properties = {k: str(v)[:500] for k, v in feature.get('properties', {}).items() if k in ('name', 'description', 'category', 'color') and isinstance(v, (str, int, float))}
        result['features'].append(dict(type='Feature', id=key, geometry=geometry, properties=properties,
                                      bbox=[min(p[0] for p in points), min(p[1] for p in points), max(p[0] for p in points), max(p[1] for p in points)]))
    result['coordinates']['crs'] = 'EPSG:4326'
    result['coordinates']['unit'] = 'degree'
    add_file(out, result, 'features.geojson', canonical(dict(type='FeatureCollection', features=result['features'])), 'application/geo+json')


def convert_ifc(source_path, out, result):
    import ifcopenshell
    import ifcopenshell.geom
    import ifcopenshell.util.element
    import ifcopenshell.util.unit
    import numpy as np
    model = ifcopenshell.open(str(source_path))
    require(model.schema in ('IFC2X3', 'IFC4'), 'unsupported_ifc_schema', 'Only IFC2X3 and IFC4 are supported')
    import ifcopenshell.validate
    log=ifcopenshell.validate.json_logger()
    ifcopenshell.validate.validate(model,log)
    require(not log.statements, 'invalid_ifc_schema', 'IFC entities fail schema validation')
    roots = model.by_type('IfcRoot')
    ids = [obj.GlobalId for obj in roots]
    require(all(ids) and len(set(ids)) == len(ids), 'identity_conflict', 'IFC GlobalId values must be nonempty and unique')
    products = sorted(model.by_type('IfcProduct'), key=lambda obj: obj.GlobalId)
    require(len(products) <= MAX_ELEMENTS, 'element_limit', 'IFC exceeds element budget')
    unit_scale = ifcopenshell.util.unit.calculate_unit_scale(model)
    doc = dict(asset=dict(version='2.0', generator=CONVERTER), scene=0, scenes=[dict(nodes=[])], nodes=[], meshes=[],
               buffers=[dict(byteLength=0)], bufferViews=[], accessors=[], materials=[dict(name='Synthetic structural material', pbrMetallicRoughness=dict(baseColorFactor=[.62,.69,.75,1], metallicFactor=0, roughnessFactor=.85))])
    binary = bytearray()
    def accessor(array, component, kind, bounds=False):
        while len(binary) % 4:
            binary.append(0)
        offset = len(binary)
        binary.extend(array.tobytes())
        view = len(doc['bufferViews'])
        doc['bufferViews'].append(dict(buffer=0, byteOffset=offset, byteLength=array.nbytes))
        entry = dict(bufferView=view, componentType=component, count=len(array), type=kind)
        if bounds:
            entry.update(min=array.min(axis=0).tolist(), max=array.max(axis=0).tolist())
        doc['accessors'].append(entry)
        return len(doc['accessors']) - 1
    settings = ifcopenshell.geom.settings()
    settings.set(settings.USE_WORLD_COORDS, True)
    triangles = 0
    supported_items = ('IfcExtrudedAreaSolid', 'IfcMappedItem', 'IfcTriangulatedFaceSet', 'IfcFacetedBrep', 'IfcBooleanClippingResult', 'IfcBoundingBox')
    for product in products:
        parent = ifcopenshell.util.element.get_parent(product)
        typ = ifcopenshell.util.element.get_type(product)
        properties = ifcopenshell.util.element.get_psets(product, should_inherit=False)
        type_properties = ifcopenshell.util.element.get_psets(typ) if typ else {}
        properties['Name'] = product.Name or ''
        properties['Description'] = product.Description or ''
        properties['IFCClass'] = product.is_a()
        properties['GlobalId'] = product.GlobalId
        properties['source_identity'] = True
        material=ifcopenshell.util.element.get_material(product)
        if material:
            properties['Material']={'type':material.is_a(),'name':getattr(material,'Name',None) or getattr(material,'LayerSetName',None) or '', 'materials':[m.Name or '' for m in ifcopenshell.util.element.get_materials(product)]}
        if typ:
            type_properties.update({'Name':typ.Name or '', 'GlobalId':typ.GlobalId, 'IFCClass':typ.is_a()})
        entry = dict(key=product.GlobalId, name=product.Name or product.GlobalId, type=product.is_a(),
                     parent=parent.GlobalId if parent else '', type_key=typ.GlobalId if typ else '', fragments=[],
                     geometry_hash='', properties=properties, type_properties=type_properties, transform=IDENTITY)
        reps = getattr(product, 'Representation', None)
        body = [r for r in reps.Representations if r.RepresentationIdentifier in ('Body', 'Facetation')] if reps else []
        if reps and not body:
            require(not any(r.Items for r in reps.Representations), 'unsupported_ifc_geometry', f'{product.GlobalId}: geometry is not a supported Body representation')
        if body:
            for representation in body:
                for item in representation.Items:
                    require(item.is_a() in supported_items, 'unsupported_ifc_geometry', f'{product.GlobalId}: unsupported {item.is_a()}')
            try:
                shape = ifcopenshell.geom.create_shape(settings, product)
            except Exception as error:
                raise ConversionError('ifc_geometry_failed', f'{product.GlobalId}: geometry conversion failed') from error
            vertices = np.asarray(shape.geometry.verts, dtype='<f4').reshape(-1, 3)
            faces = np.asarray(shape.geometry.faces, dtype='<u4').reshape(-1)
            require(len(vertices) > 0 and len(faces) > 0 and len(faces) % 3 == 0 and bool(np.isfinite(vertices).all()) and int(faces.max()) < len(vertices), 'ifc_geometry_failed', f'{product.GlobalId}: incomplete geometry')
            # IFC is Z-up in metres after IfcOpenShell; glTF display is Y-up.
            vertices = np.stack((vertices[:,0], vertices[:,2], -vertices[:,1]), axis=1).astype('<f4')
            triangles += len(faces) // 3
            require(triangles <= MAX_TRIANGLES, 'geometry_budget', 'IFC exceeds 5 million triangles')
            position = accessor(vertices, 5126, 'VEC3', True)
            indices = accessor(faces, 5125, 'SCALAR')
            mesh = len(doc['meshes'])
            doc['meshes'].append(dict(primitives=[dict(attributes=dict(POSITION=position), indices=indices, material=0)]))
            node = len(doc['nodes'])
            doc['nodes'].append(dict(name=entry['name'], mesh=mesh, extras=dict(spatial_key=product.GlobalId)))
            doc['scenes'][0]['nodes'].append(node)
            entry['fragments'] = [dict(node=node)]
            entry['geometry_hash'] = digest(vertices.tobytes() + faces.tobytes())
        result['elements'].append(entry)
    require(triangles > 0, 'ifc_empty', 'IFC has no supported visible geometry')
    # Keep legal spatial containers even when they have no geometry.
    existing = {e['key'] for e in result['elements']}
    for root in model.by_type('IfcProject'):
        if root.GlobalId not in existing:
            result['elements'].append(dict(key=root.GlobalId, name=root.Name or 'IfcProject', type='IfcProject', parent='', type_key='', fragments=[], geometry_hash='', properties={'Name':root.Name or '', 'GlobalId':root.GlobalId}, type_properties={}, transform=IDENTITY))
    finite_tree(result)
    doc['buffers'][0]['byteLength'] = len(binary)
    result['coordinates'] = dict(unit='m', up_axis='Y', source_unit=f'{unit_scale:g} m',
                                source_to_view=[unit_scale,0,0,0, 0,0,-unit_scale,0, 0,unit_scale,0,0, 0,0,0,1],
                                view_to_source=[1/unit_scale,0,0,0, 0,0,1/unit_scale,0, 0,-1/unit_scale,0,0, 0,0,0,1])
    add_file(out, result, 'model.glb', glb_pack(doc, bytes(binary)), 'model/gltf-binary')


def run(fmt, source_path, out):
    require(source_path.is_file() and not source_path.is_symlink(), 'invalid_input', 'Input must be a regular file')
    require(0 < source_path.stat().st_size <= 256 * 1024 * 1024, 'input_limit', 'Input exceeds 256 MiB')
    source = source_path.read_bytes()
    result = manifest(source, fmt)
    out.mkdir(parents=True, exist_ok=True)
    if fmt in ('png', 'jpeg'):
        convert_image(source, out, result)
    elif fmt == 'pdf':
        convert_pdf(source, out, result)
    elif fmt == 'glb':
        convert_glb(source, out, result)
    elif fmt == 'geojson':
        convert_geojson(source, out, result)
    elif fmt == 'ifc':
        convert_ifc(source_path, out, result)
    else:
        raise ConversionError('unsupported_format', 'Unsupported source format')
    require(len(canonical(result)) <= 64 * 1024 * 1024, 'manifest_limit', 'Manifest exceeds 64 MiB')
    (out / 'manifest.json').write_bytes(canonical(result))


def main():
    if sys.argv[1:] == ['--check']:
        import ifcopenshell, pypdfium2, PIL, numpy
        return 0
    resource.setrlimit(resource.RLIMIT_CPU, (120, 125))
    resource.setrlimit(resource.RLIMIT_FSIZE, (MAX_OUTPUT, MAX_OUTPUT))
    resource.setrlimit(resource.RLIMIT_NOFILE, (128, 128))
    # macOS virtual-address accounting differs from Linux; RSS is bounded by the
    # dedicated worker container in production. Linux also enforces RLIMIT_AS.
    if sys.platform.startswith('linux'):
        resource.setrlimit(resource.RLIMIT_AS, (3 * 1024**3, 3 * 1024**3))
    try:
        require(len(sys.argv) == 4, 'arguments', 'Expected format, source and output directory')
        run(sys.argv[1], Path(sys.argv[2]), Path(sys.argv[3]))
    except Exception as error:
        code = error.code if isinstance(error, ConversionError) else 'parse_failed'
        print(json.dumps(dict(code=code, message=str(error)[:2000])), file=sys.stderr)
        return 1
    return 0


if __name__ == '__main__':
    sys.exit(main())
