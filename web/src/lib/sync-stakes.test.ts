import { describe, expect, it } from 'vitest';
import { stakes, type Plan } from './sync.ts';

const plan = (over: Partial<Plan>): Plan =>
	({
		id: 'p',
		subsystem: 'firewall',
		target: 'h1',
		target_name: 'edge-a',
		title: 'Firewall on edge-a',
		trigger: 'reconcile',
		status: 'pending',
		risk: 'high',
		created_at: '2026-11-19T00:00:00Z',
		items: [],
		...over
	}) as Plan;

// Bug 35: the owner approved a high-risk firewall plan without knowing what it would do.
describe('stakes', () => {
	it('says what a high-risk firewall plan does and how it is undone', () => {
		const note =
			'10.99.0.0/16 is newly blocked (block4): nothing there can reach this host; public ports open or close';
		const p = plan({
			items: [
				{ action: 'update', object: 'table inet hashi', risk: 'high', note },
				{ action: 'update', object: '/opt/hashi/firewall/hashi.nft.json', risk: 'high', note }
			] as Plan['items']
		});
		expect(stakes(p)).toEqual([
			'10.99.0.0/16 is newly blocked (block4): nothing there can reach this host.',
			'Public ports open or close.',
			'Afterwards Hashi logs in to edge-a over a new SSH connection and checks that NetBird still reaches its management server.',
			'If either check fails, the host puts its previous rules back by itself after 3 minutes.'
		]);
	});
	it('is empty for a low-risk plan', () => {
		expect(stakes(plan({ risk: 'low' }))).toEqual([]);
	});
	it('gives only the reasons outside the firewall', () => {
		const p = plan({
			subsystem: 'dns-prune',
			items: [
				{ action: 'delete', object: 'a.example.com', risk: 'high', note: 'deletes a record' }
			] as Plan['items']
		});
		expect(stakes(p)).toEqual(['Deletes a record.']);
	});
});
