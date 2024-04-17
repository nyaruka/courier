package whatsapp

import (
	"sort"
	"strings"

	"github.com/nyaruka/courier"
	"golang.org/x/exp/maps"
)

func GetTemplatePayload(templating *courier.Templating) *Template {
	template := &Template{
		Name:       templating.Template.Name,
		Language:   &Language{Policy: "deterministic", Code: templating.Language},
		Components: []*Component{},
	}

	for _, comp := range templating.Components {
		// get the variables used by this component in order of their names 1, 2 etc
		compParams := make([]courier.TemplatingVariable, 0, len(comp.Variables))
		varNames := maps.Keys(comp.Variables)
		sort.Strings(varNames)
		for _, varName := range varNames {
			compParams = append(compParams, templating.Variables[comp.Variables[varName]])
		}

		var component *Component

		if comp.Type == "header" {
			component = &Component{Type: comp.Type}

			for _, p := range compParams {
				if p.Type == "image" {
					component.Params = append(component.Params, &Param{Type: p.Type, Image: &struct {
						Link string "json:\"link,omitempty\""
					}{Link: p.Value}})
				} else if p.Type == "video" {
					component.Params = append(component.Params, &Param{Type: p.Type, Video: &struct {
						Link string "json:\"link,omitempty\""
					}{Link: p.Value}})
				} else if p.Type == "document" {
					component.Params = append(component.Params, &Param{Type: p.Type, Document: &struct {
						Link string "json:\"link,omitempty\""
					}{Link: p.Value}})
				} else {
					component.Params = append(component.Params, &Param{Type: p.Type, Text: p.Value})
				}
			}
		} else if comp.Type == "body" {
			component = &Component{Type: comp.Type}

			for _, p := range compParams {
				component.Params = append(component.Params, &Param{Type: p.Type, Text: p.Value})
			}
		} else if strings.HasPrefix(comp.Type, "button/") {
			component = &Component{Type: "button", Index: strings.TrimPrefix(comp.Name, "button."), SubType: strings.TrimPrefix(comp.Type, "button/"), Params: []*Param{}}

			for _, p := range compParams {
				if comp.Type == "button/url" {
					component.Params = append(component.Params, &Param{Type: "text", Text: p.Value})
				} else {
					component.Params = append(component.Params, &Param{Type: "payload", Payload: p.Value})
				}
			}
		}

		if component != nil {
			template.Components = append(template.Components, component)
		}
	}

	return template
}
