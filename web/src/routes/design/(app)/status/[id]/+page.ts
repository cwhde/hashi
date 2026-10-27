import { monitors } from '$lib/design/mock';
import { error } from '@sveltejs/kit';

export function load({ params }) {
	const m = monitors.find((x) => x.id === params.id);
	if (!m) error(404, 'No such endpoint');
	return { m };
}
