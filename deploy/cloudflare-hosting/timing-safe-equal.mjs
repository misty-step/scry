const encoder = new TextEncoder();

export function timingSafeStringEqual(left, right, subtle = crypto.subtle) {
  const leftBytes = encoder.encode(left);
  const rightBytes = encoder.encode(right);
  const lengthsMatch = leftBytes.byteLength === rightBytes.byteLength;
  return lengthsMatch
    ? subtle.timingSafeEqual(leftBytes, rightBytes)
    : !subtle.timingSafeEqual(leftBytes, leftBytes);
}