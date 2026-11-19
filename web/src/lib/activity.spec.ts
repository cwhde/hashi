import { describe, expect, it } from 'vitest';
import { actionText, actorText } from './activity';

describe('activity wording (bug 129)', () => {
	it('names events in plain words', () => {
		expect(actionText('ip_set.deleted')).toBe('IP set deleted');
		expect(actionText('ip_set.element.added')).toBe('IP set item added');
		expect(actionText('sync.plan.applied')).toBe('Change applied');
		expect(actionText('monitor.endpoint.updated')).toBe('Check updated');
		expect(actionText('auth.reauth')).toBe('Confirmed with a passkey');
		expect(actionText('session.renewed')).toBe('Session renewed');
		expect(actionText('service.waf_updated')).toBe('Service WAF updated');
		expect(actionText('dns.import.decisions_saved')).toBe('DNS import decisions saved');
	});
	it('falls back to readable words for an unknown event', () => {
		expect(actionText('widget.thing_done')).toBe('Widget thing done');
	});
	it('says who did it', () => {
		expect(actorText('session:413fd30303b5')).toBe('You');
		expect(actorText('system')).toBe('Hashi');
		expect(actorText('console')).toBe('Server console');
		expect(actorText('anonymous')).toBe('Unknown visitor');
		expect(actorText('pulse:agent-1')).toBe('pulse:agent-1');
	});
});
