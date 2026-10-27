package metadata

import (
	"bytes"
	"encoding/xml"
	"fmt"

	"joycurator/internal/model"
)

type uniqueID struct {
	Type    string `xml:"type,attr"`
	Default string `xml:"default,attr"`
	Value   int    `xml:",chardata"`
}
type actor struct {
	Name string `xml:"name"`
	Role string `xml:"role"`
}
type document struct {
	XMLName       xml.Name
	Title         string    `xml:"title"`
	OriginalTitle string    `xml:"originaltitle,omitempty"`
	ShowTitle     string    `xml:"showtitle,omitempty"`
	Plot          string    `xml:"plot,omitempty"`
	Premiered     string    `xml:"premiered,omitempty"`
	Aired         string    `xml:"aired,omitempty"`
	ID            *uniqueID `xml:"uniqueid,omitempty"`
	Season        *int      `xml:"season,omitempty"`
	Episode       *int      `xml:"episode,omitempty"`
	Genres        []string  `xml:"genre"`
	Actors        []actor   `xml:"actor"`
	EpisodeGuide  string    `xml:"episodeguide,omitempty"`
}

func encode(d document) ([]byte, error) {
	data, err := xml.MarshalIndent(d, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), append(data, '\n')...), nil
}

func base(item model.Entry) document {
	d := document{
		Title:         item.Title,
		OriginalTitle: item.OriginalTitle,
		Plot:          item.Overview,
		Premiered:     item.Date,
		Genres:        item.Genres,
		ID:            &uniqueID{Type: "tmdb", Default: "true", Value: item.TMDBID},
	}
	for _, person := range item.Cast {
		d.Actors = append(d.Actors, actor{person.Name, person.Role})
	}
	return d
}

// MovieNFO renders a Kodi movie NFO.
func MovieNFO(item model.Entry) ([]byte, error) {
	d := base(item)
	d.XMLName.Local = "movie"
	return encode(d)
}

// ShowNFO renders a Kodi tvshow NFO.
func ShowNFO(item model.Entry) ([]byte, error) {
	d := base(item)
	d.XMLName.Local = "tvshow"
	d.EpisodeGuide = fmt.Sprintf(`{"tmdb":"%d"}`, item.TMDBID)
	return encode(d)
}

// EpisodeNFO renders one episodedetails block per episode number. A file that
// holds several episodes (a merged release) yields several blocks.
func EpisodeNFO(item model.Entry, season int, episodes []int) ([]byte, error) {
	var result bytes.Buffer
	for _, number := range episodes {
		season, number := season, number
		episode := base(item)
		episode.XMLName.Local = "episodedetails"
		episode.ShowTitle = item.Title
		episode.Title = fmt.Sprintf("%s S%02dE%02d", item.Title, season, number)
		episode.Season = &season
		episode.Episode = &number
		data, err := encode(episode)
		if err != nil {
			return nil, err
		}
		if result.Len() > 0 {
			data = bytes.TrimPrefix(data, []byte(xml.Header))
		}
		result.Write(data)
	}
	if result.Len() == 0 {
		return nil, fmt.Errorf("缺少单集元数据")
	}
	return result.Bytes(), nil
}
