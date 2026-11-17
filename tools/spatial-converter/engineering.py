#!/usr/bin/env python3
"""Reproducible, self-authored transport engineering acceptance package (metres/Y-up).
Not surveyed, construction-ready or a structural safety assessment. CC0-1.0.
One geometric source produces GLB, IFC4, vector drawings and GIS coordinates.
"""
from pathlib import Path
import argparse, hashlib, json, math, struct, uuid
import numpy as np
import ifcopenshell, ifcopenshell.guid
from reportlab.pdfgen import canvas
from reportlab.pdfbase import pdfmetrics
from reportlab.pdfbase.cidfonts import UnicodeCIDFont
from reportlab.lib.colors import HexColor

OUT=Path(__file__).resolve().parents[2]/'samples/spatial/engineering'
SEED='infrapro-engineering-20261116-v2'
MATERIALS={
 'concrete':('#b7b9b3',0,.88), 'precast':('#d4d3c9',0,.8), 'asphalt':('#3d4650',0,.98),
 'white':('#eeece0',0,.68), 'yellow':('#e3ba51',0,.72), 'steel':('#7e929b',.62,.38),
 'bearing':('#363b40',.12,.84), 'soil':('#958169',0,1), 'grass':('#819376',0,1),
 'rock':('#a59d8c',0,1), 'water':('#719fba',.12,.4), 'sensor':('#cf6945',.3,.38),
 'blue':('#527e91',.3,.48), 'dark':('#515b60',.2,.7), 'liner':('#cdc6b5',0,.9)}
def uid(k):return ifcopenshell.guid.compress(uuid.uuid5(uuid.NAMESPACE_URL,SEED+'/'+k).hex)
def rgb(name):
    srgb=[int(MATERIALS[name][0][i:i+2],16)/255 for i in (1,3,5)]
    return [v/12.92 if v<=.04045 else ((v+.055)/1.055)**2.4 for v in srgb]
def dump(p,obj):p.write_text(json.dumps(obj,ensure_ascii=False,indent=2)+'\n')

def triangulate(poly):
    # Ear clipping retains concave stepped sections and exact cap openings.
    p=np.array(poly,float); idx=list(range(len(p))); out=[]
    cross=lambda a,b,c:float((b[0]-a[0])*(c[1]-a[1])-(b[1]-a[1])*(c[0]-a[0]))
    area=sum(p[i,0]*p[(i+1)%len(p),1]-p[(i+1)%len(p),0]*p[i,1] for i in range(len(p)))
    if area<0:idx.reverse()
    while len(idx)>3:
        for k in range(len(idx)):
            a,b,c=idx[k-1],idx[k],idx[(k+1)%len(idx)]
            if cross(p[a],p[b],p[c])<=1e-9:continue
            if any(all(v>=-1e-9 for v in (cross(p[a],p[b],p[j]),cross(p[b],p[c],p[j]),cross(p[c],p[a],p[j]))) for j in idx if j not in (a,b,c)):continue
            out.append([a,b,c]);idx.pop(k);break
        else:raise ValueError('Non-simple profile')
    out.append(idx);return out

