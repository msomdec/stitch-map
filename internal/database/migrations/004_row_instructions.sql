CREATE TABLE row_instructions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    row_id INTEGER NOT NULL REFERENCES rows(id) ON DELETE CASCADE,
    position INTEGER NOT NULL,
    stitch_id INTEGER REFERENCES stitches(id),
    count INTEGER NOT NULL DEFAULT 1,
    "into" TEXT NOT NULL DEFAULT '',
    is_group INTEGER NOT NULL DEFAULT 0,
    parent_id INTEGER REFERENCES row_instructions(id) ON DELETE CASCADE,
    group_repeat INTEGER NOT NULL DEFAULT 1,
    note TEXT NOT NULL DEFAULT ''
);

-- Use COALESCE to handle NULL parent_id in unique index (SQLite NULLs are distinct in unique indexes).
CREATE UNIQUE INDEX idx_row_instructions_pos ON row_instructions(row_id, COALESCE(parent_id, 0), position);
CREATE INDEX idx_row_instructions_row_id ON row_instructions(row_id);
CREATE INDEX idx_row_instructions_parent_id ON row_instructions(parent_id);
