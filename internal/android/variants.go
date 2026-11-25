package android

import (
	"context"
	"fmt"
	"strings"
	"unicode"
)

func synthesizeVariants(ctx context.Context, mod AppModule) []Variant {
	flavorCombos := buildFlavorCombinations(ctx, mod.FlavorDimensions, mod.ProductFlavors)
	variants := make([]Variant, 0, len(flavorCombos)*len(mod.BuildTypes))

	for _, combo := range flavorCombos {
		for _, bt := range mod.BuildTypes {
			pkg := mod.ApplicationID
			var verName strings.Builder
			verName.WriteString(mod.VersionName)
			flavorNames := make([]string, 0, len(combo))

			for _, f := range combo {
				flavorNames = append(flavorNames, f.Name)
				if f.ApplicationID != "" {
					pkg = f.ApplicationID
				}
				if f.ApplicationIDSuffix != "" {
					pkg += f.ApplicationIDSuffix
				}
				if f.VersionNameSuffix != "" {
					verName.WriteString(f.VersionNameSuffix)
				}
			}

			if bt.ApplicationIDSuffix != "" {
				pkg += bt.ApplicationIDSuffix
			}
			if bt.VersionNameSuffix != "" {
				verName.WriteString(bt.VersionNameSuffix)
			}

			variantName := combineVariantName(ctx, flavorNames, bt.Name)
			taskSuffix := capitalizeFirst(ctx, variantName)
			taskPrefix := mod.Name
			if taskPrefix == ":" {
				taskPrefix = ""
			}

			variants = append(variants, Variant{
				ModuleName:   mod.Name,
				Name:         variantName,
				BuildType:    bt.Name,
				Flavors:      flavorNames,
				PackageName:  pkg,
				VersionCode:  mod.VersionCode,
				VersionName:  verName.String(),
				AssembleTask: fmt.Sprintf("%s:assemble%s", taskPrefix, taskSuffix),
				BundleTask:   fmt.Sprintf("%s:bundle%s", taskPrefix, taskSuffix),
			})
		}
	}

	return variants
}

func buildFlavorCombinations(ctx context.Context, dimensions []string, flavors []ProductFlavor) [][]ProductFlavor {
	_ = ctx
	if len(flavors) == 0 {
		return [][]ProductFlavor{{}}
	}

	groups := map[string][]ProductFlavor{}
	dimOrder := make([]string, 0, len(dimensions)+1)
	seenDim := map[string]struct{}{}
	for _, d := range dimensions {
		if _, ok := seenDim[d]; !ok {
			seenDim[d] = struct{}{}
			dimOrder = append(dimOrder, d)
		}
	}

	for _, f := range flavors {
		dim := f.Dimension
		if dim == "" {
			if len(dimOrder) > 0 {
				dim = dimOrder[0]
			} else {
				dim = "default"
			}
		}
		if _, ok := seenDim[dim]; !ok {
			seenDim[dim] = struct{}{}
			dimOrder = append(dimOrder, dim)
		}
		groups[dim] = append(groups[dim], f)
	}

	combos := [][]ProductFlavor{{}}
	for _, dim := range dimOrder {
		dimFlavors := groups[dim]
		if len(dimFlavors) == 0 {
			continue
		}
		next := make([][]ProductFlavor, 0, len(combos)*len(dimFlavors))
		for _, prefix := range combos {
			for _, f := range dimFlavors {
				row := make([]ProductFlavor, len(prefix)+1)
				copy(row, prefix)
				row[len(prefix)] = f
				next = append(next, row)
			}
		}
		combos = next
	}

	return combos
}

func combineVariantName(ctx context.Context, flavors []string, buildType string) string {
	if len(flavors) == 0 {
		return buildType
	}
	var b strings.Builder
	for idx, f := range flavors {
		if idx == 0 {
			b.WriteString(f)
		} else {
			b.WriteString(capitalizeFirst(ctx, f))
		}
	}
	b.WriteString(capitalizeFirst(ctx, buildType))
	return b.String()
}

func capitalizeFirst(ctx context.Context, s string) string {
	_ = ctx
	if s == "" {
		return ""
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}
