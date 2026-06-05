import {
	Database,
	FileKey,
	Globe,
	KeyRound,
	Network,
	PartyPopper,
	Settings2,
	Shield,
	Sparkles
} from 'lucide-svelte';
import type { LucideIcon } from '$lib/icons';

export type SetupStepDef = {
	slug: string;
	title: string;
	description: string;
	icon: LucideIcon;
	optional?: boolean;
};

export const SETUP_STEPS: SetupStepDef[] = [
	{
		slug: 'bootstrap-access',
		title: 'Bootstrap Access',
		description: 'Sign in with Docker log credentials from a private network.',
		icon: KeyRound
	},
	{
		slug: 'base-settings',
		title: 'Base Settings',
		description: 'Root domain, admin URL, sync interval, and public page toggles.',
		icon: Settings2
	},
	{
		slug: 'dns-provider',
		title: 'DNS Provider',
		description: 'Connect Hetzner DNS and optionally import existing records.',
		icon: Globe
	},
	{
		slug: 'certificate-provider',
		title: 'Certificate Provider',
		description: 'Configure Google Trust Services ACME with DNS challenge.',
		icon: FileKey
	},
	{
		slug: 'traefik-connection',
		title: 'Traefik Connection',
		description: 'SSH to Traefik host, validate config paths and permissions.',
		icon: Network
	},
	{
		slug: 'firewall-host',
		title: 'Firewall Host',
		description: 'Linux firewall SSH host, managed subnets, and Traefik link.',
		icon: Shield
	},
	{
		slug: 'system-resource',
		title: 'Hashi System Resource',
		description: 'Create the admin domain resource and sync edge state.',
		icon: Database
	},
	{
		slug: 'passkey-and-vault',
		title: 'Passkey & Vault',
		description: 'Register passkey, configure vault, and confirm recovery key.',
		icon: KeyRound
	},
	{
		slug: 'optional',
		title: 'Optional Setup',
		description: 'OIDC, AdGuard, blocklists, notifications, GeoIP, and dashboard widgets.',
		icon: Sparkles,
		optional: true
	},
	{
		slug: 'complete',
		title: 'Complete',
		description: 'Setup finished — enter the admin shell.',
		icon: PartyPopper
	}
];

export function stepIndex(slug: string): number {
	return SETUP_STEPS.findIndex((s) => s.slug === slug);
}

export function stepBySlug(slug: string): SetupStepDef | undefined {
	return SETUP_STEPS.find((s) => s.slug === slug);
}

export function isStepComplete(slug: string, completed: string[]): boolean {
	return completed.includes(slug);
}
