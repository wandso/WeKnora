package service

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/Tencent/WeKnora/internal/agent/skills"
)

// ErrSkillBundleInvalid marks every rejection of an uploaded archive, so the
// handler can map the whole class to 400 without matching on message text.
var ErrSkillBundleInvalid = errors.New("skill bundle is invalid")

// The skill bundle limits bound decompression so a zip bomb cannot exhaust
// memory before we ever reach the sandbox.
const (
	// maxSkillBundleFiles is the cap on files kept as the skill itself
	// (after dropping GitHub zipball extras and directory entries).
	// Real skills vendor assets: hugohe3/ppt-master's skills/ppt-master
	// tree is ~13k files, almost all under templates/.
	maxSkillBundleFiles = 20_000
	// maxSkillBundleZipEntries bounds central-directory iteration on the
	// raw archive. GitHub zipballs include the whole repository, so this
	// is higher than maxSkillBundleFiles.
	maxSkillBundleZipEntries = 100_000
	maxSkillBundleFileBytes  = 32 << 20  // 32 MiB per entry
	maxSkillBundleTotalBytes = 512 << 20 // 512 MiB across the kept skill files
)

// SkillBundle is a validated, in-memory skill archive.
type SkillBundle struct {
	Name         string
	Version      string
	Description  string
	Instructions string
	// SHA256 is over the uploaded bytes, so re-uploading the same archive is
	// recognisable in the UI and in the ledger, and a ready skill with this
	// digest can skip a billed snapshot rebuild.
	SHA256 string
	// Files maps skill-root-relative paths to contents, SKILL.md included.
	Files map[string][]byte
	// FrontmatterRepaired is true when SKILL.md YAML had to be repaired
	// before it would parse. The original file is unchanged; the install
	// prompt tells the agent to mention this so the user can fix it.
	FrontmatterRepaired bool
	// Governance metadata from SKILL.md. Empty values are normal for
	// third-party archives; the install stores the normalized defaults.
	Category   string
	RiskLevel  string
	Keywords   []string
	PromptHint string
}

// SkillBundleParseOptions relaxes the uploaded-zip rules for archives pulled
// from a git host or registry, which wrap the skill in a repo directory and
// ship README/LICENSE next to it.
type SkillBundleParseOptions struct {
	// Subdir is a skill-root path inside the archive (after any single wrap
	// directory). Empty means "find SKILL.md by the usual rules".
	Subdir string
	// AllowExtraFiles keeps files under the skill root and drops the rest,
	// instead of rejecting a GitHub zip that also contains the repo README.
	AllowExtraFiles bool
	// AllowNestedSkill accepts a unique SKILL.md nested deeper than one
	// directory, which is how monorepos and GitHub zipballs arrive.
	AllowNestedSkill bool
	// AllowMultipleSkillRoots drops the "exactly one skill root" rule so an
	// archive that holds several skills side by side (or under one wrapper
	// directory) is kept whole here and split by directory later, in
	// ParseSkillBundles. Without it the single-root check rejects the archive
	// before the splitter ever runs.
	AllowMultipleSkillRoots bool
}

// ParseSkillBundle validates an uploaded zip and extracts everything the
// install flow needs. It accepts both a flat archive and one wrapped in a
// single top-level directory, because both are what people actually upload.
func ParseSkillBundle(archive []byte) (*SkillBundle, error) {
	return ParseSkillBundleWithOptions(archive, SkillBundleParseOptions{})
}

// ParseSkillBundleWithOptions is ParseSkillBundle with the extra knobs remote
// installs need. SHA256 is still over the input bytes, not the re-rooted view.
func ParseSkillBundleWithOptions(archive []byte, opts SkillBundleParseOptions) (*SkillBundle, error) {
	raw, err := unzipSkillArchive(archive, opts)
	if err != nil {
		return nil, err
	}
	files, err := stripSkillRootPrefix(raw, opts)
	if err != nil {
		return nil, err
	}
	return skillBundleFromFiles(archive, files)
}

