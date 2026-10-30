package recognition

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

var numberRE = regexp.MustCompile(`\d+`)

func (m Metadata) value(target Target) any {
	switch target {
	case TMDBID:
		if m.TMDBID != nil {
			return *m.TMDBID
		}
	case Title:
		if m.Title != nil {
			return *m.Title
		}
	case MediaType:
		if m.MediaType != nil {
			return *m.MediaType
		}
	case Year:
		if m.Year != nil {
			return *m.Year
		}
	case Season:
		if m.Season != nil {
			return *m.Season
		}
	case Episodes:
		if len(m.Episodes) > 0 {
			return append([]int(nil), m.Episodes...)
		}
	case ReleaseTag:
		if m.ReleaseTag != nil {
			return *m.ReleaseTag
		}
	}
	return nil
}
func (m *Metadata) set(target Target, value any) {
	switch target {
	case TMDBID:
		v := value.(int)
		m.TMDBID = &v
	case Title:
		v := value.(string)
		m.Title = &v
	case MediaType:
		v := value.(string)
		m.MediaType = &v
	case Year:
		v := value.(int)
		m.Year = &v
	case Season:
		v := value.(int)
		m.Season = &v
	case Episodes:
		m.Episodes = append([]int(nil), value.([]int)...)
	case ReleaseTag:
		v := value.(string)
		m.ReleaseTag = &v
	}
}
func convert(target Target, value any) (any, error) {
	if raw, ok := value.(json.RawMessage); ok {
		var decoded any
		decoder := json.NewDecoder(strings.NewReader(string(raw)))
		decoder.UseNumber()
		if err := decoder.Decode(&decoded); err != nil {
			return nil, err
		}
		value = decoded
	}
	text := fmt.Sprint(value)
	switch target {
	case Title, ReleaseTag:
		if s, ok := value.(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s), nil
		}
	case MediaType:
		if text == "movie" || text == "tv" {
			return text, nil
		}
	case TMDBID, Year, Season:
		n, err := strconv.Atoi(text)
		if err != nil && target == Season {
			n, err = chineseNumber(text)
		}
		if err == nil && ((target == Season && n >= 0 && n <= 999) || (target != Season && n > 0 && n <= 999999999)) {
			if target != Year || n <= 9999 {
				return n, nil
			}
		}
	case Episodes:
		switch values := value.(type) {
		case []int:
			return validateEpisodes(values)
		case []any:
			nums := []int{}
			for _, v := range values {
				n, err := strconv.Atoi(fmt.Sprint(v))
				if err != nil {
					return nil, errors.New("集数必须是整数")
				}
				nums = append(nums, n)
			}
			return validateEpisodes(nums)
		}
		nums := []int{}
		for _, part := range strings.FieldsFunc(strings.TrimSpace(text), func(r rune) bool { return r == ',' || r == '，' || r == ' ' }) {
			bounds := strings.Split(part, "-")
			if len(bounds) > 2 {
				return nil, errors.New("集数范围无效")
			}
			first, err := strconv.Atoi(bounds[0])
			if err != nil {
				return nil, errors.New("集数格式无效")
			}
			last := first
			if len(bounds) == 2 {
				last, err = strconv.Atoi(bounds[1])
			}
			if err != nil || last < first || last-first >= 1000 {
				return nil, errors.New("集数范围无效")
			}
			for n := first; n <= last; n++ {
				nums = append(nums, n)
				if len(nums) > 1000 {
					return nil, errors.New("集数超过 1000 个")
				}
			}
		}
		return validateEpisodes(nums)
	}
	return nil, fmt.Errorf("%s 提取值类型或范围无效", target)
}
func validateEpisodes(nums []int) (any, error) {
	if len(nums) == 0 || len(nums) > 1000 {
		return nil, errors.New("集数数量无效")
	}
	seen := map[int]bool{}
	for _, n := range nums {
		if n < 0 || n > 9999 || seen[n] {
			return nil, errors.New("集数超出范围或重复")
		}
		seen[n] = true
	}
	return append([]int(nil), nums...), nil
}
func chineseNumber(s string) (int, error) {
	digits := map[rune]int{'零': 0, '一': 1, '二': 2, '两': 2, '三': 3, '四': 4, '五': 5, '六': 6, '七': 7, '八': 8, '九': 9}
	r := []rune(s)
	if len(r) == 1 {
		if r[0] == '十' {
			return 10, nil
		}
		if n, ok := digits[r[0]]; ok {
			return n, nil
		}
	}
	if len(r) == 2 {
		if r[0] == '十' {
			if n, ok := digits[r[1]]; ok {
				return 10 + n, nil
			}
		}
		if r[1] == '十' {
			if n, ok := digits[r[0]]; ok {
				return n * 10, nil
			}
		}
	}
	if len(r) == 3 && r[1] == '十' {
		a, ok := digits[r[0]]
		b, ok2 := digits[r[2]]
		if ok && ok2 {
			return a*10 + b, nil
		}
	}
	return 0, errors.New("中文季号无效")
}
func Equal(a, b any) bool { return reflect.DeepEqual(a, b) }
