import { error } from '@sveltejs/kit';

// The approved page mocks are a reference while building (bug 109): they only open under `make dev`, never in a release
// build, so no link can strand the owner in made-up data.
export const load = () => {
	if (!import.meta.env.DEV) error(404, 'Not found');
};
