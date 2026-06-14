package v1alpha1

// ComponentTemplates holds named Go-template bodies that render to a single Vector
// component. Pipelines reference them via sourceTemplate/transformTemplate/sinkTemplate.
type ComponentTemplates struct {
	// Sources are template bodies keyed by template name.
	// +optional
	Sources map[string]string `json:"sources,omitempty"`
	// Transforms are template bodies keyed by template name.
	// +optional
	Transforms map[string]string `json:"transforms,omitempty"`
	// Sinks are template bodies keyed by template name.
	// +optional
	Sinks map[string]string `json:"sinks,omitempty"`
}
