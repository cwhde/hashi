// App display became Public pages (07 §8.1); old links and bookmarks land there.
import { redirect } from '@sveltejs/kit';

export const load = () => redirect(308, '/public-pages');
