import { defineConfig } from "mygo-cli";

export default defineConfig({
  name: "my-app-npx",
  identifier: "com.example.myappnpx",
  version: "0.1.0",
  // `mygo dev` runs devCommand and loads devUrl; `mygo build` runs
  // buildCommand and embeds frontendDist into the app.
  devUrl: "http://localhost:5173",
  devCommand: "yarn run dev:web",
  buildCommand: "yarn run build:web",
  frontendDist: "dist",
  bindings: "src/mygo.ts",
  out: "build",
});
