// Bech32 (BIP-173) decoding for Cosmos account addresses.
// The faucet uses it to reject addresses with a bad checksum before any chain call.

const CHARSET = 'qpzry9x8gf2tvdw0s3jn54khce6mua7l';
const GENERATOR = [0x3b6a57b2, 0x26508e6d, 0x1ea119fa, 0x3d4233dd, 0x2a1462b3];

function polymod(values) {
  let chk = 1;
  for (const v of values) {
    const top = chk >>> 25;
    chk = ((chk & 0x1ffffff) << 5) ^ v;
    for (let i = 0; i < 5; i++) {
      if ((top >>> i) & 1) chk ^= GENERATOR[i];
    }
  }
  return chk >>> 0;
}

function hrpExpand(hrp) {
  const out = [];
  for (const c of hrp) out.push(c.charCodeAt(0) >> 5);
  out.push(0);
  for (const c of hrp) out.push(c.charCodeAt(0) & 31);
  return out;
}

function convertBits(data, from, to, pad) {
  let acc = 0;
  let bits = 0;
  const out = [];
  const maxv = (1 << to) - 1;
  for (const v of data) {
    if (v < 0 || v >> from !== 0) return null;
    acc = (acc << from) | v;
    bits += from;
    while (bits >= to) {
      bits -= to;
      out.push((acc >> bits) & maxv);
    }
  }
  if (pad) {
    if (bits > 0) out.push((acc << (to - bits)) & maxv);
  } else if (bits >= from || ((acc << (to - bits)) & maxv) !== 0) {
    return null;
  }
  return out;
}

/**
 * Decode a bech32 string. Returns { hrp, bytes } or throws with a reason.
 * Rejects mixed case, unknown characters, and checksum mismatches.
 */
function decode(str) {
  if (typeof str !== 'string' || str.length < 8 || str.length > 90) {
    throw new Error('length out of range');
  }
  if (str !== str.toLowerCase() && str !== str.toUpperCase()) {
    throw new Error('mixed case');
  }
  const s = str.toLowerCase();
  const sep = s.lastIndexOf('1');
  if (sep < 1 || sep + 7 > s.length) throw new Error('bad separator position');
  const hrp = s.slice(0, sep);
  for (const c of hrp) {
    const code = c.charCodeAt(0);
    if (code < 33 || code > 126) throw new Error('invalid character in prefix');
  }
  const data = [];
  for (const c of s.slice(sep + 1)) {
    const idx = CHARSET.indexOf(c);
    if (idx === -1) throw new Error('invalid character in data');
    data.push(idx);
  }
  if (polymod([...hrpExpand(hrp), ...data]) !== 1) throw new Error('checksum mismatch');
  const payload = convertBits(data.slice(0, -6), 5, 8, false);
  if (payload === null) throw new Error('invalid padding');
  return { hrp, bytes: Uint8Array.from(payload) };
}

/** Encode bytes under a prefix. Used by tests and tooling. */
function encode(hrp, bytes) {
  const data = convertBits(Array.from(bytes), 8, 5, true);
  const values = [...hrpExpand(hrp), ...data, 0, 0, 0, 0, 0, 0];
  const mod = polymod(values) ^ 1;
  const checksum = [];
  for (let i = 0; i < 6; i++) checksum.push((mod >>> (5 * (5 - i))) & 31);
  return hrp + '1' + [...data, ...checksum].map((v) => CHARSET[v]).join('');
}

/**
 * True when the address is a valid bech32 account address for the given prefix:
 * correct checksum and a 20-byte payload, as Cosmos SDK account addresses are.
 */
function isAccountAddress(address, prefix = 'cosmos') {
  try {
    const { hrp, bytes } = decode(address);
    return hrp === prefix && bytes.length === 20;
  } catch {
    return false;
  }
}

module.exports = { decode, encode, isAccountAddress };
