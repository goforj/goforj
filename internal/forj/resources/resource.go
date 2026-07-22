package resources

import (
	"context"
	"errors"
	"sort"
	"strings"
)

// Resource describes one discoverable local project resource.
type Resource struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	URL         string `json:"url,omitempty"`
	Description string `json:"description,omitempty"`
	Enabled     bool   `json:"enabled"`
	Priority    int    `json:"priority,omitempty"`
	Source      string `json:"source,omitempty"`
	App         string `json:"app,omitempty"`
	Service     string `json:"service,omitempty"`
	Runtime     string `json:"runtime,omitempty"`
	Health      string `json:"health,omitempty"`
	Auth        string `json:"auth,omitempty"`
	Owner       string `json:"owner,omitempty"`
}

// Resolver supplies resources to a registry.
type Resolver interface {
	Resolve(context.Context) ([]Resource, error)
}

// ResolverFunc adapts a function into a resource resolver.
type ResolverFunc func(context.Context) ([]Resource, error)

// Resolve returns resources from f.
func (f ResolverFunc) Resolve(ctx context.Context) ([]Resource, error) {
	return f(ctx)
}

// Registry composes resource resolvers behind one deterministic list.
type Registry struct {
	resolvers []Resolver
}

// NewRegistry creates a registry from resource resolvers.
func NewRegistry(resolvers ...Resolver) Registry {
	return Registry{resolvers: append([]Resolver(nil), resolvers...)}
}

// List returns enabled resources in deterministic presentation order.
func (r Registry) List(ctx context.Context) ([]Resource, error) {
	resources := []Resource{}
	var joined error
	for _, resolver := range r.resolvers {
		if resolver == nil {
			continue
		}
		resolved, err := resolver.Resolve(ctx)
		if err != nil {
			joined = errors.Join(joined, err)
			continue
		}
		resources = append(resources, resolved...)
	}
	resources = normalize(resources)
	return resources, joined
}

// ByID returns the first enabled resource matching id.
func (r Registry) ByID(ctx context.Context, id string) (Resource, bool, error) {
	resources, err := r.List(ctx)
	if err != nil {
		return Resource{}, false, err
	}
	id = strings.TrimSpace(id)
	for _, resource := range resources {
		if resource.ID == id {
			return resource, true, nil
		}
	}
	return Resource{}, false, nil
}

// FilterOption limits resource filter output.
type FilterOption func(Resource) bool

// Category matches resources in category.
func Category(category string) FilterOption {
	category = strings.TrimSpace(category)
	return func(resource Resource) bool {
		return resource.Category == category
	}
}

// App matches resources owned by app.
func App(app string) FilterOption {
	app = strings.TrimSpace(app)
	return func(resource Resource) bool {
		return resource.App == app
	}
}

// Runtime matches resources for runtime.
func Runtime(runtime string) FilterOption {
	runtime = strings.TrimSpace(runtime)
	return func(resource Resource) bool {
		return resource.Runtime == runtime
	}
}

// Filter returns resources matching every option.
func Filter(resources []Resource, options ...FilterOption) []Resource {
	out := make([]Resource, 0, len(resources))
	for _, resource := range resources {
		matches := true
		for _, option := range options {
			if option != nil && !option(resource) {
				matches = false
				break
			}
		}
		if matches {
			out = append(out, resource)
		}
	}
	return out
}

// normalize removes disabled and duplicate entries before applying stable presentation order.
func normalize(resources []Resource) []Resource {
	seen := map[string]struct{}{}
	out := make([]Resource, 0, len(resources))
	for _, resource := range resources {
		resource.ID = strings.TrimSpace(resource.ID)
		resource.Name = strings.TrimSpace(resource.Name)
		resource.Category = strings.TrimSpace(resource.Category)
		resource.URL = strings.TrimSpace(resource.URL)
		resource.App = strings.TrimSpace(resource.App)
		resource.Service = strings.TrimSpace(resource.Service)
		resource.Runtime = strings.TrimSpace(resource.Runtime)
		resource.Health = strings.TrimSpace(resource.Health)
		resource.Auth = strings.TrimSpace(resource.Auth)
		resource.Owner = strings.TrimSpace(resource.Owner)
		if resource.ID == "" || resource.Name == "" || resource.Category == "" || !resource.Enabled {
			continue
		}
		if _, ok := seen[resource.ID]; ok {
			continue
		}
		seen[resource.ID] = struct{}{}
		out = append(out, resource)
	}
	sort.SliceStable(out, func(i, j int) bool {
		left := out[i]
		right := out[j]
		if left.Category != right.Category {
			return left.Category < right.Category
		}
		if left.Priority != right.Priority {
			return left.Priority < right.Priority
		}
		return left.ID < right.ID
	})
	return out
}
