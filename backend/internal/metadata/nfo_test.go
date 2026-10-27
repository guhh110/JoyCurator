package metadata

import (
	"bytes"
	"encoding/xml"
	"io"
	"strings"
	"testing"

	"joycurator/internal/model"
)

func TestMovieEscapingAndEpisodeBlocks(t *testing.T) {
	item := model.Entry{TMDBID: 42, Type: "movie", Title: `A & B <Test>`, Date: "2020-01-01", Overview: `Plot < & >`}
	data, err := MovieNFO(item)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Title string `xml:"title"`
	}
	if err = xml.Unmarshal(data, &doc); err != nil || doc.Title != item.Title {
		t.Fatal("invalid XML", err)
	}
	item.Type = "tv"
	data, err = EpisodeNFO(item, 0, []int{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	blocks := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if start, ok := token.(xml.StartElement); ok && start.Name.Local == "episodedetails" {
			blocks++
		}
	}
	if blocks != 2 {
		t.Fatalf("episode blocks %d", blocks)
	}
	data, err = ShowNFO(item)
	if err != nil || !strings.Contains(string(data), "<tvshow>") {
		t.Fatal(err)
	}
}
