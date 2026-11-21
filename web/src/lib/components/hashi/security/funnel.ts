// The Challenges card on the security dashboard (06 §6.1).
export type FunnelStep = { k: string; n: number; tone: string; width: number };

export function challengeFunnel(f: {
	challenged: number;
	solved: number;
	banned: number;
}): FunnelStep[] {
	const steps = [
		{ k: 'Challenged', n: f.challenged, tone: 'bg-warn/70' },
		{ k: 'Solved', n: f.solved, tone: 'bg-ok/70' },
		{ k: 'Not solved yet', n: Math.max(0, f.challenged - f.solved - f.banned), tone: 'bg-idle' },
		{ k: 'Banned', n: f.banned, tone: 'bg-bad/70' }
	];
	const max = Math.max(1, ...steps.map((s) => s.n));
	return steps.map((s) => ({ ...s, width: (s.n / max) * 100 }));
}
