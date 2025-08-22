package test

import (
	"github.com/nyaruka/courier/core/models"
	"github.com/nyaruka/gocommon/uuids"
)

func NewMockMedia(path, contentType, url string, size, width, height, duration int, alternates []*models.Media) *models.Media {
	return &models.Media{
		UUID_:        uuids.NewV4(),
		Path_:        path,
		ContentType_: contentType,
		URL_:         url,
		Size_:        size,
		Width_:       width,
		Height_:      height,
		Duration_:    duration,
		Alternates_:  alternates,
	}
}
