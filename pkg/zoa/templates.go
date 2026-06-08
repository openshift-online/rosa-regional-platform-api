package zoa

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	workv1 "open-cluster-management.io/api/work/v1"
)

const (
	JobNamespace = "zoa-jobs"
)

// TATemplate defines a Trusted Action loaded from a YAML template file.
type TATemplate struct {
	Name        string            `yaml:"name"`
	Profile     string            `yaml:"profile"`
	Scope       string            `yaml:"scope"`
	Type        string            `yaml:"type"`
	Description string            `yaml:"description"`
	Manifests   []json.RawMessage `yaml:"manifests"`

	rawTemplate string
}

// RenderContext holds the variables available during template rendering.
type RenderContext struct {
	ExecID        string
	ActionName    string
	TargetCluster string
	Namespace     string
	OutputBucket  string
	JobRoleARN    string
	Image         string
	Params        map[string]string
}

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
		r.logger.Info("loaded TA template", "name", tmpl.Name, "path", path)
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

func parseTemplate(data []byte) (*TATemplate, error) {
	var meta struct {
		Name        string `yaml:"name"`
		Profile     string `yaml:"profile"`
		Scope       string `yaml:"scope"`
		Type        string `yaml:"type"`
		Description string `yaml:"description"`
	}
	if err := yaml.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("parsing template metadata: %w", err)
	}

	if meta.Name == "" {
		return nil, fmt.Errorf("template missing required 'name' field")
	}

	return &TATemplate{
		Name:        meta.Name,
		Profile:     meta.Profile,
		Scope:       meta.Scope,
		Type:        meta.Type,
		Description: meta.Description,
		rawTemplate: string(data),
	}, nil
}

// BuildManifestWork renders the template with the given context and constructs a ManifestWork.
func (t *TATemplate) BuildManifestWork(ctx RenderContext) (*workv1.ManifestWork, error) {
	rendered, err := t.render(ctx)
	if err != nil {
		return nil, fmt.Errorf("rendering template %s: %w", t.Name, err)
	}

	var parsed struct {
		Manifests []map[string]interface{} `yaml:"manifests"`
	}
	if err := yaml.Unmarshal([]byte(rendered), &parsed); err != nil {
		return nil, fmt.Errorf("parsing rendered template: %w", err)
	}

	manifests := make([]workv1.Manifest, 0, len(parsed.Manifests))
	var jobName string
	var jobNamespace string

	for _, m := range parsed.Manifests {
		jsonBytes, err := json.Marshal(m)
		if err != nil {
			return nil, fmt.Errorf("marshaling manifest to JSON: %w", err)
		}

		manifests = append(manifests, workv1.Manifest{
			RawExtension: runtime.RawExtension{Raw: jsonBytes},
		})

		if kind, _ := m["kind"].(string); kind == "Job" {
			if metadata, ok := m["metadata"].(map[string]interface{}); ok {
				jobName, _ = metadata["name"].(string)
				jobNamespace, _ = metadata["namespace"].(string)
			}
		}
	}

	mw := &workv1.ManifestWork{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "work.open-cluster-management.io/v1",
			Kind:       "ManifestWork",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "zoa-" + ctx.ExecID,
			Namespace: ctx.TargetCluster,
		},
		Spec: workv1.ManifestWorkSpec{
			Workload: workv1.ManifestsTemplate{
				Manifests: manifests,
			},
		},
	}

	if jobName != "" {
		ns := jobNamespace
		if ns == "" {
			ns = ctx.Namespace
		}
		mw.Spec.ManifestConfigs = []workv1.ManifestConfigOption{
			{
				ResourceIdentifier: workv1.ResourceIdentifier{
					Group:     "batch",
					Resource:  "jobs",
					Name:      jobName,
					Namespace: ns,
				},
				FeedbackRules: []workv1.FeedbackRule{
					{
						Type: workv1.JSONPathsType,
						JsonPaths: []workv1.JsonPath{
							{Name: "succeeded", Path: ".status.succeeded"},
							{Name: "failed", Path: ".status.failed"},
						},
					},
				},
			},
		}
	}

	return mw, nil
}

func (t *TATemplate) render(ctx RenderContext) (string, error) {
	tmpl, err := template.New(t.Name).Parse(t.rawTemplate)
	if err != nil {
		return "", fmt.Errorf("parsing Go template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}

	return buf.String(), nil
}
