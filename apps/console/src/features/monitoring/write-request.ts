import { useRef } from 'react';
// Retrying an unchanged form after a lost response must reuse its receipt ID.
// Editing any field starts a different request; successful forms are unmounted.
export function useWriteRequest() {
  const previous=useRef<{key:string;id:string}|null>(null);
  return <T extends {request_id:string}>(input:T):T => {
    const {request_id,...body}=input, key=JSON.stringify(body);
    if(previous.current?.key!==key)previous.current={key,id:request_id};
    return {...input,request_id:previous.current.id};
  };
}
