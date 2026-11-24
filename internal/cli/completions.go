package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/PapaDanielVi/cafebazaar_cli/internal/android"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/api/pishkhan"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/config"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/output"
	"github.com/spf13/cobra"
)

func registerFlagCompletion(ctx context.Context, cmd *cobra.Command, flagName string, fn cobra.CompletionFunc) {
	_ = ctx
	if err := cmd.RegisterFlagCompletionFunc(flagName, fn); err != nil {
		panic(fmt.Sprintf("register completion for flag %q on %q: %v", flagName, cmd.Name(), err))
	}
}

func registerGlobalFlagCompletions(ctx context.Context, cmd *cobra.Command, rt *AppRuntime) {
	registerFlagCompletion(ctx, cmd, flagApp, completeAppPackages(rt))
	registerFlagCompletion(ctx, cmd, flagOutput, func(c *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
		_ = c
		_ = args
		_ = toComplete
		return []cobra.Completion{
			cobra.CompletionWithDesc(output.FormatTable, "Styled terminal tables and cards"),
			cobra.CompletionWithDesc(output.FormatPlain, "Plain line-oriented text without ANSI"),
			cobra.CompletionWithDesc(output.FormatJSON, "Indented raw UTF-8 JSON"),
			cobra.CompletionWithDesc(output.FormatYAML, "Raw UTF-8 YAML"),
		}, cobra.ShellCompDirectiveNoFileComp
	})
	registerFlagCompletion(ctx, cmd, flagRTL, func(c *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
		_ = c
		_ = args
		_ = toComplete
		return []cobra.Completion{
			cobra.CompletionWithDesc(output.RTLAuto, "Auto-detect terminal BiDi/shaping support"),
			cobra.CompletionWithDesc(output.RTLOn, "Force Persian glyph shaping and BiDi ordering"),
			cobra.CompletionWithDesc(output.RTLOff, "Emit raw logical UTF-8 without shaping"),
		}, cobra.ShellCompDirectiveNoFileComp
	})
}

func ensureCompletionContext(ctx context.Context, rt *AppRuntime) {
	if rt.Global == nil {
		if err := rt.bootstrap(ctx); err != nil {
			return
		}
	}
}

func completeAppPackages(rt *AppRuntime) func(*cobra.Command, []string, string) ([]cobra.Completion, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
		_ = args
		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		ensureCompletionContext(ctx, rt)

		seen := map[string]struct{}{}
		var completions []cobra.Completion
		lowerPrefix := strings.ToLower(strings.TrimSpace(toComplete))

		// Suggest all Gradle build-type/flavor package names first when inside an Android project.
		if rt.Android != nil {
			for _, v := range rt.Android.AllVariants(ctx) {
				if v.PackageName == "" {
					continue
				}
				if _, exists := seen[v.PackageName]; exists {
					continue
				}
				if lowerPrefix != "" && !strings.HasPrefix(strings.ToLower(v.PackageName), lowerPrefix) {
					continue
				}
				seen[v.PackageName] = struct{}{}
				status := "unregistered"
				if rt.Global != nil {
					if _, reg := rt.Global.Apps[v.PackageName]; reg {
						status = "registered"
					}
				}
				desc := fmt.Sprintf("Gradle variant %s (%s)", v.Name, status)
				completions = append(completions, cobra.CompletionWithDesc(v.PackageName, desc))
			}
		}

		// Also include registered apps and aliases from global config.
		if rt.Global != nil {
			for _, entry := range rt.Global.SortedApps(ctx) {
				if _, exists := seen[entry.PackageName]; !exists {
					if lowerPrefix == "" || strings.HasPrefix(strings.ToLower(entry.PackageName), lowerPrefix) {
						seen[entry.PackageName] = struct{}{}
						desc := "registered app"
						if entry.Alias != "" {
							desc = fmt.Sprintf("registered app (alias: %s)", entry.Alias)
						}
						completions = append(completions, cobra.CompletionWithDesc(entry.PackageName, desc))
					}
				}
				if entry.Alias != "" {
					if lowerPrefix == "" || strings.HasPrefix(strings.ToLower(entry.Alias), lowerPrefix) {
						completions = append(completions, cobra.CompletionWithDesc(entry.Alias, fmt.Sprintf("alias for %s", entry.PackageName)))
					}
				}
			}
		}

		return completions, cobra.ShellCompDirectiveNoFileComp
	}
}

