package scanner

import (
	"path"
	"regexp"
	"strings"
)

// Folder names that hint the category (normalized with NormTitle).
var categoryFolders = map[string]Hint{
	"anime": HintAnime, "animes": HintAnime, "animacao": HintAnime, "animacoes": HintAnime,
	"filmes": HintMovie, "filme": HintMovie, "movies": HintMovie, "movie": HintMovie,
	"films": HintMovie, "film": HintMovie, "peliculas": HintMovie, "cinema": HintMovie,
	"series": HintSeries, "serie": HintSeries, "seriados": HintSeries, "tv": HintSeries,
	"tv shows": HintSeries, "tvshows": HintSeries, "shows": HintSeries, "tv series": HintSeries,
}

// Folder names that hold extras, with the extra type they imply.
var extraFolders = map[string]string{
	"extras":            ExtraOther,
	"extra":             ExtraOther,
	"featurettes":       ExtraFeaturette,
	"featurette":        ExtraFeaturette,
	"behind the scenes": ExtraBehindTheScenes,
	"behindthescenes":   ExtraBehindTheScenes,
	"deleted scenes":    ExtraDeleted,
	"deleted":           ExtraDeleted,
	"interviews":        ExtraInterview,
	"interview":         ExtraInterview,
	"shorts":            ExtraOther,
	"scenes":            ExtraOther,
	"other":             ExtraOther,
	"others":            ExtraOther,
	"bonus":             ExtraOther,
	"trailers":          ExtraTrailer,
	"trailer":           ExtraTrailer,
}

var sampleFolders = setOf("sample", "samples")

var (
	reSeasonFolder = regexp.MustCompile(`^(?:season|temporada|saison|staffel|series|s)\s*(\d{1,2})$`)
	reSpecials     = regexp.MustCompile(`^(?:specials?|especiais|extras especiais|season 0+|temporada 0+)$`)

	reSample      = regexp.MustCompile(`(?i)(?:^|[ ._\-\[(])sample(?:$|[ ._\-\])])`)
	reTrailer     = regexp.MustCompile(`(?i)(?:^|[ ._\-])trailer\d*$|(?i)^trailer\d*$|(?i)[._\-]trailer[._\-]`)
	reExtraSuffix = regexp.MustCompile(`(?i)-(featurette|behindthescenes|deleted|interview|scene|short|other)$`)
	rePromo       = regexp.MustCompile(`(?i)(?:^|[ ._\-\[(])(?:promo|1xbet|www)(?:$|[ ._\-\])])|\.(?:com|net|org)(?:$|[ ._\-\])])`)
)

// promoMaxSize is the largest file that can be a release group's ad.
const promoMaxSize = 100 << 20

// isPromo reports an ad that release groups add next to the video
// ("1XBET.COM_promo_….mp4"): small, with an ad-like name and, in its own name,
// no year or episode (those come from a real title, like "Promo.2015.mkv").
func isPromo(rel string, size int64) bool {
	if size >= promoMaxSize {
		return false
	}
	base := path.Base(rel)
	base = strings.TrimSuffix(base, path.Ext(base))
	if !rePromo.MatchString(base) {
		return false
	}
	own := parseName(base)
	return own.Year == 0 && !own.SeasonEpisode && !own.AbsoluteEp
}

var extraSuffixType = map[string]string{
	"featurette":      ExtraFeaturette,
	"behindthescenes": ExtraBehindTheScenes,
	"deleted":         ExtraDeleted,
	"interview":       ExtraInterview,
	"scene":           ExtraOther,
	"short":           ExtraOther,
	"other":           ExtraOther,
}

// folder is what a single folder name says.
type folder struct {
	rel       string // relative path of this folder
	norm      string
	hint      Hint
	extraType string // "" unless this is an extras/trailers folder
	sample    bool
	season    int
	isSeason  bool
	parsed    Parsed // parseName of the folder name
}

func readFolder(rel string) folder {
	name := path.Base(rel)
	f := folder{rel: rel, norm: NormTitle(name)}
	f.hint = categoryFolders[f.norm]
	f.extraType = extraFolders[f.norm]
	f.sample = sampleFolders[f.norm]
	switch {
	case reSpecials.MatchString(f.norm):
		f.isSeason, f.season = true, 0
	case reSeasonFolder.MatchString(f.norm):
		f.isSeason, f.season = true, atoi(reSeasonFolder.FindStringSubmatch(f.norm)[1])
	}
	if f.hint == HintNone && f.extraType == "" && !f.sample {
		f.parsed = parseName(name)
		// "Show.S02.1080p.WEB-DL" is a season pack folder with a title.
		if !f.isSeason && f.parsed.HasSeason && f.parsed.Episode == 0 && !f.parsed.AbsoluteEp {
			f.isSeason, f.season = true, f.parsed.Season
		}
	}
	return f
}

