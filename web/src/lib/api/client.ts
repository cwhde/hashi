// Typed API client generated from api/openapi.yaml (`pnpm generate:api`). Never hand-write request types.
import createClient from 'openapi-fetch';
import type { paths } from './schema.gen';

export const api = createClient<paths>({ baseUrl: '/', credentials: 'same-origin' });
