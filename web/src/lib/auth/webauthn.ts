// Browser side of the WebAuthn ceremonies. The server sends the options JSON produced by go-webauthn
// ({publicKey: …} with base64url fields); the browser's own parse/toJSON helpers do every conversion.

type Options = Record<string, unknown>;

function publicKey(options: Options): Record<string, unknown> {
	const pk = options.publicKey;
	if (!pk || typeof pk !== 'object') throw new Error('Hashi sent no passkey options.');
	return pk as Record<string, unknown>;
}

export function passkeysSupported(): boolean {
	return (
		typeof window !== 'undefined' &&
		window.isSecureContext &&
		typeof PublicKeyCredential !== 'undefined' &&
		typeof PublicKeyCredential.parseCreationOptionsFromJSON === 'function'
	);
}

export async function createPasskey(options: Options): Promise<Options> {
	const opts = PublicKeyCredential.parseCreationOptionsFromJSON(
		publicKey(options) as unknown as PublicKeyCredentialCreationOptionsJSON
	);
	const cred = (await navigator.credentials.create({
		publicKey: opts
	})) as PublicKeyCredential | null;
	if (!cred) throw new Error('No passkey was created.');
	return cred.toJSON() as unknown as Options;
}

export async function usePasskey(options: Options): Promise<Options> {
	const opts = PublicKeyCredential.parseRequestOptionsFromJSON(
		publicKey(options) as unknown as PublicKeyCredentialRequestOptionsJSON
	);
	const cred = (await navigator.credentials.get({ publicKey: opts })) as PublicKeyCredential | null;
	if (!cred) throw new Error('No passkey was used.');
	return cred.toJSON() as unknown as Options;
}

/** Message for a browser-side failure (prompt closed, timeout, no matching passkey). */
export function ceremonyText(e: unknown): string {
	if (e instanceof DOMException && (e.name === 'NotAllowedError' || e.name === 'AbortError')) {
		return 'The passkey prompt was closed or timed out. Try again, or use another passkey on this device.';
	}
	if (e instanceof DOMException && e.name === 'InvalidStateError') {
		return 'This authenticator already holds a passkey for Hashi.';
	}
	if (e instanceof Error) return e.message;
	return 'The passkey step failed. Nothing was changed.';
}