func completeGradleVariants(rt *AppRuntime) func(*cobra.Command, []string, string) ([]cobra.Completion, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
		_ = args
		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		ensureCompletionContext(ctx, rt)
		if rt.Android == nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		lowerPrefix := strings.ToLower(strings.TrimSpace(toComplete))
		var completions []cobra.Completion
		for _, v := range rt.Android.AllVariants(ctx) {
			if lowerPrefix != "" && !strings.HasPrefix(strings.ToLower(v.Name), lowerPrefix) {
				continue
			}
			desc := fmt.Sprintf("%s (%s)", v.PackageName, v.ModuleName)
			completions = append(completions, cobra.CompletionWithDesc(v.Name, desc))
		}
		return completions, cobra.ShellCompDirectiveNoFileComp
	}
}

func completeArtifactPaths(rt *AppRuntime, artifactType, ext string) func(*cobra.Command, []string, string) ([]cobra.Completion, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
		_ = args
		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		ensureCompletionContext(ctx, rt)

		var completions []cobra.Completion
		if rt.Android != nil {
			for _, mod := range rt.Android.Modules {
				for _, a := range mod.Artifacts {
					if a.Type != artifactType {
						continue
					}
					if toComplete != "" && !strings.HasPrefix(a.Path, toComplete) && !strings.HasPrefix(a.RelPath, toComplete) {
						continue
					}
					desc := fmt.Sprintf("%s (%s)", output.HumanBytes(ctx, a.SizeBytes), a.VariantName)
					completions = append(completions, cobra.CompletionWithDesc(a.Path, desc))
				}
			}
		}
		if len(completions) > 0 {
			return completions, cobra.ShellCompDirectiveDefault
		}
		return []cobra.Completion{ext}, cobra.ShellCompDirectiveFilterFileExt
	}
}

func completeArchFlag(ctx context.Context) func(*cobra.Command, []string, string) ([]cobra.Completion, cobra.ShellCompDirective) {
	_ = ctx
	return func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
		_ = cmd
		_ = args
		_ = toComplete
		return []cobra.Completion{
			cobra.CompletionWithDesc(string(pishkhan.ArchitectureAll), "Universal APK for all CPU architectures"),
			cobra.CompletionWithDesc(string(pishkhan.ArchitectureARMv7), "32-bit ARM devices"),
			cobra.CompletionWithDesc(string(pishkhan.ArchitectureARM64), "64-bit ARM devices"),
		}, cobra.ShellCompDirectiveNoFileComp
	}
}

func completeBumpFlag(ctx context.Context) func(*cobra.Command, []string, string) ([]cobra.Completion, cobra.ShellCompDirective) {
	_ = ctx
	return func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
		_ = cmd
		_ = args
		_ = toComplete
		return []cobra.Completion{
			cobra.CompletionWithDesc(string(android.BumpCode), "Increment versionCode only"),
			cobra.CompletionWithDesc(string(android.BumpPatch), "Increment patch versionName + versionCode"),
			cobra.CompletionWithDesc(string(android.BumpMinor), "Increment minor versionName + versionCode"),
			cobra.CompletionWithDesc(string(android.BumpMajor), "Increment major versionName + versionCode"),
		}, cobra.ShellCompDirectiveNoFileComp
	}
}

func isRegisteredApp(ctx context.Context, cfg *config.GlobalConfig, pkg string) bool {
	_ = ctx
	if cfg == nil {
		return false
	}
	_, ok := cfg.Apps[pkg]
	return ok
}