class Scene:
    def __init__(self,key,name,summary,geo):self.key=key;self.name=name;self.summary=summary;self.geo=geo;self.parts=[];self.components={};self.points=[]
    def component(self,key,name,category='other',parent=None):self.components[key]=dict(key=key,name=name,category=category,parent=parent);return key
    def mesh(self,key,name,v,f,mat='concrete',cls='IfcBuildingElementProxy',component=None):
        a=np.array(v,dtype=float);faces=np.array(f,dtype=int)
        assert np.isfinite(a).all() and len(faces)>0
        self.parts.append(dict(key=key,id=uid(self.key+'/'+key),name=name,v=a,f=faces,mat=mat,cls=cls,component=component));return key
    def box(self,key,name,size,center,mat='concrete',cls='IfcBuildingElementProxy',component=None):
        x,y,z=np.array(size)/2;v=np.array([[-x,-y,-z],[x,-y,-z],[x,y,-z],[-x,y,-z],[-x,-y,z],[x,-y,z],[x,y,z],[-x,y,z]])+center
        f=[[0,2,1],[0,3,2],[4,5,6],[4,6,7],[0,1,5],[0,5,4],[3,7,6],[3,6,2],[1,2,6],[1,6,5],[0,4,7],[0,7,3]]
        return self.mesh(key,name,v,f,mat,cls,component)
    def extrusion(self,key,name,profile,x0,x1,mat='concrete',cls='IfcBeam',component=None):
        # profile is (lateral Z, vertical Y); extrusion along longitudinal X.
        if sum(profile[i][0]*profile[(i+1)%len(profile)][1]-profile[(i+1)%len(profile)][0]*profile[i][1] for i in range(len(profile)))<0:profile=list(reversed(profile))
        n=len(profile);v=[[x,y,z] for x in (x0,x1) for z,y in profile];caps=triangulate(profile)
        f=[[c,b,a] for a,b,c in caps]+[[a+n,b+n,c+n] for a,b,c in caps]
        for j in range(n):k=(j+1)%n;f.extend([[j,k,k+n],[j,k+n,j+n]])
        self.mesh(key,name,v,[list(reversed(face)) for face in f],mat,cls,component)
        self.parts[-1]['profile']=profile
        return key
    def rod(self,key,name,a,b,r,mat='steel',cls='IfcMember',component=None,sides=12):
        a=np.array(a,float);b=np.array(b,float);d=(b-a)/np.linalg.norm(b-a);u=np.cross(d,[0,0,1] if abs(d[2])<.9 else [0,1,0]);u/=np.linalg.norm(u);w=np.cross(d,u)
        v=[p+r*(math.cos(t)*u+math.sin(t)*w) for p in (a,b) for t in np.linspace(0,2*math.pi,sides,endpoint=False)]
        f=[]
        for i in range(sides):j=(i+1)%sides;f.extend([[i,j,j+sides],[i,j+sides,i+sides]])
        for i in range(1,sides-1):f.extend([[0,i+1,i],[sides,sides+i,sides+i+1]])
        return self.mesh(key,name,v,f,mat,cls,component)
    def point(self,code,name,component,part,xyz):self.points.append(dict(code=code,name=name,component=component,part=part,xyz=xyz))
    def road(self,key,x0,x1,y,width=18,z=0,component=None):
        self.box(key+'-surf','沥青铺装 / 80 mm',(x1-x0,.08,width),((x0+x1)/2,y-.04,z),'asphalt','IfcSlab',component)
        for zz in (-width/2+.45,width/2-.45):self.box(key+f'-edge-{zz}','道路边缘实线',(x1-x0,.008,.12),((x0+x1)/2,y+.007,z+zz),'white','IfcBuildingElementProxy',component)
        for xx in np.arange(x0+2,x1-2,6):
            for zz in (-width/4,width/4):self.box(key+f'-dash-{xx}-{zz}','车道虚线',(3,.008,.12),(float(xx),y+.01,z+zz),'white','IfcBuildingElementProxy',component)
        for zz in (-.12,.12):self.box(key+f'-center-{zz}','中心双黄线',(x1-x0,.009,.1),((x0+x1)/2,y+.01,z+zz),'yellow','IfcBuildingElementProxy',component)
    def rails(self,key,x0,x1,y,z,component=None):
        for xx in np.arange(x0,x1+.1,2.5):self.rod(key+f'-post-{xx}','防护栏立柱',[xx,y,z],[xx,y+1.05,z],.055,component=component)
        for h in (.4,.9):self.rod(key+f'-rail-{h}','连续防护横梁',[x0,y+h,z],[x1,y+h,z],.06,component=component)

