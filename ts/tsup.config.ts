import { defineConfig } from "tsup";

export default defineConfig({
  // The generated *_pb modules are entries too, exposed as
  // @gopherex/schemapb/schemapb/*. ESM splitting puts the shared code in one
  // chunk, so the root export and the subpaths share one instance of each
  // descriptor.
  entry: ["src/index.ts", "src/zod.ts", "src/gen/schemapb/*_pb.ts"],
  format: ["esm"],
  dts: true,
  sourcemap: true,
  clean: true,
});
