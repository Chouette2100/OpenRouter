package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v2"
)

func readFileWithSops(path string) ([]byte, error) {
	if strings.Contains(filepath.Base(path), ".enc.") {
		if os.Getenv("SOPS_AGE_KEY_FILE") == "" {
			return nil, fmt.Errorf("SOPS_AGE_KEY_FILE is empty")
		}
		out, err := exec.Command("sops", "--decrypt", path).Output()
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				return nil, fmt.Errorf("sops --decrypt failed: %w: %s", err, strings.TrimSpace(string(exitErr.Stderr)))
			}
			return nil, fmt.Errorf("sops --decrypt failed: %w", err)
		}
		return out, nil
	}
	return os.ReadFile(path)
}

func loadSystemPrompts() error {
	candidates := []string{
		filepath.Join(".", "systemlist.enc.yml"),
		filepath.Join(".", "systemlist.yml"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			content, err := readFileWithSops(p)
			if err != nil {
				return err
			}
			var cfg struct {
				Systemlist yaml.MapSlice     `yaml:"systemlist"`
				Modellist  []OpenRouterModel `yaml:"modellist"`
			}
			if err := yaml.Unmarshal(content, &cfg); err != nil {
				return err
			}
			if len(cfg.Modellist) > 0 {
				openRouterModels = cfg.Modellist
			} else {
				openRouterModels = []OpenRouterModel{{
					Name:    "deepseek/deepseek-v4.1-flash",
					Vendor:  "DeepSeek",
					Comment: "fallback",
				}}
			}
			systems = map[string]string{}
			systemKeys = make([]string, 0, len(cfg.Systemlist))
			for _, item := range cfg.Systemlist {
				key, ok := item.Key.(string)
				if !ok {
					continue
				}
				val, ok := item.Value.(string)
				if !ok {
					continue
				}
				systems[key] = val
				systemKeys = append(systemKeys, key)
			}
			if len(systemKeys) > 0 {
				return nil
			}
		}
	}
	systems = map[string]string{"default": "You are a helpful assistant."}
	systemKeys = []string{"default"}
	openRouterModels = []OpenRouterModel{{
		Name:    "deepseek/deepseek-v4.1-flash",
		Vendor:  "DeepSeek",
		Comment: "fallback",
	}}
	return nil
}

func defaultModelName() string {
	if len(openRouterModels) > 0 && openRouterModels[0].Name != "" {
		return openRouterModels[0].Name
	}
	return "deepseek/deepseek-v4.1-flash"
}
