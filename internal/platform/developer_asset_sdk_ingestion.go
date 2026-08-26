package platform

import (
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

const (
	sdkIngestionPipelineVersion   = "sdk-static-ingestion/1"
	sdkIngestionParserVersion     = "sdk-static-parser/2"
	sdkIngestionNormalizerVersion = "sdk-text-normalizer/1"
	sdkIngestionMapperVersion     = "sdk-map/1"
	sdkIngestionMapVersion        = "sdk-map/2026-08-26"
	sdkIngestionMaxFiles          = 500
	sdkIngestionMaxFileBytes      = 2 << 20
	sdkIngestionMaxTotalBytes     = 20 << 20
)

var (
	sdkMarkdownHeadingPattern = regexp.MustCompile(`^(#{1,6})[ \t]+(.+?)\s*$`)
	sdkFencePattern           = regexp.MustCompile("^\\s*(```+|~~~+)\\s*([^\\s`]*)?.*$")
	sdkSecretPattern          = regexp.MustCompile(`(?im)(-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----|\bAKIA[0-9A-Z]{16}\b|\bgh[pousr]_[A-Za-z0-9_]{30,}\b|\bnpm_[A-Za-z0-9]{30,}\b|authorization\s*:\s*bearer\s+[A-Za-z0-9._~+/=-]{16,})`)
	sdkDrivePathPattern       = regexp.MustCompile(`^[A-Za-z]:/`)
	sdkSourceHashPattern      = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	sdkJSImportPattern        = regexp.MustCompile(`(?m)(?:from\s+|require\s*\(\s*)["']([^"']+)["']`)
	sdkPythonImportPattern    = regexp.MustCompile(`(?m)^\s*(?:from|import)\s+([A-Za-z0-9_.]+)`)
	sdkGoImportPattern        = regexp.MustCompile("(?m)^\\s*import\\s+(?:[A-Za-z0-9_.]+\\s+)?[\"`]([^\"`]+)[\"`]")
	sdkGenericSymbolPatterns  = map[string][]sdkSymbolPattern{
		"typescript": {
			{regexp.MustCompile(`^(?:export\s+)?(?:declare\s+)?class\s+([A-Za-z_$][\w$]*)`), "class"},
			{regexp.MustCompile(`^(?:export\s+)?(?:declare\s+)?interface\s+([A-Za-z_$][\w$]*)`), "interface"},
			{regexp.MustCompile(`^(?:export\s+)?(?:declare\s+)?type\s+([A-Za-z_$][\w$]*)`), "type"},
			{regexp.MustCompile(`^(?:export\s+)?(?:async\s+)?function\s+([A-Za-z_$][\w$]*)`), "function"},
		},
		"javascript": {
			{regexp.MustCompile(`^(?:export\s+)?(?:default\s+)?class\s+([A-Za-z_$][\w$]*)`), "class"},
			{regexp.MustCompile(`^(?:export\s+)?(?:async\s+)?function\s+([A-Za-z_$][\w$]*)`), "function"},
		},
		"python": {
			{regexp.MustCompile(`^class\s+([A-Za-z_]\w*)`), "class"},
			{regexp.MustCompile(`^(?:async\s+)?def\s+([A-Za-z_]\w*)`), "function"},
		},
		"java": {
			{regexp.MustCompile(`^(?:public\s+|protected\s+|private\s+)?(?:abstract\s+|final\s+)?class\s+([A-Za-z_]\w*)`), "class"},
			{regexp.MustCompile(`^(?:public\s+|protected\s+|private\s+)?interface\s+([A-Za-z_]\w*)`), "interface"},
		},
		"csharp": {
			{regexp.MustCompile(`^(?:public\s+|internal\s+|protected\s+|private\s+)?(?:sealed\s+|abstract\s+)?class\s+([A-Za-z_]\w*)`), "class"},
			{regexp.MustCompile(`^(?:public\s+|internal\s+|protected\s+|private\s+)?interface\s+([A-Za-z_]\w*)`), "interface"},
		},
		"ruby": {
			{regexp.MustCompile(`^class\s+([A-Za-z_:]\w*(?:::\w+)*)`), "class"},
			{regexp.MustCompile(`^def\s+(?:self\.)?([A-Za-z_]\w*[!?=]?)`), "method"},
		},
	}
)

type sdkSymbolPattern struct {
	pattern *regexp.Regexp
	kind    string
}

// SDKIngestionFile is a bounded, text-only file supplied for one exact SDK
// release. DokoSoko records reviewable normalized content; it does not host or
// execute package archives.
type SDKIngestionFile struct {
	SourcePath        string `json:"source_path"`
	Content           string `json:"content"`
	MediaType         string `json:"media_type,omitempty"`
	Language          string `json:"language,omitempty"`
	Role              string `json:"role,omitempty"`
	Attribution       string `json:"attribution,omitempty"`
	LicenseExpression string `json:"license_expression,omitempty"`
	EntryType         string `json:"entry_type,omitempty"`
}

type SDKContentIngestionInput struct {
	Files                  []SDKIngestionFile `json:"files"`
	ResolvedSourceURI      string             `json:"resolved_source_uri,omitempty"`
	ResolvedSourceRevision string             `json:"resolved_source_revision,omitempty"`
	ExpectedSourceHash     string             `json:"expected_source_hash,omitempty"`
}

type SDKContentIngestionResult struct {
	Run             model.DeveloperAssetIngestionRun `json:"run"`
	Candidate       store.SDKContentCandidateRecord  `json:"candidate"`
	AlreadyIngested bool                             `json:"already_ingested"`
}

type normalizedSDKFile struct {
	input              SDKIngestionFile
	path               string
	content            string
	mediaType          string
	language           string
	role               string
	disposition        string
	exclusionReason    string
	rawHash            string
	contentHash        string
	byteSize           int64
	containsCredential bool
}

type sdkSourceManifestEntry struct {
	SourcePath           string `json:"source_path"`
	MediaType            string `json:"media_type"`
	Language             string `json:"language,omitempty"`
	Role                 string `json:"role"`
	ByteSize             int64  `json:"byte_size"`
	RawHash              string `json:"raw_hash"`
	NormalizedHash       string `json:"normalized_hash,omitempty"`
	SuggestedDisposition string `json:"suggested_disposition"`
	ExclusionReason      string `json:"exclusion_reason,omitempty"`
}

type sdkIngestionDiagnostic struct {
	Code       string `json:"code"`
	Severity   string `json:"severity"`
	SourcePath string `json:"source_path,omitempty"`
	Message    string `json:"message"`
}

type sdkBuild struct {
	record       store.SDKContentCandidateRecord
	manifest     []sdkSourceManifestEntry
	diagnostics  []sdkIngestionDiagnostic
	manifestHash string
	contentHash  string
	mapHash      string
}

func normalizeSDKLanguage(value, sourcePath string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	aliases := map[string]string{
		"ts": "typescript", "tsx": "typescript", "js": "javascript", "jsx": "javascript",
		"py": "python", "golang": "go", "cs": "csharp", "c#": "csharp", "rb": "ruby",
		"sh": "shell", "bash": "shell", "yml": "yaml", "md": "markdown",
	}
	if canonical := aliases[value]; canonical != "" {
		return canonical
	}
	if value != "" {
		return value
	}
	ext := strings.ToLower(path.Ext(sourcePath))
	return map[string]string{
		".go": "go", ".ts": "typescript", ".tsx": "typescript", ".js": "javascript", ".jsx": "javascript",
		".mjs": "javascript", ".cjs": "javascript", ".py": "python", ".java": "java", ".cs": "csharp",
		".rb": "ruby", ".php": "php", ".rs": "rust", ".swift": "swift", ".kt": "kotlin",
		".sh": "shell", ".bash": "shell", ".json": "json", ".yaml": "yaml", ".yml": "yaml",
		".md": "markdown", ".mdx": "markdown", ".toml": "toml", ".xml": "xml",
	}[ext]
}

func normalizeSDKPath(value string) (string, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, `\`, "/"))
	if value == "" || strings.ContainsRune(value, 0) || strings.HasPrefix(value, "/") || sdkDrivePathPattern.MatchString(value) {
		return "", errors.New("SDK source paths must be non-empty relative paths")
	}
	cleaned := path.Clean(value)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", errors.New("SDK source paths cannot escape the package root")
	}
	if len(cleaned) > 500 {
		return "", errors.New("SDK source path exceeds 500 characters")
	}
	return cleaned, nil
}

func inferSDKFileRole(sourcePath, requested string) (string, string, error) {
	requested = strings.ToLower(strings.TrimSpace(requested))
	allowed := map[string]bool{
		"readme": true, "guide": true, "reference": true, "example": true, "manifest": true,
		"source": true, "test": true, "generated": true, "vendor": true, "other": true,
	}
	if requested != "" && !allowed[requested] {
		return "", "", errors.New("unsupported SDK file role")
	}
	if requested != "" {
		if requested == "test" || requested == "generated" || requested == "vendor" {
			return requested, "excluded", nil
		}
		return requested, "included", nil
	}
	lower := strings.ToLower(sourcePath)
	base := path.Base(lower)
	switch {
	case strings.HasPrefix(base, "readme"):
		return "readme", "included", nil
	case strings.Contains(lower, "/node_modules/") || strings.Contains(lower, "/vendor/") || strings.HasPrefix(lower, "vendor/"):
		return "vendor", "excluded", nil
	case strings.Contains(lower, "/generated/") || strings.Contains(lower, "/dist/") || strings.Contains(lower, "/build/") || strings.HasSuffix(base, ".generated.go"):
		return "generated", "excluded", nil
	case strings.Contains(lower, "/test/") || strings.Contains(lower, "/tests/") || strings.HasSuffix(base, "_test.go") || strings.Contains(base, ".test.") || strings.Contains(base, ".spec."):
		return "test", "excluded", nil
	case base == "package.json" || base == "pyproject.toml" || base == "setup.py" || base == "cargo.toml" || base == "go.mod" || base == "pom.xml" || base == "build.gradle" || strings.HasSuffix(base, ".csproj"):
		return "manifest", "included", nil
	case strings.Contains(lower, "/example") || strings.Contains(lower, "/sample") || strings.HasPrefix(lower, "example") || strings.HasPrefix(lower, "sample"):
		return "example", "included", nil
	case strings.HasPrefix(lower, "docs/") || strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".mdx"):
		if strings.Contains(lower, "reference") || strings.Contains(lower, "api") {
			return "reference", "included", nil
		}
		return "guide", "included", nil
	case normalizeSDKLanguage("", sourcePath) != "":
		return "source", "included", nil
	default:
		return "other", "included", nil
	}
}

func inferSDKMediaType(sourcePath, requested string) string {
	if value := strings.TrimSpace(strings.ToLower(strings.Split(requested, ";")[0])); value != "" {
		return value
	}
	return map[string]string{
		".md": "text/markdown", ".mdx": "text/mdx", ".json": "application/json", ".yaml": "application/yaml",
		".yml": "application/yaml", ".toml": "application/toml", ".xml": "application/xml", ".go": "text/x-go",
		".ts": "text/typescript", ".tsx": "text/typescript", ".js": "text/javascript", ".jsx": "text/javascript",
		".py": "text/x-python", ".java": "text/x-java", ".cs": "text/x-csharp", ".rb": "text/x-ruby",
	}[strings.ToLower(path.Ext(sourcePath))]
}

func normalizeSDKText(value string) (string, bool) {
	if !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
		return "", false
	}
	value = strings.TrimPrefix(value, "\ufeff")
	value = strings.ReplaceAll(strings.ReplaceAll(value, "\r\n", "\n"), "\r", "\n")
	lines := strings.Split(value, "\n")
	for index := range lines {
		lines[index] = strings.TrimRight(lines[index], " \t")
	}
	value = strings.TrimRight(strings.Join(lines, "\n"), "\n")
	if value != "" {
		value += "\n"
	}
	return value, true
}

func prepareSDKFiles(inputs []SDKIngestionFile) ([]normalizedSDKFile, []sdkIngestionDiagnostic, error) {
	if len(inputs) == 0 {
		return nil, nil, errors.New("at least one SDK file is required")
	}
	if len(inputs) > sdkIngestionMaxFiles {
		return nil, nil, fmt.Errorf("SDK ingestion is limited to %d files", sdkIngestionMaxFiles)
	}
	files := make([]normalizedSDKFile, 0, len(inputs))
	diagnostics := make([]sdkIngestionDiagnostic, 0)
	seen := make(map[string]bool, len(inputs))
	var total int
	for _, input := range inputs {
		sourcePath, err := normalizeSDKPath(input.SourcePath)
		if err != nil {
			return nil, nil, fmt.Errorf("%q: %w", input.SourcePath, err)
		}
		key := strings.ToLower(sourcePath)
		if seen[key] {
			return nil, nil, fmt.Errorf("duplicate or case-colliding SDK source path %q", sourcePath)
		}
		seen[key] = true
		if input.EntryType != "" && input.EntryType != "file" {
			return nil, nil, fmt.Errorf("%s: only regular files are accepted; links and directories are rejected", sourcePath)
		}
		byteSize := len(input.Content)
		if byteSize > sdkIngestionMaxFileBytes {
			return nil, nil, fmt.Errorf("%s exceeds the %d-byte per-file limit", sourcePath, sdkIngestionMaxFileBytes)
		}
		total += byteSize
		if total > sdkIngestionMaxTotalBytes {
			return nil, nil, fmt.Errorf("SDK ingestion exceeds the %d-byte total limit", sdkIngestionMaxTotalBytes)
		}
		role, disposition, err := inferSDKFileRole(sourcePath, input.Role)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", sourcePath, err)
		}
		normalized, textOK := normalizeSDKText(input.Content)
		reason := ""
		if !textOK {
			disposition, reason = "unsupported", "binary or invalid UTF-8 content is not indexed"
			diagnostics = append(diagnostics, sdkIngestionDiagnostic{Code: "unsupported_text_encoding", Severity: "warning", SourcePath: sourcePath, Message: reason})
		}
		containsCredential := textOK && sdkSecretPattern.MatchString(normalized)
		if containsCredential {
			disposition, reason, normalized = "quarantined", "possible credential material was detected and the content was not persisted", ""
			diagnostics = append(diagnostics, sdkIngestionDiagnostic{Code: "possible_secret", Severity: "error", SourcePath: sourcePath, Message: reason})
		}
		if disposition == "excluded" {
			reason = "tests, generated output, and vendored dependencies are excluded by default"
		}
		mediaType := inferSDKMediaType(sourcePath, input.MediaType)
		if mediaType == "" {
			mediaType = "text/plain"
		}
		files = append(files, normalizedSDKFile{
			input: input, path: sourcePath, content: normalized, mediaType: mediaType,
			language: normalizeSDKLanguage(input.Language, sourcePath), role: role,
			disposition: disposition, exclusionReason: reason, rawHash: contentHash([]byte(input.Content)),
			contentHash: contentHash([]byte(normalized)), byteSize: int64(byteSize), containsCredential: containsCredential,
		})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].path < files[j].path })
	return files, diagnostics, nil
}

func sdkAnchor(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	dash := false
	for _, current := range value {
		if (current >= 'a' && current <= 'z') || (current >= '0' && current <= '9') {
			builder.WriteRune(current)
			dash = false
		} else if builder.Len() > 0 && !dash {
			builder.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(builder.String(), "-")
}

func sdkTokenEstimate(value string) int {
	if value == "" {
		return 0
	}
	return (len([]rune(value)) + 3) / 4
}

type sdkMarkdownPart struct {
	title      string
	level      int
	breadcrumb []string
	content    string
	start      int
	end        int
	code       bool
}

type sdkFenceSample struct {
	language string
	title    string
	code     string
	start    int
	end      int
}

func parseSDKMarkdown(content, fallback string) ([]sdkMarkdownPart, []sdkFenceSample) {
	lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	parts := make([]sdkMarkdownPart, 0)
	samples := make([]sdkFenceSample, 0)
	stack := make([]string, 0, 6)
	partStart, partTitle, partLevel := 0, fallback, 1
	partBreadcrumb := []string{fallback}
	inFence, fenceMarker, fenceLanguage, fenceTitle, fenceStart := false, "", "", "", 0
	flush := func(end int) {
		if end <= partStart {
			return
		}
		value := strings.TrimSpace(strings.Join(lines[partStart:end], "\n"))
		if value != "" {
			parts = append(parts, sdkMarkdownPart{title: partTitle, level: partLevel, breadcrumb: append([]string(nil), partBreadcrumb...), content: value + "\n", start: partStart, end: end, code: strings.Contains(value, "```") || strings.Contains(value, "~~~")})
		}
	}
	for index, line := range lines {
		if match := sdkFencePattern.FindStringSubmatch(line); len(match) > 0 {
			marker := match[1]
			if !inFence {
				inFence, fenceMarker, fenceLanguage, fenceTitle, fenceStart = true, marker[:1], normalizeSDKLanguage(match[2], ""), partTitle, index+1
			} else if strings.HasPrefix(strings.TrimSpace(line), fenceMarker+fenceMarker+fenceMarker) {
				code := strings.TrimSpace(strings.Join(lines[fenceStart:index], "\n"))
				if code != "" && fenceLanguage != "" {
					samples = append(samples, sdkFenceSample{language: fenceLanguage, title: fenceTitle, code: code + "\n", start: fenceStart, end: index})
				}
				inFence = false
			}
			continue
		}
		if inFence {
			continue
		}
		match := sdkMarkdownHeadingPattern.FindStringSubmatch(line)
		if len(match) == 0 {
			continue
		}
		flush(index)
		level := len(match[1])
		title := strings.TrimSpace(match[2])
		if level <= len(stack) {
			stack = stack[:level-1]
		}
		for len(stack) < level-1 {
			stack = append(stack, fallback)
		}
		stack = append(stack, title)
		partStart, partTitle, partLevel, partBreadcrumb = index, title, level, append([]string(nil), stack...)
	}
	flush(len(lines))
	return parts, samples
}

