import { defineConfig } from "astro/config";
import starlight from "@astrojs/starlight";
import tailwindcss from "@tailwindcss/vite";
import fs from "node:fs";

// Load Starlark TextMate grammar for syntax highlighting
const starlarkGrammar = JSON.parse(
  fs.readFileSync(
    new URL("./starlark.tmLanguage.json", import.meta.url),
    "utf-8"
  )
);

export default defineConfig({
  site: "https://albertocavalcante.github.io",
  base: "/bazelbump",

  vite: {
    plugins: [tailwindcss()],
  },

  integrations: [
    starlight({
      title: "bazelbump",
      description: "Bazel dependency upgrade tool with Starlark recipes",
      favicon: "/favicon.svg",
      lastUpdated: true,
      tableOfContents: { minHeadingLevel: 2, maxHeadingLevel: 3 },
      expressiveCode: {
        themes: ["github-dark", "github-light"],
        shiki: {
          langs: [starlarkGrammar],
          langAlias: {
            bzl: "starlark",
            bazel: "starlark",
            build: "starlark",
            star: "starlark",
          },
        },
        styleOverrides: {
          borderRadius: "0.625rem",
          codeFontFamily:
            "'JetBrains Mono', 'SF Mono', 'Fira Code', ui-monospace, monospace",
          codeFontSize: "0.875rem",
          codeLineHeight: "1.65",
        },
      },
      social: [
        {
          icon: "github",
          label: "GitHub",
          href: "https://github.com/albertocavalcante/bazelbump",
        },
      ],
      editLink: {
        baseUrl:
          "https://github.com/albertocavalcante/bazelbump/edit/main/docs-website/",
      },
      customCss: ["./src/styles/global.css"],
      sidebar: [
        {
          label: "Overview",
          items: [
            { label: "Introduction", slug: "introduction" },
            { label: "Why bazelbump?", slug: "why-bazelbump" },
          ],
        },
        {
          label: "Getting Started",
          items: [
            { label: "Installation", slug: "getting-started/installation" },
            { label: "Quick Start", slug: "getting-started/quickstart" },
            {
              label: "Your First Recipe",
              slug: "getting-started/first-recipe",
            },
          ],
        },
        {
          label: "Recipes",
          items: [
            { label: "Recipe Basics", slug: "recipes/basics" },
            { label: "Scan Steps", slug: "recipes/scan-steps" },
            { label: "Edit Steps", slug: "recipes/edit-steps" },
            { label: "Operations", slug: "recipes/operations" },
            { label: "Context & Outputs", slug: "recipes/context" },
          ],
        },
        {
          label: "Reference",
          items: [
            { label: "CLI Commands", slug: "reference/cli" },
            { label: "Builtins", slug: "reference/builtins" },
            { label: "Configuration", slug: "reference/configuration" },
          ],
        },
      ],
    }),
  ],
});
