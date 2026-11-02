// Package skills provides Agent Skills functionality following Claude's Progressive Disclosure pattern.
// Skills are modular capabilities that extend the agent's functionality through instruction files.
package skills

import (
	"bufio"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// Skill validation constants following Claude's specification
const (
	MaxNameLength        = 64
	MaxDescriptionLength = 1024
	SkillFileName        = "SKILL.md"
)

// Reserved words that cannot be used in skill names
var reservedWords = []string{"anthropic", "claude"}

// namePattern is the install identity: a single path segment of letters
// (any script), digits, hyphens, and underscores. Display titles such as
// "Word / DOCX" are not this; those are rewritten via slug / slugify
// before Validate. Slashes stay out so the name cannot walk SkillsImageRoot.
var namePattern = regexp.MustCompile(`^[\p{L}\p{N}_-]+$`)

var skillNameSepRE = regexp.MustCompile(`[^\p{L}\p{N}]+`)

// xmlTagPattern detects XML tags in content
var xmlTagPattern = regexp.MustCompile(`<[^>]+>`)

// Skill represents a loaded skill with its metadata and content
// It follows the Progressive Disclosure pattern:
// - Level 1 (Metadata): Name and Description are always loaded
// - Level 2 (Instructions): The main body of SKILL.md, loaded on demand
// - Level 3 (Resources): Additional files in the skill directory, loaded as needed
type Skill struct {
	// Metadata (Level 1) - always loaded
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	// Slug is an optional filesystem-safe id. ClawHub / SkillHub often put a
	// display title in name ("Word / DOCX") and the install id in slug
	// ("word-docx"). When name is not a valid install identity, slug wins.
	Slug string `yaml:"slug,omitempty"`

	// Governance metadata. These are optional in SKILL.md: an archive written
	// for any other agent runtime installs unchanged, and the catalog then
	// classifies it as unclassified / medium instead of refusing it. The values
	// drive the risk badges and category filter in the toolbox, and are shown
	// next to a skill when an agent picks one.
	//
	// Category describes the side effect a skill can have (safe, read_local,
	// read_remote, write); RiskLevel is the severity of getting it wrong (low,
	// medium, high, critical). Keywords are extra discovery hints.
	Category   string        `yaml:"category,omitempty"`
	RiskLevel  string        `yaml:"risk_level,omitempty"`
	Keywords   SkillKeywords `yaml:"keywords,omitempty"`
	PromptHint string        `yaml:"prompt_hint,omitempty"`

	// Filesystem information
	BasePath string // Absolute path to the skill directory
	FilePath string // Absolute path to SKILL.md

	// Instructions (Level 2) - loaded on demand
	Instructions string // The main body of SKILL.md (after frontmatter)
	Loaded       bool   // Whether Level 2 instructions have been loaded

	// FrontmatterRepaired is true when the YAML between the --- markers had
	// to be repaired (keys nested under a scalar, or an unquoted colon)
	// before it would parse. The original SKILL.md is unchanged; callers
	// that install the archive should tell the user so they can fix it.
	FrontmatterRepaired bool
}

// SkillMetadata represents the minimal metadata for system prompt injection (Level 1)
// This is the lightweight representation used during skill discovery
type SkillMetadata struct {
	Name        string
	Description string
	BasePath    string // Path to skill directory for later loading
	Category    string
	RiskLevel   string
	Keywords    []string
	PromptHint  string
}

// SkillKeywords accepts both a YAML sequence and a comma-separated scalar.
// Third-party SKILL.md files use both spellings; a strict []string would make
// the whole frontmatter fail to parse, which reads to the user as an
// uninstallable skill rather than a one-line metadata nit.
type SkillKeywords []string

// UnmarshalYAML implements yaml.Unmarshaler.
func (k *SkillKeywords) UnmarshalYAML(value *yaml.Node) error {
	if value == nil {
		return nil
	}
	if value.Kind == yaml.ScalarNode {
		var raw string
		if err := value.Decode(&raw); err != nil {
			return err
		}
		out := make(SkillKeywords, 0, 4)
		for _, part := range strings.Split(raw, ",") {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, part)
			}
		}
		*k = out
		return nil
	}
	var list []string
	if err := value.Decode(&list); err != nil {
		return err
	}
	out := make(SkillKeywords, 0, len(list))
	for _, item := range list {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	*k = out
	return nil
}

// SkillFile represents an additional file within a skill directory (Level 3)
type SkillFile struct {
	Name     string // Filename (e.g., "FORMS.md", "scripts/validate.py")
	Path     string // Absolute path to the file
	Content  string // File content
	IsScript bool   // Whether this is an executable script
}

