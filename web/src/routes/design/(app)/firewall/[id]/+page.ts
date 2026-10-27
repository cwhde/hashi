import { firewallHosts } from '$lib/design/mock';
import { error } from '@sveltejs/kit';

export function load({ params }) {
	const h = firewallHosts.find((x) => x.id === params.id);
	if (!h) error(404, 'No such host');
	return { h };
}
