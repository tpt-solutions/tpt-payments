// Run with: node --test
const test = require('node:test');
const assert = require('node:assert/strict');
const { decode, encode, isAccountAddress } = require('./bech32');

test('BIP-173 vector: empty payload with prefix "a" decodes', () => {
  const { hrp, bytes } = decode('A12UEL5L');
  assert.equal(hrp, 'a');
  assert.equal(bytes.length, 0);
});

test('encode then decode round-trips a 20-byte account payload', () => {
  const payload = Uint8Array.from({ length: 20 }, (_, i) => i * 7 + 1);
  const address = encode('cosmos', payload);
  const decoded = decode(address);
  assert.equal(decoded.hrp, 'cosmos');
  assert.deepEqual(Array.from(decoded.bytes), Array.from(payload));
  assert.equal(isAccountAddress(address), true);
});

test('a single corrupted character fails the checksum', () => {
  const address = encode('cosmos', new Uint8Array(20).fill(0x11));
  const last = address[address.length - 1];
  const replacement = last === 'q' ? 'p' : 'q';
  const corrupted = address.slice(0, -1) + replacement;
  assert.throws(() => decode(corrupted), /checksum mismatch/);
  assert.equal(isAccountAddress(corrupted), false);
});

test('the wrong prefix is rejected', () => {
  const address = encode('osmo', new Uint8Array(20).fill(0x22));
  assert.equal(isAccountAddress(address, 'cosmos'), false);
  assert.equal(isAccountAddress(address, 'osmo'), true);
});

test('payloads other than 20 bytes are not account addresses', () => {
  assert.equal(isAccountAddress(encode('cosmos', new Uint8Array(32).fill(1))), false);
});

test('mixed case, bad characters, and bad length are refused', () => {
  const address = encode('cosmos', new Uint8Array(20).fill(3));
  const mixed = address[0].toUpperCase() + address.slice(1);
  assert.throws(() => decode(mixed), /mixed case/);
  assert.throws(() => decode('cosmos1bioi'), /separator|invalid character|checksum|length/);
  assert.throws(() => decode('cosmos1'), /length/);
  assert.equal(isAccountAddress('cosmos1qyqqq'), false);
  assert.equal(isAccountAddress(''), false);
  assert.equal(isAccountAddress(undefined), false);
});
