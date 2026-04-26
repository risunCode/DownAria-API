package media

import (
	"fmt"
	"strconv"
	"strings"

	"downaria-api/internal/extract"
)

type Candidate struct {
	FormatID      string
	URL           string
	MIMEType      string
	Quality       string
	FileSizeBytes int64
	HasAudio      bool
	HasVideo      bool
	IsProgressive bool
	Protocol      string
	Container     string
	Referer       string
	Origin        string
	Width         int
	Height        int
	Source        extract.MediaSource
}

type Selection struct {
	Mode        string
	Video       *Candidate
	Audio       *Candidate
	SelectedIDs []string
}

func Select(result *extract.Result, quality, format string) (*Selection, error) {
	return selectWithExcludedURLs(result, quality, format, nil)
}

func selectWithExcludedURLs(result *extract.Result, quality, format string, excludedURLs map[string]struct{}) (*Selection, error) {
	if result == nil {
		return nil, extract.WrapCode(extract.KindExtractionFailed, "selector_empty_result", "extraction result is empty", false, nil)
	}
	container := normalizeContainer(format)
	videoCandidates, audioCandidates := collectCandidates(result)
	videoCandidates = filterExcludedCandidates(videoCandidates, excludedURLs)
	audioCandidates = filterExcludedCandidates(audioCandidates, excludedURLs)
	targetHeight := parseTargetHeight(quality)

	if container == "mp4" {
		if progressive := pickProgressive(videoCandidates, targetHeight, "mp4"); progressive != nil && progressiveMatchesTarget(progressive, targetHeight) {
			return selectionFor("progressive", progressive, nil), nil
		}
		video := pickVideo(videoCandidates, targetHeight, "mp4")
		audio := pickAudio(audioCandidates, []string{"140", "139"}, []string{"m4a", "aac", "mp4"})
		if video != nil && audio != nil {
			return selectionFor("separate", video, audio), nil
		}
		if progressive := pickProgressive(videoCandidates, targetHeight, "mp4"); progressive != nil {
			return selectionFor("progressive", progressive, nil), nil
		}
	}

	if progressive := pickProgressive(videoCandidates, targetHeight, container); progressive != nil {
		return selectionFor("progressive", progressive, nil), nil
	}
	video := pickVideo(videoCandidates, targetHeight, container)
	audio := pickAudio(audioCandidates, nil, nil)
	if video != nil && audio != nil {
		return selectionFor("fallback", video, audio), nil
	}
	return nil, extract.WrapCode(extract.KindExtractionFailed, "selector_no_match", fmt.Sprintf("no source matched quality %q", strings.TrimSpace(quality)), false, nil)
}

func selectionFor(mode string, video, audio *Candidate) *Selection {
	ids := make([]string, 0, 2)
	appendID := func(candidate *Candidate) {
		if candidate != nil && strings.TrimSpace(candidate.FormatID) != "" {
			ids = append(ids, strings.TrimSpace(candidate.FormatID))
		}
	}
	appendID(video)
	appendID(audio)
	return &Selection{Mode: mode, Video: video, Audio: audio, SelectedIDs: ids}
}

func collectCandidates(result *extract.Result) ([]Candidate, []Candidate) {
	videos := []Candidate{}
	audios := []Candidate{}
	for _, item := range result.Media {
		for _, source := range item.Sources {
			height := source.Height
			if height <= 0 {
				height = inferHeightFromSource(source)
			}
			quality := strings.TrimSpace(source.Quality)
			if quality == "" && height > 0 {
				quality = fmt.Sprintf("%dp", height)
			}
			candidate := Candidate{
				FormatID:      strings.TrimSpace(source.FormatID),
				URL:           strings.TrimSpace(source.URL),
				Referer:       strings.TrimSpace(source.Referer),
				Origin:        strings.TrimSpace(source.Origin),
				MIMEType:      strings.TrimSpace(source.MIMEType),
				Quality:       quality,
				FileSizeBytes: source.FileSizeBytes,
				HasAudio:      source.HasAudio,
				HasVideo:      source.HasVideo,
				IsProgressive: source.IsProgressive,
				Protocol:      strings.TrimSpace(source.Protocol),
				Container:     strings.ToLower(strings.TrimSpace(source.Container)),
				Width:         source.Width,
				Height:        height,
				Source:        source,
			}
			if candidate.URL == "" {
				continue
			}
			switch {
			case item.Type == "video":
				candidate.HasVideo = true
				videos = append(videos, candidate)
			case item.Type == "audio":
				candidate.HasAudio = true
				audios = append(audios, candidate)
			case candidate.HasVideo:
				videos = append(videos, candidate)
			case candidate.HasAudio:
				audios = append(audios, candidate)
			}
		}
	}
	return dedupeCandidates(videos), dedupeCandidates(audios)
}

