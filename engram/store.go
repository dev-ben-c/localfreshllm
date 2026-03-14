package engram

import (
	"database/sql"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

// Memory represents a single engram memory record.
type Memory struct {
	ID          string
	Content     string
	MemoryType  string
	Category    string
	Key         string
	Tags        string
	Confidence  float64
	Source      string
	CreatedAt   string
	UpdatedAt   string
	AccessedAt  string
	AccessCount int
	Model       string
	Context     string
	Score       float64
}

// Store provides direct read/write access to engram's SQLite database.
type Store struct {
	db *sql.DB
}

// DefaultDBPath returns ~/.engram/memory.db.
func DefaultDBPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".engram", "memory.db")
}

// NewStore opens the engram database at the given path.
func NewStore(dbPath string) (*Store, error) {
	if dbPath == "" {
		dbPath = DefaultDBPath()
	}
	if _, err := os.Stat(dbPath); err != nil {
		return nil, fmt.Errorf("engram db not found: %s", dbPath)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open engram db: %w", err)
	}

	// WAL mode for concurrent reads, busy timeout for writer contention.
	db.Exec("PRAGMA journal_mode=wal")
	db.Exec("PRAGMA busy_timeout=5000")

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping engram db: %w", err)
	}

	return &Store{db: db}, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// Recall searches memories using FTS5 full-text search with composite scoring.
func (s *Store) Recall(query, category string, limit int) ([]Memory, error) {
	if limit <= 0 {
		limit = 10
	}
	ftsQuery := buildFTSQuery(query)
	overfetch := limit * 3

	q := `SELECT m.id, m.content, m.memory_type, m.category,
	             COALESCE(m.key, '') AS key, m.tags, m.confidence,
	             COALESCE(m.source, '') AS source,
	             m.created_at, m.updated_at, m.accessed_at,
	             m.access_count, m.model, COALESCE(m.context, '') AS context,
	             fts.rank AS fts_rank
	      FROM memories_fts fts
	      JOIN memories m ON m.rowid = fts.rowid
	      WHERE memories_fts MATCH ?`
	args := []any{ftsQuery}

	if category != "" {
		q += ` AND m.category = ?`
		args = append(args, category)
	}

	q += ` ORDER BY fts.rank LIMIT ?`
	args = append(args, overfetch)

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("recall query: %w", err)
	}
	defer rows.Close()

	var memories []Memory
	for rows.Next() {
		var m Memory
		var ftsRank float64
		err := rows.Scan(&m.ID, &m.Content, &m.MemoryType, &m.Category,
			&m.Key, &m.Tags, &m.Confidence, &m.Source,
			&m.CreatedAt, &m.UpdatedAt, &m.AccessedAt,
			&m.AccessCount, &m.Model, &m.Context, &ftsRank)
		if err != nil {
			continue
		}
		updatedAt, _ := time.Parse(time.RFC3339Nano, m.UpdatedAt)
		m.Score = computeScore(ftsRank, updatedAt, m.AccessCount, m.Confidence)
		memories = append(memories, m)
	}

	// Sort by composite score descending.
	sort.Slice(memories, func(i, j int) bool {
		return memories[i].Score > memories[j].Score
	})

	if len(memories) > limit {
		memories = memories[:limit]
	}

	// Update access tracking for returned results.
	s.updateAccess(memories)

	return memories, nil
}

// Remember stores a new memory or updates an existing fact with the same category+key+model.
func (s *Store) Remember(content, memoryType, category, key, tagsJSON string,
	confidence float64, model, ctx string) (string, error) {
	if content == "" {
		return "", fmt.Errorf("content is required")
	}
	if memoryType == "" {
		memoryType = "fact"
	}
	if category == "" {
		category = "general"
	}
	if confidence <= 0 {
		confidence = 1.0
	}
	if tagsJSON == "" {
		tagsJSON = "[]"
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)

	// For facts with a key, check for existing record to upsert.
	if memoryType == "fact" && key != "" {
		var existingID, oldContent string
		var oldConfidence float64
		err := s.db.QueryRow(
			`SELECT id, content, confidence FROM memories WHERE category = ? AND key = ? AND model = ?`,
			category, key, model,
		).Scan(&existingID, &oldContent, &oldConfidence)

		if err == nil {
			// Record history then update.
			s.recordHistory(existingID, "updated", model, oldContent, content, oldConfidence, confidence, ctx, now)
			_, err = s.db.Exec(
				`UPDATE memories SET content = ?, tags = ?, confidence = ?,
				 updated_at = ?, accessed_at = ?, model = ?, context = ?
				 WHERE id = ?`,
				content, tagsJSON, confidence, now, now, model, ctx, existingID,
			)
			if err != nil {
				return "", fmt.Errorf("update memory: %w", err)
			}
			return existingID, nil
		}
	}

	// Insert new memory.
	id := strings.ReplaceAll(uuid.New().String(), "-", "")[:12]

	var keyVal any
	if key != "" {
		keyVal = key
	}

	_, err := s.db.Exec(
		`INSERT INTO memories (id, content, memory_type, category, key, tags,
		 confidence, source, created_at, updated_at, accessed_at, access_count, model, context)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?)`,
		id, content, memoryType, category, keyVal, tagsJSON,
		confidence, "localfreshllm", now, now, now, model, ctx,
	)
	if err != nil {
		return "", fmt.Errorf("insert memory: %w", err)
	}

	s.recordHistory(id, "created", model, "", content, 0, confidence, ctx, now)
	return id, nil
}

