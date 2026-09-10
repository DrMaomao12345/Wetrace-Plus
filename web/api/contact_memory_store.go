package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	// ContactMemorySchemaVersion is bumped when the persisted memory schema changes
	// incompatibly. It belongs to Wetrace rather than to the model response.
	ContactMemorySchemaVersion = 1

	contactMemoryFileName = "ai_contact_memories.json"
)

var ErrInvalidContactMemoryKey = errors.New("联系人记忆的 talker 不能为空")

// ContactMemory is Wetrace's durable, evidence-backed view of one contact. The
// language profile is deliberately separate from any future VoiceStudio voice
// profile: this record describes what the contact says, not how their voice sounds.
type ContactMemory struct {
	SchemaVersion int                        `json:"schema_version"`
	AccountID     string                     `json:"account_id"`
	Talker        string                     `json:"talker"`
	TargetName    string                     `json:"target_name,omitempty"`
	Status        string                     `json:"status,omitempty"`
	Profile       ContactMemoryProfile       `json:"profile"`
	Source        ContactMemorySource        `json:"source"`
	Generator     ContactMemoryGenerator     `json:"generator"`
	UserOverrides ContactMemoryUserOverrides `json:"user_overrides,omitempty"`
	CreatedAt     time.Time                  `json:"created_at"`
	UpdatedAt     time.Time                  `json:"updated_at"`
}

// ContactMemoryProfile contains only information inferred from real messages.
// Generated simulated replies must never be fed back into this profile.
type ContactMemoryProfile struct {
	Summary             string                            `json:"summary,omitempty"`
	Traits              []string                          `json:"traits,omitempty"`
	SpeakingStyle       ContactMemorySpeakingStyle        `json:"speaking_style"`
	Facts               []ContactMemoryFact               `json:"facts,omitempty"`
	InteractionPatterns []ContactMemoryInteractionPattern `json:"interaction_patterns,omitempty"`
	TypicalExamples     []ContactMemoryExample            `json:"typical_examples,omitempty"`
}

type ContactMemorySpeakingStyle struct {
	Tone              []string                   `json:"tone,omitempty"`
	SentenceLength    string                     `json:"sentence_length,omitempty"`
	Vocabulary        []string                   `json:"vocabulary,omitempty"`
	Catchphrases      []ContactMemoryCatchphrase `json:"catchphrases,omitempty"`
	EmojiHabits       []string                   `json:"emoji_habits,omitempty"`
	PunctuationHabits []string                   `json:"punctuation_habits,omitempty"`
	ResponsePatterns  []string                   `json:"response_patterns,omitempty"`
}

// ContactMemoryCatchphrase keeps evidence next to a purported catchphrase so
// callers can reject phrases that do not occur verbatim in the sampled messages.
type ContactMemoryCatchphrase struct {
	Text         string  `json:"text"`
	EvidenceSeqs []int64 `json:"evidence_seqs,omitempty"`
}

type ContactMemoryFact struct {
	Content      string     `json:"content"`
	Subject      string     `json:"subject,omitempty"`   // contact, user, or relationship
	Stability    string     `json:"stability,omitempty"` // stable or time_bound
	ObservedAt   *time.Time `json:"observed_at,omitempty"`
	EvidenceSeqs []int64    `json:"evidence_seqs,omitempty"`
	Confidence   string     `json:"confidence"` // high or medium
}

type ContactMemoryInteractionPattern struct {
	Context      string  `json:"context"`
	Response     string  `json:"response"`
	EvidenceSeqs []int64 `json:"evidence_seqs,omitempty"`
}

type ContactMemoryExample struct {
	User         string  `json:"user"`
	Contact      string  `json:"contact"`
	EvidenceSeqs []int64 `json:"evidence_seqs,omitempty"`
}

