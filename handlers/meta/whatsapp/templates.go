package whatsapp

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/utils"
	"github.com/pkg/errors"
	"golang.org/x/exp/maps"
)

type MsgTemplating struct {
	Template struct {
		Name string `json:"name" validate:"required"`
		UUID string `json:"uuid" validate:"required"`
	} `json:"template" validate:"required,dive"`
	Namespace string `json:"namespace"`
	Params    map[string][]struct {
		Type  string `json:"type"`
		Value string `json:"value"`
	} `json:"params"`
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

	compKeys := maps.Keys(templating.Params)
	sort.Strings(compKeys) // so that final component order is deterministic

	for _, k := range compKeys {
		v := templating.Params[k]
		var component *Component

		if k == "header" {
			component = &Component{Type: "header"}

			for _, p := range v {
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
		} else if k == "body" {
			component = &Component{Type: "body"}

			for _, p := range v {
				component.Params = append(component.Params, &Param{Type: p.Type, Text: p.Value})
			}
		} else if strings.HasPrefix(k, "button.") {
			component = &Component{Type: "button", Index: strings.TrimPrefix(k, "button."), SubType: "quick_reply", Params: []*Param{}}

			for _, p := range v {
				if p.Type == "url" {
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
