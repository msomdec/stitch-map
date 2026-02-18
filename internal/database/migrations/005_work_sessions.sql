CREATE TABLE work_sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    pattern_id INTEGER NOT NULL REFERENCES patterns(id) ON DELETE CASCADE,
    started_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    last_active_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    completed_at TEXT
);

-- Only one active (non-completed) session per user per pattern.
CREATE UNIQUE INDEX idx_work_sessions_active ON work_sessions(user_id, pattern_id)
    WHERE completed_at IS NULL;

CREATE INDEX idx_work_sessions_user ON work_sessions(user_id);

CREATE TABLE work_progress (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER UNIQUE NOT NULL REFERENCES work_sessions(id) ON DELETE CASCADE,
    section_id INTEGER NOT NULL REFERENCES pattern_sections(id),
    row_id INTEGER NOT NULL REFERENCES rows(id),
    row_repeat_index INTEGER NOT NULL DEFAULT 0,
    instruction_id INTEGER NOT NULL REFERENCES row_instructions(id),
    stitch_index INTEGER NOT NULL DEFAULT 0,
    group_repeat_index INTEGER NOT NULL DEFAULT 0,
    stitches_completed_in_row INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);
