package rapidpro_test

import (
	"encoding/json"
	"testing"

	"github.com/nyaruka/courier/backends/rapidpro"
	"github.com/stretchr/testify/assert"
)

func TestDBMedia(t *testing.T) {
	media1 := &rapidpro.Media{
		UUID_:        "5310f50f-9c8e-4035-9150-be5a1f78f21a",
		Path_:        "/orgs/1/media/5310/5310f50f-9c8e-4035-9150-be5a1f78f21a/test.mp3",
		ContentType_: "audio/mp3",
		URL_:         "http://nyaruka.s3.com/orgs/1/media/5310/5310f50f-9c8e-4035-9150-be5a1f78f21a/test.mp3",
		Size_:        123,
		Duration_:    500,
		Alternates_: []*rapidpro.Media{
			{
				UUID_:        "514c552c-e585-40e2-938a-fe9450172da8",
				Path_:        "/orgs/1/media/514c/514c552c-e585-40e2-938a-fe9450172da8/test.m4a",
				ContentType_: "audio/mp4",
				URL_:         "http://nyaruka.s3.com/orgs/1/media/514c/514c552c-e585-40e2-938a-fe9450172da8/test.m4a",
				Size_:        114,
				Duration_:    500,
			},
		},
	}

	// test that JSON serialization and deserialization gives the same object
	media1JSON, err := json.Marshal(media1)
	assert.NoError(t, err)

	media2 := &rapidpro.Media{}
	err = json.Unmarshal(media1JSON, media2)
	assert.NoError(t, err)
	assert.Equal(t, media1, media2)
}
