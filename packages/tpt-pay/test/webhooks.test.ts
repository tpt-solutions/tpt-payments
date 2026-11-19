import { createHmac } from 'node:crypto';
import { constructEvent, SignatureVerificationError, verifySignature } from '../src/webhooks';

const SECRET = 'whsec_test_secret';

function sign(payload: string, secret = SECRET, t = Math.floor(Date.now() / 1000)): string {
  const v1 = createHmac('sha256', secret).update(`${t}.${payload}`).digest('hex');
  return `t=${t},v1=${v1}`;
}

describe('verifySignature', () => {
  it('accepts a valid delivery', () => {
    const payload = '{"id":"evt_1","type":"payment_intent.succeeded"}';
    expect(() => verifySignature(payload, sign(payload), SECRET)).not.toThrow();
  });

  it('rejects a tampered body', () => {
    const header = sign('{"amount":1000}');
    expect(() => verifySignature('{"amount":9999}', header, SECRET)).toThrow(/no signature matches/);
  });

  it('rejects the wrong secret', () => {
    const payload = '{}';
    expect(() => verifySignature(payload, sign(payload, 'whsec_other'), SECRET)).toThrow(SignatureVerificationError);
  });

  it('rejects stale timestamps', () => {
    const payload = '{}';
    const old = Math.floor(Date.now() / 1000) - 3600;
    expect(() => verifySignature(payload, sign(payload, SECRET, old), SECRET)).toThrow(/too old/);
  });

  it('rejects future timestamps', () => {
    const payload = '{}';
    const future = Math.floor(Date.now() / 1000) + 3600;
    expect(() => verifySignature(payload, sign(payload, SECRET, future), SECRET)).toThrow(/in the future/);
  });

  it('accepts any matching v1 during secret rotation', () => {
    const payload = '{"id":"evt_rot"}';
    const t = Math.floor(Date.now() / 1000);
    const newSig = createHmac('sha256', 'whsec_new').update(`${t}.${payload}`).digest('hex');
    const oldSig = createHmac('sha256', 'whsec_old').update(`${t}.${payload}`).digest('hex');
    const header = `t=${t},v1=${newSig},v1=${oldSig}`;
    expect(() => verifySignature(payload, header, 'whsec_new')).not.toThrow();
    expect(() => verifySignature(payload, header, 'whsec_old')).not.toThrow();
  });

  it('verifies raw bytes exactly as received', () => {
    const bytes = new Uint8Array([0xff, 0xfe, 0x20, 0x7b, 0x7d]);
    const t = Math.floor(Date.now() / 1000);
    const v1 = createHmac('sha256', SECRET)
      .update(`${t}.`)
      .update(Buffer.from(bytes))
      .digest('hex');
    expect(() => verifySignature(bytes, `t=${t},v1=${v1}`, SECRET)).not.toThrow();
  });

  it.each([
    ['', /missing timestamp/],
    ['t=123', /missing v1/],
    ['v1=abc', /missing timestamp/],
    ['t=1,t=2,v1=abc', /duplicate timestamp/],
    ['t=notanumber,v1=abc', /malformed timestamp/],
  ])('rejects malformed header %j', (header, pattern) => {
    expect(() => verifySignature('{}', header, SECRET)).toThrow(pattern);
  });

  it('honours a custom tolerance', () => {
    const payload = '{}';
    const t = 1_000_000;
    const header = sign(payload, SECRET, t);
    expect(() => verifySignature(payload, header, SECRET, { now: t + 100, toleranceSeconds: 300 })).not.toThrow();
    expect(() => verifySignature(payload, header, SECRET, { now: t + 400, toleranceSeconds: 300 })).toThrow(/too old/);
  });
});

describe('constructEvent', () => {
  it('returns the parsed event for a valid delivery', () => {
    const payload = JSON.stringify({ id: 'evt_1', type: 'payment_intent.succeeded' });
    const event = constructEvent<{ id: string }>(payload, sign(payload), SECRET);
    expect(event.id).toBe('evt_1');
  });

  it('throws without parsing when verification fails', () => {
    const payload = '{"id":"evt_1"}';
    expect(() => constructEvent(payload, sign('{"id":"other"}'), SECRET)).toThrow(SignatureVerificationError);
  });
});