// ContactMemorySource identifies the real-message snapshot used to build the
// profile. ContentHash lets the caller cheaply determine whether a rebuild is
// necessary even when transcript text changes without a new message sequence.
type ContactMemorySource struct {
	MinSeq               int64     `json:"min_seq,omitempty"`
	MaxSeq               int64     `json:"max_seq,omitempty"`
	LastMessageAt        time.Time `json:"last_message_at,omitempty"`
	MessageCount         int       `json:"message_count"`
	DataVersion          string    `json:"data_version,omitempty"`
	VoiceTranscriptCount int       `json:"voice_transcript_count,omitempty"`
	VoiceTranscriptIDs   []string  `json:"voice_transcript_ids,omitempty"`
	VoiceTranscriptHash  string    `json:"voice_transcript_hash,omitempty"`
	ContentHash          string    `json:"content_hash,omitempty"`
}

type ContactMemoryGenerator struct {
	Provider      string    `json:"provider,omitempty"`
	Model         string    `json:"model,omitempty"`
	PromptVersion string    `json:"prompt_version"`
	GeneratedAt   time.Time `json:"generated_at"`
}

// ContactMemoryUserOverrides is stored separately so a later automatic rebuild
// cannot silently erase a user's corrections.
type ContactMemoryUserOverrides struct {
	Notes             []string  `json:"notes,omitempty"`
	StyleInstructions []string  `json:"style_instructions,omitempty"`
	PinnedFacts       []string  `json:"pinned_facts,omitempty"`
	ExcludedFacts     []string  `json:"excluded_facts,omitempty"`
	UpdatedAt         time.Time `json:"updated_at,omitempty"`
}

type contactMemoryKey struct {
	accountID string
	talker    string
}

type contactMemoryFile struct {
	Version  int             `json:"version"`
	Memories []ContactMemory `json:"memories"`
}

// ContactMemoryStore persists contact memories to
// DataDir/ai_contact_memories.json. Mutations are copy-on-write: the in-memory
// snapshot is swapped only after an atomic disk replacement succeeds.
type ContactMemoryStore struct {
	mu      sync.RWMutex
	path    string
	data    map[contactMemoryKey]ContactMemory
	now     func() time.Time
	persist func([]byte) error
}

// NewContactMemoryStore loads a contact-memory store. A missing file is treated
// as an empty store; malformed or unsupported files are returned as errors rather
// than being overwritten on the next Put.
func NewContactMemoryStore(dataDir string) (*ContactMemoryStore, error) {
	s := &ContactMemoryStore{
		path: filepath.Join(dataDir, contactMemoryFileName),
		data: make(map[contactMemoryKey]ContactMemory),
		now:  time.Now,
	}
	s.persist = func(data []byte) error {
		return atomicWriteContactMemories(s.path, data)
	}
	if err := s.load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return s, nil
}

// Get returns an isolated copy of one contact memory. Mutating the returned
// value cannot race with or modify the store.
func (s *ContactMemoryStore) Get(accountID, talker string) (ContactMemory, bool) {
	key, err := makeContactMemoryKey(accountID, talker)
	if err != nil {
		return ContactMemory{}, false
	}
	s.mu.RLock()
	memory, ok := s.data[key]
	s.mu.RUnlock()
	if !ok {
		return ContactMemory{}, false
	}
	return cloneContactMemory(memory), true
}

// Put creates or replaces a contact memory. Schema and timestamps are owned by
// the store. On any persistence error, both the previous in-memory snapshot and
// the previous on-disk file remain the active state.
func (s *ContactMemoryStore) Put(memory ContactMemory) error {
	key, err := makeContactMemoryKey(memory.AccountID, memory.Talker)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	memory = cloneContactMemory(memory)
	memory.AccountID = key.accountID
	memory.Talker = key.talker
	memory.SchemaVersion = ContactMemorySchemaVersion
	now := s.now().UTC()
	if previous, ok := s.data[key]; ok {
		memory.CreatedAt = previous.CreatedAt
	} else {
		memory.CreatedAt = now
	}
	memory.UpdatedAt = now

	next := cloneContactMemoryMap(s.data, 1)
	next[key] = memory
	if err := s.saveSnapshot(next); err != nil {
		return err
	}
	s.data = next
	return nil
}