def bridge():
    s=Scene('bridge','青澜二桥（仿真工程）','预应力混凝土连续箱梁桥 · 3 × 40 m · 全宽 18 m', [120.18,30.28]);deck=s.component('super','上部结构','beam');sub=s.component('sub','下部结构','pier');acc=s.component('deck','桥面与附属设施','other')
    profiles=[('top',[(-4.4,13.74),(4.4,13.74),(4.4,14),(-4.4,14)]),('bottom',[(-2.7,11.4),(2.7,11.4),(2.8,11.7),(-2.8,11.7)]),('web-l',[(-2.8,11.7),(-2.48,11.7),(-3.1,13.74),(-3.45,13.74)]),('web-r',[(2.48,11.7),(2.8,11.7),(3.45,13.74),(3.1,13.74)]),('web-m',[(-.16,11.7),(.16,11.7),(.16,13.74),(-.16,13.74)])]
    for span,(a,b) in enumerate(((-60,-20),(-20,20),(20,60)),1):
        comp=s.component('span'+str(span),f'第 {span} 跨箱梁','beam',deck)
        for side,zc in [('L',-4.5),('R',4.5)]:
            for pname,profile in profiles:s.extrusion(f'span{span}-{side}-{pname}',f'第{span}跨 {side}幅箱梁 {pname}',[(z+zc,y) for z,y in profile],a,b,'precast','IfcBeam',comp)
            for xx in (a+.3,b-.3):s.extrusion(f'diaphragm{span}-{side}-{xx}',f'第{span}跨端横隔梁',[(zc-2.8,11.7),(zc+2.8,11.7),(zc+3.4,13.74),(zc-3.4,13.74)],xx-.25,xx+.25,'concrete','IfcBeam',comp)
        for code,x,z in [(f'QL-YB-{span:02d}',(a+b)/2,-7.1),(f'QL-WY-{span:02d}',(a+b)/2,4.5)]:s.point(code,'箱梁应变' if 'YB' in code else '跨中挠度',comp,f'span{span}-L-bottom' if 'YB' in code else f'span{span}-R-bottom',[x,11.65,z])
    for i,x in enumerate((-20,20),1):
        comp=s.component('pier'+str(i),f'{i} 号桥墩','pier',sub)
        for z in (-4.5,4.5):
            s.rod(f'pier{i}-{z}',f'{i}号墩圆柱 D2.2 m',[x,.3,z],[x,9.5,z],1.1,'concrete','IfcColumn',comp,sides=40)
            s.box(f'foot{i}-{z}',f'{i}号墩承台',(6,2.1,5),(x,-.75,z),'concrete','IfcFooting',comp)
            for dx in (-1.8,1.8):
                for dz in (-1.4,1.4):s.rod(f'pile{i}-{z}-{dx}-{dz}','钻孔灌注桩 D1.2 m',[x+dx,-12,z+dz],[x+dx,-1.8,z+dz],.6,'concrete','IfcPile',comp,sides=20)
        s.extrusion('cap'+str(i),f'{i}号墩盖梁',[(-8.3,9.5),(8.3,9.5),(8.3,10.85),(-8.3,10.85)],x-1.8,x+1.8,'concrete','IfcBeam',comp)
        for z in (-6.4,-2.6,2.6,6.4):
            s.box(f'pedestal{i}-{z}','支座垫石',(1.05,.25,.95),(x,10.975,z),'concrete','IfcFooting',comp)
            s.box(f'bearing{i}-{z}','盆式支座',(0.85,.30,.75),(x,11.25,z),'bearing','IfcBuildingElementProxy',comp)
        s.point(f'QL-QJ-{i:02d}',f'{i}号墩倾角',comp,f'pier{i}--4.5',[x+1.1,5,-4.5])
    for i,x in enumerate((-60,60)):
        comp=s.component('abut'+str(i),f'{i*3} 号桥台','abutment',sub)
        s.box('abut'+str(i),'重力式桥台',(3,10.8,18),(x,5.7,0),'concrete','IfcWall',comp)
        s.box('abut-foot'+str(i),'桥台扩大基础',(5,2,20),(x,-.7,0),'concrete','IfcFooting',comp)
        for z in (-6.4,-2.6,2.6,6.4):s.box(f'abut-bearing{i}-{z}','桥台支座',(.9,.3,.8),(x,11.25,z),'bearing','IfcBuildingElementProxy',comp)
        for zz in (-9,9):s.box(f'wing{i}-{zz}','桥台翼墙',(9,3,.5),(x+(-4 if i==0 else 4),12.5,zz),'concrete','IfcWall',comp)
        s.road(f'approach{i}',x-20 if i==0 else x,x if i==0 else x+20,14.08,18,component=acc)
        s.extrusion(f'embank{i}','引道路基与放坡',[(-14,-1),(14,-1),(9,14),( -9,14)],x-20 if i==0 else x+1.5,x-1.5 if i==0 else x+20,'soil','IfcBuildingElementProxy',comp)
    s.road('bridge-deck',-60,60,14.08,18,component=acc)
    for z in (-8.85,8.85):
        s.extrusion('parapet'+str(z),'混凝土防撞护栏',[(z-.18,14.08),(z+.18,14.08),(z+.12,14.7),(z-.10,15.0)],-60,60,'concrete','IfcRailing',acc)
        s.rails('bridge'+str(z),-59,59,14.4,z,acc)
        for x in range(-55,60,10):s.rod(f'drain-{x}-{z}','泄水管',[x,14,z-.3*np.sign(z)],[x,11.3,z-.3*np.sign(z)],.075,'dark','IfcPipeSegment',acc)
    for x in (-60,60):s.box('joint'+str(x),'伸缩缝装置',(.25,.04,17.6),(x,14.13,0),'steel','IfcBuildingElementProxy',acc)
    s.box('river','河道水面 / 基础展示剖切',(43,.05,55),(0,-2.3,0),'water')
    for x in (-42,42):s.box('bank'+str(x),'河岸剖切地层',(30,1.0,46),(x,-2.8,0),'grass')
    return s

