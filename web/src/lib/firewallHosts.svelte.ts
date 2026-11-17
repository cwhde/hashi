import { api } from '#lib/api/client.ts';

// The firewall hosts by name, shared by the IP item editor and view (bug 56: no page passed them, so a Host item could
// never be added). A page without them (or the design mocks) just shows none.
class FirewallHosts {
	list = $state<{ id: string; name: string }[]>([]);
	#loaded = false;

	async load() {
		if (this.#loaded) return;
		this.#loaded = true;
		try {
			const r = await api.GET('/api/firewall/hosts');
			if (r.data) this.list = r.data.hosts.map((h) => ({ id: h.id, name: h.name }));
			else this.#loaded = false;
		} catch {
			this.#loaded = false;
		}
	}
	name(id: string) {
		return this.list.find((h) => h.id === id)?.name;
	}
}

export const firewallHosts = new FirewallHosts();
