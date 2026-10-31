package scanner

// Kind is the result of classifying a file.
type Kind string

// Classification results.
const (
	KindMovie   Kind = "movie"
	KindSeries  Kind = "series"
	KindAnime   Kind = "anime"
	KindReview  Kind = "review"  // ambiguous: goes to the review queue
	KindIgnored Kind = "ignored" // samples (never shown, never touched)
	KindTrailer Kind = "trailer"
	KindExtra   Kind = "extra"
)

// Review reasons shown in the review queue.
const (
	ReasonNoTitle       = "no_title"
	ReasonNoYear        = "no_year"
	ReasonNoSeason      = "episode_without_season"
	ReasonSeasonNoEp    = "season_without_episode"
	ReasonDuplicate     = "duplicate_episode"
	ReasonNoOwner       = "extra_without_owner"
	ReasonHintConflict  = "folder_conflict"
	ReasonNotRecognized = "not_recognized"
)

// Classification is the category of a file plus why it went to review.
type Classification struct {
	Kind   Kind   `json:"kind"`
	Reason string `json:"reason,omitempty"`
}

// Classify applies the table of AGENTS.md §4. Folder hints only break ties:
// a clear signal in the file name always wins.
func Classify(p Parsed) Classification {
	switch p.Secondary {
	case SecondarySample:
		return Classification{Kind: KindIgnored}
	case SecondaryTrailer:
		return Classification{Kind: KindTrailer}
	case SecondaryExtra:
		return Classification{Kind: KindExtra}
	}
	if p.Title == "" {
		return review(ReasonNoTitle)
	}

	animeEvidence := p.Hint == HintAnime || (p.Group != "" && p.Hint != HintSeries && p.Hint != HintMovie)

	switch {
	case p.SeasonEpisode:
		if p.Hint == HintMovie && !animeEvidence && p.TitleDir == "" {
			return review(ReasonHintConflict) // S01E01 dumped in the movies folder root
		}
		if animeEvidence {
			return Classification{Kind: KindAnime}
		}
		return Classification{Kind: KindSeries}

	case p.AbsoluteEp:
		switch {
		case animeEvidence:
			return Classification{Kind: KindAnime}
		case p.Hint == HintSeries || p.TitleDir != "":
			return Classification{Kind: KindSeries} // "Show/Show - 01.mkv": season 1
		default:
			return review(ReasonNoSeason)
		}

	case p.HasSeason:
		return review(ReasonSeasonNoEp)

	case p.Year != 0:
		return Classification{Kind: KindMovie}

	case p.Hint == HintMovie:
		return Classification{Kind: KindMovie}

	case p.Hint == HintSeries || p.Hint == HintAnime:
		return review(ReasonNoSeason)

	default:
		return review(ReasonNoYear)
	}
}

func review(reason string) Classification {
	return Classification{Kind: KindReview, Reason: reason}
}