def tunnel():
    s=Scene('tunnel','北山2号隧道（仿真工程）','双洞双车道公路隧道 · 展示洞口 120 m 区段 · 净宽 10.5 m',[120.21,30.285]);root=s.component('lining','洞身衬砌','lining');portal=s.component('portal','进洞口与洞门','portal');access=s.component('road','隧道路面与机电','other')
    for tube,zc in [('L',-8),('R',8)]:
        comp=s.component('lining-'+tube,f'{tube} 洞二次衬砌','lining',root)
        # Semi-elliptical arch and vertical sidewalls. Near bore has a deliberate longitudinal roof cutaway.
        for seg in range(12):
            x0=-60+seg*10;x1=x0+9.96
            for arc in range(18):
                if tube=='R' and seg>=3 and 3<=arc<=14:continue
                a=arc*math.pi/18;b=(arc+1)*math.pi/18
                poly=[(zc+5.25*math.cos(a),2.3+4.5*math.sin(a)),(zc+5.7*math.cos(a),2.3+4.95*math.sin(a)),(zc+5.7*math.cos(b),2.3+4.95*math.sin(b)),(zc+5.25*math.cos(b),2.3+4.5*math.sin(b))]
                s.extrusion(f'arch-{tube}-{seg}-{arc}',f'{tube}洞 K0+{seg*10:03d} 衬砌拱段 {arc+1}',poly,x0,x1,'liner','IfcWall',comp)
            for z in (zc-5.48,zc+5.48):s.box(f'wall-{tube}-{seg}-{z}','衬砌边墙',(9.96,2.7,.45),((x0+x1)/2,1,z),'liner','IfcWall',comp)
            s.box(f'invert-{tube}-{seg}','仰拱填充与底板',(9.96,.6,11.4),((x0+x1)/2,-.4,zc),'concrete','IfcSlab',comp)
        s.road('tunnel-road'+tube,-72,60,.10,8.5,zc,access)
        for z in (zc-4.8,zc+4.8):
            s.box('walk'+tube+str(z),'检修道 / 排水沟盖板',(120,.32,.7),(0,.16,z),'concrete','IfcSlab',access)
            s.rod('pipe'+tube+str(z),'纵向排水管',[ -60,-.22,z],[60,-.22,z],.16,'blue','IfcPipeSegment',access)
        for x in range(-55,60,10):
            for z in (zc-3.5,zc+3.5):s.box(f'light-{tube}-{x}-{z}','隧道灯具',(.7,.13,.22),(x,5.4,z),'white','IfcLightFixture',access)
        for x in (-40,20):
            for z in (zc-1.3,zc+1.3):s.rod(f'fan-{tube}-{x}-{z}','射流风机',[x-1.4,6.0,z],[x+1.4,6.0,z],.34,'steel','IfcBuildingElementProxy',access,sides=24)
        # Ring-shaped portal follows the real opening; no solid block across traffic clearance.
        for arc in range(24):
            a=arc*math.pi/24;b=(arc+1)*math.pi/24
            poly=[(zc+5.7*math.cos(a),2.3+4.95*math.sin(a)),(zc+6.5*math.cos(a),2.3+5.8*math.sin(a)),(zc+6.5*math.cos(b),2.3+5.8*math.sin(b)),(zc+5.7*math.cos(b),2.3+4.95*math.sin(b))]
            s.extrusion(f'portal-{tube}-{arc}',f'{tube}洞洞门拱圈',poly,-62,-60,'precast','IfcWall',portal)
        for z in (zc-6.1,zc+6.1):s.box(f'portal-foot-{tube}-{z}','洞门侧墙',(2,3.0,.8),(-61,1.05,z),'concrete','IfcWall',portal)
        for i,(x,z,y) in enumerate([(-55,zc,6.8),(-25,zc-5.25,2),(15,zc+5.25,2)]):s.point(f'BS-{tube}-{i+1:02d}','拱顶沉降' if i==0 else '周边收敛',comp,f'arch-{tube}-0-8' if i==0 else f'wall-{tube}-{int((x+60)//10)}-{zc-5.48 if i==1 else zc+5.48}',[x,y,z])
    s.box('terrain-base','洞口地形切片',(150,1.3,37),(-5,-1.4,0),'rock')
    for z in (-17.5,17.5):s.extrusion('hillside'+str(z),'洞口边坡剖切',[(z-2,-.7),(z+2,-.7),(z+2,9),(z-2,7)],-50,60,'grass')
    return s

def slope():
    s=Scene('slope','南岭K18+600边坡（仿真工程）','三级路堑边坡 · 单级高 8 m · 平台宽 2 m · 长 80 m',[120.195,30.29]);body=s.component('body','分级边坡','slope');support=s.component('support','框架锚固体系','support');drain=s.component('drain','坡面截排水','drainage');road=s.component('toe','坡脚防护与道路','other')
    profile=[(-10,-2),(30,-2),(30,24),(24,24),(18,16),(16,16),(10,8),(8,8),(2,0),(-10,0)]
    s.extrusion('slope-earth','边坡分级地形',profile,-40,40,'soil','IfcBuildingElementProxy',body)
    for level in range(3):
        comp=s.component('bench'+str(level+1),f'第 {level+1} 级边坡','slope',body);z0=2+8*level;y0=level*8
        s.extrusion('face'+str(level),'植生护坡面',[(z0,y0+.03),(z0+6,y0+8.03),(z0+6.03,y0+8.03),(z0+.03,y0+.03)],-40,40,'grass','IfcCovering',comp)
        for x in range(-38,40,4):
            s.rod(f'frame-v-{level}-{x}','顺坡框架梁',[x,y0+.13,z0],[x,y0+8.13,z0+6],.18,'concrete','IfcBeam',support,8)
            for step in (1,2):
                y=y0+step*3.5;z=z0+step*2.625
                s.box(f'anchor-head-{level}-{x}-{step}','锚具承压板',(.55,.22,.55),(x,y+.18,z),'steel','IfcPlate',support)
                s.rod(f'anchor-{level}-{x}-{step}','锚杆 / 剖面可见',[x,y,z],[x,y-2.7,z+6],.045,'steel','IfcMember',support,8)
        for step in (0,1,2):
            y=y0+step*4;z=z0+step*3
            s.rod(f'frame-h-{level}-{step}','水平框架梁',[-40,y+.14,z],[40,y+.14,z],.18,'concrete','IfcBeam',support,8)
        # Open U-shaped bench drain, connected to stepped down-chutes.
        zz=z0+6.7;yy=y0+8
        for offset in (-.28,.28):s.box(f'drain-wall-{level}-{offset}','平台排水沟侧壁',(80,.32,.10),(0,yy+.1,zz+offset),'concrete','IfcWall',drain)
        s.box(f'drain-bed-{level}','平台沟底',(80,.10,.65),(0,yy-.06,zz),'concrete','IfcSlab',drain)
        for i,x in enumerate((-24,0,24)):s.point(f'NL-WY-{level+1}{i+1}',f'{level+1}级坡面位移',comp,'face'+str(level),[x,y0+6.0,z0+4.52])
    for x in (-38,38):
        for level in range(3):
            for step in range(12):s.box(f'chute-{x}-{level}-{step}','跌水急流槽踏步',(.9,.15,.6),(x,level*8+step*8/12+.12,2+level*8+step*.5),'concrete','IfcSlab',drain)
    s.box('toe-wall','坡脚挡墙',(80,1.4,.7),(0,.7,1.4),'concrete','IfcWall',road)
    s.road('toe-road',-45,45,.08,8,-5,road)
    s.rails('toe-fence',-40,40,1.35,1.4,road)
    return s

