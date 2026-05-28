import { defineConfig } from "tsup";

// esbuild resolves the .ts import extensions emitted by protoc-gen-es; tsc
// generates the .d.ts files.
export default defineConfig({
  entry: ["index.ts"],
  format: ["esm"],
  dts: true,
  clean: true,
  sourcemap: true,
  // Ship the wasm + Go loader next to the bundle so `new URL("./schemapb.wasm",
  // import.meta.url)` resolves from dist/ exactly as it does from source.
  onSuccess: "cp schemapb.wasm wasm_exec.js dist/",
});
