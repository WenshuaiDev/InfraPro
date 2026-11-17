/** Compile diagnostics are relative to a rule; request diagnostics include /body.
 * Binding parameters use named keys, while rule declarations use array indexes. */
export function issueFieldPath(path: string): string {
  const field = path.replace(/^\/body(?=\/)/, '');
  return /^\/(inputs|nodes|outputs|clock|policies)(\/|$)/.test(field) || /^\/parameters\/\d+(\/|$)/.test(field) ? `/rule${field}` : field;
}

export function derivedWorkspaceTab(requested: 'channels' | 'definitions' | undefined, canReadChannels: boolean): 'channels' | 'definitions' {
  return requested ?? (canReadChannels ? 'channels' : 'definitions');
}