def road():
    s=Scene('road','S218沿江示范段（仿真工程）','双向四车道公路 · 展示 K12+400—K12+560 · 路基宽 23 m',[120.175,30.268]);base=s.component('base','路基与排水','roadbed');pave=s.component('pave','路面结构','pavement');acc=s.component('access','交通安全设施','other')
    s.extrusion('roadbed','填方路基',[(-17,-3),(17,-3),(11.5,1.2),(-11.5,1.2)],-80,80,'soil','IfcBuildingElementProxy',base)
    for i,(name,y,h,w,mat) in enumerate([('级配碎石垫层',1.2,.2,22,'rock'),('水泥稳定碎石底基层',1.4,.18,21,'concrete'),('水泥稳定碎石基层',1.58,.36,20.5,'precast')]):s.box('pavement'+str(i),name,(160,h,w),(0,y+h/2,0),mat,'IfcSlab',pave)
    s.road('road',-80,80,2.02,20,0,pave)
    for z in (-10.8,10.8):
        s.rails('guard'+str(z),-80,80,2.02,z,acc)
        s.extrusion('ditch'+str(z),'梯形边沟',[(z+np.sign(z)*3.8,-2.4),(z+np.sign(z)*4.3,-2.9),(z+np.sign(z)*4.9,-2.9),(z+np.sign(z)*5.4,-2.4)],-80,80,'concrete','IfcBuildingElementProxy',base)
    for x in (-55,5,65):
        for z in (-10.3,10.3):
            s.rod(f'delineator{x}-{z}','轮廓标柱',[x,2.02,z],[x,2.9,z],.045,'white',component=acc)
    for x in (-50,40):
        s.rod('signpost'+str(x),'标志支柱',[x,1.9,11.1],[x,5,11.1],.08,'steel',component=acc)
        s.box('sign'+str(x),'交通指示牌',(.14,1.5,2.2),(x,4.6,11.1),'blue','IfcBuildingElementProxy',acc)
    for i,x in enumerate((-60,-20,20,60)):s.point(f'YJ-CJ-{i+1:02d}','路基沉降',base,'roadbed',[x,1.2,11.5])
    return s

def glb(s,path):
    doc=dict(asset=dict(version='2.0',generator=SEED),scene=0,scenes=[dict(nodes=[])],nodes=[],meshes=[],bufferViews=[],accessors=[],buffers=[dict(byteLength=0)],materials=[]);binary=bytearray()
    names=list(MATERIALS)
    for name in names:
        _,metal,rough=MATERIALS[name];doc['materials'].append(dict(name=name,pbrMetallicRoughness=dict(baseColorFactor=rgb(name)+[1],metallicFactor=metal,roughnessFactor=rough),doubleSided=False))
    def accessor(a,ctype,kind,bounds=False):
        while len(binary)%4:binary.append(0)
        start=len(binary);binary.extend(a.tobytes());doc['bufferViews'].append(dict(buffer=0,byteOffset=start,byteLength=a.nbytes));entry=dict(bufferView=len(doc['bufferViews'])-1,componentType=ctype,count=len(a),type=kind)
        if bounds:entry.update(min=a.min(axis=0).tolist(),max=a.max(axis=0).tolist())
        doc['accessors'].append(entry);return len(doc['accessors'])-1
    for p in s.parts:
        # Flat normals keep structural edges crisp; rods use sufficient angular subdivisions.
        v=p['v'][p['f']].reshape(-1,3);tri=v.reshape(-1,3,3);n=np.cross(tri[:,1]-tri[:,0],tri[:,2]-tri[:,0]);n/=np.maximum(np.linalg.norm(n,axis=1,keepdims=True),1e-12);n=np.repeat(n,3,axis=0)
        pos=accessor(v.astype('<f4'),5126,'VEC3',True);normal=accessor(n.astype('<f4'),5126,'VEC3');indices=accessor(np.arange(len(v),dtype='<u4'),5125,'SCALAR')
        doc['meshes'].append(dict(primitives=[dict(attributes=dict(POSITION=pos,NORMAL=normal),indices=indices,material=names.index(p['mat']))]));doc['scenes'][0]['nodes'].append(len(doc['nodes']));doc['nodes'].append(dict(name=p['name'],mesh=len(doc['meshes'])-1,extras=dict(source_id=p['id'])))
    doc['buffers'][0]['byteLength']=len(binary);j=json.dumps(doc,separators=(',',':'),ensure_ascii=False).encode();j+=b' '*(-len(j)%4);binary+=b'\0'*(-len(binary)%4);path.write_bytes(struct.pack('<4sII',b'glTF',2,28+len(j)+len(binary))+struct.pack('<II',len(j),0x4e4f534a)+j+struct.pack('<II',len(binary),0x004e4942)+binary)

