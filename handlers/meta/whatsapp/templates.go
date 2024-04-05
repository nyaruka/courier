package whatsapp

import (
	"encoding/json"
	"strings"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/utils"
	"github.com/pkg/errors"
)

type MsgTemplating struct {
	Template struct {
		Name string `json:"name" validate:"required"`
		UUID string `json:"uuid" validate:"required"`
	} `json:"template" validate:"required,dive"`
	Namespace  string `json:"namespace"`
	ExternalID string `json:"external_id"`
	Components []struct {
		Type   string `json:"type"`
		Name   string `json:"name"`
		Params []struct {
			Type  string `json:"type"`
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"params"`
	} `json:"components"`
	Language string `json:"language"`
}

func GetTemplating(msg courier.MsgOut) (*MsgTemplating, error) {
	if len(msg.Metadata()) == 0 {
		return nil, nil
	}

	metadata := &struct {
		Templating *MsgTemplating `json:"templating"`
	}{}
	if err := json.Unmarshal(msg.Metadata(), metadata); err != nil {
		return nil, err
	}

	if metadata.Templating == nil {
		return nil, nil
	}

	if err := utils.Validate(metadata.Templating); err != nil {
		return nil, errors.Wrapf(err, "invalid templating definition")
	}

	return metadata.Templating, nil
}

func GetTemplatePayload(templating *MsgTemplating) *Template {
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
