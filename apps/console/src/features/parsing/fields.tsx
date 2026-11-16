import type {ComponentProps} from 'react';
import {DeviceSelect as BaseSelect} from '../devices/fields';
export {DeviceField} from '../devices/fields';
// Radix reserves an empty value for the placeholder; domain commands use it for
// an explicit clear/unconfigured choice. Keep the sentinel inside the UI.
const empty='__parsing_empty__';
export function DeviceSelect(props:ComponentProps<typeof BaseSelect>){return <BaseSelect {...props} value={props.value||empty} options={props.options.map(option=>({...option,value:option.value||empty}))} onChange={value=>props.onChange(value===empty?'':value)}/>;}
