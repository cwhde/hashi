import { api, type Schemas } from '$lib/api/client';

// The agents, shared by the IP item editor and list. A page without them (or the design mocks) just shows none.
class PulseAgents {
	list = $state<Schemas['PulseAgent'][]>([]);
	#loaded = false;

	async load() {
		if (this.#loaded) return;
		this.#loaded = true;
		try {
			const r = await api.GET('/api/pulse/agents');
			if (r.data) this.list = r.data.agents;
			else this.#loaded = false;
		} catch {
			this.#loaded = false;
		}
	}
}

export const pulseAgents = new PulseAgents();
