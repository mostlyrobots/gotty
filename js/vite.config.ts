import { defineConfig } from "vite";
import { svelte } from "@sveltejs/vite-plugin-svelte";
import tailwindcss from "@tailwindcss/vite";
import path from "path";

export default defineConfig({
    plugins: [
        tailwindcss(),
        svelte(),
    ],
    build: {
        outDir: path.resolve(__dirname, "../bindata/static"),
        emptyOutDir: false,
        sourcemap: true,
        cssCodeSplit: false,
        lib: {
            entry: path.resolve(__dirname, "src/main.ts"),
            name: "gotty",
            formats: ["iife"],
            fileName: () => "js/gotty.js",
        },
        rollupOptions: {
            output: {
                assetFileNames: (assetInfo) => {
                    if (assetInfo.name && assetInfo.name.endsWith(".css")) {
                        return "css/index.css";
                    }
                    return "assets/[name][extname]";
                },
            },
        },
    },
});
