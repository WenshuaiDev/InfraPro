// The Go DTO schemas remain the source for public TypeScript wire structures.
export function readTypes(reads) {
  const type = (s) => {
    if (s.$ref) return s.$ref.split('/').at(-1);
    if (s.const !== undefined) return JSON.stringify(s.const);
    if (s.enum) return s.enum.map(JSON.stringify).join(' | ');
    if (s.anyOf || s.oneOf) return `(${(s.anyOf ?? s.oneOf).map(type).join(' | ')})`;
    if (s.type === 'null') return 'null';
    if (s.type === 'string') return 'string';
    if (s.type === 'boolean') return 'boolean';
    if (s.type === 'number' || s.type === 'integer') return 'number';
    if (s.type === 'array') return `Array<${type(s.items)}>`;
    if (s.type === 'object') {
      const fields = Object.entries(s.properties ?? {}).map(([k, v]) => `${JSON.stringify(k)}${s.required?.includes(k) ? '' : '?'}: ${type(v)}`);
      if (s.additionalProperties && typeof s.additionalProperties === 'object') fields.push(`[key: string]: ${type(s.additionalProperties)}`);
      return fields.length ? `{ ${fields.join('; ')} }` : 'Record<string, never>';
    }
    return 'ReadJSONValue';
  };
  const declarations = Object.entries(reads.$defs).filter(([name])=>name!=='ReadResult').map(([name, schema]) => {
    if(name==='ReadTarget')return 'export type ReadTarget = { view: string; ids?: string[]; selection?: never } | { selection: string; ids?: string[]; view?: never };';
    return `export type ${name} = ${type(schema)};`;
  });
  return `// Generated from Go displayread DTOs. Do not edit.\n${declarations.join('\n')}\nexport interface ReadRequests {\n${Object.entries(reads['x-method-requests']).map(([m,s])=>`  ${JSON.stringify(m)}: ${type(s)};`).join('\n')}\n}\nexport interface ReadData {\n${Object.entries(reads['x-method-data']).map(([m,s])=>`  ${JSON.stringify(m)}: ${type(s)};`).join('\n')}\n}\n`;
}