// Delete removes one contact memory. Deleting an absent key is a no-op.
func (s *ContactMemoryStore) Delete(accountID, talker string) error {
	key, err := makeContactMemoryKey(accountID, talker)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data[key]; !ok {
		return nil
	}
	next := cloneContactMemoryMap(s.data, 0)
	delete(next, key)
	if err := s.saveSnapshot(next); err != nil {
		return err
	}
	s.data = next
	return nil
}

// List returns isolated copies of all memories for accountID, sorted by talker.
// An empty accountID addresses the legacy/default-account namespace; use
// ListAll when memories across every account are needed.
func (s *ContactMemoryStore) List(accountID string) []ContactMemory {
	accountID = strings.TrimSpace(accountID)
	s.mu.RLock()
	out := make([]ContactMemory, 0)
	for key, memory := range s.data {
		if key.accountID == accountID {
			out = append(out, cloneContactMemory(memory))
		}
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Talker < out[j].Talker })
	return out
}

// ListAll returns isolated copies of every memory, sorted by account and talker.
func (s *ContactMemoryStore) ListAll() []ContactMemory {
	s.mu.RLock()
	out := make([]ContactMemory, 0, len(s.data))
	for _, memory := range s.data {
		out = append(out, cloneContactMemory(memory))
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].AccountID == out[j].AccountID {
			return out[i].Talker < out[j].Talker
		}
		return out[i].AccountID < out[j].AccountID
	})
	return out
}

func (s *ContactMemoryStore) load() error {
	b, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}
	// Older development builds may have created this privacy-sensitive file with
	// broader permissions. Tighten it as soon as it is opened.
	if err := os.Chmod(s.path, 0600); err != nil {
		return fmt.Errorf("收紧联系人记忆文件权限失败: %w", err)
	}

	var file contactMemoryFile
	if err := json.Unmarshal(b, &file); err != nil {
		return fmt.Errorf("解析联系人记忆文件失败: %w", err)
	}
	if file.Version != ContactMemorySchemaVersion {
		return fmt.Errorf("不支持的联系人记忆文件版本 %d", file.Version)
	}

	loaded := make(map[contactMemoryKey]ContactMemory, len(file.Memories))
	for i, memory := range file.Memories {
		key, err := makeContactMemoryKey(memory.AccountID, memory.Talker)
		if err != nil {
			return fmt.Errorf("联系人记忆 #%d: %w", i+1, err)
		}
		if _, exists := loaded[key]; exists {
			return fmt.Errorf("联系人记忆文件包含重复项: account_id=%q talker=%q", key.accountID, key.talker)
		}
		if memory.SchemaVersion != 0 && memory.SchemaVersion != ContactMemorySchemaVersion {
			return fmt.Errorf("联系人记忆 %q 使用不支持的版本 %d", key.talker, memory.SchemaVersion)
		}
		memory.AccountID = key.accountID
		memory.Talker = key.talker
		memory.SchemaVersion = ContactMemorySchemaVersion
		loaded[key] = cloneContactMemory(memory)
	}
	s.data = loaded
	return nil
}