def ifc(s,path):
    f=ifcopenshell.file(schema='IFC4');point=lambda xyz:f.create_entity('IfcCartesianPoint',Coordinates=tuple(float(v) for v in xyz));axis=lambda:f.create_entity('IfcAxis2Placement3D',Location=point([0,0,0]));root=lambda cls,key,**kw:f.create_entity(cls,GlobalId=uid(s.key+'/'+key),**kw)
    ctx=f.create_entity('IfcGeometricRepresentationContext',ContextIdentifier='Model',ContextType='Model',CoordinateSpaceDimension=3,Precision=1e-5,WorldCoordinateSystem=axis());units=f.create_entity('IfcUnitAssignment',Units=[f.create_entity('IfcSIUnit',UnitType='LENGTHUNIT',Name='METRE')]);project=root('IfcProject','project',Name=s.name+' / 仿真工程',RepresentationContexts=[ctx],UnitsInContext=units);site=root('IfcSite','site',Name=s.summary,CompositionType='ELEMENT');root('IfcRelAggregates','site-rel',RelatingObject=project,RelatedObjects=[site]);objects=[];materials={};styles={}
    for mat in MATERIALS:
        materials[mat]=f.create_entity('IfcMaterial',Name=mat);r,g,b=rgb(mat);colour=f.create_entity('IfcColourRgb',Name=mat,Red=r,Green=g,Blue=b);style=f.create_entity('IfcSurfaceStyleRendering',SurfaceColour=colour,Transparency=0.,ReflectanceMethod='NOTDEFINED');styles[mat]=f.create_entity('IfcSurfaceStyle',Name=mat,Side='BOTH',Styles=[style])
    for p in s.parts:
        v=p['v'];coords=np.column_stack([v[:,0],-v[:,2],v[:,1]]);plist=f.create_entity('IfcCartesianPointList3D',CoordList=coords.tolist());item=f.create_entity('IfcTriangulatedFaceSet',Coordinates=plist,Closed=True,CoordIndex=(p['f']+1).tolist());f.create_entity('IfcStyledItem',Item=item,Styles=[styles[p['mat']]]);rep=f.create_entity('IfcShapeRepresentation',ContextOfItems=ctx,RepresentationIdentifier='Body',RepresentationType='Tessellation',Items=[item]);definition=f.create_entity('IfcProductDefinitionShape',Representations=[rep]);obj=root(p['cls'],p['key'],Name=p['name'],Description='Self-authored engineering acceptance geometry; not construction design.',ObjectPlacement=f.create_entity('IfcLocalPlacement',RelativePlacement=axis()),Representation=definition)
        prop=[f.create_entity('IfcPropertySingleValue',Name=k,NominalValue=f.create_entity('IfcLabel',val)) for k,val in [('PartCode',p['key']),('ResourceGroup',p['component'] or 'context'),('Origin','自行设计 / 仿真工程'),('Material',p['mat'])]];ps=root('IfcPropertySet',p['key']+'/ps',Name='Pset_InfraProEngineering',HasProperties=prop);root('IfcRelDefinesByProperties',p['key']+'/props',RelatedObjects=[obj],RelatingPropertyDefinition=ps);root('IfcRelAssociatesMaterial',p['key']+'/mat',RelatedObjects=[obj],RelatingMaterial=materials[p['mat']]);objects.append(obj)
    root('IfcRelContainedInSpatialStructure','contains',RelatingStructure=site,RelatedElements=objects);f.write(str(path))

def gis(s,path):
    features=[];lon,lat=s.geo
    def coord(x,z):return [round(lon+x/(111320*math.cos(math.radians(lat))),8),round(lat-z/110540,8)]
    def feature(key,name,kind,coordinates,category):features.append(dict(type='Feature',id=s.key+'-'+key,properties=dict(name=name,category=category,source='自行设计的仿真工程',facility=s.name),geometry=dict(type=kind,coordinates=coordinates)))
    extent=np.concatenate([p['v'] for p in s.parts]);lo=extent.min(axis=0);hi=extent.max(axis=0)
    feature('boundary',s.name+'工程范围','Polygon',[[coord(lo[0],lo[2]),coord(hi[0],lo[2]),coord(hi[0],hi[2]),coord(lo[0],hi[2]),coord(lo[0],lo[2])]],'boundary')
    for p in s.parts:
        if p['cls'] in ('IfcColumn','IfcFooting') or p['key'].startswith(('roadbed','bridge-deck-surf','face','wall-L','wall-R')):
            b0=p['v'].min(axis=0);b1=p['v'].max(axis=0)
            feature(p['key'],p['name'],'Polygon',[[coord(b0[0],b0[2]),coord(b1[0],b0[2]),coord(b1[0],b1[2]),coord(b0[0],b1[2]),coord(b0[0],b0[2])]],'structure')
    for z in (-8,0,8) if s.key=='tunnel' else (0,):feature('alignment'+str(z),'工程中心线','LineString',[coord(float(x),z) for x in np.linspace(lo[0]-10,hi[0]+10,25)],'alignment')
    for p in s.points:feature(p['code'],p['name']+' '+p['code'],'Point',coord(p['xyz'][0],p['xyz'][2]),'monitoring')
    if s.key=='bridge':feature('river','青澜河虚构河道','Polygon',[[coord(-17,-100),coord(22,-100),coord(25,100),coord(-20,100),coord(-17,-100)]],'water')
    for i in range(5):feature('contour'+str(i),f'地形等高线 {i*5} m','LineString',[coord(float(x),float(hi[2]+8+i*5+2*math.sin(x/30))) for x in np.linspace(lo[0]-20,hi[0]+20,32)],'contour')
    dump(path,dict(type='FeatureCollection',features=features));return len(features)

