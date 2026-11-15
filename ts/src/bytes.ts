/**
 * Reports whether v is a Uint8Array from any realm. `instanceof Uint8Array`
 * sees only this realm's constructor, so bytes made by jsdom's TextEncoder, an
 * iframe, a worker or node:vm would be rejected as plain objects.
 * ArrayBuffer.isView reads the internal slot, which works across realms and
 * which a plain object cannot fake; the tag then tells Uint8Array from the
 * other views.
 */
export function isBytes(v: unknown): v is Uint8Array {
  return ArrayBuffer.isView(v) && Object.prototype.toString.call(v) === "[object Uint8Array]";
}

/**
 * Returns v as a Uint8Array of this realm, or undefined when v is not bytes.
 * A foreign array is copied where it enters stored data: protobuf-es and
 * other code of this realm test `instanceof`, so equality and field checks
 * would otherwise fail on it further down.
 */
export function localBytes(v: unknown): Uint8Array | undefined {
  if (v instanceof Uint8Array) {
    return v;
  }

  return isBytes(v) ? new Uint8Array(v) : undefined;
}
