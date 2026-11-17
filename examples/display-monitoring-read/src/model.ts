import * as THREE from 'three';
import {GLTFLoader} from 'three/addons/loaders/GLTFLoader.js';
import {OrbitControls} from 'three/addons/controls/OrbitControls.js';
import type {ReadSpatialSettings,ReadSpatialElement,ReadSpatialPlacement} from '@infrapro/display-sdk';
export async function showModel(host:HTMLElement,bytes:ArrayBuffer,elements:ReadSpatialElement[],placements:ReadSpatialPlacement[],settings:ReadSpatialSettings|undefined,signal:AbortSignal){
  const manager=new THREE.LoadingManager();manager.setURLModifier(url=>{if(url.startsWith('blob:')||url.startsWith('data:'))return url;throw Error('模型引用外部文件，展示文件须自包含。');});
  const gltf=await new GLTFLoader(manager).parseAsync(bytes,'');
  const disposeTree=(root:THREE.Object3D)=>root.traverse(node=>{if(node instanceof THREE.Mesh){node.geometry.dispose();for(const material of Array.isArray(node.material)?node.material:[node.material]){for(const value of Object.values(material))if(value instanceof THREE.Texture)value.dispose();material.dispose();}}});
  try{for(const element of elements){if(!settings?.hidden_elements.includes(element.key))continue;for(const fragment of element.fragments){const node=await gltf.parser.getDependency('node',fragment.node);if(node)node.visible=false;}}}catch(error){disposeTree(gltf.scene);throw error;}
  if(signal.aborted){disposeTree(gltf.scene);return;}
  const scene=new THREE.Scene();scene.add(gltf.scene);scene.add(new THREE.HemisphereLight(0xffffff,0x70809a,3));
  const light=new THREE.DirectionalLight(0xffffff,3);light.position.set(10,20,10);scene.add(light);
  const box=new THREE.Box3().setFromObject(gltf.scene),size=box.getSize(new THREE.Vector3()),center=box.getCenter(new THREE.Vector3()),distance=Math.max(size.length(),1);
  const camera=new THREE.PerspectiveCamera(45,1,Math.max(distance/10000,.001),distance*100);
  camera.position.copy(center).add(new THREE.Vector3(distance*.8,distance*.6,distance*.8));camera.lookAt(center);
  const renderer=new THREE.WebGLRenderer({antialias:true});renderer.setPixelRatio(Math.min(devicePixelRatio,2));renderer.setClearColor(0x0f172a);host.replaceChildren(renderer.domElement);
  const controls=new OrbitControls(camera,renderer.domElement);controls.target.copy(center);controls.enableDamping=true;
  if(settings?.camera){camera.position.fromArray(settings.camera.position);controls.target.fromArray(settings.camera.target);}
  for(const placement of placements){if(placement.anchor.length<3||placement.basis_changed)continue;const marker=new THREE.Mesh(new THREE.SphereGeometry(distance*.006,12,8),new THREE.MeshBasicMaterial({color:0x65e49b}));marker.position.fromArray(placement.anchor);marker.name=placement.name;scene.add(marker);}
  const resize=()=>{const w=Math.max(host.clientWidth,1),h=380;renderer.setSize(w,h);camera.aspect=w/h;camera.updateProjectionMatrix();};resize();const observer=new ResizeObserver(resize);observer.observe(host);
  let frame=0,closed=false;
  const stop=()=>{if(closed)return;closed=true;cancelAnimationFrame(frame);observer.disconnect();controls.dispose();scene.traverse(node=>{if(node instanceof THREE.Mesh){node.geometry.dispose();for(const material of Array.isArray(node.material)?node.material:[node.material]){for(const value of Object.values(material))if(value instanceof THREE.Texture)value.dispose();material.dispose();}}});renderer.dispose();renderer.forceContextLoss();signal.removeEventListener('abort',stop);};
  signal.addEventListener('abort',stop,{once:true});if(signal.aborted){stop();return;}
  const render=()=>{if(closed)return;controls.update();renderer.render(scene,camera);frame=requestAnimationFrame(render);};render();
}
