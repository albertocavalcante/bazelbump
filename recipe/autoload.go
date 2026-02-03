// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package recipe

// AutoloadMigrationRecipe returns the built-in recipe for migrating
// native rules to explicit load statements (required for Bazel 9.0+).
//
// This recipe:
// 1. Scans BUILD files for native rule usage without load statements
// 2. Adds required bazel_dep entries to MODULE.bazel
// 3. Adds load statements to BUILD files
func AutoloadMigrationRecipe() *Recipe {
	return &Recipe{
		Name:            "autoload-migration",
		Version:         "1.0.0",
		Description:     "Migrate native rules to explicit load statements for Bazel 9.0+",
		MinBazelVersion: "8.0.0",

		Steps: []Step{
			{
				ID:          "detect-native-rules",
				Name:        "Detect native rule usage",
				Description: "Find BUILD files using native rules without load statements",
				Scan: &ScanStep{
					FilePatterns: []string{"BUILD", "BUILD.bazel"},
					Patterns: []string{
						// Shell rules
						`\bsh_binary\s*\(`,
						`\bsh_test\s*\(`,
						`\bsh_library\s*\(`,
						// C++ rules
						`\bcc_binary\s*\(`,
						`\bcc_library\s*\(`,
						`\bcc_test\s*\(`,
						`\bcc_import\s*\(`,
						`\bcc_shared_library\s*\(`,
						// Java rules
						`\bjava_binary\s*\(`,
						`\bjava_library\s*\(`,
						`\bjava_test\s*\(`,
						`\bjava_import\s*\(`,
						`\bjava_plugin\s*\(`,
						// Python rules
						`\bpy_binary\s*\(`,
						`\bpy_library\s*\(`,
						`\bpy_test\s*\(`,
						// Proto rules
						`\bproto_library\s*\(`,
						`\bcc_proto_library\s*\(`,
						`\bjava_proto_library\s*\(`,
						`\bpy_proto_library\s*\(`,
					},
					OutputKey: "detected_rules",
				},
			},
			{
				ID:          "add-module-deps",
				Name:        "Add bazel_dep entries",
				Description: "Add required rules_* dependencies to MODULE.bazel",
				DependsOn:   []string{"detect-native-rules"},
				Edit: &EditStep{
					Operations: []Operation{
						{Type: "ensure_bazel_dep", DepName: "rules_shell", DepVersion: "0.4.1"},
						{Type: "ensure_bazel_dep", DepName: "rules_cc", DepVersion: "0.1.1"},
						{Type: "ensure_bazel_dep", DepName: "rules_java", DepVersion: "8.6.3"},
						{Type: "ensure_bazel_dep", DepName: "rules_python", DepVersion: "1.4.1"},
						{Type: "ensure_bazel_dep", DepName: "protobuf", DepVersion: "29.3"},
					},
				},
			},
			{
				ID:          "add-load-statements",
				Name:        "Add load statements",
				Description: "Add load() statements for native rules in BUILD files",
				DependsOn:   []string{"add-module-deps"},
				Edit: &EditStep{
					Operations: []Operation{
						{Type: "ensure_load"},
					},
				},
			},
		},
	}
}

// IncompatibleFlagCleanupRecipe returns a recipe for removing
// incompatible flags that are no longer needed.
func IncompatibleFlagCleanupRecipe() *Recipe {
	return &Recipe{
		Name:        "incompatible-flag-cleanup",
		Version:     "1.0.0",
		Description: "Remove incompatible flags that are now default or removed",

		Steps: []Step{
			{
				ID:          "remove-autoload-flag",
				Name:        "Remove autoload flag",
				Description: "Remove --incompatible_autoload_externally flag",
				Edit: &EditStep{
					Operations: []Operation{
						{
							Type:       "remove_flag",
							File:       ".bazelrc",
							OldContent: "--incompatible_autoload_externally",
						},
					},
				},
			},
		},
	}
}
