# Shared Knowledge System

The shared knowledge system allows Tron agents to share discoveries, insights, decisions, and task results with each other. When one agent learns something, other agents can see it in their knowledge feed and query for details.

## Overview

```
┌─────────────────────────────────────────────────────────────┐
│                    Agent Action                              │
│  (creates project, completes research, makes decision)      │
└─────────────────────────────────────────────────────────────┘
                            │
                            ▼
                    [share_knowledge tool]
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│                  KnowledgeStore                              │
│  entries.json | index.json | feed.json                      │
└─────────────────────────────────────────────────────────────┘
                            │
            ┌───────────────┴───────────────┐
            ▼                               ▼
    [Prompt Injection]              [Tool Queries]
    Recent feed (24h)               query_knowledge()
    ~500 tokens                     get_knowledge_feed()
```

---

## Storage

Knowledge is stored in the persona directory:

```
tron.persona/
├── memory.md           # Existing memory system (unchanged)
└── knowledge/
    ├── entries.json    # All entries (30-day rolling window)
    ├── index.json      # Fast lookup by domain/author/tag
    └── feed.json       # Cached recent activity feed
```

### Rolling Window

Entries older than 30 days are automatically filtered out when the store loads. This keeps the knowledge base focused on recent, relevant information.

---

## Data Model

### Entry

```go
type Entry struct {
    ID        string    `json:"id"`         // 8-character UUID prefix
    Type      EntryType `json:"type"`       // discovery, insight, decision, task_result, resource
    Domain    Domain    `json:"domain"`     // tech, marketing, finance, ops, product, general
    Author    string    `json:"author"`     // Tony, Maya, Gary, etc.
    Title     string    `json:"title"`      // Brief summary (1-2 sentences)
    Content   string    `json:"content"`    // Full details
    Tags      []string  `json:"tags"`       // Categorization tags
    Source    *Source   `json:"source"`     // Provenance (optional)
    CreatedAt time.Time `json:"created_at"` // When entry was created
}
```

### Entry Types

| Type | Description | Example |
|------|-------------|---------|
| `discovery` | New finding or learning | "Redis caching reduces API latency by 40%" |
| `insight` | Analysis or interpretation | "B2B AI adoption up 40% this quarter" |
| `decision` | Choice or direction taken | "Moving to Go 1.22 for generics support" |
| `task_result` | Outcome of completed work | "API redesign complete, ready for review" |
| `resource` | Useful reference or link | "Found comprehensive OAuth2 guide" |

### Domains

| Domain | Owner | Description |
|--------|-------|-------------|
| `tech` | Tony | Technology, architecture, engineering |
| `marketing` | Maya | Brand, messaging, customer insights |
| `finance` | Alex | Metrics, ROI, budgets |
| `ops` | Jordan | Operations, processes, scaling |
| `product` | Riley | UX, features, roadmap |
| `general` | - | Cross-functional or unclassified |

### Source

```go
type Source struct {
    ProcessID string `json:"process_id"` // Vega process that created this
    TaskID    string `json:"task_id"`    // Related task ID
    URL       string `json:"url"`        // External source URL
}
```

---

## Tools

Three tools are available to agents:

### share_knowledge

Share a discovery, insight, or decision with the team.