func unzipSkillArchive(archive []byte, opts SkillBundleParseOptions) (map[string][]byte, error) {
	entries, err := skillZipEntries(archive, opts)
	if err != nil {
		return nil, err
	}

	raw := make(map[string][]byte, len(entries))
	var totalBytes int64
	for _, item := range entries {
		entryBytes := item.size
		if totalBytes+entryBytes > maxSkillBundleTotalBytes {
			return nil, fmt.Errorf("%w: archive is too large", ErrSkillBundleInvalid)
		}
		totalBytes += entryBytes
		content, err := readLimitedZipEntry(item.file)
		if err != nil {
			return nil, err
		}
		if int64(len(content)) > entryBytes {
			actualExcess := int64(len(content)) - entryBytes
			if totalBytes+actualExcess > maxSkillBundleTotalBytes {
				return nil, fmt.Errorf("%w: archive is too large", ErrSkillBundleInvalid)
			}
			totalBytes += actualExcess
		}
		raw[item.name] = content
	}
	return raw, nil
}

// errSkillFileMissing means the zip is a valid skill archive but the requested
// path is not in it. The browser maps this to 404; every other zip problem is
// still ErrSkillBundleInvalid.
var errSkillFileMissing = errors.New("skill file not found")

type skillZipEntry struct {
	file *zip.File
	name string
	size int64
}

func skillZipEntries(archive []byte, opts SkillBundleParseOptions) ([]skillZipEntry, error) {
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, fmt.Errorf("%w: not a readable zip archive: %v", ErrSkillBundleInvalid, err)
	}
	if len(reader.File) > maxSkillBundleZipEntries {
		return nil, fmt.Errorf("%w: archive has more than %d zip entries",
			ErrSkillBundleInvalid, maxSkillBundleZipEntries)
	}

	pending := make([]skillZipEntry, 0, len(reader.File))
	names := make(map[string][]byte, len(reader.File))
	for _, entry := range reader.File {
		name, skip, err := inspectSkillZipPath(entry)
		if err != nil {
			return nil, err
		}
		if skip {
			continue
		}
		pending = append(pending, skillZipEntry{
			file: entry,
			name: name,
			size: entry.FileInfo().Size(),
		})
		names[name] = nil
	}

	prefix, err := skillRootPrefix(names, opts)
	if err != nil {
		return nil, err
	}

	out := make([]skillZipEntry, 0, len(pending))
	var totalBytes int64
	for _, item := range pending {
		if prefix != "" && !strings.HasPrefix(item.name, prefix+"/") {
			if opts.AllowExtraFiles {
				continue
			}
			return nil, fmt.Errorf("%w: archive holds files outside the skill directory %q",
				ErrSkillBundleInvalid, prefix)
		}
		if err := inspectKeptSkillZipEntry(item); err != nil {
			return nil, err
		}
		if totalBytes+item.size > maxSkillBundleTotalBytes {
			return nil, fmt.Errorf("%w: archive is too large", ErrSkillBundleInvalid)
		}
		totalBytes += item.size
		out = append(out, item)
		if len(out) > maxSkillBundleFiles {
			return nil, fmt.Errorf("%w: skill directory holds more than %d files",
				ErrSkillBundleInvalid, maxSkillBundleFiles)
		}
	}
	return out, nil
}

func inspectSkillZipPath(entry *zip.File) (name string, skip bool, err error) {
	if entry.FileInfo().IsDir() {
		return "", true, nil
	}
	name = path.Clean(entry.Name)
	if name == "." || strings.HasPrefix(name, "..") ||
		path.IsAbs(name) || strings.Contains(name, "../") {
		return "", false, fmt.Errorf("%w: entry %q escapes the archive root",
			ErrSkillBundleInvalid, entry.Name)
	}
	return name, false, nil
}

func inspectKeptSkillZipEntry(item skillZipEntry) error {
	if item.file.FileInfo().Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: entry %q is a symlink", ErrSkillBundleInvalid, item.file.Name)
	}
	if err := validateSkillEntryName(item.name); err != nil {
		return err
	}
	if item.size > maxSkillBundleFileBytes {
		return fmt.Errorf("%w: entry %q is too large", ErrSkillBundleInvalid, item.file.Name)
	}
	return nil
}

func readLimitedZipEntry(entry *zip.File) ([]byte, error) {
	rc, err := entry.Open()
	if err != nil {
		return nil, fmt.Errorf("%w: cannot read %q: %v", ErrSkillBundleInvalid, entry.Name, err)
	}
	content, err := io.ReadAll(io.LimitReader(rc, maxSkillBundleFileBytes+1))
	_ = rc.Close()
	if err != nil {
		return nil, fmt.Errorf("%w: cannot read %q: %v", ErrSkillBundleInvalid, entry.Name, err)
	}
	if len(content) > maxSkillBundleFileBytes {
		return nil, fmt.Errorf("%w: entry %q is too large", ErrSkillBundleInvalid, entry.Name)
	}
	return content, nil
}

