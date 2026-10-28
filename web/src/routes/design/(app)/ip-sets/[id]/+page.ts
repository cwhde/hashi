import { ipSets } from '$lib/design/security';
import { error } from '@sveltejs/kit';

export function load({ params }) {
	const set = ipSets.find((x) => x.id === params.id);
	if (!set) error(404, 'No such IP set');
	return { set };
}
