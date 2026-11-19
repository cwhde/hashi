// Plain words for audit events (bug 129): the Overview and Activity show these instead of `ip_set.deleted` and
// `session:<id>`. Hashi has one user, so every session is "You".

const whole: Record<string, string> = {
	'auth.reauth': 'Confirmed with a passkey',
	'auth.passkey.login': 'Signed in with a passkey',
	'auth.bootstrap.login': 'Signed in with the setup password',
	'auth.denied': 'Sign-in refused',
	'session.issued': 'Signed in',
	'session.revoked': 'Signed out',
	'edge.login': 'Signed in to a service',
	'script.run': 'Script run',
	'setup.completed': 'Setup finished',
	'setup.https.proof': 'HTTPS confirmed during setup',
	'pulse.address_changed': 'Pulse agent address changed',
	'pulse.dependency_impact': 'Pulse agent address affects other objects'
};

// Longest subject first, so `ip_set.element` wins over `ip_set`.
const nouns: [string, string][] = [
	['sync.plan', 'Change'],
	['ip_set.element', 'IP set item'],
	['ip_set', 'IP set'],
	['rule_set', 'Rule set'],
	['monitor.endpoint', 'Check'],
	['auth.passkey', 'Passkey'],
	['adguard.connection', 'AdGuard connection'],
	['traefik.connection', 'Traefik host'],
	['traefik.identity', 'Traefik SSH key'],
	['traefik', 'Traefik'],
	['firewall.host_access', 'Host access'],
	['firewall.host', 'Firewall host'],
	['firewall.bindings', 'Firewall policy'],
	['firewall.takeover', 'Firewall takeover'],
	['firewall', 'Firewall'],
	['pulse.agent', 'Pulse agent'],
	['notification.provider', 'Notification provider'],
	['notification.route', 'Alert route'],
	['vault.recovery_key', 'Recovery key'],
	['vault.unlocker', 'Vault unlocker'],
	['dns.record', 'DNS record'],
	['dns.provider', 'DNS provider'],
	['dns', 'DNS'],
	['settings.internal_agent_dns', 'Agent name settings'],
	['settings', 'Settings'],
	['service.system', 'System service'],
	['service.dashboard', 'Service dashboard entry'],
	['service', 'Service'],
	['certificates.provider', 'Certificate provider'],
	['captcha', 'CAPTCHA'],
	['geoip', 'GeoIP'],
	['sso', 'SSO'],
	['waf', 'WAF']
];

const words = (s: string) =>
	s
		.split(/[._]/)
		.filter(Boolean)
		.map((w) => nouns.find(([k]) => k === w)?.[1] ?? w)
		.join(' ');

/** An audit action in plain words, e.g. `ip_set.deleted` → "IP set deleted". */
export function actionText(action: string): string {
	if (whole[action]) return whole[action];
	const hit = nouns.find(([k]) => action === k || action.startsWith(k + '.'));
	const rest = hit ? action.slice(hit[0].length + 1) : action;
	const text = hit ? `${hit[1]} ${words(rest)}`.trim() : words(rest);
	return text.charAt(0).toUpperCase() + text.slice(1);
}

/** Who did it: every session is the one user. */
export function actorText(actor: string): string {
	if (actor.startsWith('session:')) return 'You';
	return (
		{ system: 'Hashi', console: 'Server console', anonymous: 'Unknown visitor' }[actor] ?? actor
	);
}