// listSkillZipFiles returns skill-root-relative paths and declared uncompressed
// sizes from the zip central directory. It does not inflate file bodies.
func listSkillZipFiles(archive []byte) ([]SkillFileEntry, error) {
	index, err := skillZipFileIndex(archive)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(index))
	for name := range index {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]SkillFileEntry, 0, len(names))
	for _, name := range names {
		out = append(out, SkillFileEntry{Path: name, Size: index[name].size})
	}
	return out, nil
}

// readSkillZipFile inflates one skill-root-relative path and leaves the rest
// of the archive compressed.
func readSkillZipFile(archive []byte, rel string) ([]byte, error) {
	index, err := skillZipFileIndex(archive)
	if err != nil {
		return nil, err
	}
	item, ok := index[rel]
	if !ok {
		return nil, errSkillFileMissing
	}
	return readLimitedZipEntry(item.file)
}

func skillZipFileIndex(archive []byte) (map[string]skillZipEntry, error) {
	entries, err := skillZipEntries(archive, SkillBundleParseOptions{})
	if err != nil {
		return nil, err
	}
	raw := make(map[string][]byte, len(entries))
	byArchiveName := make(map[string]skillZipEntry, len(entries))
	for _, item := range entries {
		raw[item.name] = nil
		byArchiveName[item.name] = item
	}
	opts := SkillBundleParseOptions{}
	stripped, err := stripSkillRootPrefix(raw, opts)
	if err != nil {
		return nil, err
	}
	prefix, err := skillRootPrefix(raw, opts)
	if err != nil {
		return nil, err
	}
	out := make(map[string]skillZipEntry, len(stripped))
	for rel := range stripped {
		archiveName := rel
		if prefix != "" {
			archiveName = prefix + "/" + rel
		}
		item, ok := byArchiveName[archiveName]
		if !ok {
			return nil, fmt.Errorf("%w: SKILL.md is missing", ErrSkillBundleInvalid)
		}
		out[rel] = item
	}
	return out, nil
}

func skillBundleFromFiles(archive []byte, files map[string][]byte) (*SkillBundle, error) {
	manifest, ok := files["SKILL.md"]
	if !ok {
		return nil, fmt.Errorf("%w: SKILL.md is missing", ErrSkillBundleInvalid)
	}
	skill, err := skills.ParseSkillFile(string(manifest))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSkillBundleInvalid, err)
	}
	version, err := parseSkillBundleVersion(string(manifest))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSkillBundleInvalid, err)
	}

	return &SkillBundle{
		Name:                skill.Name,
		Version:             version,
		Description:         skill.Description,
		Instructions:        skill.Instructions,
		SHA256:              skillArchiveSHA256(archive),
		Files:               files,
		FrontmatterRepaired: skill.FrontmatterRepaired,
		Category:            skill.NormalizedCategory(),
		RiskLevel:           skill.NormalizedRiskLevel(),
		Keywords:            []string(skill.Keywords),
		PromptHint:          skill.PromptHint,
	}, nil
}

func skillArchiveSHA256(archive []byte) string {
	sum := sha256.Sum256(archive)
	return hex.EncodeToString(sum[:])
}

// archiveMatchesSHA reports whether archive is the digest the row claims.
// An empty expected digest is treated as unknown, not as a match against
// whatever happens to be on disk — callers that want a fallback must opt in.
func archiveMatchesSHA(archive []byte, want string) bool {
	want = strings.TrimSpace(want)
	if want == "" || len(archive) == 0 {
		return false
	}
	return skillArchiveSHA256(archive) == want
}

