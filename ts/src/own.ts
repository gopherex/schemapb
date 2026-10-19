/**
 * Sets an own enumerable property. Keys come from user data, where
 * "__proto__" is an ordinary key: plain assignment on an object without
 * that own key would invoke the prototype setter instead, and the entry
 * would vanish from whatever is built next (CEL inputs, wire values, baked
 * snapshots), escaping the checks that read it there.
 */
export function setOwn<V>(obj: Record<string, V>, key: string, value: V): void {
  Object.defineProperty(obj, key, { value, enumerable: true, writable: true, configurable: true });
}
