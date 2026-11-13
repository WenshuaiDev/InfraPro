import { useState } from 'react';
export function useCursor() {
 const [pages,setPages]=useState<string[]>(['']);
 return {cursor:pages.at(-1)??'',next:(v:string)=>setPages(p=>[...p,v]),previous:pages.length>1?()=>setPages(p=>p.slice(0,-1)):undefined,reset:()=>setPages([''])};
}
