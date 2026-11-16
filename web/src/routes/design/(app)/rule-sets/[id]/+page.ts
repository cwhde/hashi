import { ruleSets } from '#lib/design/security.ts';
import { error } from '@sveltejs/kit';

export function load({ params }) {
	const rs = ruleSets.find((x) => x.id === params.id);
	if (!rs) error(404, 'No such rule set');
	return { rs };
}
