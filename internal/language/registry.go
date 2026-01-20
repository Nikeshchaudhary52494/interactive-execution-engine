package language

import "fmt"

var registry = map[string]Spec{}

func Register(spec Spec) {
	registry[spec.Name] = spec
}

func Resolve(name string) (Spec, error) {
	spec, ok := registry[name]
	if !ok {
		return Spec{}, fmt.Errorf("unsupported language: %s", name)
	}
	return spec, nil
}

func AllSpecs() []Spec {
	specs := make([]Spec, 0, len(registry))
	for _, spec := range registry {
		specs = append(specs, spec)
	}
	return specs
}
