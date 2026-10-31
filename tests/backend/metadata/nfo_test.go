package metadata

import (
	"bytes"
	"encoding/xml"
	"io"
	"strings"
	"testing"

	"joycurator/internal/model"
)

func TestSeasonNFOUsesSeasonMetadata(t *testing.T) {
	show := model.Entry{TMDBID: 2, Type: "tv", Title: "剧集 & 特别篇"}
	season := model.Season{TMDBID: 24, Number: 1, Name: "第一季", Overview: "本季 <简介>", Date: "2023-02-01"}
	data, err := SeasonNFO(show, season)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		XMLName xml.Name `xml:"season"`
		Title string `xml:"title"`
		ShowTitle string `xml:"showtitle"`
		Plot string `xml:"plot"`
		Premiered string `xml:"premiered"`
		Number int `xml:"season"`
		ID struct { Type string `xml:"type,attr"`; Value int `xml:",chardata"` } `xml:"uniqueid"`
	}
	if err = xml.Unmarshal(data, &doc); err != nil || doc.XMLName.Local != "season" || doc.Title != season.Name || doc.ShowTitle != show.Title || doc.Plot != season.Overview || doc.Premiered != season.Date || doc.Number != season.Number || doc.ID.Type != "tmdb" || doc.ID.Value != season.TMDBID {
		t.Fatalf("unexpected season NFO: %s, %v", data, err)
	}
}

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
	data, err = EpisodeNFO(item, []model.Episode{{TMDBID: 101, Season: 0, Number: 1, Name: "首集", Overview: "第一集剧情", AirDate: "2023-01-02"}, {TMDBID: 102, Season: 0, Number: 2, Name: "次集", Overview: "第二集剧情", AirDate: "2023-01-09"}})
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
	if !bytes.Contains(data, []byte("<title>首集</title>")) || !bytes.Contains(data, []byte("<plot>第二集剧情</plot>")) || !bytes.Contains(data, []byte("<aired>2023-01-02</aired>")) || bytes.Contains(data, []byte("<plot>Plot")) {
		t.Fatalf("episode-specific metadata missing: %s", data)
	}
	data, err = ShowNFO(item)
	if err != nil || !strings.Contains(string(data), "<tvshow>") {
		t.Fatal(err)
	}
}
