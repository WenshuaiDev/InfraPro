export interface Issue {path:string;code:string;message:string}
export interface Manifest {manifest_version:1;app_key:string;name?:string;version:string;entry:string;routing:{mode:'hash'|'history';fallback?:string[]};sdk_protocol:{major:1};required_capabilities:Record<string,number>;optional_capabilities?:Record<string,number>;config_schema:Record<string,unknown>;binding_schema:Record<string,unknown>;files:{path:string;size:number;sha256:string;media_type:string}[]}
export function validate(input:string,manifestPath?:string):Promise<{manifest:Manifest;files:Map<string,Buffer>}>;
export function pack(directory:string,manifestPath:string,out:string):Promise<{size:number;sha256:string;files:number}>;