// zipSkillFiles writes a deterministic skill-root zip so a remote archive that
// had to be re-rooted can go through the same InstallSkill path as an upload.
func zipSkillFiles(files map[string][]byte) ([]byte, error) {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)

	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	for _, name := range names {
		entry, err := writer.Create(name)
		if err != nil {
			return nil, fmt.Errorf("write skill zip entry %q: %w", name, err)
		}
		if _, err := entry.Write(files[name]); err != nil {
			return nil, fmt.Errorf("write skill zip entry %q: %w", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close skill zip: %w", err)
	}
	return buf.Bytes(), nil
}

// validateSkillEntryName bans control characters and nothing else.
//
// Shell safety is not this function's job: every interpolation of a bundle
// path goes through sandbox.ShellQuote, which makes any name inert. What
// quoting cannot fix is a name that rewrites the lines it is printed into —
// the entry names also reach log lines, error messages and the image
// manifest, and a newline or an escape sequence there forges a second record.
// So control characters (NUL included) are refused and ordinary punctuation is
// not: real bundles vendor node_modules/@scope/pkg and wheels named
// numpy-1.26.4+cpu.whl, and a charset that rejected '@' or '+' broke them for
// no security gain. Traversal and symlinks are checked separately by the
// caller.
func validateSkillEntryName(name string) error {
	for _, r := range name {
		if unicode.IsControl(r) || r == 0 {
			return fmt.Errorf("%w: entry %q holds unsupported character %q",
				ErrSkillBundleInvalid, name, r)
		}
	}
	return nil
}

func parseSkillBundleVersion(manifest string) (string, error) {
	manifest = strings.TrimPrefix(manifest, "\ufeff")
	lines := strings.Split(manifest, "\n")
	frontmatterStart := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "---" {
			frontmatterStart = i
			break
		}
		if strings.TrimSpace(line) != "" {
			break
		}
	}
	if frontmatterStart < 0 {
		return "", nil
	}

	frontmatterEnd := -1
	for i := frontmatterStart + 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			frontmatterEnd = i
			break
		}
	}
	if frontmatterEnd < 0 {
		return "", nil
	}

	var metadata struct {
		Version string `yaml:"version"`
	}
	frontmatter := strings.Join(lines[frontmatterStart+1:frontmatterEnd], "\n")
	if _, err := skills.UnmarshalSkillFrontmatter(frontmatter, &metadata); err != nil {
		return "", err
	}
	return metadata.Version, nil
}

// stripSkillRootPrefix re-roots the archive at the directory holding SKILL.md.
func stripSkillRootPrefix(raw map[string][]byte, opts SkillBundleParseOptions) (map[string][]byte, error) {
	prefix, err := skillRootPrefix(raw, opts)
	if err != nil {
		return nil, err
	}
	if prefix == "" {
		return raw, nil
	}
	out := make(map[string][]byte, len(raw))
	for name, content := range raw {
		if !strings.HasPrefix(name, prefix+"/") {
			if opts.AllowExtraFiles {
				continue
			}
			return nil, fmt.Errorf("%w: archive holds files outside the skill directory %q",
				ErrSkillBundleInvalid, prefix)
		}
		out[strings.TrimPrefix(name, prefix+"/")] = content
	}
	if _, ok := out["SKILL.md"]; !ok {
		return nil, fmt.Errorf("%w: SKILL.md is missing", ErrSkillBundleInvalid)
	}
	return out, nil
}

func skillRootPrefix(raw map[string][]byte, opts SkillBundleParseOptions) (string, error) {
	subdir := path.Clean(strings.Trim(opts.Subdir, "/"))
	if subdir == "." {
		subdir = ""
	}
	// A SKILL.md at the zip root is the skill. Nested SKILL.md files are then
	// just extra files, the same way an uploaded bundle vendors them.
	if subdir == "" {
		if _, ok := raw["SKILL.md"]; ok {
			return "", nil
		}
	}

	var matches []string
	for name := range raw {
		if path.Base(name) != "SKILL.md" {
			continue
		}
		dir := path.Dir(name)
		if dir == "." {
			dir = ""
		}
		if subdir != "" {
			if dir == subdir || strings.HasSuffix(dir, "/"+subdir) {
				matches = append(matches, dir)
			}
			continue
		}
		if dir == "" || !strings.Contains(dir, "/") || opts.AllowNestedSkill {
			matches = append(matches, dir)
		}
	}
	if len(matches) == 0 {
		if subdir != "" {
			return "", fmt.Errorf("%w: SKILL.md is missing under %q", ErrSkillBundleInvalid, subdir)
		}
		// No skill root at the top layer is normal for a multi-skill archive
		// wrapped in a repo directory: ParseSkillBundles walks down from here.
		if opts.AllowMultipleSkillRoots {
			return "", nil
		}
		return "", fmt.Errorf("%w: SKILL.md is missing", ErrSkillBundleInvalid)
	}
	uniq := uniqueStrings(matches)
	if len(uniq) > 1 {
		// Several skills side by side: there is no single root to keep, so
		// keep everything and let the caller split it per directory.
		if opts.AllowMultipleSkillRoots {
			return "", nil
		}
		return "", fmt.Errorf("%w: archive holds more than one skill", ErrSkillBundleInvalid)
	}
	return uniq[0], nil
}

func uniqueStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// joinSkillKeywords stores frontmatter keywords in the single column the
// install row carries. A comma inside a keyword is not meaningful for
// discovery, so trimming and joining is lossless enough for display.
func joinSkillKeywords(keywords []string) string {
	if len(keywords) == 0 {
		return ""
	}
	out := make([]string, 0, len(keywords))
	for _, keyword := range keywords {
		keyword = strings.TrimSpace(keyword)
		if keyword == "" {
			continue
		}
		out = append(out, keyword)
	}
	return strings.Join(out, ",")
}

// synthesizeSkillFrontmatter fills in the name and description that a
// community SKILL.md often leaves out before it reaches the strict parser.
// The directory name supplies the name; the first prose line of the body
// supplies the description. A file that already declares both is returned
// byte-for-byte unchanged.
func synthesizeSkillFrontmatter(manifest []byte, dirName string) []byte {
	text := string(manifest)
	body := text
	front := ""
	if hasFrontmatterDelimiter(text) {
		if end := findFrontmatterEnd(text); end >= 0 {
			front = text[len("---"):end]
			body = text[end+len("---"):]
		}
	}

	needName := !frontmatterHasKey(front, "name")
	needDesc := !frontmatterHasKey(front, "description")
	if !needName && !needDesc {
		return manifest
	}

	var b strings.Builder
	b.WriteString("---\n")
	if needName {
		b.WriteString("name: " + skillNameFromDir(dirName) + "\n")
	}
	b.WriteString(strings.TrimPrefix(strings.TrimPrefix(front, "\r\n"), "\n"))
	if !strings.HasSuffix(front, "\n") {
		b.WriteString("\n")
	}
	if needDesc {
		b.WriteString("description: " + quoteForFrontmatter(skillDescriptionFromBody(body, dirName)) + "\n")
	}
	b.WriteString("---")
	b.WriteString(body)
	return []byte(b.String())
}

func hasFrontmatterDelimiter(text string) bool {
	return strings.HasPrefix(text, "---\n") || strings.HasPrefix(text, "---\r\n")
}

// findFrontmatterEnd returns the offset of the closing "---" line, or -1.
func findFrontmatterEnd(text string) int {
	rest := text[3:]
	for idx := 0; ; {
		next := strings.Index(rest[idx:], "\n---")
		if next < 0 {
			return -1
		}
		at := idx + next
		after := rest[at+4:]
		if after == "" || strings.HasPrefix(after, "\n") || strings.HasPrefix(after, "\r\n") {
			return 3 + at + 1 // points at the delimiter itself
		}
		idx = at + 4
	}
}

func frontmatterHasKey(front, key string) bool {
	for _, line := range strings.Split(front, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, key+":") {
			return true
		}
	}
	return false
}

// skillNameFromDir keeps only the characters the parser accepts, so a
// directory called "my.skill" still yields a usable name instead of a
// confusing validation error.
func skillNameFromDir(dirName string) string {
	var b strings.Builder
	for _, r := range dirName {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	name := strings.Trim(b.String(), "-")
	if name == "" {
		return "skill"
	}
	return name
}

// skillDescriptionFromBody takes the first meaningful line of the body: the
// text after the H1, or failing that the H1 itself. Truncated because the
// description is also what the agent sees in its skill list.
func skillDescriptionFromBody(body, dirName string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == "---" {
			continue
		}
		line = strings.TrimSpace(strings.TrimLeft(line, "#>*- "))
		if line == "" {
			continue
		}
		line = strings.Join(strings.Fields(line), " ")
		if runes := []rune(line); len(runes) > 160 {
			line = string(runes[:160])
		}
		return line
	}
	return dirName
}

func quoteForFrontmatter(value string) string {
	return strconv.Quote(value)
}

