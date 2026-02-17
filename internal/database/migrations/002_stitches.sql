CREATE TABLE IF NOT EXISTS stitches (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    abbreviation TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    is_builtin INTEGER NOT NULL DEFAULT 0
);

-- Built-in stitches have user_id IS NULL; custom stitches are unique per user.
CREATE UNIQUE INDEX IF NOT EXISTS idx_stitches_builtin_abbr
    ON stitches(abbreviation) WHERE user_id IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_stitches_user_abbr
    ON stitches(user_id, abbreviation) WHERE user_id IS NOT NULL;

-- Seed built-in stitches.
INSERT OR IGNORE INTO stitches (user_id, name, abbreviation, description, is_builtin) VALUES
    (NULL, 'Chain',                     'ch',    'Foundation stitch; yarn over, pull through loop',      1),
    (NULL, 'Slip Stitch',              'sl st', 'Join or move yarn without adding height',              1),
    (NULL, 'Single Crochet',           'sc',    'Short stitch; insert, yarn over, pull through twice',  1),
    (NULL, 'Half Double Crochet',      'hdc',   'Medium height; yarn over before inserting',            1),
    (NULL, 'Double Crochet',           'dc',    'Tall stitch; yarn over, insert, three pull-throughs',  1),
    (NULL, 'Treble Crochet',           'tr',    'Extra tall; yarn over twice before inserting',         1),
    (NULL, 'Magic Ring',               'MR',    'Adjustable starting loop for working in the round',   1),
    (NULL, 'Increase',                 'inc',   'Two single crochets worked into the same stitch',     1),
    (NULL, 'Decrease',                 'dec',   'Single crochet two together (sc2tog)',                 1),
    (NULL, 'Front Post Double Crochet','FPdc',  'dc worked around the front of previous row''s post',  1),
    (NULL, 'Back Post Double Crochet', 'BPdc',  'dc worked around the back of previous row''s post',   1);