func sdkImports(language, code string) []string {
	pattern := sdkJSImportPattern
	switch language {
	case "go":
		pattern = sdkGoImportPattern
	case "python":
		pattern = sdkPythonImportPattern
	}
	seen := map[string]bool{}
	result := make([]string, 0)
	for _, match := range pattern.FindAllStringSubmatch(code, -1) {
		if len(match) > 1 && !seen[match[1]] {
			seen[match[1]] = true
			result = append(result, match[1])
		}
	}
	sort.Strings(result)
	return result
}

func sdkModuleName(sourcePath string) string {
	value := strings.TrimSuffix(sourcePath, path.Ext(sourcePath))
	value = strings.Trim(value, "/")
	if value == "" {
		return "root"
	}
	return strings.ReplaceAll(value, "/", ".")
}

func sdkSymbolHash(language, kind, qualifiedName, signature string) string {
	value, _ := json.Marshal(map[string]string{"language": language, "kind": kind, "qualified_name": qualifiedName, "signature": signature})
	return contentHash(value)
}

func extractGoSDKSymbols(candidateID, fileID, sectionID string, file normalizedSDKFile) []model.SDKSymbol {
	fileset := token.NewFileSet()
	parsed, err := parser.ParseFile(fileset, file.path, file.content, parser.ParseComments)
	if err != nil {
		return nil
	}
	packageName := parsed.Name.Name
	result := make([]model.SDKSymbol, 0)
	add := func(kind, name, qualified, signature, documentation string, node ast.Node) {
		start, end := fileset.Position(node.Pos()).Line-1, fileset.Position(node.End()).Line
		startCopy, endCopy := start, end
		result = append(result, model.SDKSymbol{
			SDKContentCandidateID: candidateID, SDKPublicationFileID: fileID, SDKSectionID: sectionID,
			Language: "go", Kind: kind, QualifiedName: qualified, DisplayName: name, Signature: strings.TrimSpace(signature),
			Documentation: strings.TrimSpace(documentation), Identifiers: []string{name, qualified}, SourceStart: &startCopy, SourceEnd: &endCopy,
			ContentHash: sdkSymbolHash("go", kind, qualified, signature), Metadata: json.RawMessage(`{"parser":"go/ast"}`),
		})
	}
	for _, declaration := range parsed.Decls {
		switch value := declaration.(type) {
		case *ast.FuncDecl:
			kind, qualified := "function", packageName+"."+value.Name.Name
			if value.Recv != nil && len(value.Recv.List) > 0 {
				kind = "method"
				qualified = packageName + "." + value.Name.Name
			}
			documentation := ""
			if value.Doc != nil {
				documentation = value.Doc.Text()
			}
			add(kind, value.Name.Name, qualified, value.Name.Name, documentation, value)
		case *ast.GenDecl:
			for _, spec := range value.Specs {
				switch item := spec.(type) {
				case *ast.TypeSpec:
					kind := "type"
					switch item.Type.(type) {
					case *ast.StructType:
						kind = "class"
					case *ast.InterfaceType:
						kind = "interface"
					}
					add(kind, item.Name.Name, packageName+"."+item.Name.Name, item.Name.Name, "", item)
				case *ast.ValueSpec:
					kind := "property"
					if value.Tok == token.CONST {
						kind = "constant"
					}
					for _, name := range item.Names {
						add(kind, name.Name, packageName+"."+name.Name, name.Name, "", item)
					}
				}
			}
		}
	}
	return result
}

