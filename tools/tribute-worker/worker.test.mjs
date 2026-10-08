// node --test tools/tribute-worker/worker.test.mjs
import assert from 'node:assert/strict';
import { createHmac } from 'node:crypto';
import test from 'node:test';
import worker from './worker.js';

const env = { TRIBUTE_API_KEY: 'tribute-test-key', GITHUB_TOKEN: 'gh-test-token', GITHUB_REPO: 'Miroshka000/mikan' };

function donation(sentAt = '2026-10-08T10:00:00.5Z', extra = {}) {
	return JSON.stringify({
		name: 'new_donation',
		created_at: '2026-10-08T10:00:00Z',
		sent_at: sentAt,
		payload: { donation_request_id: 42, donation_name: 'День на продление', amount: 100, currency: 'usd', anonymously: false, telegram_user_id: 123, telegram_username: 'someone', message: 'hi', ...extra },
	});
}

const sign = (body, enc = 'hex') => createHmac('sha256', env.TRIBUTE_API_KEY).update(body).digest(enc);

async function post(body, signature) {
	const sent = [];
	globalThis.fetch = async (url, init) => {
		sent.push({ url, init });
		return new Response(null, { status: globalThis.githubStatus ?? 204 });
	};
	const headers = signature === undefined ? {} : { 'trbt-signature': signature };
	const res = await worker.fetch(new Request('https://w.example/', { method: 'POST', body, headers }), env);
	return { status: res.status, body: await res.json(), sent };
}

test('a signed donation starts the workflow with nothing about the donor', async () => {
	const body = donation();
	const r = await post(body, sign(body));
	assert.equal(r.status, 200);
	assert.equal(r.sent.length, 1);
	assert.equal(r.sent[0].url, 'https://api.github.com/repos/Miroshka000/mikan/dispatches');
	const sent = JSON.parse(r.sent[0].init.body);
	assert.equal(sent.event_type, 'tribute-donation');
	assert.deepEqual(Object.keys(sent.client_payload).sort(), ['amount', 'currency', 'key', 'name', 'request_id']);
	assert.equal(sent.client_payload.currency, 'USD');
	assert.match(sent.client_payload.key, /^[0-9a-f]{64}$/);
	assert.ok(!r.sent[0].init.body.includes('someone') && !r.sent[0].init.body.includes('123'));
});

test('a retry (new sent_at) has the same key; base64 signatures pass too', async () => {
	const a = donation('2026-10-08T10:00:00.5Z');
	const b = donation('2026-10-08T10:05:00.5Z');
	const ka = JSON.parse((await post(a, sign(a))).sent[0].init.body).client_payload.key;
	const kb = JSON.parse((await post(b, sign(b, 'base64'))).sent[0].init.body).client_payload.key;
	assert.equal(ka, kb);
});

test('bad or missing signatures, other events and bad payloads start nothing', async () => {
	const body = donation();
	for (const sig of [undefined, '', 'deadbeef', sign(body + ' ')]) {
		const r = await post(body, sig);
		assert.equal(r.status, 401);
		assert.equal(r.sent.length, 0);
	}
	const sub = JSON.stringify({ name: 'new_subscription', created_at: 'x', payload: {} });
	const rs = await post(sub, sign(sub));
	assert.equal(rs.status, 200);
	assert.equal(rs.sent.length, 0);
	const bad = donation(undefined, { amount: -5 });
	assert.equal((await post(bad, sign(bad))).status, 400);
});

test('GitHub refusing it is an error, so Tribute retries', async () => {
	globalThis.githubStatus = 500;
	const body = donation();
	assert.equal((await post(body, sign(body))).status, 502);
	globalThis.githubStatus = undefined;
});
