import { services } from '#lib/design/mock.ts';
import { error } from '@sveltejs/kit';

export function load({ params }) {
	const r = services.find((x) => x.id === params.id);
	if (!r) error(404, 'No such service');
	return { r };
}
