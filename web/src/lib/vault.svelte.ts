// The vault's mode and lock state (02 §5.4–5.5), shown in the top bar and on every page while locked.
import { api, type Schemas } from '$lib/api/client';

class VaultStore {
	current = $state<Schemas['VaultState'] | null>(null);

	async refresh() {
		const res = await api.GET('/api/vault');
		if (res.data) this.current = res.data;
	}

	set(s: Schemas['VaultState']) {
		this.current = s;
	}
}

export const vaultState = new VaultStore();
