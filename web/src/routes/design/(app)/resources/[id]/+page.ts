import { resources } from '$lib/design/mock';
import { error } from '@sveltejs/kit';

export function load({ params }) {
	const r = resources.find((x) => x.id === params.id);
	if (!r) error(404, 'No such resource');
	return { r };
}
