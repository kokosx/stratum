package themes

import "fmt"

type Registry struct {
	definitions map[string]*Definition
}

func NewRegistry() (*Registry, error) {
	defaultTheme, err := loadDefaultDefinition()
	if err != nil {
		return nil, err
	}
	return &Registry{definitions: map[string]*Definition{defaultTheme.ID: defaultTheme}}, nil
}

func (r *Registry) Exact(id string, version int) (*Definition, error) {
	if id == "default" { // Compatibility with the original site_settings seed.
		id = "stratum/default"
	}
	definition := r.definitions[id]
	if definition == nil || definition.Version != version {
		return nil, fmt.Errorf("theme %s@%d is not registered", id, version)
	}
	return definition, nil
}

func (r *Registry) Current(id string) (*Definition, error) {
	if id == "default" {
		id = "stratum/default"
	}
	definition := r.definitions[id]
	if definition == nil {
		return nil, fmt.Errorf("theme %s is not registered", id)
	}
	return definition, nil
}
