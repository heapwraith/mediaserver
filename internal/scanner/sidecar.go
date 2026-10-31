package scanner

import (
	"path"
	"regexp"
	"strings"
)

// ExternalSub is a sidecar subtitle matched to a video.
type ExternalSub struct {
	RelPath  string
	Language string // BCP 47 when known: "pt-BR", "en", "ja"
	Codec    string // srt, ass, ssa, webvtt
	Forced   bool
	Default  bool
	Title    string
}

// Folders that hold subtitles for the videos of their parent folder.
var subFolders = setOf("subs", "sub", "subtitles", "subtitle", "legendas", "legenda")

var subCodec = map[string]string{".srt": "srt", ".ass": "ass", ".ssa": "ssa", ".vtt": "webvtt"}

// videoRef is the part of a video file sidecar matching needs.
type videoRef struct {
	id   string
	rel  string
	dir  string
	base string // file name without extension
}

// matchSubtitles links sidecar subtitles to videos:
//  1. same folder (or a Subs/ folder below it) and name starting with the
//     video name: "Movie.mkv" ← "Movie.pt-BR.srt", "Movie.en.forced.ass";
//  2. otherwise, when that folder has a single video, it owns the subtitle.
func matchSubtitles(videos []videoRef, subs []string) map[string][]ExternalSub {
	byDir := map[string][]videoRef{}
	for _, v := range videos {
		byDir[v.dir] = append(byDir[v.dir], v)
	}
	out := map[string][]ExternalSub{}
	for _, rel := range subs {
		if !IsSubtitle(rel) {
			continue
		}
		dir, file := splitRel(rel)
		ext := strings.ToLower(path.Ext(file))
		base := strings.TrimSuffix(file, path.Ext(file))

		videoDir, wantBase := dir, ""
		if subFolders[strings.ToLower(path.Base(dir))] {
			videoDir = parentDir(dir) // Movie/Subs/English.srt
		} else if subFolders[strings.ToLower(path.Base(parentDir(dir)))] {
			videoDir, wantBase = parentDir(parentDir(dir)), path.Base(dir) // Show/Subs/<episode>/2_English.srt
		}
		candidates := byDir[videoDir]
		if len(candidates) == 0 {
			continue
		}

		var owner *videoRef
		rest := ""
		for i := range candidates {
			v := &candidates[i]
			switch {
			case wantBase != "" && v.base == wantBase:
				owner, rest = v, base
			case wantBase == "" && (base == v.base || strings.HasPrefix(base, v.base+".")):
				if owner == nil || len(v.base) > len(owner.base) {
					owner, rest = v, strings.TrimPrefix(strings.TrimPrefix(base, v.base), ".")
				}
			}
		}
		if owner == nil && len(candidates) == 1 {
			owner, rest = &candidates[0], base
		}
		if owner == nil {
			continue
		}
		sub := ExternalSub{RelPath: rel, Codec: subCodec[ext]}
		sub.Language, sub.Forced, sub.Default, sub.Title = subLanguage(rest)
		out[owner.id] = append(out[owner.id], sub)
	}
	return out
}

var reLangTag = regexp.MustCompile(`^([a-z]{2,3})(?:[-_]([a-z]{2}))?$`)

// languageNames maps words and ISO 639-2 codes to BCP 47.
var languageNames = map[string]string{
	"english": "en", "eng": "en", "en": "en",
	"portuguese": "pt", "portugues": "pt", "por": "pt", "pt": "pt",
	"brazilian": "pt-BR", "pob": "pt-BR", "ptbr": "pt-BR", "pt-br": "pt-BR", "pt_br": "pt-BR", "brazil": "pt-BR",
	"japanese": "ja", "jpn": "ja", "jap": "ja", "ja": "ja", "jp": "ja",
	"spanish": "es", "spa": "es", "es": "es", "espanol": "es", "latino": "es-419",
	"french": "fr", "fre": "fr", "fra": "fr", "fr": "fr",
	"german": "de", "ger": "de", "deu": "de", "de": "de",
	"italian": "it", "ita": "it", "it": "it",
	"korean": "ko", "kor": "ko", "ko": "ko",
	"chinese": "zh", "chi": "zh", "zho": "zh", "zh": "zh",
	"russian": "ru", "rus": "ru", "ru": "ru",
	"arabic": "ar", "ara": "ar", "ar": "ar",
	"dutch": "nl", "dut": "nl", "nld": "nl", "nl": "nl",
	"polish": "pl", "pol": "pl", "pl": "pl",
	"turkish": "tr", "tur": "tr", "tr": "tr",
}

