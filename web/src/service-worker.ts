/// <reference types="@sveltejs/kit" />
import { build, files, version } from '$service-worker';

const CACHE_NAME = `hashi-cdn-cache-${version}`;

const ASSETS = [
	...build,
	...files
];

self.addEventListener('install', (event: any) => {
	event.waitUntil(
		caches.open(CACHE_NAME).then((cache) => {
			return cache.addAll(ASSETS);
		}).then(() => {
			(self as any).skipWaiting();
		})
	);
});

self.addEventListener('activate', (event: any) => {
	event.waitUntil(
		caches.keys().then((keys) => {
			return Promise.all(
				keys.map((key) => {
					if (key !== CACHE_NAME) {
						return caches.delete(key);
					}
				})
			);
		}).then(() => {
			(self as any).clients.claim();
		})
	);
});

self.addEventListener('fetch', (event: any) => {
	if (event.request.method !== 'GET') return;

	const url = new URL(event.request.url);

	// Intercept and cache requests to the static.juzo.io CDN
	if (url.hostname === 'static.juzo.io') {
		event.respondWith(
			caches.open(CACHE_NAME).then(async (cache) => {
				const cachedResponse = await cache.match(event.request);
				if (cachedResponse) {
					return cachedResponse;
				}

				try {
					const networkResponse = await fetch(event.request);
					if (networkResponse.status === 200) {
						await cache.put(event.request, networkResponse.clone());
					}
					return networkResponse;
				} catch (error) {
					return new Response('CDN resource offline and not cached', {
						status: 503,
						statusText: 'Service Unavailable'
					});
				}
			})
		);
		return;
	}

	event.respondWith(
		caches.match(event.request).then((response) => {
			return response || fetch(event.request);
		})
	);
});