// Validate checks if the skill metadata is valid according to Claude's specification
func (s *Skill) Validate() error {
	// Validate name
	if s.Name == "" {
		return errors.New("skill name is required")
	}
	if n := utf8.RuneCountInString(s.Name); n > MaxNameLength {
		return fmt.Errorf("skill name is %d characters; maximum is %d", n, MaxNameLength)
	}
	if !namePattern.MatchString(s.Name) {
		return errors.New("skill name must contain only letters, numbers, hyphens, and underscores")
	}
	for _, reserved := range reservedWords {
		if strings.Contains(s.Name, reserved) {
			return fmt.Errorf("skill name cannot contain reserved word: %s", reserved)
		}
	}
	if xmlTagPattern.MatchString(s.Name) {
		return errors.New("skill name cannot contain XML tags")
	}

	// Validate description
	if s.Description == "" {
		return errors.New("skill description is required")
	}
	if n := utf8.RuneCountInString(s.Description); n > MaxDescriptionLength {
		return fmt.Errorf("skill description is %d characters; maximum is %d", n, MaxDescriptionLength)
	}
	if xmlTagPattern.MatchString(s.Description) {
		return errors.New("skill description cannot contain XML tags")
	}

	return nil
}

// applyInstallName picks the directory / tool identity. Third-party SKILL.md
// files often put a display title in name ("Word / DOCX") and a kebab-case
// id in slug ("word-docx"). A title that is already a valid identity is
// left alone so Chinese names such as 律师助手 stay as-is.
func (s *Skill) applyInstallName() error {
	if s == nil {
		return errors.New("skill name is required")
	}
	if installableSkillName(s.Name) {
		s.Name = strings.TrimSpace(s.Name)
		return nil
	}
	if installableSkillName(s.Slug) {
		s.Name = strings.TrimSpace(s.Slug)
		return nil
	}
	if derived := slugifySkillName(s.Name); installableSkillName(derived) {
		s.Name = derived
		return nil
	}
	return errors.New("skill name must contain only letters, numbers, hyphens, and underscores (or set slug)")
}

func installableSkillName(name string) bool {
	name = strings.TrimSpace(name)
	return name != "" && namePattern.MatchString(name)
}

func slugifySkillName(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = skillNameSepRE.ReplaceAllString(s, "-")
	return strings.Trim(s, "-_")
}

// ToMetadata converts a Skill to its lightweight metadata representation
func (s *Skill) ToMetadata() *SkillMetadata {
	return &SkillMetadata{
		Name:        s.Name,
		Description: s.Description,
		BasePath:    s.BasePath,
		Category:    s.NormalizedCategory(),
		RiskLevel:   s.NormalizedRiskLevel(),
		Keywords:    []string(s.Keywords),
		PromptHint:  s.PromptHint,
	}
}

// Skill governance vocabularies. They mirror the AIOps skill catalog so
// operators moving between the two products read the same words.
const (
	SkillCategorySafe         = "safe"
	SkillCategoryReadLocal    = "read_local"
	SkillCategoryReadRemote   = "read_remote"
	SkillCategoryWrite        = "write"
	SkillCategoryUnclassified = "unclassified"

	SkillRiskLow      = "low"
	SkillRiskMedium   = "medium"
	SkillRiskHigh     = "high"
	SkillRiskCritical = "critical"
)

// NormalizeSkillCategory maps a free-form frontmatter value onto the known set.
// Unknown or empty values become unclassified rather than guessing "safe":
// a badge that overstates safety is worse than one that admits ignorance.
//
// It is exported as a free function because stored rows and read models carry
// the raw string without a parsed Skill around it.
func NormalizeSkillCategory(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case SkillCategorySafe:
		return SkillCategorySafe
	case SkillCategoryReadLocal:
		return SkillCategoryReadLocal
	case SkillCategoryReadRemote:
		return SkillCategoryReadRemote
	case SkillCategoryWrite:
		return SkillCategoryWrite
	default:
		return SkillCategoryUnclassified
	}
}

// NormalizeSkillRiskLevel maps a free-form frontmatter value onto the known set.
// Empty becomes medium: the operator can always lower it, but an unlabelled
// skill should not present itself as harmless.
func NormalizeSkillRiskLevel(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case SkillRiskLow:
		return SkillRiskLow
	case SkillRiskMedium:
		return SkillRiskMedium
	case SkillRiskHigh:
		return SkillRiskHigh
	case SkillRiskCritical:
		return SkillRiskCritical
	default:
		return SkillRiskMedium
	}
}

