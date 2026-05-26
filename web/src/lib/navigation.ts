import { goto as kitGoto, invalidateAll } from '$app/navigation';
import { resolve } from '$app/paths';

type GotoOptions = Parameters<typeof kitGoto>[1];

/** Navigate using SvelteKit path resolution (eslint svelte/no-navigation-without-resolve). */
export async function navigate(path: string, options?: GotoOptions) {
	return kitGoto(resolve(path as '/'), options);
}

export { invalidateAll };