func dedupeCandidates(candidates []Candidate) []Candidate {
	if len(candidates) < 2 {
		return candidates
	}
	bestByURL := make(map[string]Candidate, len(candidates))
	order := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		key := strings.TrimSpace(candidate.URL)
		if key == "" {
			continue
		}
		current, ok := bestByURL[key]
		if !ok {
			bestByURL[key] = candidate
			order = append(order, key)
			continue
		}
		if candidateRichness(candidate) > candidateRichness(current) {
			bestByURL[key] = candidate
		}
	}
	result := make([]Candidate, 0, len(order))
	for _, key := range order {
		result = append(result, bestByURL[key])
	}
	return result
}

func filterExcludedCandidates(candidates []Candidate, excludedURLs map[string]struct{}) []Candidate {
	if len(candidates) == 0 || len(excludedURLs) == 0 {
		return candidates
	}
	filtered := make([]Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		if _, blocked := excludedURLs[strings.TrimSpace(candidate.URL)]; blocked {
			continue
		}
		filtered = append(filtered, candidate)
	}
	return filtered
}

func candidateRichness(candidate Candidate) int {
	score := 0
	if candidate.Height > 0 {
		score += 4
	}
	if candidate.Width > 0 {
		score += 2
	}
	if strings.TrimSpace(candidate.Quality) != "" {
		score += 3
	}
	if candidate.FileSizeBytes > 0 {
		score += 2
	}
	if candidate.HasAudio {
		score += 1
	}
	if candidate.IsProgressive {
		score += 1
	}
	return score
}

func inferHeightFromSource(source extract.MediaSource) int {
	quality := strings.ToLower(strings.TrimSpace(source.Quality))
	if strings.HasSuffix(quality, "p") {
		if value, err := strconv.Atoi(strings.TrimSuffix(quality, "p")); err == nil {
			return value
		}
	}
	lowerURL := strings.ToLower(strings.TrimSpace(source.URL))
	for _, token := range []string{"2160p", "1440p", "1080p", "720p", "480p", "360p", "240p", "144p"} {
		if strings.Contains(lowerURL, token) {
			if value, err := strconv.Atoi(strings.TrimSuffix(token, "p")); err == nil {
				return value
			}
		}
	}
	return 0
}

func pickProgressive(candidates []Candidate, targetHeight int, container string) *Candidate {
	filtered := make([]Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		if !candidate.HasVideo || !candidate.HasAudio || !candidate.IsProgressive {
			continue
		}
		filtered = append(filtered, candidate)
	}
	return bestVideoCandidate(filtered, targetHeight, container)
}

func pickVideo(candidates []Candidate, targetHeight int, container string) *Candidate {
	filtered := make([]Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		if !candidate.HasVideo || candidate.IsProgressive {
			continue
		}
		filtered = append(filtered, candidate)
	}
	return bestVideoCandidate(filtered, targetHeight, container)
}

func bestVideoCandidate(candidates []Candidate, targetHeight int, container string) *Candidate {
	if len(candidates) == 0 {
		return nil
	}
	preferred := filterContainer(candidates, container)
	if len(preferred) == 0 {
		preferred = candidates
	}
	best := preferred[0]
	for i := 1; i < len(preferred); i++ {
		if compareVideo(preferred[i], best, targetHeight, container) < 0 {
			best = preferred[i]
		}
	}
	return &best
}

func pickAudio(candidates []Candidate, preferredIDs, preferredContainers []string) *Candidate {
	if len(candidates) == 0 {
		return nil
	}
	best := candidates[0]
	for i := 1; i < len(candidates); i++ {
		if compareAudio(candidates[i], best, preferredIDs, preferredContainers) < 0 {
			best = candidates[i]
		}
	}
	return &best
}

func filterContainer(candidates []Candidate, container string) []Candidate {
	container = normalizeContainer(container)
	if container == "" {
		return candidates
	}
	filtered := make([]Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Container == container {
			filtered = append(filtered, candidate)
		}
	}
	return filtered
}

