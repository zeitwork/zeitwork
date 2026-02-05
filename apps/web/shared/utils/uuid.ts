const BASE58_ALPHABET = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz";

/**
 * Converts a UUID string to Base58 encoding.
 * Encodes the 16-byte hex value of the UUID.
 */
export function uuidToBase58(uuid: string): string {
  // Normalize: remove dashes and validate hex
  const hex = uuid.replace(/-/g, "").toLowerCase();
  if (!/^[0-9a-f]{32}$/.test(hex)) {
    throw new Error("Invalid UUID format");
  }

  // Parse hex to Uint8Array (16 bytes)
  const bytes = new Uint8Array(16);
  for (let i = 0; i < 32; i += 2) {
    bytes[i >> 1] = parseInt(hex.slice(i, i + 2), 16);
  }

  // Count leading zeros (each 0x00 byte becomes a '1' in Base58)
  let leadingZeros = 0;
  while (leadingZeros < 16 && bytes[leadingZeros] === 0) {
    leadingZeros++;
  }

  // Convert remaining bytes to BigInt
  let num = BigInt(0);
  for (let i = leadingZeros; i < 16; i++) {
    num = (num << BigInt(8)) | BigInt(bytes[i]);
  }

  // Encode to Base58
  let result = "";
  const base = BigInt(58);
  while (num > 0) {
    result = BASE58_ALPHABET[Number(num % base)] + result;
    num = num / base;
  }

  // Prepend '1' for each leading zero byte
  return "1".repeat(leadingZeros) + result || BASE58_ALPHABET[0];
}

/**
 * Converts a Base58 encoded UUID back to standard UUID format (with dashes).
 * Reverse of uuidToBase58.
 */
export function base58ToUuid(base58: string): string {
  if (!base58 || typeof base58 !== "string") {
    throw new Error("Invalid Base58 input");
  }

  // Validate characters and count leading '1's (representing 0x00 bytes)
  let leadingZeroBytes = 0;
  for (let i = 0; i < base58.length; i++) {
    const char = base58[i];
    if (char === "1" && i === leadingZeroBytes) {
      leadingZeroBytes++;
    } else if (!BASE58_ALPHABET.includes(char)) {
      throw new Error(`Invalid Base58 character: ${char}`);
    }
  }

  // Decode the non-'1' portion
  const dataPart = base58.slice(leadingZeroBytes);
  let num = BigInt(0);
  const base = BigInt(58);

  for (const char of dataPart) {
    num = num * base + BigInt(BASE58_ALPHABET.indexOf(char));
  }

  // Convert BigInt to byte array (big-endian)
  const bytes: number[] = [];
  let temp = num;
  while (temp > 0) {
    bytes.unshift(Number(temp & BigInt(0xff)));
    temp = temp >> BigInt(8);
  }

  // Prepend leading zero bytes
  const allBytes = new Array(leadingZeroBytes).fill(0).concat(bytes);

  // UUID must be exactly 16 bytes (128 bits)
  if (allBytes.length !== 16) {
    throw new Error(`Invalid Base58 UUID: expected 16 bytes, got ${allBytes.length}`);
  }

  // Convert to hex
  const hex = allBytes.map((b) => b.toString(16).padStart(2, "0")).join("");

  // Insert dashes: 8-4-4-4-12
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20, 32)}`;
}
