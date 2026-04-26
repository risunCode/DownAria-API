package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Config struct {
	Level, Format string
	Output        io.Writer
}

type startupInfo struct {
	Addr         string
	Mode         string
	Dependencies map[string]string
}

func FallbackLogger(loggers ...*slog.Logger) *slog.Logger {
	for _, l := range loggers {
		if l != nil {
			return l
		}
	}
	return slog.Default()
}

func NewLogger(cfg Config) (*slog.Logger, error) {
	level, err := parseLevel(cfg.Level)
	if err != nil {
		return nil, err
	}
	output := cfg.Output
	if output == nil {
		output = os.Stdout
	}
	format := normalizeFormat(cfg.Format, output)
	options := &slog.HandlerOptions{Level: level}
	switch format {
	case "json":
		return slog.New(slog.NewJSONHandler(output, options)), nil
	case "text":
		return slog.New(slog.NewTextHandler(output, options)), nil
	case "pretty":
		return slog.New(newPrettyHandler(output, options)), nil
	default:
		return nil, fmt.Errorf("unsupported log format %q", cfg.Format)
	}
}

func PrintStartupBanner(output io.Writer, addr string, dependencies map[string]string, format string) {
	if output == nil {
		output = os.Stdout
	}
	mode := normalizeFormat(format, output)
	if mode != "pretty" {
		return
	}
	printer := newPrettyPrinter(output)
	banner := []string{
		" ______                        _              ",
		`|  __  \                      / \        _    `,
		`| |  \  | ___  _      __ _   /   \  _ __(_)   `,
		`| |   | |/ _ \| \ /\ / / _` + "`" + ` | / / \ \ | '_ \ |   `,
		`| |__/  | (_) \ V  V / (_| |/ /___\ \| |_) |   `,
		`|______/ \___/ \_/\_/ \__,_/_/     \_\ .__/|_| `,
		"                                     |_|       ",
		"                    API                        ",
	}
	for _, line := range banner {
		printer.writeLine(printer.accent(line))
	}
	info := startupInfo{Addr: strings.TrimSpace(addr), Mode: mode, Dependencies: dependencies}
	if info.Addr == "" {
		info.Addr = ":8080"
	}
	depNames := make([]string, 0, len(info.Dependencies))
	for name := range info.Dependencies {
		depNames = append(depNames, name)
	}
	sort.Strings(depNames)
	parts := []string{printer.key("addr") + "=" + printer.value(info.Addr), printer.key("mode") + "=" + printer.value(info.Mode)}
	for _, name := range depNames {
		parts = append(parts, printer.key(name)+"="+printer.status(info.Dependencies[name]))
	}
	printer.writeLine(strings.Join(parts, "  "))
	printer.writeLine("")
}

func parseLevel(value string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("unsupported log level %q", value)
	}
}

func normalizeFormat(value string, output io.Writer) string {
	format := strings.ToLower(strings.TrimSpace(value))
	if format == "" {
		if isTerminalWriter(output) {
			return "pretty"
		}
		return "json"
	}
	if format == "auto" {
		if isTerminalWriter(output) {
			return "pretty"
		}
		return "json"
	}
	return format
}

type prettyHandler struct {
	printer *prettyPrinter
	attrs   []slog.Attr
	groups  []string
	mu      sync.Mutex
	level   slog.Leveler
}

func newPrettyHandler(output io.Writer, options *slog.HandlerOptions) slog.Handler {
	if options == nil {
		options = &slog.HandlerOptions{}
	}
	return &prettyHandler{printer: newPrettyPrinter(output), level: options.Level}
}

func (h *prettyHandler) Enabled(_ context.Context, level slog.Level) bool {
	if h.level == nil {
		return true
	}
	return level >= h.level.Level()
}

