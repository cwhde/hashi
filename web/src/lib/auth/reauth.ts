import { api } from '$lib/api/client';
import {
	isWebAuthnSupported,
	loginPasskeyFromServerOptions,
	serializeAuthentication
} from '$lib/auth/webauthn';

export async function performPasskeyReauthentication(): Promise<boolean> {
	if (!isWebAuthnSupported()) {
		return false;
	}

	const begin = await api.reauthenticateBegin();
	const options = begin.options as Record<string, unknown> | undefined;
	const challengeSessionId = String(begin.challengeSessionId ?? begin.sessionId ?? '');
	if (!options || !challengeSessionId) {
		return false;
	}

	const credential = await loginPasskeyFromServerOptions(options);
	await api.reauthenticateComplete(serializeAuthentication(credential), challengeSessionId);
	return true;
}
