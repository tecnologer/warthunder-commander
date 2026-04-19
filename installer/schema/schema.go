package schema

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// FieldType controls the input widget shown in the TUI.
type FieldType string

const (
	FieldTypeText     FieldType = "text"
	FieldTypePassword FieldType = "password"
	FieldTypeBool     FieldType = "bool"
	FieldTypeSelect   FieldType = "select"
)

// ShowIf makes a field conditional on another field's value.
type ShowIf struct {
	Key    string   `yaml:"key"`    // key of the controlling field
	Values []string `yaml:"values"` // show this field only when the controlling field matches one of these values
}

// Field describes a single config entry in the TOML file.
type Field struct {
	Key         string    `yaml:"key"`          // TOML key, supports dot notation e.g. "server.host"
	Label       string    `yaml:"label"`        // Human-readable label shown in TUI
	Description string    `yaml:"description"`  // Helper text shown below the input
	Type        FieldType `yaml:"type"`         // text | password | bool | select
	Default     string    `yaml:"default"`      // Pre-filled value
	Required    bool      `yaml:"required"`     // Validation: cannot be empty
	Options      []string  `yaml:"options"`       // Only for type: select
	OptionLabels []string  `yaml:"option_labels"` // Human-readable labels parallel to Options
	ShowIf      *ShowIf   `yaml:"show_if"`      // Optional: hide field unless condition is met
	EnvVar      bool      `yaml:"env_var"`      // true when this field's value is an env-var name; wizard will offer to set its value and write it to .env
}

// Schema is the top-level structure of schema.yaml.
type Schema struct {
	AppName    string  `yaml:"app_name"`    // e.g. "mycli"
	GithubRepo string  `yaml:"github_repo"` // e.g. "yourorg/mycli"
	BinaryName string  `yaml:"binary_name"` // e.g. "mycli" (without .exe)
	Fields     []Field `yaml:"fields"`
}

// Load reads and parses a schema YAML file from the given path.
func Load(path string) (*Schema, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading schema file: %w", err)
	}
	return LoadBytes(data)
}

// LoadBytes parses a schema from raw YAML bytes (e.g. from an embedded file).
func LoadBytes(data []byte) (*Schema, error) {
	var s Schema
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parsing schema YAML: %w", err)
	}

	if err := s.validate(); err != nil {
		return nil, fmt.Errorf("invalid schema: %w", err)
	}

	return &s, nil
}

func (s *Schema) validate() error {
	if s.AppName == "" {
		return fmt.Errorf("app_name is required")
	}
	if s.GithubRepo == "" {
		return fmt.Errorf("github_repo is required")
	}
	if s.BinaryName == "" {
		return fmt.Errorf("binary_name is required")
	}
	for i, f := range s.Fields {
		if f.Key == "" {
			return fmt.Errorf("field[%d] missing key", i)
		}
		if f.Label == "" {
			return fmt.Errorf("field[%d] missing label", i)
		}
		if f.Type == FieldTypeSelect && len(f.Options) == 0 {
			return fmt.Errorf("field %q is type select but has no options", f.Key)
		}
	}
	return nil
}
