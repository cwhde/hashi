import { listSets, type IPSet } from '#lib/ipsets.ts';
import { pulseAgents } from '#lib/pulseAgents.svelte.ts';
import type { Schemas } from '#lib/api/client.ts';

// Names for what a service upstream follows (bug 63): a set or agent shows by name, never by its internal ID.
type Target = Schemas['ServiceTarget'];

class TargetNames {
	sets = $state<IPSet[]>([]);
	#loaded = false;

	async load() {
		const agents = pulseAgents.load();
		if (this.#loaded) return agents;
		this.#loaded = true;
		try {
			this.sets = await listSets();
		} catch {
			this.#loaded = false;
		}
		await agents; // route targets name agents (bug 198)
	}
	get agents() {
		return pulseAgents.list;
	}
	setName(id: string) {
		return this.sets.find((s) => s.id === id)?.name;
	}
	agentName(id: string) {
		return pulseAgents.list.find((a) => a.id === id)?.name;
	}
	/** "nas (Pulse agent):8080", "office (IP set):443", or the literal address. */
	label(t: Target): string {
		if (t.agent_id) return `${this.agentName(t.agent_id) ?? 'Pulse agent'} (agent):${t.port}`;
		if (t.set_id) return `${this.setName(t.set_id) ?? 'IP set'} (set):${t.port}`;
		const host = t.address.includes(':') ? `[${t.address}]` : t.address;
		return `${host}:${t.port}`;
	}
}

export const targetNames = new TargetNames();