# A3 vector engineering drawing, consistent projection from model coordinates.
W,H=1190.55,841.89
pdfmetrics.registerFont(UnicodeCIDFont('STSong-Light'))
def drawings(s,path):
    c=canvas.Canvas(str(path),pagesize=(W,H),invariant=1);c.setTitle(s.name+'综合布设与构造图（仿真工程）');maps=[]
    part_map={p['key']:p for p in s.parts};v=np.concatenate([p['v'] for p in s.parts]);bounds=[v.min(0),v.max(0)]
    def text(x,y,t,size=9,col='#283948'):
        c.setFillColor(HexColor(col));c.setFont('STSong-Light',size);c.drawString(x,y,str(t))
    def line(x1,y1,x2,y2,col='#71808b',width=.5):c.setStrokeColor(HexColor(col));c.setLineWidth(width);c.line(x1,y1,x2,y2)
    def dim(x1,y1,x2,y2,label):
        line(x1,y1,x2,y2,'#526579',.6);line(x1-3,y1-4,x1+3,y1+4);line(x2-3,y2-4,x2+3,y2+4);text((x1+x2)/2-15,(y1+y2)/2+5,label,9)
    for page,(title,axes) in enumerate([('总平面布设图',(0,2)),('纵向结构与监测立面',(0,1)),('典型横断面与构造详图',(2,1))],1):
        c.setStrokeColor(HexColor('#455965'));c.setLineWidth(1);c.rect(28,25,W-56,H-50);text(46,H-61,s.name+' / '+title,22);text(46,H-85,s.summary,11);text(W-335,H-61,'INFRAPRO · 工程空间验收资料',11);text(W-335,H-82,'自行设计 / 非实测 / 非施工设计',10,'#8b513e');line(28,H-100,W-28,H-100)
        shown=[p for p in s.parts if p['component'] is not None]
        if page==3:
            section_x=-55 if s.key=='tunnel' else 0
            shown=[p for p in shown if p['v'][:,0].min()<=section_x<=p['v'][:,0].max() or s.key=='bridge' and p['key'].startswith(('pier1','pile1','cap1','foot1','bearing1','pedestal1'))]
        vv=np.concatenate([p['v'] for p in shown]);drawbounds=[vv.min(0),vv.max(0)];ax,ay=axes;low=drawbounds[0][[ax,ay]];high=drawbounds[1][[ax,ay]];scale=min(950/max(high[0]-low[0],1),380/max(high[1]-low[1],1));ox=W/2-(low[0]+high[0])*scale/2;oy=515-(low[1]+high[1])*scale/2
        def xy(xyz):return ox+xyz[ax]*scale,oy+xyz[ay]*scale
        # Projection silhouette and structural seams. Hide context on structural drawing for clarity.
        for p in sorted(shown,key=lambda p: float(p['v'][:,1].mean()) if page==1 else 0):
            verts=p['v'];proj=verts[:,[ax,ay]]
            # unique projected convex hull via monotonic chain; sections have separate slab/web solids.
            pts=sorted(set((float(q[0]),float(q[1])) for q in proj))
            if len(pts)<3:continue
            def cross(o,a,b):return (a[0]-o[0])*(b[1]-o[1])-(a[1]-o[1])*(b[0]-o[0])
            lo=[];up=[]
            for q in pts:
                while len(lo)>1 and cross(lo[-2],lo[-1],q)<=0:lo.pop()
                lo.append(q)
            for q in reversed(pts):
                while len(up)>1 and cross(up[-2],up[-1],q)<=0:up.pop()
                up.append(q)
            hull=p['profile'] if page==3 and 'profile' in p else lo[:-1]+up[:-1];p0=c.beginPath();p0.moveTo(ox+hull[0][0]*scale,oy+hull[0][1]*scale)
            for q in hull[1:]:p0.lineTo(ox+q[0]*scale,oy+q[1]*scale)
            p0.close();c.setFillColor(HexColor(MATERIALS[p['mat']][0]));c.setStrokeColor(HexColor('#5f7079'));c.setLineWidth(.35);c.drawPath(p0,fill=1,stroke=1)
        pagepoints=[]
        plotted=s.points if page!=3 else [p for p in s.points if abs(p['xyz'][0])<1 or s.key=='bridge' and p['code']=='QL-QJ-01' or s.key=='tunnel' and p['code'].endswith('01') or s.key=='road' and p['code'].endswith('02')]
        for i,p in enumerate(plotted):
            x,y=xy(p['xyz']);c.setStrokeColor(HexColor('#b64f32'));c.setFillColor(HexColor('#fffaf3'));c.setLineWidth(1.2);c.circle(x,y,4,stroke=1,fill=1)
            lx=x+13;ly=y+18+(i%3)*15;line(x+3,y+3,lx,ly,'#b64f32');c.setFillColor(HexColor('#fffdf7'));c.rect(lx,ly-2,65,12,fill=1,stroke=0);text(lx+3,ly,p['code'],9,'#973a26');pagepoints.append(dict(code=p['code'],anchor=[round(x/W,6),round(1-y/H,6)]))
        x1,y1=xy(drawbounds[0]);x2,y2=xy(drawbounds[1]);dim(x1,280,x2,280,f'{high[0]-low[0]:.2f} m');
        if s.key=='bridge' and page!=3:
            for aa,bb in [(-60,-20),(-20,20),(20,60)]:dim(ox+aa*scale,305,ox+bb*scale,305,'40.00 m')
            text(70,715,'桥跨编号：01 / 02 / 03     横向：L 幅 / R 幅     主线中心桩号：K8+240',10)
        if page==1:
            line(W-85,665,W-85,710,'#283948',1.4);line(W-85,710,W-91,698,'#283948',1.4);line(W-85,710,W-79,698,'#283948',1.4);text(W-90,720,'N',12)
        text(55,252,'尺寸单位：m   模型单位：m   图示比例随视口变化，尺寸以标注为准',10)
        if s.key=='bridge':
            text(55,226,'构造说明：双箱双室截面；支座位于端横隔梁下方；桥墩通过盖梁、垫石与支座传递竖向荷载。',10)
            text(55,207,'跨径 40 + 40 + 40 m；梁高 2.60 m；桥面铺装 0.08 m；墩柱直径 2.20 m；基础以剖切形式展示。',10)
        elif s.key=='tunnel':
            text(55,226,'构造说明：拱形衬砌、边墙、仰拱与排水成体系；R 洞部分拱顶按展示剖切，便于检查净空与内部设施。',10)
            text(55,207,'单洞内净宽 10.50 m；路面宽 8.50 m；拱部衬砌约 0.45 m；展示长度 120 m；模型不含围岩计算。',10)
        elif s.key=='slope':
            text(55,226,'构造说明：三级放坡、框架锚固、平台沟与急流槽；锚杆进入坡体，几何与排水关系用于交互验收。',10)
            text(55,207,'单级高 8 m；坡面水平投影 6 m；平台宽 2 m；防护框架间距 4 m；稳定性未做设计验算。',10)
        else:
            text(55,226,'构造说明：路基、垫层、底基层、基层和沥青面层分层建模；两侧防撞设施、标志和边沟完整展示。',10)
            text(55,207,'展示路段 160 m；路基顶宽 23 m；行车铺装宽 20 m；沉降测点设置于路肩。',10)
        text(55,176,'监测点表',12);cols=[55,185,355,520,710,900]
        for x,head in zip(cols,['测点编号','监测对象','所属构件','X / 纵向','Y / 高程','Z / 横向']):text(x,155,head,9)
        for row,p in enumerate(s.points[:9]):
            yy=138-row*10.2
            vals=[p['code'],p['name'],s.components[p['component']]['name'],f"{p['xyz'][0]:.2f}",f"{p['xyz'][1]:.2f}",f"{p['xyz'][2]:.2f}"]
            for x,val in zip(cols,vals):text(x,yy,val,8)
        line(28,42,W-28,42);text(45,30,SEED+' / CC0-1.0',8);text(W-290,30,f'{s.key.upper()}-SP-{page:02d}   共 3 页 / 第 {page} 页',9)
        # Every component receives a distinct projected rectangle where visible.
        regions=[]
        for key in s.components:
            parts=[p for p in shown if p['component']==key]
            if not parts:continue
            xyz=np.concatenate([p['v'] for p in parts]);a=xyz.min(0);b=xyz.max(0);xa,ya=xy(a);xb,yb=xy(b);xa=max(0,min(1,xa/W));xb=max(0,min(1,xb/W));ya=max(0,min(1,1-ya/H));yb=max(0,min(1,1-yb/H))
            if abs(xa-xb)>.001 and abs(ya-yb)>.001:regions.append(dict(component=key,region=[[xa,ya],[xb,ya],[xb,yb],[xa,yb],[xa,ya]]))
        maps.append(dict(number=page,points=pagepoints,regions=regions));c.showPage()
    c.save();return maps

