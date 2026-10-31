package scanner

import (
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Parsed is everything the file name and its folders say about a file.
// Parsing never touches the disk.
type Parsed struct {
	Title        string `json:"title,omitempty"`
	Year         int    `json:"year,omitempty"`
	Season       int    `json:"season,omitempty"`
	HasSeason    bool   `json:"has_season,omitempty"`
	Episode      int    `json:"episode,omitempty"`
	EpisodeEnd   int    `json:"episode_end,omitempty"` // S01E01-E02
	Absolute     int    `json:"absolute,omitempty"`    // anime absolute number
	Version      int    `json:"version,omitempty"`     // v2
	EpisodeTitle string `json:"episode_title,omitempty"`
	Group        string `json:"group,omitempty"` // [SubsPlease]
	Ext          string `json:"ext,omitempty"`

	// Signals used by Classify.
	SeasonEpisode bool `json:"season_episode,omitempty"` // S01E01, 1x01, season + episode
	AbsoluteEp    bool `json:"absolute_ep,omitempty"`    // " - 01", EP01, bare number after a group tag
	Special       bool `json:"special,omitempty"`        // OVA, OAD, Special

	// Folder hints.
	Dir          string `json:"dir,omitempty"`          // relative folder of the file ("" = root)
	FolderTitle  string `json:"folder_title,omitempty"` // cleaned name of the nearest title folder
	FolderYear   int    `json:"folder_year,omitempty"`
	TitleDir     string `json:"title_dir,omitempty"` // relative path of that folder
	SeasonFolder bool   `json:"season_folder,omitempty"`
	Hint         Hint   `json:"hint,omitempty"`

	// Secondary files (AGENTS.md §4).
	Secondary Secondary `json:"secondary,omitempty"`
	ExtraType string    `json:"extra_type,omitempty"`
	OwnerDir  string    `json:"owner_dir,omitempty"` // folder whose movie/series owns the extra
}

// Hint is a category suggested by a folder name.
type Hint string

// Folder hints.
const (
	HintNone   Hint = ""
	HintMovie  Hint = "movie"
	HintSeries Hint = "series"
	HintAnime  Hint = "anime"
)

// Secondary marks files that are not main media.
type Secondary string

// Secondary kinds.
const (
	SecondaryNone    Secondary = ""
	SecondarySample  Secondary = "sample"
	SecondaryTrailer Secondary = "trailer"
	SecondaryExtra   Secondary = "extra"
)

// Extra types stored in extras.extra_type.
const (
	ExtraTrailer         = "trailer"
	ExtraFeaturette      = "featurette"
	ExtraBehindTheScenes = "behind_the_scenes"
	ExtraDeleted         = "deleted"
	ExtraInterview       = "interview"
	ExtraOther           = "other"
)

// boundary replaces bracket groups: the title never continues past it.
const boundary = "\x1f"

var (
	reLeadingGroup = regexp.MustCompile(`^\s*[\[【]([^\]】]*)[\]】]`)
	reBracket      = regexp.MustCompile(`[\[(\{【]([^\[\](){}【】]*)[\])\}】]`)

	reTokSxxEyy = regexp.MustCompile(`^s(\d{1,2})[ ._]?e(\d{1,4})(?:-?e(\d{1,4})|-(\d{1,4}))?(?:v(\d))?$`)
	reTokSeason = regexp.MustCompile(`^s(\d{1,2})$`)
	reTokEpOnly = regexp.MustCompile(`^e(\d{1,4})(?:-?e?(\d{1,4}))?(?:v(\d))?$`)
	reTok1x01   = regexp.MustCompile(`^(\d{1,2})x(\d{2,3})(?:-(?:\d{1,2}x)?(\d{2,3}))?$`)
	reTokEpWord = regexp.MustCompile(`^(?:ep|episode|episodio|episódio|cap|capitulo|capítulo|#)(\d{1,4})?(?:v(\d))?$`)
	reTokNumber = regexp.MustCompile(`^(\d{1,4})(?:v(\d))?$`)
	reTokVer    = regexp.MustCompile(`^v(\d)$`)
	reTokOrd    = regexp.MustCompile(`^(\d{1,2})(?:st|nd|rd|th|ª|º|a)$`)
	reTokSpec   = regexp.MustCompile(`^(?:ova|oad|ona|special|specials|sp)(\d{1,3})?$`)
)

var seasonWords = setOf("season", "temporada", "saison", "staffel", "series")

// Parse reads a path relative to the media folder ("/" separators).
func Parse(relPath string) Parsed {
	relPath = strings.ReplaceAll(relPath, "\\", "/")
	dir, file := path.Split(relPath)
	dir = strings.TrimSuffix(dir, "/")
	ext := path.Ext(file)
	base := strings.TrimSuffix(file, ext)

	p := parseName(base)
	p.Ext = strings.ToLower(ext)
	p.Dir = dir
	applyFolders(&p, dir)
	detectSecondary(&p, base)
	return p
}

// parseName extracts title, year and episode markers from a single name
// (a file name without extension, or a folder name).
func parseName(name string) Parsed {
	var p Parsed
	s := name

	// Leading [Group] (anime fansub / release group).
	if m := reLeadingGroup.FindStringSubmatchIndex(s); m != nil {
		content := strings.TrimSpace(s[m[2]:m[3]])
		if content != "" && !isTagGroup(content) && !reYear.MatchString(content) {
			p.Group = content
		}
		s = s[m[1]:]
	}

	// Bracket groups: a year stays as a token; everything else becomes a
	// boundary, unless it is plain text like "(US)" that belongs to the title.
	s = reBracket.ReplaceAllStringFunc(s, func(g string) string {
		content := strings.TrimSpace(g[1 : len(g)-1])
		inner := strings.ToLower(content)
		switch {
		case reYear.MatchString(content):
			return " " + content + " " + boundary + " "
		case isTagGroup(content), reHash.MatchString(content):
			return " " + boundary + " "
		case reTokSxxEyy.MatchString(inner), reTokNumber.MatchString(inner), reTok1x01.MatchString(inner),
			reSeasonFolder.MatchString(NormTitle(content)):
			return " " + content + " " + boundary + " " // [01], (S01E01), (Season 1): keep the marker
		default:
			return " " + g + " "
		}
	})

	tokens := tokenize(s)
	for len(tokens) > 0 && tokens[0] == boundary {
		tokens = tokens[1:]
	}
	lower := make([]string, len(tokens))
	for i, t := range tokens {
		lower[i] = strings.ToLower(t)
	}

	n := len(tokens)
	cut := -1
	setCut := func(i int) {
		if cut < 0 {
			cut = i
		}
	}
	epTitleFrom := -1
	dashEp, dashVer := 0, 0
	wordEp, wordVer := 0, 0
	tagSeen := false
	years := []int{}

	for i := 0; i < n; i++ {
		t := lower[i]
		next := ""
		if i+1 < n {
			next = lower[i+1]
		}
		switch {
		case t == boundary:
			setCut(i)

		case reTokSxxEyy.MatchString(t):
			m := reTokSxxEyy.FindStringSubmatch(t)
			if !p.SeasonEpisode {
				p.Season, p.HasSeason = atoi(m[1]), true
				p.Episode = atoi(m[2])
				p.EpisodeEnd = endEpisode(p.Episode, firstNonEmpty(m[3], m[4]))
				p.Version = atoi(m[5])
				p.SeasonEpisode = true
				epTitleFrom = i + 1
			}
			setCut(i)

		case reTokSeason.MatchString(t) && reTokEpOnly.MatchString(next):
			// "S01 E01"
			m, e := reTokSeason.FindStringSubmatch(t), reTokEpOnly.FindStringSubmatch(next)
			if !p.SeasonEpisode {
				p.Season, p.HasSeason = atoi(m[1]), true
				p.Episode = atoi(e[1])
				p.EpisodeEnd = endEpisode(p.Episode, e[2])
				p.Version = atoi(e[3])
				p.SeasonEpisode = true
				epTitleFrom = i + 2
			}
			setCut(i)
			i++

		case !tagSeen && reTok1x01.MatchString(t):
			m := reTok1x01.FindStringSubmatch(t)
			if !p.SeasonEpisode {
				p.Season, p.HasSeason = atoi(m[1]), true
				p.Episode = atoi(m[2])
				p.EpisodeEnd = endEpisode(p.Episode, m[3])
				p.SeasonEpisode = true
				epTitleFrom = i + 1
			}
			setCut(i)

		case reTokSeason.MatchString(t):
			// season pack or anime "S2 - 05"
			if !p.HasSeason {
				p.Season, p.HasSeason = atoi(reTokSeason.FindStringSubmatch(t)[1]), true
			}
			setCut(i)
			i += takeEpisode(lower, i+1, &dashEp, &dashVer)

		case seasonWords[t] && reTokNumber.MatchString(next) && (t != "series" || i > 0):
			// "Season 2", "Temporada 02"
			if !p.HasSeason {
				p.Season, p.HasSeason = atoi(reTokNumber.FindStringSubmatch(next)[1]), true
			}
			setCut(i)
			i++
			i += takeEpisode(lower, i+1, &dashEp, &dashVer)

		case reTokOrd.MatchString(t) && seasonWords[next]:
			// "2nd Season", "2ª Temporada"
			if !p.HasSeason {
				p.Season, p.HasSeason = atoi(reTokOrd.FindStringSubmatch(t)[1]), true
			}
			setCut(i)
			i++
			i += takeEpisode(lower, i+1, &dashEp, &dashVer)

		case !tagSeen && reTokEpOnly.MatchString(t):
			// "E05", "E01-E02"
			m := reTokEpOnly.FindStringSubmatch(t)
			if wordEp == 0 {
				wordEp, wordVer = atoi(m[1]), atoi(m[3])
				p.EpisodeEnd = endEpisode(wordEp, m[2])
				epTitleFrom = i + 1
			}
			setCut(i)

		case !tagSeen && reTokEpWord.MatchString(t):
			// "EP01", "Episode 5", "Ep. 5"
			m := reTokEpWord.FindStringSubmatch(t)
			num, ver := m[1], m[2]
			if num == "" {
				if !reTokNumber.MatchString(next) {
					continue // the word alone ("Episode") is part of the title
				}
				nm := reTokNumber.FindStringSubmatch(next)
				num, ver = nm[1], nm[2]
				i++
			}
			if wordEp == 0 {
				wordEp, wordVer = atoi(num), atoi(ver)
				epTitleFrom = i + 1
			}
			setCut(i - boolInt(m[1] == ""))

		case !tagSeen && t == "-" && reTokNumber.MatchString(next) && i > 0:
			// anime " - 01", " - 1080", " - 05v2"
			m := reTokNumber.FindStringSubmatch(next)
			v := atoi(m[1])
			if reYear.MatchString(m[1]) && p.Group == "" && m[2] == "" {
				years = append(years, i+1) // "Title - 2019"
				i++
				continue
			}
			if dashEp == 0 {
				dashEp, dashVer = v, atoi(m[2])
				if i+2 < n && reTokVer.MatchString(lower[i+2]) {
					dashVer = atoi(reTokVer.FindStringSubmatch(lower[i+2])[1])
				}
			}
			setCut(i)
			i++

		case !tagSeen && reTokSpec.MatchString(t) && i > 0 && len(years) == 0:
			// OVA/Special; after a year it is a movie edition ("1986 Special Edition")
			p.Special = true
			setCut(i)
			if m := reTokSpec.FindStringSubmatch(t); m[1] != "" {
				wordEp = atoi(m[1])
			} else if reTokNumber.MatchString(next) {
				wordEp = atoi(reTokNumber.FindStringSubmatch(next)[1])
				i++
			}

		case reYear.MatchString(t):
			years = append(years, i)

		case isStrongTag(t):
			tagSeen = true
			setCut(i)
		}
	}
	if cut < 0 {
		cut = n
	}

	// Year: the last year-like token before the cut that has a title before it.
	maxYear := time.Now().Year() + 1
	for _, i := range slices.Backward(years) {
		y := atoi(lower[i])
		if i >= 1 && i <= cut && y <= maxYear && !onlyBoundaries(lower[:i]) {
			p.Year = y
			if i < cut {
				cut = i
			}
			break
		}
	}

	// Anime bare number after a group tag: "[Group] Title 01 [1080p]".
	if p.Group != "" && !p.SeasonEpisode && dashEp == 0 && wordEp == 0 {
		for i := cut - 1; i >= 1; i-- {
			if m := reTokNumber.FindStringSubmatch(lower[i]); m != nil && !reYear.MatchString(m[1]) {
				dashEp, dashVer = atoi(m[1]), atoi(m[2])
				cut = i
				break
			}
			if lower[i] != "-" {
				break // only a trailing number counts
			}
		}
	}

	switch {
	case p.SeasonEpisode:
	case p.HasSeason && (dashEp > 0 || wordEp > 0):
		p.Episode = max(dashEp, wordEp)
		p.Version = max(dashVer, wordVer)
		p.SeasonEpisode = true
	case dashEp > 0 || wordEp > 0:
		p.Absolute = max(dashEp, wordEp)
		p.Version = max(dashVer, wordVer)
		p.AbsoluteEp = true
		if p.Special {
			p.Season, p.HasSeason = 0, true
			p.Episode, p.Absolute = p.Absolute, 0
			p.AbsoluteEp, p.SeasonEpisode = false, true
		}
	case p.Special:
		p.Season, p.HasSeason = 0, true
	}

	p.Title = joinTitle(tokens[:cut])
	if epTitleFrom > 0 {
		p.EpisodeTitle = episodeTitle(tokens, lower, epTitleFrom)
	}
	return p
}

// tokenize splits on spaces, dots and underscores. Standalone dashes become
// their own "-" token; in-word dashes (Spider-Man, WEB-DL) are kept.
func tokenize(s string) []string {
	s = strings.Map(func(r rune) rune {
		switch r {
		case '.', '_', '\t':
			return ' '
		case '–', '—', '~':
			return '-'
		}
		return r
	}, s)
	fields := strings.Fields(s)
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		switch {
		case f == "-" || f == boundary:
			out = append(out, f)
		case strings.Trim(f, "-") == "":
			out = append(out, "-")
		default:
			// "Title-01" style leftovers: a dash glued at the end/start.
			if strings.HasSuffix(f, "-") && len(f) > 1 {
				out = append(out, strings.TrimSuffix(f, "-"), "-")
				continue
			}
			if strings.HasPrefix(f, "-") && len(f) > 1 {
				out = append(out, "-", strings.TrimPrefix(f, "-"))
				continue
			}
			out = append(out, f)
		}
	}
	return out
}