func (h *prettyHandler) Handle(_ context.Context, record slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	fields := map[string]string{}
	for _, attr := range h.attrs {
		appendPrettyAttr(fields, h.groups, attr)
	}
	record.Attrs(func(attr slog.Attr) bool {
		appendPrettyAttr(fields, h.groups, attr)
		return true
	})
	timestamp := record.Time
	if timestamp.IsZero() {
		timestamp = time.Now()
	}
	parts := []string{h.printer.timestamp(timestamp), h.printer.level(record.Level)}
	if line := h.printer.specialLine(record.Message, fields); line != "" {
		parts = append(parts, line)
		h.printer.writeLine(strings.Join(parts, "  "))
		return nil
	}
	parts = append(parts, h.printer.message(humanizeMessage(record.Message)))
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range preferredKeys(keys) {
		parts = append(parts, h.printer.key(key)+"="+h.printer.fieldValue(key, fields[key]))
	}
	h.printer.writeLine(strings.Join(parts, "  "))
	return nil
}

func (h *prettyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &prettyHandler{
		printer: h.printer,
		attrs:   append(append([]slog.Attr{}, h.attrs...), attrs...),
		groups:  append([]string{}, h.groups...),
		level:   h.level,
	}
}

func (h *prettyHandler) WithGroup(name string) slog.Handler {
	return &prettyHandler{
		printer: h.printer,
		attrs:   append([]slog.Attr{}, h.attrs...),
		groups:  append(append([]string{}, h.groups...), name),
		level:   h.level,
	}
}

func appendPrettyAttr(fields map[string]string, groups []string, attr slog.Attr) {
	attr.Value = attr.Value.Resolve()
	if attr.Equal(slog.Attr{}) {
		return
	}
	key := attr.Key
	if len(groups) > 0 {
		key = strings.Join(append(append([]string{}, groups...), key), ".")
	}
	switch attr.Value.Kind() {
	case slog.KindGroup:
		nextGroups := append(append([]string{}, groups...), attr.Key)
		for _, nested := range attr.Value.Group() {
			appendPrettyAttr(fields, nextGroups, nested)
		}
	default:
		fields[key] = prettyValue(attr.Value)
	}
}

func prettyValue(value slog.Value) string {
	switch value.Kind() {
	case slog.KindString:
		return value.String()
	case slog.KindTime:
		return value.Time().Format(time.RFC3339)
	case slog.KindDuration:
		return value.Duration().String()
	case slog.KindBool:
		if value.Bool() {
			return "true"
		}
		return "false"
	case slog.KindInt64:
		return fmt.Sprintf("%d", value.Int64())
	case slog.KindUint64:
		return fmt.Sprintf("%d", value.Uint64())
	case slog.KindFloat64:
		return fmt.Sprintf("%g", value.Float64())
	case slog.KindAny:
		return fmt.Sprint(value.Any())
	default:
		return value.String()
	}
}

func preferredKeys(keys []string) []string {
	priority := map[string]int{"request_id": 0, "stage": 1, "platform": 2, "method": 3, "path": 4, "status": 5, "addr": 6, "url": 7, "quality": 8, "format": 9, "bytes": 10, "error": 11}
	sorted := append([]string{}, keys...)
	sort.SliceStable(sorted, func(i, j int) bool {
		li, lok := priority[sorted[i]]
		lj, jok := priority[sorted[j]]
		switch {
		case lok && jok && li != lj:
			return li < lj
		case lok != jok:
			return lok
		default:
			return sorted[i] < sorted[j]
		}
	})
	return sorted
}

type prettyPrinter struct {
	output   io.Writer
	isTTY    bool
	useColor bool
}

func newPrettyPrinter(output io.Writer) *prettyPrinter {
	return &prettyPrinter{output: output, isTTY: isTerminalWriter(output), useColor: isTerminalWriter(output) && os.Getenv("NO_COLOR") == ""}
}

func (p *prettyPrinter) writeLine(line string) {
	_, _ = io.WriteString(p.output, line+"\n")
}