def main():
    parser=argparse.ArgumentParser();parser.add_argument('--out',type=Path,default=OUT);args=parser.parse_args();args.out.mkdir(parents=True,exist_ok=True);manifest=[]
    for make in (bridge,tunnel,slope,road):
        s=make();prefix=args.out/s.key;glb(s,prefix.with_suffix('.glb'));ifc(s,prefix.with_suffix('.ifc'));count=gis(s,prefix.with_suffix('.geojson'));pages=drawings(s,prefix.with_suffix('.pdf'));allv=np.concatenate([p['v'] for p in s.parts]);entry=dict(key=s.key,name=s.name,summary=s.summary,origin=s.geo,components=list(s.components.values()),points=s.points,pages=pages,parts=[dict(key=p['key'],id=p['id'],name=p['name'],component=p['component'],ifc_class=p['cls']) for p in s.parts],stats=dict(elements=len(s.parts),triangles=sum(len(p['f']) for p in s.parts),bbox=[allv.min(0).tolist(),allv.max(0).tolist()],features=count),files={ext:dict(name=prefix.with_suffix('.'+ext).name,size=prefix.with_suffix('.'+ext).stat().st_size,sha256=hashlib.sha256(prefix.with_suffix('.'+ext).read_bytes()).hexdigest()) for ext in ('glb','ifc','pdf','geojson')});manifest.append(entry);print(s.name,entry['stats'],flush=True)
    dump(args.out/'manifest.json',dict(generator=SEED,source='Self-authored synthetic engineering; not surveyed or certified',license='CC0-1.0',facilities=manifest))
if __name__=='__main__':main()