// NormalizedCategory is the Skill-level form of NormalizeSkillCategory.
func (s *Skill) NormalizedCategory() string {
	if s == nil {
		return SkillCategoryUnclassified
	}
	return NormalizeSkillCategory(s.Category)
}

// NormalizedRiskLevel is the Skill-level form of NormalizeSkillRiskLevel.
func (s *Skill) NormalizedRiskLevel() string {
	if s == nil {
		return SkillRiskMedium
	}
	return NormalizeSkillRiskLevel(s.RiskLevel)
}

// ParseSkillFile parses a SKILL.md file content and extracts metadata and body
// It handles YAML frontmatter enclosed in --- delimiters
func ParseSkillFile(content string) (*Skill, error) {
	skill := &Skill{}
	// Some editors emit a UTF-8 BOM, which strings.TrimSpace does not remove.
	content = strings.TrimPrefix(content, "\ufeff")

	// Check for YAML frontmatter
	if !strings.HasPrefix(strings.TrimSpace(content), "---") {
		return nil, errors.New("SKILL.md must start with YAML frontmatter (---)")
	}

	// Find the end of frontmatter
	scanner := bufio.NewScanner(strings.NewReader(content))
	var frontmatterLines []string
	var bodyLines []string
	inFrontmatter := false
	frontmatterEnded := false

	for scanner.Scan() {
		line := scanner.Text()

		if !inFrontmatter && !frontmatterEnded && strings.TrimSpace(line) == "---" {
			inFrontmatter = true
			continue
		}

		if inFrontmatter && strings.TrimSpace(line) == "---" {
			inFrontmatter = false
			frontmatterEnded = true
			continue
		}

		if inFrontmatter {
			frontmatterLines = append(frontmatterLines, line)
		} else if frontmatterEnded {
			bodyLines = append(bodyLines, line)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading SKILL.md: %w", err)
	}

	if !frontmatterEnded {
		return nil, errors.New("SKILL.md frontmatter is not properly closed with ---")
	}

	// Parse YAML frontmatter
	frontmatter := strings.Join(frontmatterLines, "\n")
	repaired, err := UnmarshalSkillFrontmatter(frontmatter, skill)
	if err != nil {
		return nil, fmt.Errorf("failed to parse YAML frontmatter: %w", err)
	}
	skill.FrontmatterRepaired = repaired
	if err := skill.applyInstallName(); err != nil {
		return nil, err
	}

	// Set body instructions
	skill.Instructions = strings.TrimSpace(strings.Join(bodyLines, "\n"))
	skill.Loaded = true

	// Validate
	if err := skill.Validate(); err != nil {
		return nil, fmt.Errorf("skill validation failed: %w", err)
	}

	return skill, nil
}

// ParseSkillMetadata parses only the metadata from a SKILL.md file content
// This is a lightweight operation for skill discovery (Level 1 only)
func ParseSkillMetadata(content string) (*SkillMetadata, error) {
	skill, err := ParseSkillFile(content)
	if err != nil {
		return nil, err
	}
	return skill.ToMetadata(), nil
}

// IsOnDemandInstallerPath reports whether path is a first-use dependency
// installer (scripts/install_deps.py and friends). Skills ship these to defer
// optional packages to chat time, which is the wrong moment: the work is
// repeated by every session built from the image and fails wherever the
// sandbox has no egress. The installer agent's prompt ("install these extras
// now") and the runtime hint ("skip this installer") key off the same list.
func IsOnDemandInstallerPath(scriptPath string) bool {
	base := strings.ToLower(filepath.Base(strings.TrimSpace(scriptPath)))
	switch {
	case strings.Contains(base, "install_dep"):
		return true
	case base == "setup_deps.py", base == "bootstrap_deps.py":
		return true
	default:
		return false
	}
}

// IsScript checks if a file path represents an executable script
func IsScript(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	scriptExtensions := map[string]bool{
		".py":   true,
		".sh":   true,
		".bash": true,
		".js":   true,
		".mjs":  true,
		".cjs":  true,
		".ts":   true,
		".rb":   true,
		".pl":   true,
		".php":  true,
	}
	return scriptExtensions[ext]
}

// GetScriptLanguage returns the language/interpreter for a script file
func GetScriptLanguage(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	languages := map[string]string{
		".py":   "python",
		".sh":   "bash",
		".bash": "bash",
		".js":   "node",
		".mjs":  "node",
		".cjs":  "node",
		".ts":   "ts-node",
		".rb":   "ruby",
		".pl":   "perl",
		".php":  "php",
	}
	if lang, ok := languages[ext]; ok {
		return lang
	}
	return "unknown"
}
