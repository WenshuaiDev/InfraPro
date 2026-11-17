#!/usr/bin/env node
import { cp, mkdir, stat } from "node:fs/promises";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { pack, validate } from "./package.mjs";
import { startServer } from "./server.mjs";
const [command, ...args] = process.argv.slice(2),
  options = {},
  pos = [];
for (let i = 0; i < args.length; i++) {
  if (args[i].startsWith("--")) {
    const key = args[i].slice(2);
    options[key] =
      args[i + 1] && !args[i + 1].startsWith("--") ? args[++i] : true;
  } else pos.push(args[i]);
}
try {
  switch (command) {
    case "init": {
      const dir = resolve(pos[0] ?? "display-app");
      try {
        await stat(dir);
        throw new Error("target already exists");
      } catch (e) {
        if (e.code !== "ENOENT") throw e;
      }
      await mkdir(dir, { recursive: true });
      if(options.template && !["monitoring-read","engineering-structure"].includes(options.template))throw new Error("Unknown template");
      await cp(fileURLToPath(new URL(options.template === "engineering-structure" ? "../lib/templates-structure/" : options.template === "monitoring-read" ? "../lib/templates-read/" : "../templates/", import.meta.url)), dir, {
        recursive: true,
      });
      console.log(
        `Created ${dir}. Install the supplied SDK tarball and TypeScript, then npm run build.`,
      );
      break;
    }
    case "validate": {
      const r = await validate(pos[0] ?? "dist", options.manifest);
      console.log(
        JSON.stringify({
          valid: true,
          app_key: r.manifest.app_key,
          files: r.files.size,
        }),
      );
      break;
    }
    case "pack":
      console.log(
        JSON.stringify(
          await pack(
            pos[0] ?? "dist",
            options.manifest ?? "display.manifest.json",
            options.out ?? "display.zip",
          ),
        ),
      );
      break;
    case "dev":
    case "login":
    case "upload": {
      if (command === "upload") await validate(pos[0] ?? "display.zip");
      const server = await startServer({
        ...options,
        dir: pos[0] ?? "dist",
        real: command === "login" || !!options.real,
        upload: command === "upload",
      });
      console.log(`Local shell: ${server.url}`);
      if (server.loginURL)
        console.log(
          `Open in your signed-in console browser:\n${server.loginURL}`,
        );
      else console.log(`SIMULATED DATA: ${options.scenario ?? "normal"}`);
      process.on("SIGINT", () => {
        server.close();
        process.exit(0);
      });
      process.on("SIGTERM", () => {
        server.close();
        process.exit(0);
      });
      if (command === "upload") {
        const timer = setTimeout(() => {
          server.close();
          console.error("Pairing timed out");
          process.exitCode = 1;
        }, 120000);
        server.authorized.then(()=>clearTimeout(timer));
        try {
          const result = await server.upload(pos[0] ?? "display.zip");
          console.log(JSON.stringify(result));
        } finally {
          clearTimeout(timer);
          server.close();
        }
      }
      break;
    }
    default:
      console.log(
        "infrapro-display init DIR [--template monitoring-read|engineering-structure] | validate DIR|ZIP [--manifest FILE] | pack DIR --manifest FILE --out ZIP | dev DIR [--scenario normal|empty|pending|partial|disconnected|revoked|unavailable|overlap|scope-changed] | login DIR --console URL --runtime ORIGIN --instance UUID --config UUID | upload ZIP --console URL --runtime ORIGIN --instance UUID [--resume UPLOAD_UUID]",
      );
      if (command) process.exitCode = 1;
  }
} catch (error) {
  console.error(
    JSON.stringify({
      code: error.code ?? "command_failed",
      message: error.message,
      issues: error.issues,
      upload_id: error.upload_id,
    }),
  );
  process.exitCode = 1;
}