**Parameters**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `type` | string | Yes | Entry type: `discovery`, `insight`, `decision`, `task_result`, `resource` |
| `title` | string | Yes | Brief summary (1-2 sentences) |
| `content` | string | Yes | Full details |
| `domain` | string | No | Domain override (defaults to author's domain) |
| `tags` | string | No | Comma-separated tags |

**Example Usage**

```
share_knowledge(
  type: "discovery",
  title: "New caching strategy reduces API latency by 40%",
  content: "After testing Redis vs in-memory caching with our production traffic patterns, Redis with 5-minute TTL provides the best balance of freshness and performance. Key findings: 1) Hot path queries benefit most, 2) Cache invalidation on writes prevents stale data, 3) Memory usage stays under 512MB.",
  tags: "performance, redis, caching, api"
)
```

**Response**

```
Knowledge shared: [discovery] New caching strategy reduces API latency by 40%
This will appear in the team's knowledge feed.
```

---

### query_knowledge

Search the shared knowledge base.

**Parameters**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `domain` | string | No | Filter by domain |
| `author` | string | No | Filter by author name |
| `type` | string | No | Filter by entry type |
| `tags` | string | No | Comma-separated tags to filter by |
| `limit` | number | No | Maximum results (default: 10) |

**Example Usage**

```
query_knowledge(domain: "tech", limit: 5)
```

**Response**

```
Found 3 entries:

1. **New caching strategy reduces API latency by 40%** [discovery]
   ID: a1b2c3d4 | Author: Tony | Domain: tech
   Jan 15, 2024
   After testing Redis vs in-memory caching with our production traffic patterns...

2. **Moving to Go 1.22** [decision]
   ID: e5f6g7h8 | Author: Tony | Domain: tech
   Jan 14, 2024
   Decided to upgrade to Go 1.22 for improved generics support...

3. **API redesign complete** [task_result]
   ID: i9j0k1l2 | Author: Gary | Domain: tech
   Jan 13, 2024
   Completed the API v2 redesign. All endpoints now follow RESTful conventions...
```

---

### get_knowledge_feed

Get a digest of recent team activity from the last 24 hours.

**Parameters**

None.

**Response**

```
## Recent Team Activity (last 24h)

**Tech (Tony's team)**
- [discovery] Tony: New caching strategy reduces API latency by 40%
- [task_result] Gary: API redesign complete

**Marketing (Maya's team)**
- [insight] Maya: B2B AI adoption up 40% this quarter

Use query_knowledge for details.
```

---

## Prompt Injection

The knowledge feed is automatically injected into agent system prompts when:

1. **Slack sessions** are created (`internal/slack/handler.go`)
2. **Voice/API sessions** are created (`internal/server/server.go`)

### Feed Format

The feed section is appended to the system prompt:

```
## Recent Team Activity (last 24h)

**Tech (Tony's team)**
- [discovery] Tony: New caching strategy reduces API latency by 40%
- [task_result] Gary: API redesign complete

**Marketing (Maya's team)**
- [insight] Maya: B2B AI adoption up 40% this quarter

Use query_knowledge for details.
```

### Token Budget

The feed is designed to stay under ~500 tokens:
- Maximum 3 entries per domain
- Only entries from last 24 hours
- Compact format (type, author, title only)

---

## Index

The index provides fast lookup without scanning all entries:

```json
{
  "by_domain": {
    "tech": ["a1b2c3d4", "e5f6g7h8", "i9j0k1l2"],
    "marketing": ["m1n2o3p4"]
  },
  "by_author": {
    "Tony": ["a1b2c3d4", "e5f6g7h8"],
    "Gary": ["i9j0k1l2"],
    "Maya": ["m1n2o3p4"]
  },
  "by_tag": {
    "performance": ["a1b2c3d4"],
    "api": ["a1b2c3d4", "i9j0k1l2"]
  },
  "by_type": {
    "discovery": ["a1b2c3d4"],
    "decision": ["e5f6g7h8"],
    "task_result": ["i9j0k1l2"],
    "insight": ["m1n2o3p4"]
  }
}
```

The index is automatically rebuilt when entries are added.

---

## Example Workflow

### 1. Tony discovers something

Tony is researching caching strategies and discovers Redis works well:

```
Tony: share_knowledge(
  type: "discovery",
  title: "Redis caching reduces API latency by 40%",
  content: "Tested Redis vs in-memory. Redis with 5-minute TTL is optimal...",
  tags: "performance, redis, caching"
)
```

### 2. Maya sees it in her feed

When Maya starts a new Slack conversation, her system prompt includes:

```
## Recent Team Activity (last 24h)

**Tech (Tony's team)**
- [discovery] Tony: Redis caching reduces API latency by 40%

Use query_knowledge for details.
```

### 3. Maya queries for details

Maya wants to know more for a customer conversation:

```
Maya: query_knowledge(author: "Tony", type: "discovery")
```

She gets full details including the content, tags, and when it was shared.

### 4. Maya shares an insight

Based on customer feedback, Maya shares marketing insights:

```
Maya: share_knowledge(
  type: "insight",
  title: "Customers love the performance improvements",
  content: "Three enterprise customers mentioned faster API response times. This is a strong selling point for Q2 campaigns.",
  tags: "customers, feedback, performance"
)
```

### 5. Tony sees Maya's insight

Tony's next session includes Maya's insight in the feed, creating a virtuous cycle of knowledge sharing.

---

## API Reference

### Package: `internal/knowledge`

#### Store

```go
// NewStore creates a new knowledge store
func NewStore(baseDir string) (*Store, error)

// Add adds a new knowledge entry
func (s *Store) Add(entry Entry) error

// Query returns entries matching the given filters
func (s *Store) Query(opts QueryOptions) []Entry

// GetRecent returns entries from the last duration
func (s *Store) GetRecent(d time.Duration) []Entry

// GetByID returns a specific entry by ID
func (s *Store) GetByID(id string) *Entry

// Count returns the total number of entries
func (s *Store) Count() int
```

#### QueryOptions

```go
type QueryOptions struct {
    Domain Domain
    Author string
    Type   EntryType
    Tags   []string
    Since  *time.Time
    Limit  int
}
```

#### Feed Generation

```go
// GetFeedPromptSection generates a prompt section from recent knowledge
func GetFeedPromptSection(store *Store) string

// FormatEntryForQuery formats an entry for tool result display
func FormatEntryForQuery(e *Entry) string

// FormatEntriesForQuery formats multiple entries for tool result display
func FormatEntriesForQuery(entries []Entry) string
```

#### Domain Helpers

```go
// DomainFromPersona maps a persona name to their primary domain
func DomainFromPersona(persona string) Domain

// PersonaFromDomain maps a domain to the primary persona
func PersonaFromDomain(domain Domain) string
```

---

## Configuration

The knowledge store is automatically initialized when `PersonaTools` is created:

```go
// In NewPersonaTools
if ks, err := knowledge.NewStore(tronDir); err == nil {
    pt.knowledgeStore = ks
}
```

For Slack handlers, wire the store:

```go
if ks := customTools.GetKnowledgeStore(); ks != nil {
    handler.SetKnowledgeStore(ks)
}
```

---

## Implementation Status

| Phase | Status | Description |
|-------|--------|-------------|
| Phase 1: Core Storage | ✅ Complete | KnowledgeStore, CRUD, persistence |
| Phase 2: Query & Feed | ✅ Complete | Index, tools, feed generation |
| Phase 3: Prompt Injection | ✅ Complete | Slack + server integration |
| Phase 4: Automatic Capture | ⏳ TODO | Auto-capture task results on completion |
| Phase 5: Enhanced Search | ⏳ Future | Full-text and semantic search |
| Phase 6: Knowledge Graph | ⏳ Future | Cross-references and lineage |

See [TODO.md](TODO.md) for full roadmap and implementation details.

---

## Future Enhancements

### Phase 4: Automatic Capture (Next)

Hook `OnProcessComplete` to automatically capture task results:

```go
orch.OnProcessComplete(func(p *vega.Process, result string) {
    if p.Task != "" {
        store.Add(knowledge.Entry{
            Type:    knowledge.TypeTaskResult,
            Author:  p.Agent.Name,
            Title:   summarize(p.Task),
            Content: result,
            Source:  &knowledge.Source{ProcessID: p.ID},
        })
    }
})
```

### Phase 5: Enhanced Search

- Full-text search within content
- Semantic search with vector embeddings
- Relevance scoring based on recency and references

### Phase 6: Knowledge Graph

- Cross-reference related entries
- Track knowledge lineage (insight → decision → result)
- Visualize knowledge flow between agents

### Govega Dependencies

Some enhancements require govega changes:

| Feature | Govega Change | Status |
|---------|---------------|--------|
| Automatic capture | None (use existing `OnProcessComplete`) | Ready |
| Process metadata | Add `Metadata` field to Process | TODO |
| Real-time events | Add event bus | Future |

---

## Changelog

### v1.0.0 (February 2025)

- Initial implementation (Phases 1-3)
- Core storage with 30-day rolling window
- Three tools: `share_knowledge`, `query_knowledge`, `get_knowledge_feed`
- Prompt injection for Slack and server sessions
- Index for fast queries by domain/author/tag/type
- Full documentation
