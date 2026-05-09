package initcmd

import _ "embed"

//go:embed templates/INDEX.md
var indexContent string

//go:embed templates/SOURCES.md
var sourcesIndexContent string

//go:embed templates/ASSETS.md
var assetsIndexContent string

//go:embed templates/CHANGELOG.md
var changelogContent string

//go:embed templates/BRAIN.sum
var brainSumContent string

//go:embed templates/tags.md
var tagsContent string

//go:embed templates/AGENTS.md
var agentsContent string

//go:embed templates/skills/lumbrera-ingest/SKILL.md
var ingestSkillContent string

//go:embed templates/skills/lumbrera-query/SKILL.md
var querySkillContent string

//go:embed templates/skills/lumbrera-health/SKILL.md
var healthSkillContent string

//go:embed templates/skills/lumbrera-delete/SKILL.md
var deleteSkillContent string