func (p *prettyPrinter) timestamp(t time.Time) string { return p.dim(t.Format("15:04:05")) }
func (p *prettyPrinter) message(v string) string      { return p.bold(v) }
func (p *prettyPrinter) key(v string) string          { return p.dim(v) }
func (p *prettyPrinter) value(v string) string        { return p.plain(v) }
func (p *prettyPrinter) accent(v string) string       { return p.color("36", v) }

func (p *prettyPrinter) status(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "ok":
		return p.color("32", v)
	case "unavailable":
		return p.color("31", v)
	default:
		return p.color("33", v)
	}
}

func (p *prettyPrinter) level(level slog.Level) string {
	switch {
	case level >= slog.LevelError:
		return p.color("31", "ERROR")
	case level >= slog.LevelWarn:
		return p.color("33", "WARN")
	case level >= slog.LevelInfo:
		return p.color("32", "INFO")
	default:
		return p.color("36", "DEBUG")
	}
}

func (p *prettyPrinter) fieldValue(key string, value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return p.dim(`""`)
	}
	switch key {
	case "request_id":
		return p.dim(shortRequestID(trimmed))
	case "error":
		return p.color("31", trimmed)
	case "status", "stage", "platform", "mode":
		return p.color("35", trimmed)
	case "bytes", "duration":
		return p.color("34", trimmed)
	default:
		if strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "{") {
			return p.color("36", trimmed)
		}
		return p.plain(trimmed)
	}
}

func (p *prettyPrinter) specialLine(message string, fields map[string]string) string {
	switch strings.TrimSpace(message) {
	case "extract_stage":
		return p.extractLine(fields)
	case "http_request":
		return p.httpLine(fields)
	case "download_stage":
		return p.downloadLine(fields)
	}
	return ""
}

func (p *prettyPrinter) extractLine(fields map[string]string) string {
	stage := strings.TrimSpace(fields["stage"])
	event := humanizeStageEvent(stage, map[string]string{"native_extract": "Extract Start", "native_extract_completed": "Extract Complete", "universal_extract": "Extract Start", "universal_extract_completed": "Extract Complete", "cache_hit": "Extract Cache Hit"})
	parts := []string{p.message(event)}
	if platform := strings.TrimSpace(fields["platform"]); platform != "" {
		parts = append(parts, p.color("35", platform))
	}
	if id := shortRequestID(fields["request_id"]); id != "" {
		parts = append(parts, p.dim("id="+id))
	}
	if stage == "native_extract_completed" || stage == "universal_extract_completed" {
		if duration := compactDuration(fields["duration"]); duration != "" {
			parts = append(parts, p.color("34", duration))
		}
		return strings.Join(parts, "   ")
	}
	if rawURL := strings.TrimSpace(fields["url"]); rawURL != "" {
		parts = append(parts, p.plain(compactURL(rawURL)))
	}
	return strings.Join(parts, "   ")
}

func (p *prettyPrinter) httpLine(fields map[string]string) string {
	parts := []string{p.message("HTTP Request")}
	method := strings.TrimSpace(fields["method"])
	path := strings.TrimSpace(fields["path"])
	if method != "" || path != "" {
		parts = append(parts, p.plain(strings.TrimSpace(method+" "+path)))
	}
	if status := strings.TrimSpace(fields["status"]); status != "" {
		parts = append(parts, p.httpStatus(status))
	}
	if id := shortRequestID(fields["request_id"]); id != "" {
		parts = append(parts, p.dim("id="+id))
	}
	if duration := compactDuration(fields["duration"]); duration != "" {
		parts = append(parts, p.color("34", duration))
	}
	return strings.Join(parts, "   ")
}

