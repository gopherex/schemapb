import { defineConfig } from "tsup";

// esbuild resolves the .ts import extensions emitted by protoc-gen-es; tsc
// generates the .d.ts files.
export default defineConfig({
  entry: ["index.ts"],
  format: ["esm"],
  dts: true,
  clean: true,
  sourcemap: true,
});
