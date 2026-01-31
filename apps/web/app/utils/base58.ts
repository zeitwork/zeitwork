const ALPHABET = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz";
const BASE = BigInt(58);

/**
 * Encode a UUID as a Base58-encoded string for shorter, URL-friendly IDs.
 * @param uuid - A UUID string (with or without dashes)
 * @returns Base58-encoded string
 */
export function uuidToB58(uuid: string): string {
  // Remove dashes from UUID
  const hex = uuid.replace(/-/g, "");

  // Validate hex string
  if (!/^[0-9a-fA-F]{32}$/.test(hex)) {
    throw new Error("Invalid UUID format");
  }

  // Convert hex string to BigInt
  let num = BigInt("0x" + hex);

  // Convert to base58
  let encoded = "";
  while (num > 0) {
    const remainder = Number(num % BASE);
    encoded = ALPHABET[remainder]! + encoded;
    num = num / BASE;
  }

  return encoded || ALPHABET[0]!;
}

/**
 * Decode a Base58-encoded string back to a UUID.
 * @param b58 - Base58-encoded string
 * @returns UUID string with dashes (formatted as xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx)
 */
export function b58ToUuid(b58: string): string {
  // Convert base58 to big integer
  let num = BigInt(0);
  for (const char of b58) {
    const index = ALPHABET.indexOf(char);
    if (index === -1) {
      throw new Error(`Invalid Base58 character: ${char}`);
    }
    num = num * BASE + BigInt(index);
  }

  // Convert to hex string (pad to 32 characters)
  let hex = num.toString(16).padStart(32, "0");

  // Format as UUID with dashes
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}