// subLanguage reads "pt-BR", "en.forced", "2_English", "Portuguese (Brazil)".
func subLanguage(s string) (lang string, forced, def bool, title string) {
	words := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return r == '.' || r == ' ' || r == '(' || r == ')' || r == '[' || r == ']' || r == ','
	})
	for _, w := range words {
		w = strings.TrimLeft(w, "0123456789_") // "2_English"
		switch w {
		case "forced", "foreign":
			forced = true
			continue
		case "default":
			def = true
			continue
		case "sdh", "cc", "hi":
			title = "SDH"
			continue
		case "brazil", "brasil", "br":
			if lang == "pt" {
				lang = "pt-BR"
				continue
			}
		}
		if lang != "" {
			continue
		}
		if l, ok := languageNames[w]; ok {
			lang = l
			continue
		}
		if m := reLangTag.FindStringSubmatch(w); m != nil {
			if l, ok := languageNames[m[1]]; ok {
				lang = l
				if m[2] != "" {
					lang = strings.SplitN(l, "-", 2)[0] + "-" + strings.ToUpper(m[2])
				}
			}
		}
	}
	return lang, forced, def, title
}

// Artwork holds local images for a movie, series or season (relative paths).
type Artwork struct {
	Poster string
	Fanart string
}

var (
	posterNames = setOf("poster", "folder", "cover", "movie", "show")
	fanartNames = setOf("fanart", "backdrop", "background", "art", "banner")
	reSeasonArt = regexp.MustCompile(`^season[ _-]?(\d{1,2}|specials|all)-poster$`)
)

// imagesByDir indexes sidecar images by folder: dir → lowercase base → rel.
func imagesByDir(sidecars []string) map[string]map[string]string {
	out := map[string]map[string]string{}
	for _, rel := range sidecars {
		if !IsImage(rel) {
			continue
		}
		dir, file := splitRel(rel)
		base := strings.ToLower(strings.TrimSuffix(file, path.Ext(file)))
		if out[dir] == nil {
			out[dir] = map[string]string{}
		}
		if _, dup := out[dir][base]; !dup {
			out[dir][base] = rel
		}
	}
	return out
}

// folderArt finds poster.jpg / folder.jpg / fanart.jpg in a folder.
// prefix ("Movie Name") also accepts "Movie Name-poster.jpg".
func folderArt(images map[string]string, prefix string) Artwork {
	var art Artwork
	prefix = strings.ToLower(prefix)
	for base, rel := range images {
		kind := base
		if prefix != "" {
			if rest, ok := strings.CutPrefix(base, prefix+"-"); ok {
				kind = rest
			}
		}
		switch {
		case posterNames[kind] && (art.Poster == "" || kind == "poster"):
			art.Poster = rel
		case fanartNames[kind] && (art.Fanart == "" || kind == "fanart"):
			art.Fanart = rel
		}
	}
	return art
}

// seasonArt finds "season01-poster.jpg" / "season-specials-poster.jpg" in
// the series folder.
func seasonArt(images map[string]string, season int) string {
	for base, rel := range images {
		m := reSeasonArt.FindStringSubmatch(base)
		if m == nil {
			continue
		}
		if (m[1] == "specials" && season == 0) || (m[1] != "all" && m[1] != "specials" && atoi(m[1]) == season) {
			return rel
		}
	}
	return ""
}

func splitRel(rel string) (dir, file string) {
	dir, file = path.Split(rel)
	return strings.TrimSuffix(dir, "/"), file
}