// isTitleFolder reports whether the folder name can be a movie/series title.
func (f folder) isTitleFolder() bool {
	if f.hint != HintNone || f.extraType != "" || f.sample {
		return false
	}
	return f.parsed.Title != ""
}

// applyFolders fills the folder hints of p, nearest folder first.
func applyFolders(p *Parsed, dir string) {
	if dir == "" || dir == "." {
		return
	}
	seasonSet := false
	for d := dir; d != "" && d != "."; d = parentDir(d) {
		f := readFolder(d)
		if f.hint != HintNone && p.Hint == HintNone {
			p.Hint = f.hint
		}
		if f.isSeason && !seasonSet && p.TitleDir == "" {
			seasonSet = true
			p.SeasonFolder = true
			if !p.HasSeason {
				p.Season, p.HasSeason = f.season, true
			}
		}
		if p.TitleDir == "" && f.isTitleFolder() {
			p.TitleDir = d
			p.FolderTitle = f.parsed.Title
			p.FolderYear = f.parsed.Year
			if f.parsed.Group != "" && p.Group == "" {
				p.Group = f.parsed.Group
			}
		}
	}

	// A bare number ("01.mkv") inside a season/title folder is an episode.
	if p.Title != "" && !p.SeasonEpisode && !p.AbsoluteEp && (p.SeasonFolder || p.TitleDir != "") {
		if m := reTokNumber.FindStringSubmatch(strings.ToLower(p.Title)); m != nil && !reYear.MatchString(m[1]) {
			p.Absolute, p.Version, p.AbsoluteEp = atoi(m[1]), atoi(m[2]), true
			p.Title = ""
		}
	}
	// "Bleach/Bleach 366.mkv": the folder title followed by a number.
	if p.Title != "" && !p.SeasonEpisode && !p.AbsoluteEp && p.FolderTitle != "" {
		ft, t := NormTitle(p.FolderTitle), NormTitle(p.Title)
		if rest, ok := strings.CutPrefix(t, ft+" "); ok {
			if m := reTokNumber.FindStringSubmatch(rest); m != nil && !reYear.MatchString(m[1]) {
				p.Absolute, p.Version, p.AbsoluteEp = atoi(m[1]), atoi(m[2]), true
				p.Title = p.FolderTitle
			}
		}
	}
	// Season folder + episode number = regular season episode.
	if p.SeasonFolder && p.AbsoluteEp {
		p.Episode, p.Absolute = p.Absolute, 0
		p.AbsoluteEp, p.SeasonEpisode = false, true
	}
	if p.Title == "" {
		p.Title = p.FolderTitle
	}
	if p.Year == 0 && p.FolderYear != 0 && NormTitle(p.Title) == NormTitle(p.FolderTitle) {
		p.Year = p.FolderYear
	}
	// Movie in its own folder: "Inception (2010)/movie.mkv".
	if !p.SeasonEpisode && !p.AbsoluteEp && p.Year == 0 && p.FolderYear != 0 && !p.SeasonFolder {
		p.Title, p.Year = p.FolderTitle, p.FolderYear
	}
}

// detectSecondary marks samples, trailers and extras.
func detectSecondary(p *Parsed, base string) {
	// Folders: nearest extras/trailers/sample folder wins.
	for d := p.Dir; d != "" && d != "."; d = parentDir(d) {
		f := readFolder(d)
		switch {
		case f.sample:
			p.Secondary = SecondarySample
			return
		case f.extraType == ExtraTrailer:
			p.Secondary, p.ExtraType, p.OwnerDir = SecondaryTrailer, ExtraTrailer, parentDir(d)
			return
		case f.extraType != "":
			p.Secondary, p.ExtraType, p.OwnerDir = SecondaryExtra, f.extraType, parentDir(d)
			// a trailer inside "extras/" is still a trailer
			if reTrailer.MatchString(base) {
				p.Secondary, p.ExtraType = SecondaryTrailer, ExtraTrailer
			}
			return
		}
	}
	switch {
	case reSample.MatchString(base):
		p.Secondary = SecondarySample
	case reTrailer.MatchString(base):
		p.Secondary, p.ExtraType, p.OwnerDir = SecondaryTrailer, ExtraTrailer, p.Dir
	case reExtraSuffix.MatchString(base):
		m := reExtraSuffix.FindStringSubmatch(base)
		p.Secondary, p.ExtraType, p.OwnerDir = SecondaryExtra, extraSuffixType[strings.ToLower(m[1])], p.Dir
	}
}

func parentDir(d string) string {
	p := path.Dir(d)
	if p == "." || p == "/" {
		return ""
	}
	return p
}
