// Package buildsettings reads explicit XcodeGen Info.plist injection. It does
// not execute XcodeGen or claim to resolve the complete Xcode build graph.
package buildsettings

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"strings"
)

type settings struct {
	Base    map[string]any            `yaml:"base"`
	Configs map[string]map[string]any `yaml:"configs"`
}
type target struct {
	Type     string `yaml:"type"`
	Platform string `yaml:"platform"`
	Sources  []any  `yaml:"sources"`
	Info     struct {
		Path       string         `yaml:"path"`
		Properties map[string]any `yaml:"properties"`
	} `yaml:"info"`
	Settings settings `yaml:"settings"`
}
type project struct {
	Settings settings          `yaml:"settings"`
	Targets  map[string]target `yaml:"targets"`
}

// Injected returns only nonempty values consistently configured for every
// matching target/configuration. Other targets cannot satisfy this plist.
func Injected(root, plistPath string) map[string]string {
	b, err := os.ReadFile(filepath.Join(root, "project.yml"))
	if err != nil {
		return nil
	}
	var p project
	if yaml.Unmarshal(b, &p) != nil {
		return nil
	}
	abs, _ := filepath.Abs(plistPath)
	result := map[string]string{}
	first := true
	for _, t := range p.Targets {
		candidate, _ := filepath.Abs(filepath.Join(root, t.Info.Path))
		if t.Info.Path == "" || candidate != abs {
			continue
		}
		configs := map[string]bool{"": true}
		for c := range p.Settings.Configs {
			configs[c] = true
		}
		for c := range t.Settings.Configs {
			configs[c] = true
		}
		for config := range configs {
			merged := map[string]any{}
			for _, m := range []map[string]any{p.Settings.Base, p.Settings.Configs[config], t.Settings.Base, t.Settings.Configs[config]} {
				for k, v := range m {
					merged[k] = v
				}
			}
			values := map[string]string{}
			for key, value := range t.Info.Properties {
				if value != nil && strings.TrimSpace(fmt.Sprint(value)) != "" {
					values[key] = fmt.Sprint(value)
				}
			}
			generated := strings.EqualFold(fmt.Sprint(merged["GENERATE_INFOPLIST_FILE"]), "YES") || merged["GENERATE_INFOPLIST_FILE"] == true
			if generated {
				for k, v := range merged {
					if strings.HasPrefix(k, "INFOPLIST_KEY_") {
						key := strings.TrimPrefix(k, "INFOPLIST_KEY_")
						delete(values, key)
						if v != nil && strings.TrimSpace(fmt.Sprint(v)) != "" {
							values[key] = fmt.Sprint(v)
						}
					}
				}
			}
			if first {
				result = values
				first = false
			} else {
				for k, v := range result {
					if values[k] != v {
						delete(result, k)
					}
				}
			}
		}
	}
	return result
}

// AppleSources returns explicit local source paths from shipping Apple XcodeGen
// targets. Unknown source syntax falls back to a broad scan rather than omission.
func AppleSources(root string) []string {
	b, err := os.ReadFile(filepath.Join(root, "project.yml"))
	if err != nil {
		return nil
	}
	var p project
	if yaml.Unmarshal(b, &p) != nil {
		return nil
	}
	var roots []string
	for _, t := range p.Targets {
		platform := strings.ToLower(t.Platform)
		if platform != "ios" && platform != "watchos" && platform != "macos" && platform != "tvos" && platform != "visionos" {
			continue
		}
		if strings.Contains(t.Type, "test") {
			continue
		}
		if len(t.Sources) == 0 {
			return nil
		}
		for _, source := range t.Sources {
			path := ""
			switch v := source.(type) {
			case string:
				path = v
			case map[string]any:
				path, _ = v["path"].(string)
			}
			if path == "" || strings.ContainsAny(path, "*?${}") || filepath.IsAbs(path) || filepath.Clean(path) == ".." || strings.HasPrefix(filepath.Clean(path), ".."+string(filepath.Separator)) {
				return nil
			}
			path = filepath.ToSlash(filepath.Clean(path))
			roots = append(roots, path)
		}
	}
	return roots
}
func InSources(rel string, roots []string) bool {
	if len(roots) == 0 {
		return true
	}
	rel = filepath.ToSlash(filepath.Clean(rel))
	for _, root := range roots {
		if root == "." || rel == root || strings.HasPrefix(rel, root+"/") {
			return true
		}
	}
	return false
}
