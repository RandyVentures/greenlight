import test from 'node:test';
import assert from 'node:assert/strict';
import { initial, counts, validate, verdict, sessionToken } from './model.mjs';

test('purchase providers require appropriate evidence without blocking optional catalogs', () => {
  assert.equal(validate(initial), '');
  assert.match(validate({ ...initial, payments: 'storekit' }), /product ID/);
  assert.equal(validate({ ...initial, payments: 'storekit', products: [{ id: 'pro' }] }), '');
  assert.equal(
    validate({
      ...initial,
      payments: 'revenuecat',
      products: [
        { id: 'monthly', entitlement: 'pro' },
        { id: 'racepass', entitlement: '' },
      ],
    }),
    '',
  );
  assert.match(
    validate({ ...initial, payments: 'revenuecat', products: [{ id: 'pro' }] }),
    /entitlement/,
  );
});
test('missing screenshot and live ASC inputs do not silently run', () => {
  assert.match(validate({ ...initial, screenshots: true }), /screenshot/);
  assert.match(validate({ ...initial, live_asc: true }), /version resource/);
});
test('missing coverage is separate from passing checks', () => {
  assert.deepEqual(counts([{ status: 'passed' }, { status: 'unverified' }]), {
    passed: 1,
    unverified: 1,
    failed: 0,
    suspected: 0,
  });
  assert.equal(verdict({ incomplete: true }), 'Some evidence is missing.');
});
test('session token stays tab scoped and requires a generated token shape', () => {
  const values = new Map();
  const storage = { getItem: (k) => values.get(k), setItem: (k, v) => values.set(k, v) };
  assert.equal(sessionToken({ hash: '#untrusted' }, storage), '');
  assert.equal(sessionToken({ hash: '#' + 'a'.repeat(64) }, storage), 'a'.repeat(64));
  assert.equal(sessionToken({ hash: '' }, storage), 'a'.repeat(64));
});
