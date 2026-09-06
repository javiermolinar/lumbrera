package initcmd

// MigrationTemplate describes a scaffolded product-facing file that may be
// upgraded only when its bytes match a known older scaffold.
type MigrationTemplate struct {
	Path         string
	Content      string
	LegacySHA256 []string
}

func MigrationTemplates() []MigrationTemplate {
	return []MigrationTemplate{
		{
			Path:    agentsPath,
			Content: agentsContent,
			LegacySHA256: []string{
				"116c1b6e1a6a9cbe36988a6b6f9ed9a0c1a4bad26a371061beca2310d82eb31c",
				"b95f6b7de766fd2a56e835e3d480d2632f7e17dfc64523d5ed3574fe0f6d52d2",
			},
		},
		{
			Path:    ".agents/skills/lumbrera-ingest/SKILL.md",
			Content: ingestSkillContent,
			LegacySHA256: []string{
				"ac0db8ac968989f6e037b60df0af5c4cc515e4fdb3181c7292696d2f0cc38e08",
				"99b0efc831b1b6f376185dfeaebe2cd52c0fb4197c292624719b434da9371355",
			},
		},
		{
			Path:    ".agents/skills/lumbrera-query/SKILL.md",
			Content: querySkillContent,
			LegacySHA256: []string{
				"8a14c3ab7804e90c0a0a63d16ef45c456c9b95095819b85eb75d753970b2c1fe",
				"105842d6da3cd4c6bee22c332a781fdba1dbcbc92d2533e78decbfea801f4951",
			},
		},
		{
			Path:    ".agents/skills/lumbrera-health/SKILL.md",
			Content: healthSkillContent,
			LegacySHA256: []string{
				"f4d88b1d9ffa84a930f41eb3620753f7ad8a5f5751e415c668ae53bad6ce3083",
				"d8e7714bb922d091b4cb55933f4ac0ca4fc8e4f30f98d54c570d70ff8d51919e",
			},
		},
		{
			Path:    ".agents/skills/lumbrera-delete/SKILL.md",
			Content: deleteSkillContent,
			LegacySHA256: []string{
				"64bd1a97766af8126b083ac4580afad939c04c51dc8012feb17fa670f0b986b1",
				"ce9f8f4d18c33b5e338f5ce1b5cc1abecb84be7d241b5a8edbb0fae0acd64b1e",
			},
		},
	}
}

func NoteSkillTemplate() (path, content string) {
	return ".agents/skills/lumbrera-note/SKILL.md", noteSkillContent
}
