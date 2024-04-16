package whatsapp

import (
	"strings"

	"github.com/nyaruka/courier"
)

func GetTemplatePayload(templating *courier.Templating) *Template {
	template := &Template{
		Name:       templating.Template.Name,
		Language:   &Language{Policy: "deterministic", Code: templating.Language},
		Components: []*Component{},
	}

	for _, comp := range templating.Components {
		var component *Component

		if comp.Type == "header" {
			component = &Component{Type: comp.Type}

			for _, p := range comp.Params {
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

			for _, p := range comp.Params {
				component.Params = append(component.Params, &Param{Type: p.Type, Text: p.Value})
			}
		} else if strings.HasPrefix(comp.Type, "button/") {
			component = &Component{Type: "button", Index: strings.TrimPrefix(comp.Name, "button."), SubType: strings.TrimPrefix(comp.Type, "button/"), Params: []*Param{}}

			for _, p := range comp.Params {
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