func (p *prettyPrinter) downloadLine(fields map[string]string) string {
	stage := strings.TrimSpace(fields["stage"])
	event := humanizeStageEvent(stage, map[string]string{
		"grab_start":                       "Grab Download Start",
		"grab_complete":                    "Grab Download Complete",
		"grab_failed":                      "Grab Download Failed",
		"grab_upstream_status":             "Grab Download Failed",
		"ytdlp_format_download":            "YTDLP Download Start",
		"direct_preflight_failed_fallback": "Download Fallback",
		"ytdlp_direct_attempt":             "YTDLP Download Start",
		"ytdlp_direct_failed":              "YTDLP Download Failed",
		"ytdlp_direct_completed":           "YTDLP Download Complete",
	})
	parts := []string{p.message(event)}
	if id := shortRequestID(fields["request_id"]); id != "" {
		parts = append(parts, p.dim("id="+id))
	}
	switch stage {
	case "direct_preflight_failed_fallback":
		parts = append(parts, p.plain("method=direct -> ytdlp"))
	case "ytdlp_direct_attempt":
		parts = append(parts, p.plain("method=ytdlp"))
	case "grab_upstream_status":
		if status := strings.TrimSpace(fields["status"]); status != "" {
			parts = append(parts, p.httpStatus(status), p.plain("source=grab"))
		}
	case "completed", "grab_complete", "ytdlp_direct_completed":
		if bytes := strings.TrimSpace(fields["bytes"]); bytes != "" {
			parts = append(parts, p.color("34", bytes+" B"))
		}
	}
	if stage != "direct_preflight_failed_fallback" && stage != "grab_upstream_status" && stage != "ytdlp_direct_attempt" && stage != "grab_start" {
		if rawURL := strings.TrimSpace(firstNonEmpty(fields["source_url"], fields["url"])); rawURL != "" {
			parts = append(parts, p.plain(compactURL(rawURL)))
		}
	}
	return strings.Join(parts, "   ")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (p *prettyPrinter) httpStatus(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "2") {
		return p.color("32", trimmed)
	}
	if strings.HasPrefix(trimmed, "4") {
		return p.color("33", trimmed)
	}
	if strings.HasPrefix(trimmed, "5") {
		return p.color("31", trimmed)
	}
	return p.plain(trimmed)
}

func humanizeMessage(message string) string {
	replacer := strings.NewReplacer("_", " ", "-", " ")
	parts := strings.Fields(replacer.Replace(strings.TrimSpace(message)))
	for i, part := range parts {
		parts[i] = titleWord(strings.ToLower(part))
	}
	return strings.Join(parts, " ")
}

func titleWord(value string) string {
	if value == "" {
		return ""
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func humanizeStageEvent(stage string, mapped map[string]string) string {
	if value := mapped[strings.TrimSpace(stage)]; value != "" {
		return value
	}
	return humanizeMessage(stage)
}

func compactURL(rawURL string) string {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return ""
	}
	trimmed = strings.TrimPrefix(trimmed, "https://")
	trimmed = strings.TrimPrefix(trimmed, "http://")
	if idx := strings.Index(trimmed, "/"); idx >= 0 {
		return trimmed[idx:]
	}
	return trimmed
}

func shortRequestID(value string) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) > 8 {
		return trimmed[:8]
	}
	return trimmed
}

func compactDuration(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	trimmed = strings.TrimSuffix(trimmed, "s")
	if f, err := strconv.ParseFloat(trimmed, 64); err == nil {
		return fmt.Sprintf("%.2fs", f)
	}
	return strings.TrimSpace(value)
}

func (p *prettyPrinter) bold(v string) string  { return p.color("1", v) }
func (p *prettyPrinter) dim(v string) string   { return p.color("90", v) }
func (p *prettyPrinter) plain(v string) string { return v }

func (p *prettyPrinter) color(code string, value string) string {
	if !p.useColor || strings.TrimSpace(value) == "" {
		return value
	}
	return "\x1b[" + code + "m" + value + "\x1b[0m"
}

func isTerminalWriter(output io.Writer) bool {
	file, ok := output.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}
