// How many plans wait for the admin's approval, for the top bar (10b §2). Pages that change plans call refresh().
import { api } from '$lib/api/client';

class PendingPlans {
	count = $state(0);

	async refresh() {
		const res = await api.GET('/api/sync/plans');
		if (res.data) this.count = res.data.open.filter((p) => p.status === 'pending').length;
	}
}

export const pendingPlans = new PendingPlans();
