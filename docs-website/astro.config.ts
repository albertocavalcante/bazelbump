import { defineConfig } from "astro/config";
import starlight from "@astrojs/starlight";

export default defineConfig({
  site: "https://anthropics.github.io/bazelbump",
  base: "/bazelbump",
  integrations: [
    starlight({
      title: "bazelbump",
      description: "Bazel dependency upgrade tool with Starlark recipes",
      social: {
        github: "https://github.com/anthropics/bazelbump",
      },
      editLink: {
        baseUrl: "https://github.com/anthropics/bazelbump/edit/main/docs-website/",
      },
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
            { label: "Your First Recipe", slug: "getting-started/first-recipe" },
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