// GetContext returns preferences and recent facts for bootstrapping context.
func (s *Store) GetContext(topic string, limit int) (string, error) {
	if limit <= 0 {
		limit = 10
	}

	var sb strings.Builder

	// Preferences.
	rows, err := s.db.Query(
		`SELECT id, content, category, COALESCE(key, '') AS key, confidence, model
		 FROM memories WHERE memory_type = 'preference'
		 ORDER BY confidence DESC LIMIT 10`,
	)
	if err == nil {
		sb.WriteString("## Preferences\n")
		count := 0
		for rows.Next() {
			var id, content, cat, key, model string
			var confidence float64
			rows.Scan(&id, &content, &cat, &key, &confidence, &model)
			sb.WriteString(fmt.Sprintf("- [%s] %s\n", truncID(id), content))
			count++
		}
		rows.Close()
		if count == 0 {
			sb.WriteString("(none)\n")
		}
	}

	// Recent facts.
	rows2, err := s.db.Query(
		`SELECT id, content, category, COALESCE(key, '') AS key, confidence, model, updated_at
		 FROM memories WHERE memory_type = 'fact'
		 ORDER BY updated_at DESC LIMIT ?`, limit,
	)
	if err == nil {
		sb.WriteString("\n## Recent Facts\n")
		count := 0
		for rows2.Next() {
			var id, content, cat, key, model, updatedAt string
			var confidence float64
			rows2.Scan(&id, &content, &cat, &key, &confidence, &model, &updatedAt)
			sb.WriteString(fmt.Sprintf("- [%s] (%s/%s) %s\n", truncID(id), cat, key, truncStr(content, 200)))
			count++
		}
		rows2.Close()
		if count == 0 {
			sb.WriteString("(none)\n")
		}
	}

	// Topic-specific recall.
	if topic != "" {
		memories, err := s.Recall(topic, "", limit)
		if err == nil && len(memories) > 0 {
			sb.WriteString(fmt.Sprintf("\n## Topic: %s\n", topic))
			for _, m := range memories {
				keyStr := ""
				if m.Key != "" {
					keyStr = "/" + m.Key
				}
				sb.WriteString(fmt.Sprintf("- [%s] (%s%s) score=%.2f\n  %s\n",
					truncID(m.ID), m.Category, keyStr, m.Score, truncStr(m.Content, 200)))
			}
		}
	}

	return sb.String(), nil
}

// buildFTSQuery converts natural language into an FTS5 MATCH expression.
func buildFTSQuery(query string) string {
	operators := map[string]bool{"AND": true, "OR": true, "NOT": true, "NEAR": true}
	var words []string
	for _, word := range strings.Fields(query) {
		cleaned := strings.Trim(word, `".,;:!?()[]{}`)
		if cleaned != "" && !operators[strings.ToUpper(cleaned)] && len(cleaned) > 1 {
			words = append(words, `"`+cleaned+`"`)
		}
	}
	if len(words) == 0 {
		return `"` + query + `"`
	}
	return strings.Join(words, " OR ")
}

// computeScore replicates engram's composite scoring.
func computeScore(ftsRank float64, updatedAt time.Time, accessCount int, confidence float64) float64 {
	ftsScore := math.Abs(ftsRank)
	ageDays := time.Since(updatedAt).Hours() / 24.0
	recencyScore := math.Exp(-0.02 * ageDays)
	freqScore := math.Log1p(float64(accessCount)) / 10.0
	return (ftsScore * 0.5) + (recencyScore * 0.3) + (freqScore * 0.1) + (confidence * 0.1)
}

// updateAccess bumps accessed_at and access_count for returned memories.
func (s *Store) updateAccess(memories []Memory) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, m := range memories {
		s.db.Exec(`UPDATE memories SET accessed_at = ?, access_count = access_count + 1 WHERE id = ?`,
			now, m.ID)
	}
}

// recordHistory inserts an audit trail entry.
func (s *Store) recordHistory(memoryID, action, model, oldContent, newContent string,
	oldConfidence, newConfidence float64, context, timestamp string) {
	id := strings.ReplaceAll(uuid.New().String(), "-", "")[:12]
	s.db.Exec(
		`INSERT INTO memory_history (id, memory_id, action, model, old_content, new_content,
		 old_confidence, new_confidence, context, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, memoryID, action, model, oldContent, newContent,
		oldConfidence, newConfidence, context, timestamp,
	)
}

func truncStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func truncID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