func joinTitle(tokens []string) string {
	parts := make([]string, 0, len(tokens))
	for _, t := range tokens {
		if t == boundary {
			continue
		}
		parts = append(parts, t)
	}
	return cleanTitle(strings.Join(parts, " "))
}

func episodeTitle(tokens, lower []string, from int) string {
	var parts []string
	for i := from; i < len(tokens); i++ {
		t := lower[i]
		if t == boundary || isReleaseTag(t) || reYear.MatchString(t) || reResolution.MatchString(t) {
			break
		}
		parts = append(parts, tokens[i])
	}
	return cleanTitle(strings.Join(parts, " "))
}

func onlyBoundaries(toks []string) bool {
	for _, t := range toks {
		if t != boundary && t != "-" {
			return false
		}
	}
	return true
}

// takeEpisode reads a bare episode number right after a season marker
// ("3rd Season 05"). It returns how many tokens it consumed.
func takeEpisode(lower []string, at int, ep, ver *int) int {
	if at >= len(lower) || *ep != 0 {
		return 0
	}
	m := reTokNumber.FindStringSubmatch(lower[at])
	if m == nil || reYear.MatchString(m[1]) {
		return 0
	}
	*ep, *ver = atoi(m[1]), atoi(m[2])
	return 1
}

func endEpisode(start int, s string) int {
	if s == "" {
		return 0
	}
	end := atoi(s)
	if end <= start || end-start > 20 {
		return 0
	}
	return end
}

func atoi(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}
