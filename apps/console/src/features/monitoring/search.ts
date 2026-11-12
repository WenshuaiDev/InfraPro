const uuid=/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
export function monitoringSearch(value:Record<string,unknown>){return {point:typeof value.point==='string'&&uuid.test(value.point)?value.point:undefined,channel:typeof value.channel==='string'&&uuid.test(value.channel)?value.channel:undefined};}
