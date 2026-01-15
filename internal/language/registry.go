package language

import "fmt"

var registry = map[string]Spec{
	"python": Python,
}

func Resolve(name string) (Spec, error) {
	spec, ok := registry[name]
	if !ok {
		return Spec{}, fmt.Errorf("unsupported language: %s", name)
	}
	return spec, nil
}
