package hls

import (
	"bytes"
	"fmt"

	"github.com/grafov/m3u8"
)

type Parser struct{}

func NewParser() *Parser { return &Parser{} }

func (p *Parser) ParsePlaylist(content []byte) (playlist m3u8.Playlist, listType m3u8.ListType, err error) {
	playlist, listType, err = m3u8.DecodeFrom(bytes.NewReader(content), true)
	if err != nil {
		return nil, listType, fmt.Errorf("parse playlist: %w", err)
	}
	return playlist, listType, nil
}
