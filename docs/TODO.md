# Tron Development TODO

This document tracks implementation phases and future work for Tron and its dependencies.

---

## Shared Knowledge System

Cross-agent knowledge sharing so agents can learn from each other's discoveries.

### Phase 1: Core Storage ✅ COMPLETE

- [x] Create `internal/knowledge/` package
- [x] Implement `KnowledgeStore` with CRUD operations
- [x] 30-day rolling window for entries
- [x] File-based persistence (`tron.persona/knowledge/entries.json`)
- [x] Thread-safe with mutex locking

### Phase 2: Query & Feed ✅ COMPLETE

- [x] Implement index building (`index.json`)
- [x] Fast lookup by domain/author/tag/type
- [x] Implement `query_knowledge` tool
- [x] Implement `get_knowledge_feed` tool
- [x] Feed generation with 24-hour window
- [x] ~500 token budget for prompt injection

### Phase 3: Prompt Injection ✅ COMPLETE

- [x] Add `GetFeedPromptSection()` function
- [x] Inject into `internal/slack/handler.go`
- [x] Inject into `internal/server/server.go`
- [x] Wire knowledge store in `cmd/tron/main.go`
- [x] Create `docs/KNOWLEDGE.md` documentation

### Phase 4: Automatic Capture ⏳ TODO

Automatically capture task results when spawned agents complete.

- [ ] Hook `OnProcessComplete` callback in `PersonaTools`
- [ ] Auto-capture task results with process metadata
- [ ] Extract meaningful title from task description
- [ ] Include parent process context in source
- [ ] Auto-capture project creations from `create_project` tool
- [ ] Auto-capture server starts/stops

**Implementation notes:**
- `OnProcessComplete` callback already exists in govega
- Need to filter for meaningful completions (avoid noise)
- Consider adding `metadata` field to spawned processes for richer capture

### Phase 5: Enhanced Search ⏳ FUTURE

Better discovery and retrieval of knowledge.

- [ ] Full-text search within content
- [ ] Semantic search with embeddings (requires vector store)
- [ ] Relevance scoring based on recency and references
- [ ] "Related entries" suggestions
- [ ] Search across multiple time windows

### Phase 6: Knowledge Graph ⏳ FUTURE

Connect related knowledge entries.

- [ ] Cross-reference related entries
- [ ] Track knowledge lineage (this insight led to that decision)
- [ ] Visualize knowledge flow between agents
- [ ] Identify knowledge gaps by domain

---

## Govega Enhancements

Improvements needed in the govega orchestration framework.

### Process Metadata ⏳ TODO

Attach arbitrary metadata to processes for richer tracking.

- [ ] Add `Metadata map[string]any` to `Process` struct
- [ ] Add `WithMetadata(map[string]any)` spawn option
- [ ] Propagate metadata to children on spawn
- [ ] Include metadata in `OnProcessComplete` callback

**Use cases:**
- Track which channel/user initiated a process
- Attach knowledge entry IDs to related processes
- Custom tags for filtering and grouping

### Event Bus ⏳ FUTURE

Real-time event propagation across the system.

- [ ] Design event types (ProcessStarted, ProcessCompleted, KnowledgeShared, etc.)
- [ ] Implement pub/sub mechanism
- [ ] Allow agents to subscribe to events
- [ ] WebSocket endpoint for external subscribers

**Use cases:**
- Real-time knowledge propagation (no polling)
- Live dashboard updates
- Cross-agent notifications

### Budget Improvements ⏳ TODO

Better cost control and reporting.

- [ ] Per-session budget tracking
- [ ] Budget inheritance from parent to child
- [ ] Budget alerts via callback
- [ ] Cost breakdown by tool vs LLM

---

## Life Loop Improvements

Enhancements to the autonomous persona behavior.

### Knowledge Integration ⏳ TODO

Life loops should contribute to shared knowledge.

- [ ] Auto-share insights from news analysis
- [ ] Auto-share decisions from goal planning
- [ ] Auto-share learnings from reflections
- [ ] Query knowledge before making decisions

### Cross-Persona Collaboration ⏳ FUTURE

Personas should coordinate on shared goals.

- [ ] Shared goal tracking across personas
- [ ] Handoff mechanism for cross-domain work
- [ ] Meeting/sync activity between personas
- [ ] Conflict resolution for competing priorities

---

## API & Integration

### Knowledge API ⏳ TODO

HTTP endpoints for knowledge access.

- [ ] `GET /api/knowledge` - List recent entries
- [ ] `GET /api/knowledge/:id` - Get entry by ID
- [ ] `GET /api/knowledge/feed` - Get current feed
- [ ] `POST /api/knowledge` - Add entry (internal use)
- [ ] `GET /api/knowledge/stats` - Knowledge base statistics

### Webhook Notifications ⏳ FUTURE

Push notifications for knowledge events.

- [ ] Webhook registration endpoint
- [ ] Event filtering by domain/author/type
- [ ] Retry logic for failed deliveries
- [ ] Webhook signature verification

---

## Infrastructure

### Persistence ⏳ FUTURE

Move from file-based to database storage for scale.

- [ ] Abstract storage interface
- [ ] SQLite implementation for single-node
- [ ] PostgreSQL implementation for production
- [ ] Migration tooling

### Observability ⏳ TODO

Better visibility into system behavior.

- [ ] Structured logging for knowledge events
- [ ] Metrics for knowledge usage (shares, queries, feed views)
- [ ] Tracing across agent spawns
- [ ] Dashboard for knowledge flow

---

## Completed Work

### January 2024

- ✅ Initial Tron implementation with 5 C-suite personas
- ✅ Slack integration with per-persona handlers
- ✅ Voice integration (VAPI, ElevenLabs)
- ✅ Life loop system for autonomous behavior
- ✅ Spawn tree visualization
- ✅ Control panel API

### February 2024

- ✅ Shared knowledge system (Phases 1-3)
  - Knowledge store with 30-day rolling window
  - Three tools: share_knowledge, query_knowledge, get_knowledge_feed
  - Automatic prompt injection in Slack and server sessions
  - Full documentation in docs/KNOWLEDGE.md

---

## Priority Matrix

| Priority | Item | Effort | Impact |
|----------|------|--------|--------|
| **High** | Phase 4: Automatic Capture | Medium | High |
| **High** | Knowledge API endpoints | Low | Medium |
| **Medium** | Process Metadata in govega | Medium | High |
| **Medium** | Life Loop Knowledge Integration | Medium | Medium |
| **Low** | Phase 5: Enhanced Search | High | Medium |
| **Low** | Event Bus | High | High |
| **Low** | Phase 6: Knowledge Graph | High | Medium |

---

## Notes

### Design Principles

1. **Explicit over implicit** - Agents actively share knowledge, not just logging
2. **Compact feeds** - Keep prompt injection under 500 tokens
3. **30-day relevance** - Old knowledge fades, recent knowledge surfaces
4. **Attribution matters** - Always track who learned what, when

### Known Limitations

1. No real-time propagation - agents see knowledge on next session start
2. No semantic search - queries are exact match on fields
3. Single-node only - file-based storage doesn't scale horizontally
4. No conflict resolution - multiple agents can share contradictory knowledge
