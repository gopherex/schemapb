import { defineConfig } from "tsup";

// React and the core SDK stay external (peer deps); only this package's own
// code is bundled.
export default defineConfig({
  entry: ["index.ts"],
  format: ["esm"],
  dts: true,
  clean: true,
  sourcemap: true,
  external: ["react", "@stroppy-io/schemapb"],
});
