// Tribute's donation webhooks → the repository's donations workflow, with no server of our
// own: a Cloudflare Worker (free plan). It checks Tribute's signature, keeps only what a goal
// needs (which goal, how much, the currency) and starts .github/workflows/donations.yml with
// a repository_dispatch. Nothing about the donor (name, Telegram id, message, e-mail) leaves
// the Worker.
//
// Secrets (Cloudflare → the Worker → Settings → Variables and Secrets):
//   TRIBUTE_API_KEY  Tribute → Settings → API keys; it signs the webhooks
//   GITHUB_TOKEN     a fine-grained token for this repository only, "Contents: Read and write"
// Variable:
//   GITHUB_REPO      Miroshka000/mikan

const DONATIONS = new Set(['new_donation', 'recurrent_donation']);
const MAX_BODY = 64 * 1024;

export default {
	async fetch(request, env) {
		if (request.method !== 'POST') {
			return json({ service: 'mikan donations' });
		}
		const body = await request.arrayBuffer();
		if (body.byteLength > MAX_BODY) {
			return json({ error: 'too_big' }, 413);
		}
		if (!env.TRIBUTE_API_KEY || !(await signedByTribute(env.TRIBUTE_API_KEY, body, request.headers.get('trbt-signature')))) {
			return json({ error: 'unauthorized' }, 401);
		}
		let event;
		try {
			event = JSON.parse(new TextDecoder().decode(body));
		} catch {
			return json({ error: 'bad_json' }, 400);
		}
		// Subscriptions and the rest are not ours (yet): a 200 so Tribute does not retry them.
		if (!DONATIONS.has(event?.name)) {
			return json({ status: 'ignored' });
		}
		const p = event.payload ?? {};
		if (!Number.isInteger(p.amount) || p.amount <= 0 || typeof p.currency !== 'string') {
			return json({ error: 'bad_payload' }, 400);
		}
		// A retry has a new sent_at and the same event: the key is of what does not change, so
		// the workflow counts it once.
		const key = await sha256(`${event.name}\n${event.created_at}\n${JSON.stringify(p)}`);
		const res = await fetch(`https://api.github.com/repos/${env.GITHUB_REPO}/dispatches`, {
			method: 'POST',
			headers: {
				Authorization: `Bearer ${env.GITHUB_TOKEN}`,
				Accept: 'application/vnd.github+json',
				'X-GitHub-Api-Version': '2022-11-28',
				'User-Agent': 'mikan-donations-worker',
			},
			body: JSON.stringify({
				event_type: 'tribute-donation',
				client_payload: {
					key,
					request_id: Number.isInteger(p.donation_request_id) ? p.donation_request_id : 0,
					name: typeof p.donation_name === 'string' ? p.donation_name.slice(0, 200) : '',
					amount: p.amount,
					currency: p.currency.toUpperCase(),
				},
			}),
		});
		// GitHub did not take it: an error, and Tribute retries later.
		if (!res.ok) {
			return json({ error: 'github', status: res.status }, 502);
		}
		return json({ status: 'ok' });
	},
};

// signedByTribute checks trbt-signature, an HMAC-SHA256 of the raw body with the API key.
// Tribute's docs do not say how it is written: hex and base64 are both taken.
async function signedByTribute(secret, body, header) {
	const sig = (header ?? '').trim();
	if (!sig) return false;
	const key = await crypto.subtle.importKey('raw', new TextEncoder().encode(secret), { name: 'HMAC', hash: 'SHA-256' }, false, ['sign']);
	const mac = new Uint8Array(await crypto.subtle.sign('HMAC', key, body));
	const hex = [...mac].map((b) => b.toString(16).padStart(2, '0')).join('');
	const b64 = btoa(String.fromCharCode(...mac));
	return same(sig.toLowerCase(), hex) || same(sig, b64);
}

// same compares in constant time for strings of one length.
function same(a, b) {
	if (a.length !== b.length) return false;
	let diff = 0;
	for (let i = 0; i < a.length; i++) diff |= a.charCodeAt(i) ^ b.charCodeAt(i);
	return diff === 0;
}

async function sha256(text) {
	const d = new Uint8Array(await crypto.subtle.digest('SHA-256', new TextEncoder().encode(text)));
	return [...d].map((b) => b.toString(16).padStart(2, '0')).join('');
}

function json(data, status = 200) {
	return new Response(JSON.stringify(data), { status, headers: { 'content-type': 'application/json' } });
}