func (s *ContactMemoryStore) saveSnapshot(snapshot map[contactMemoryKey]ContactMemory) error {
	memories := make([]ContactMemory, 0, len(snapshot))
	for _, memory := range snapshot {
		memories = append(memories, memory)
	}
	sort.Slice(memories, func(i, j int) bool {
		if memories[i].AccountID == memories[j].AccountID {
			return memories[i].Talker < memories[j].Talker
		}
		return memories[i].AccountID < memories[j].AccountID
	})
	b, err := json.MarshalIndent(contactMemoryFile{
		Version:  ContactMemorySchemaVersion,
		Memories: memories,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化联系人记忆失败: %w", err)
	}
	b = append(b, '\n')
	if err := s.persist(b); err != nil {
		return fmt.Errorf("保存联系人记忆失败: %w", err)
	}
	return nil
}

func makeContactMemoryKey(accountID, talker string) (contactMemoryKey, error) {
	key := contactMemoryKey{
		accountID: strings.TrimSpace(accountID),
		talker:    strings.TrimSpace(talker),
	}
	if key.talker == "" {
		return contactMemoryKey{}, ErrInvalidContactMemoryKey
	}
	return key, nil
}

func cloneContactMemoryMap(src map[contactMemoryKey]ContactMemory, extraCapacity int) map[contactMemoryKey]ContactMemory {
	dst := make(map[contactMemoryKey]ContactMemory, len(src)+extraCapacity)
	for key, memory := range src {
		dst[key] = cloneContactMemory(memory)
	}
	return dst
}

func cloneContactMemory(memory ContactMemory) ContactMemory {
	memory.Source.VoiceTranscriptIDs = cloneStrings(memory.Source.VoiceTranscriptIDs)
	memory.Profile.Traits = cloneStrings(memory.Profile.Traits)
	memory.Profile.SpeakingStyle.Tone = cloneStrings(memory.Profile.SpeakingStyle.Tone)
	memory.Profile.SpeakingStyle.Vocabulary = cloneStrings(memory.Profile.SpeakingStyle.Vocabulary)
	memory.Profile.SpeakingStyle.EmojiHabits = cloneStrings(memory.Profile.SpeakingStyle.EmojiHabits)
	memory.Profile.SpeakingStyle.PunctuationHabits = cloneStrings(memory.Profile.SpeakingStyle.PunctuationHabits)
	memory.Profile.SpeakingStyle.ResponsePatterns = cloneStrings(memory.Profile.SpeakingStyle.ResponsePatterns)
	memory.Profile.SpeakingStyle.Catchphrases = append([]ContactMemoryCatchphrase(nil), memory.Profile.SpeakingStyle.Catchphrases...)
	for i := range memory.Profile.SpeakingStyle.Catchphrases {
		memory.Profile.SpeakingStyle.Catchphrases[i].EvidenceSeqs = cloneInt64s(memory.Profile.SpeakingStyle.Catchphrases[i].EvidenceSeqs)
	}
	memory.Profile.Facts = append([]ContactMemoryFact(nil), memory.Profile.Facts...)
	for i := range memory.Profile.Facts {
		memory.Profile.Facts[i].EvidenceSeqs = cloneInt64s(memory.Profile.Facts[i].EvidenceSeqs)
		if memory.Profile.Facts[i].ObservedAt != nil {
			observedAt := *memory.Profile.Facts[i].ObservedAt
			memory.Profile.Facts[i].ObservedAt = &observedAt
		}
	}
	memory.Profile.InteractionPatterns = append([]ContactMemoryInteractionPattern(nil), memory.Profile.InteractionPatterns...)
	for i := range memory.Profile.InteractionPatterns {
		memory.Profile.InteractionPatterns[i].EvidenceSeqs = cloneInt64s(memory.Profile.InteractionPatterns[i].EvidenceSeqs)
	}
	memory.Profile.TypicalExamples = append([]ContactMemoryExample(nil), memory.Profile.TypicalExamples...)
	for i := range memory.Profile.TypicalExamples {
		memory.Profile.TypicalExamples[i].EvidenceSeqs = cloneInt64s(memory.Profile.TypicalExamples[i].EvidenceSeqs)
	}
	memory.UserOverrides.Notes = cloneStrings(memory.UserOverrides.Notes)
	memory.UserOverrides.StyleInstructions = cloneStrings(memory.UserOverrides.StyleInstructions)
	memory.UserOverrides.PinnedFacts = cloneStrings(memory.UserOverrides.PinnedFacts)
	memory.UserOverrides.ExcludedFacts = cloneStrings(memory.UserOverrides.ExcludedFacts)
	return memory
}

func cloneStrings(src []string) []string {
	return append([]string(nil), src...)
}

func cloneInt64s(src []int64) []int64 {
	return append([]int64(nil), src...)
}

// atomicWriteContactMemories writes a complete snapshot without exposing a
// partially-written JSON document. The temporary file is fsynced before rename;
// rename is the commit point.
func atomicWriteContactMemories(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("创建数据目录失败: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".ai_contact_memories-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(tmpPath)
		}
	}()

	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	committed = true

	// Best-effort directory fsync makes the rename durable after a power loss.
	// Rename is already the logical commit, so a filesystem that does not support
	// directory Sync must not turn a successful Put into an apparent failure.
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}
