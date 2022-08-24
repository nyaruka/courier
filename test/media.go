package test

import "github.com/nyaruka/courier"

type mockMedia struct {
	name        string
	contentType string
	url         string
	size        int
	width       int
	height      int
	duration    int
	alternates  []courier.Media
}

func (m *mockMedia) Name() string                { return m.name }
func (m *mockMedia) ContentType() string         { return m.contentType }
func (m *mockMedia) URL() string                 { return m.url }
func (m *mockMedia) Size() int                   { return m.size }
func (m *mockMedia) Width() int                  { return m.width }
func (m *mockMedia) Height() int                 { return m.height }
func (m *mockMedia) Duration() int               { return m.duration }
func (m *mockMedia) Alternates() []courier.Media { return m.alternates }

func NewMockMedia(name, contentType, url string, size, width, height, duration int, alternates []courier.Media) courier.Media {
	return &mockMedia{
		name:        name,
		contentType: contentType,
		url:         url,
		size:        size,
		width:       width,
		height:      height,
		duration:    duration,
		alternates:  alternates,
	}
}