// ParseSkillBundles parses a zip that holds several skills, one per directory.
// Each candidate directory must contain a SKILL.md; the directory name becomes
// the skill name when SKILL.md declares none. Files outside every candidate
// directory (a repo README, LICENSE, or a zipball wrapper folder) are dropped,
// and a single flat skill (SKILL.md at the zip root) still comes back as one
// bundle, so the caller never has to know which shape arrived.
//
// Each bundle is re-rooted and repacked into its own archive, so its SHA256
// and catalog bytes describe that skill alone instead of the whole upload.
func ParseSkillBundles(archive []byte) ([]*SkillBundle, error) {
	// AllowMultipleSkillRoots is what makes a bundle OF skills parse at all:
	// the single-root rule below is for one skill, and this entry point exists
	// precisely to handle several.
	raw, err := unzipSkillArchive(archive, SkillBundleParseOptions{
		AllowExtraFiles:         true,
		AllowMultipleSkillRoots: true,
	})
	if err != nil {
		return nil, err
	}

	var dirs []string
	if _, ok := raw["SKILL.md"]; ok {
		// A flat single-skill archive is one bundle with an empty root.
		dirs = []string{""}
	} else {
		dirs = skillLayerDirs(raw, "")
	}
	if len(dirs) == 0 {
		return nil, fmt.Errorf("%w: archive holds no skill (no directory containing SKILL.md)",
			ErrSkillBundleInvalid)
	}

	bundles := make([]*SkillBundle, 0, len(dirs))
	for _, dir := range dirs {
		sub := reRootSkillFiles(raw, dir)
		manifest, ok := sub["SKILL.md"]
		if !ok {
			return nil, fmt.Errorf("%w: SKILL.md is missing under %q", ErrSkillBundleInvalid, dir)
		}
		// Repair the two required fields a community SKILL.md routinely omits.
		// Doing it here instead of inside skillBundleFromFiles keeps the
		// single-skill upload strict: there a human can go fix the file, while
		// a forty-skill archive has nobody to ask.
		sub["SKILL.md"] = synthesizeSkillFrontmatter(manifest, path.Base(dir))
		subArchive, err := zipSkillFiles(sub)
		if err != nil {
			return nil, fmt.Errorf("%w: repack skill %q: %v", ErrSkillBundleInvalid, dir, err)
		}
		bundle, err := skillBundleFromFiles(subArchive, sub)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(bundle.Name) == "" {
			bundle.Name = path.Base(dir)
		}
		bundles = append(bundles, bundle)
	}
	return bundles, nil
}

// skillLayerDirs returns the directories that directly contain a SKILL.md at
// the shallowest level where any exist. It scans one level at a time so a
// zipball that wraps the skills in a single outer folder (repo-main/skills/…)
// still yields the skill directories and not the wrapper.
func skillLayerDirs(raw map[string][]byte, prefix string) []string {
	children := childDirs(raw, prefix)
	if len(children) == 0 {
		return nil
	}
	var direct []string
	for _, child := range children {
		if _, ok := raw[path.Join(child, "SKILL.md")]; ok {
			direct = append(direct, child)
		}
	}
	if len(direct) > 0 {
		sort.Strings(direct)
		return direct
	}
	var out []string
	for _, child := range children {
		out = append(out, skillLayerDirs(raw, child)...)
	}
	return out
}

// childDirs returns the first-level directories directly under prefix, each
// as a prefix-qualified path. Files at prefix itself (dir == ".") are skipped.
func childDirs(raw map[string][]byte, prefix string) []string {
	seen := make(map[string]struct{})
	for name := range raw {
		dir := path.Dir(name)
		if dir == "." {
			dir = ""
		}
		rel := dir
		if prefix != "" {
			if !strings.HasPrefix(dir, prefix+"/") {
				continue
			}
			rel = strings.TrimPrefix(dir, prefix+"/")
		}
		first := rel
		if idx := strings.Index(rel, "/"); idx >= 0 {
			first = rel[:idx]
		}
		if first == "" {
			continue
		}
		full := first
		if prefix != "" {
			full = prefix + "/" + first
		}
		seen[full] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for d := range seen {
		out = append(out, d)
	}
	return out
}

// reRootSkillFiles returns the files under dir with the dir prefix stripped.
// An empty dir is the archive root and keeps every file.
func reRootSkillFiles(raw map[string][]byte, dir string) map[string][]byte {
	out := make(map[string][]byte, len(raw))
	for name, content := range raw {
		if dir == "" {
			out[name] = content
			continue
		}
		if !strings.HasPrefix(name, dir+"/") {
			continue
		}
		out[strings.TrimPrefix(name, dir+"/")] = content
	}
	return out
}
