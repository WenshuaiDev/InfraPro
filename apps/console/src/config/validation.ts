import { z } from 'zod';

// Use the normal parser: strict script CSP forbids Zod's Function/JIT probe.
// Configure before any object schema is constructed, including lazy route chunks.
z.config({ jitless: true });

export { z };