func extractGenericSDKSymbols(candidateID, fileID, sectionID string, file normalizedSDKFile) []model.SDKSymbol {
	patterns := sdkGenericSymbolPatterns[file.language]
	if len(patterns) == 0 {
		return nil
	}
	module := sdkModuleName(file.path)
	result := make([]model.SDKSymbol, 0)
	for lineNumber, line := range strings.Split(strings.TrimSuffix(file.content, "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		for _, candidate := range patterns {
			match := candidate.pattern.FindStringSubmatch(trimmed)
			if len(match) < 2 {
				continue
			}
			name, start, end := match[1], lineNumber, lineNumber+1
			qualified := module + "." + name
			result = append(result, model.SDKSymbol{
				SDKContentCandidateID: candidateID, SDKPublicationFileID: fileID, SDKSectionID: sectionID,
				Language: file.language, Kind: candidate.kind, QualifiedName: qualified, DisplayName: name,
				Signature: trimmed, Identifiers: []string{name, qualified}, SourceStart: &start, SourceEnd: &end,
				ContentHash: sdkSymbolHash(file.language, candidate.kind, qualified, trimmed), Metadata: json.RawMessage(`{"parser":"static-pattern"}`),
			})
			break
		}
	}
	return result
}

func sdkMapEntry(id, kind, title, summary string, aliases ...string) model.KnowledgeMapEntry {
	return model.KnowledgeMapEntry{ID: id, Kind: kind, Title: title, Summary: summary, Aliases: aliases}
}

func canonicalSDKBuildHash(value any) string {
	encoded, _ := json.Marshal(value)
	return contentHash(encoded)
}

// sdkIngestionUUID is an RFC 4122 UUIDv5. The fixed namespace separates SDK
// ingestion identities from caller-supplied UUIDs; the canonical JSON name
// makes an exact release replay produce the same immutable graph in another
// process or store.
func sdkIngestionUUID(kind string, logicalIdentity any) string {
	namespace := [...]byte{0x23, 0x59, 0x55, 0xe4, 0x47, 0xa3, 0x5e, 0xed, 0x98, 0x71, 0xed, 0x2c, 0xa7, 0x39, 0x57, 0x6a}
	name, _ := json.Marshal(logicalIdentity)
	hash := sha1.New() // UUIDv5 is deliberately SHA-1 by specification.
	_, _ = hash.Write(namespace[:])
	_, _ = hash.Write([]byte(kind))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(name)
	id := hash.Sum(nil)[:16]
	id[6] = id[6]&0x0f | 0x50
	id[8] = id[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", id[0:4], id[4:6], id[6:8], id[8:10], id[10:16])
}
