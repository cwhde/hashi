function base64UrlToBuffer(value: string): ArrayBuffer {
	const padded = value.replace(/-/g, '+').replace(/_/g, '/');
	const pad = padded.length % 4 === 0 ? '' : '='.repeat(4 - (padded.length % 4));
	const binary = atob(padded + pad);
	const bytes = new Uint8Array(binary.length);
	for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
	return bytes.buffer;
}

function bufferToBase64Url(buffer: ArrayBuffer): string {
	const bytes = new Uint8Array(buffer);
	let binary = '';
	for (const byte of bytes) binary += String.fromCharCode(byte);
	return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

function decodeCreationOptions(options: Record<string, unknown>): PublicKeyCredentialCreationOptions {
	const publicKey = { ...(options as Record<string, unknown>) };
	if (typeof publicKey.challenge === 'string') {
		publicKey.challenge = base64UrlToBuffer(publicKey.challenge);
	}
	if (publicKey.user && typeof publicKey.user === 'object') {
		const user = { ...(publicKey.user as Record<string, unknown>) };
		if (typeof user.id === 'string') user.id = base64UrlToBuffer(user.id);
		publicKey.user = user;
	}
	if (Array.isArray(publicKey.excludeCredentials)) {
		publicKey.excludeCredentials = publicKey.excludeCredentials.map((cred) => {
			const item = { ...(cred as Record<string, unknown>) };
			if (typeof item.id === 'string') item.id = base64UrlToBuffer(item.id);
			return item;
		});
	}
	return publicKey as unknown as PublicKeyCredentialCreationOptions;
}

function decodeRequestOptions(options: Record<string, unknown>): PublicKeyCredentialRequestOptions {
	const publicKey = { ...(options as Record<string, unknown>) };
	if (typeof publicKey.challenge === 'string') {
		publicKey.challenge = base64UrlToBuffer(publicKey.challenge);
	}
	if (Array.isArray(publicKey.allowCredentials)) {
		publicKey.allowCredentials = publicKey.allowCredentials.map((cred) => {
			const item = { ...(cred as Record<string, unknown>) };
			if (typeof item.id === 'string') item.id = base64UrlToBuffer(item.id);
			return item;
		});
	}
	return publicKey as unknown as PublicKeyCredentialRequestOptions;
}

export function isWebAuthnSupported(): boolean {
	return typeof window !== 'undefined' && !!window.PublicKeyCredential;
}

export async function isPrfSupported(): Promise<boolean> {
	if (!isWebAuthnSupported()) return false;
	try {
		return await PublicKeyCredential.getClientCapabilities().then(
			(caps) => caps['extension:prf'] === true
		);
	} catch {
		return false;
	}
}

export async function registerPasskeyFromServerOptions(
	options: Record<string, unknown>
): Promise<PublicKeyCredential> {
	const credential = await navigator.credentials.create({
		publicKey: decodeCreationOptions(options)
	});
	if (!credential) throw new Error('Passkey registration was cancelled.');
	return credential as PublicKeyCredential;
}

export async function loginPasskeyFromServerOptions(
	options: Record<string, unknown>
): Promise<PublicKeyCredential> {
	const credential = await navigator.credentials.get({
		publicKey: decodeRequestOptions(options)
	});
	if (!credential) throw new Error('Passkey login was cancelled.');
	return credential as PublicKeyCredential;
}

export function serializeRegistration(credential: PublicKeyCredential): Record<string, unknown> {
	const response = credential.response as AuthenticatorAttestationResponse;
	return {
		id: credential.id,
		rawId: bufferToBase64Url(credential.rawId),
		type: credential.type,
		response: {
			clientDataJSON: bufferToBase64Url(response.clientDataJSON),
			attestationObject: bufferToBase64Url(response.attestationObject)
		}
	};
}

export function serializeAuthentication(credential: PublicKeyCredential): Record<string, unknown> {
	const response = credential.response as AuthenticatorAssertionResponse;
	return {
		id: credential.id,
		rawId: bufferToBase64Url(credential.rawId),
		type: credential.type,
		response: {
			clientDataJSON: bufferToBase64Url(response.clientDataJSON),
			authenticatorData: bufferToBase64Url(response.authenticatorData),
			signature: bufferToBase64Url(response.signature),
			userHandle: response.userHandle ? bufferToBase64Url(response.userHandle) : null
		}
	};
}

export function extractPrfOutput(credential: PublicKeyCredential): string | null {
	const extensions = credential.getClientExtensionResults() as {
		prf?: { results?: { first?: ArrayBuffer } };
	};
	const first = extensions.prf?.results?.first;
	return first ? bufferToBase64Url(first) : null;
}
