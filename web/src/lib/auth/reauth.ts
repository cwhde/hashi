import { api, isProblem } from '$lib/api/client';
import { session } from './session.svelte';
import { usePasskey } from './webauthn';

/** Confirms the admin with a passkey (valid five minutes, 02-setup-auth.md §2). */
export async function reauthenticate(): Promise<void> {
	const opts = await api.POST('/api/auth/reauth/options');
	if (opts.error || !opts.data) throw opts.error;
	const credential = await usePasskey(opts.data.options);
	const res = await api.POST('/api/auth/reauth', {
		body: { ceremony_id: opts.data.ceremony_id, credential }
	});
	if (res.error || !res.data) throw res.error;
	session.set(res.data);
}

/**
 * Runs call; when the server answers reauth-required, asks for the passkey once and runs call again.
 * call returns openapi-fetch's {data, error}.
 */
export async function withReauth<T>(
	call: () => Promise<{ data?: T; error?: unknown }>
): Promise<T> {
	let res = await call();
	if (res.error && isProblem(res.error, '/problems/reauth-required')) {
		await reauthenticate();
		res = await call();
	}
	if (res.error) throw res.error;
	return res.data as T;
}
