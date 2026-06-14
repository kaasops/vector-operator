package config

import (
	"bytes"
	"errors"
	"fmt"
	"text/template"

	"github.com/mitchellh/mapstructure"
	goyaml "sigs.k8s.io/yaml"

	vectorv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
)

const (
	sourceTemplateKey    = "sourceTemplate"
	transformTemplateKey = "transformTemplate"
	sinkTemplateKey      = "sinkTemplate"
)

var (
	ErrComponentTemplatesNotConfigured = errors.New("componentTemplates is not configured")
	ErrComponentTemplateNotFound       = errors.New("component template not found")
	ErrComponentTemplateTypeConflict   = errors.New("component template cannot be used together with type")
	ErrComponentTemplateRenderFailed   = errors.New("component template render failed")
)

type ComponentTemplatesConfig struct {
	Sources    map[string]string
	Transforms map[string]string
	Sinks      map[string]string
}

type componentTemplateRef struct {
	Name   string         `mapstructure:"name"`
	Params map[string]any `mapstructure:"params"`
}

func ComponentTemplatesFromCR(spec *vectorv1alpha1.ComponentTemplates) *ComponentTemplatesConfig {
	if spec == nil {
		return nil
	}
	if len(spec.Sources) == 0 && len(spec.Transforms) == 0 && len(spec.Sinks) == 0 {
		return nil
	}

	return &ComponentTemplatesConfig{
		Sources:    spec.Sources,
		Transforms: spec.Transforms,
		Sinks:      spec.Sinks,
	}
}

func applyComponentTemplates(p *PipelineConfig, tpls *ComponentTemplatesConfig) error {
	for name, source := range p.Sources {
		expanded, err := expandSourceComponent(name, source, tpls)
		if err != nil {
			return err
		}
		p.Sources[name] = expanded
	}

	for name, transform := range p.Transforms {
		expanded, err := expandTransformComponent(name, transform, tpls)
		if err != nil {
			return err
		}
		p.Transforms[name] = expanded
	}

	for name, sink := range p.Sinks {
		expanded, err := expandSinkComponent(name, sink, tpls)
		if err != nil {
			return err
		}
		p.Sinks[name] = expanded
	}

	return nil
}

func expandSourceComponent(name string, source *Source, tpls *ComponentTemplatesConfig) (*Source, error) {
	refValue, hasRef := lookupOption(source.Options, sourceTemplateKey)
	if source.Type != "" && hasRef {
		return nil, fmt.Errorf("source %q: %w", name, ErrComponentTemplateTypeConflict)
	}
	if !hasRef {
		return source, nil
	}

	ref, err := parseTemplateRef(refValue)
	if err != nil {
		return nil, fmt.Errorf("source %q: %w", name, err)
	}
	if tpls == nil {
		return nil, fmt.Errorf("%s references template %q: %w", sourceTemplateKey, ref.Name, ErrComponentTemplatesNotConfigured)
	}

	rendered, err := renderComponent(ref, tpls.Sources)
	if err != nil {
		return nil, fmt.Errorf("source %q: %w", name, err)
	}

	expanded := &Source{}
	if err := mapstructure.Decode(rendered, expanded); err != nil {
		return nil, fmt.Errorf("source %q: failed to decode rendered template: %w", name, err)
	}
	return expanded, nil
}

func expandTransformComponent(name string, transform *Transform, tpls *ComponentTemplatesConfig) (*Transform, error) {
	refValue, hasRef := lookupOption(transform.Options, transformTemplateKey)
	if transform.Type != "" && hasRef {
		return nil, fmt.Errorf("transform %q: %w", name, ErrComponentTemplateTypeConflict)
	}
	if !hasRef {
		return transform, nil
	}

	ref, err := parseTemplateRef(refValue)
	if err != nil {
		return nil, fmt.Errorf("transform %q: %w", name, err)
	}
	if tpls == nil {
		return nil, fmt.Errorf("%s references template %q: %w", transformTemplateKey, ref.Name, ErrComponentTemplatesNotConfigured)
	}

	rendered, err := renderComponent(ref, tpls.Transforms)
	if err != nil {
		return nil, fmt.Errorf("transform %q: %w", name, err)
	}

	expanded := &Transform{}
	if err := mapstructure.Decode(rendered, expanded); err != nil {
		return nil, fmt.Errorf("transform %q: failed to decode rendered template: %w", name, err)
	}

	if len(transform.Inputs) > 0 {
		expanded.Inputs = append([]string(nil), transform.Inputs...)
	}
	return expanded, nil
}

func expandSinkComponent(name string, sink *Sink, tpls *ComponentTemplatesConfig) (*Sink, error) {
	refValue, hasRef := lookupOption(sink.Options, sinkTemplateKey)
	if sink.Type != "" && hasRef {
		return nil, fmt.Errorf("sink %q: %w", name, ErrComponentTemplateTypeConflict)
	}
	if !hasRef {
		return sink, nil
	}

	ref, err := parseTemplateRef(refValue)
	if err != nil {
		return nil, fmt.Errorf("sink %q: %w", name, err)
	}
	if tpls == nil {
		return nil, fmt.Errorf("%s references template %q: %w", sinkTemplateKey, ref.Name, ErrComponentTemplatesNotConfigured)
	}

	rendered, err := renderComponent(ref, tpls.Sinks)
	if err != nil {
		return nil, fmt.Errorf("sink %q: %w", name, err)
	}

	expanded := &Sink{}
	if err := mapstructure.Decode(rendered, expanded); err != nil {
		return nil, fmt.Errorf("sink %q: failed to decode rendered template: %w", name, err)
	}

	if len(sink.Inputs) > 0 {
		expanded.Inputs = append([]string(nil), sink.Inputs...)
	}
	return expanded, nil
}

func renderComponent(ref *componentTemplateRef, definitions map[string]string) (map[string]any, error) {
	body, ok := definitions[ref.Name]
	if !ok {
		return nil, fmt.Errorf("template %q: %w", ref.Name, ErrComponentTemplateNotFound)
	}

	tmpl, err := template.New(ref.Name).Option("missingkey=error").Parse(body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template %q: %w", ref.Name, err)
	}

	var rendered bytes.Buffer
	params := ref.Params
	if params == nil {
		params = map[string]any{}
	}
	if err := tmpl.Execute(&rendered, params); err != nil {
		return nil, fmt.Errorf("%w rendering template %q: %w", ErrComponentTemplateRenderFailed, ref.Name, err)
	}

	component := map[string]any{}
	if err := goyaml.Unmarshal(rendered.Bytes(), &component); err != nil {
		return nil, fmt.Errorf("failed to parse rendered template %q: %w", ref.Name, err)
	}
	if len(component) == 0 {
		return nil, fmt.Errorf("rendered template %q is empty", ref.Name)
	}

	return component, nil
}

func parseTemplateRef(value any) (*componentTemplateRef, error) {
	ref := &componentTemplateRef{}
	if err := mapstructure.Decode(value, ref); err != nil {
		return nil, fmt.Errorf("invalid template reference: %w", err)
	}
	if ref.Name == "" {
		return nil, fmt.Errorf("template reference name is required")
	}
	if ref.Params == nil {
		ref.Params = map[string]any{}
	}
	return ref, nil
}

func lookupOption(options map[string]any, key string) (any, bool) {
	if options == nil {
		return nil, false
	}
	value, ok := options[key]
	return value, ok
}