func compareVideo(left, right Candidate, targetHeight int, preferredContainer string) int {
	if cmp := compareVideoTarget(left, right, targetHeight); cmp != 0 {
		return cmp
	}
	if cmp := compareInt(containerRank(left.Container, preferredContainer), containerRank(right.Container, preferredContainer)); cmp != 0 {
		return cmp
	}
	if cmp := compareInt(protocolRank(left.Protocol), protocolRank(right.Protocol)); cmp != 0 {
		return cmp
	}
	if cmp := compareBool(left.IsProgressive, right.IsProgressive); cmp != 0 {
		return cmp
	}
	if cmp := compareInt64(right.FileSizeBytes, left.FileSizeBytes); cmp != 0 {
		return cmp
	}
	if cmp := compareInt(right.Width, left.Width); cmp != 0 {
		return cmp
	}
	return strings.Compare(left.FormatID+left.URL, right.FormatID+right.URL)
}

func compareAudio(left, right Candidate, preferredIDs, preferredContainers []string) int {
	if cmp := compareInt(idRank(left.FormatID, preferredIDs), idRank(right.FormatID, preferredIDs)); cmp != 0 {
		return cmp
	}
	if cmp := compareInt(containerListRank(left.Container, preferredContainers), containerListRank(right.Container, preferredContainers)); cmp != 0 {
		return cmp
	}
	if cmp := compareInt(protocolRank(left.Protocol), protocolRank(right.Protocol)); cmp != 0 {
		return cmp
	}
	if cmp := compareInt64(right.FileSizeBytes, left.FileSizeBytes); cmp != 0 {
		return cmp
	}
	return strings.Compare(left.FormatID+left.URL, right.FormatID+right.URL)
}

func compareVideoTarget(left, right Candidate, targetHeight int) int {
	if targetHeight <= 0 {
		if cmp := compareInt(right.Height, left.Height); cmp != 0 {
			return cmp
		}
		return 0
	}
	leftClass, leftDiff := targetClass(left.Height, targetHeight)
	rightClass, rightDiff := targetClass(right.Height, targetHeight)
	if cmp := compareInt(leftClass, rightClass); cmp != 0 {
		return cmp
	}
	if cmp := compareInt(leftDiff, rightDiff); cmp != 0 {
		return cmp
	}
	if leftClass == 1 || leftClass == 0 {
		if cmp := compareInt(right.Height, left.Height); cmp != 0 {
			return cmp
		}
	}
	if leftClass == 2 {
		if cmp := compareInt(left.Height, right.Height); cmp != 0 {
			return cmp
		}
	}
	return 0
}

func targetClass(height, target int) (int, int) {
	if height <= 0 {
		return 3, target
	}
	if height == target {
		return 0, 0
	}
	if height < target {
		return 1, target - height
	}
	return 2, height - target
}

func progressiveMatchesTarget(candidate *Candidate, targetHeight int) bool {
	if candidate == nil {
		return false
	}
	if targetHeight <= 0 {
		return true
	}
	return candidate.Height >= targetHeight
}

func containerRank(container, preferred string) int {
	container = normalizeContainer(container)
	preferred = normalizeContainer(preferred)
	if preferred == "" || container == preferred {
		return 0
	}
	if preferred == "mp4" && (container == "m4a" || container == "aac") {
		return 1
	}
	return 2
}

func containerListRank(container string, preferred []string) int {
	container = normalizeContainer(container)
	if len(preferred) == 0 {
		if container == "" {
			return 1
		}
		return 0
	}
	for index, item := range preferred {
		if container == normalizeContainer(item) {
			return index
		}
	}
	if container == "" {
		return len(preferred) + 1
	}
	return len(preferred)
}

func idRank(formatID string, preferred []string) int {
	formatID = strings.TrimSpace(formatID)
	if len(preferred) == 0 {
		if formatID == "" {
			return 1
		}
		return 0
	}
	for index, item := range preferred {
		if formatID == item {
			return index
		}
	}
	if formatID == "" {
		return len(preferred) + 1
	}
	return len(preferred)
}

func protocolRank(protocol string) int {
	switch normalizedProtocol(protocol) {
	case "https", "http":
		return 0
	case "http_dash_segments":
		return 1
	case "m3u8_native", "m3u8":
		return 2
	case "dash":
		return 3
	case "":
		return 4
	default:
		return 2
	}
}

func normalizedProtocol(protocol string) string {
	return strings.ToLower(strings.TrimSpace(protocol))
}

func normalizeContainer(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "aac" {
		return "m4a"
	}
	return value
}

func parseTargetHeight(value string) int {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.TrimSuffix(value, "p")
	height, _ := strconv.Atoi(value)
	return height
}

func compareInt(left, right int) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

func compareInt64(left, right int64) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

func compareBool(left, right bool) int {
	switch {
	case left == right:
		return 0
	case left:
		return -1
	default:
		return 1
	}
}
