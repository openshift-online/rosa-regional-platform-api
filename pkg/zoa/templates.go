package zoa

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// TemplateRegistry manages all loaded TA templates.
type TemplateRegistry struct {
	templates map[string]*TATemplate
	logger    *slog.Logger
}

// NewTemplateRegistry creates an empty registry.
func NewTemplateRegistry(logger *slog.Logger) *TemplateRegistry {
	return &TemplateRegistry{
		templates: make(map[string]*TATemplate),
		logger:    logger,
	}
}

// LoadFromDir reads all .yaml files from a directory and registers them.
func (r *TemplateRegistry) LoadFromDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("reading templates dir %s: %w", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			r.logger.Error("failed to read template file", "path", path, "error", err)
			continue
		}

		tmpl, err := parseTemplate(data)
		if err != nil {
			r.logger.Error("failed to parse template file", "path", path, "error", err)
			continue
		}

		r.templates[tmpl.Name] = tmpl
		r.logger.Info("loaded TA template", "name", tmpl.Name, "profile", tmpl.Profile, "type", tmpl.Type, "path", path)
	}

	if len(r.templates) == 0 {
		return fmt.Errorf("no valid templates found in %s", dir)
	}

	return nil
}

// Get retrieves a template by action name.
func (r *TemplateRegistry) Get(action string) (*TATemplate, bool) {
	t, ok := r.templates[action]
	return t, ok
}

// List returns all registered template names.
func (r *TemplateRegistry) List() []string {
	names := make([]string, 0, len(r.templates))
	for name := range r.templates {
		names = append(names, name)
	}
	return names
}

// ListAll returns all registered templates (for catalog endpoint).
func (r *TemplateRegistry) ListAll() []*TATemplate {
	templates := make([]*TATemplate, 0, len(r.templates))
	for _, t := range r.templates {
		templates = append(templates, t)
	}
	return templates
}

func parseTemplate(data []byte) (*TATemplate, error) {
	var tmpl TATemplate
	if err := yaml.Unmarshal(data, &tmpl); err != nil {
		return nil, fmt.Errorf("parsing template: %w", err)
	}

	if tmpl.Name == "" {
		return nil, fmt.Errorf("template missing required 'name' field")
	}
	if tmpl.Profile == "" {
		return nil, fmt.Errorf("template %s missing required 'profile' field", tmpl.Name)
	}
	if tmpl.Script == "" {
		return nil, fmt.Errorf("template %s missing required 'script' field", tmpl.Name)
	}

	return &tmpl, nil
}

// LoadJobConfig reads the zoa-job-config ConfigMap data from a directory.
func LoadJobConfig(dir string) (*JobConfig, error) {
	cfg := &JobConfig{
		Revision:                "unknown",
		CPURequest:              "100m",
		MemoryRequest:           "128Mi",
		CPULimit:                "500m",
		MemoryLimit:             "512Mi",
		TTLSeconds:              3600,
		ExecutionTimeoutSeconds: 1800,
	}

	readFile := func(name string) string {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(data))
	}

	if v := readFile("image"); v != "" {
		cfg.Image = v
	}
	if v := readFile("revision"); v != "" {
		cfg.Revision = v
	}
	if v := readFile("cpu_request"); v != "" {
		cfg.CPURequest = v
	}
	if v := readFile("memory_request"); v != "" {
		cfg.MemoryRequest = v
	}
	if v := readFile("cpu_limit"); v != "" {
		cfg.CPULimit = v
	}
	if v := readFile("memory_limit"); v != "" {
		cfg.MemoryLimit = v
	}
	if v := readFile("ttl_seconds"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 32); err == nil {
			cfg.TTLSeconds = int32(n)
		}
	}
	if v := readFile("execution_timeout_seconds"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			cfg.ExecutionTimeoutSeconds = int(n)
		}
	}
	if v := readFile("entrypoint.sh"); v != "" {
		cfg.EntrypointScript = v
	}

	if cfg.Image == "" {
		return nil, fmt.Errorf("zoa-job-config missing required 'image' field")
	}
	if cfg.EntrypointScript == "" {
		return nil, fmt.Errorf("zoa-job-config missing required 'entrypoint.sh' field")
	}

	return cfg, nil
}
