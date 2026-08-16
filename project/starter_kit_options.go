package project

// StarterKitOptions controls optional surfaces layered onto a selected starter kit.
type StarterKitOptions struct {
	ComponentLibrary *bool `yaml:"component_library,omitempty" json:"component_library,omitempty"`
}

// NewStarterKitOptions creates an explicit starter-kit option selection.
func NewStarterKitOptions(componentLibrary bool) *StarterKitOptions {
	return &StarterKitOptions{ComponentLibrary: &componentLibrary}
}

// ComponentLibraryEnabled resolves omitted starter-kit options to the compatibility default.
func (o *StarterKitOptions) ComponentLibraryEnabled() bool {
	return o == nil || o.ComponentLibrary == nil || *o.ComponentLibrary
}
