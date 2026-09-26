-- SQLite migration is retained for local/test compatibility only.
-- +goose Up
CREATE TABLE hmi_page (
    page_id TEXT NOT NULL PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    draft_document TEXT NOT NULL,
    draft_revision INTEGER NOT NULL DEFAULT 1,
    published_version_id TEXT,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    CONSTRAINT ck_hmi_page_name_not_blank CHECK (length(trim(name)) > 0),
    CONSTRAINT ck_hmi_page_draft_revision CHECK (draft_revision > 0)
);
CREATE TABLE hmi_page_version (
    version_id TEXT NOT NULL PRIMARY KEY,
    page_id TEXT NOT NULL,
    version_no INTEGER NOT NULL,
    source_draft_revision INTEGER NOT NULL,
    document TEXT NOT NULL,
    published_by INTEGER NOT NULL,
    published_at DATETIME NOT NULL,
    CONSTRAINT fk_hmi_page_version_page FOREIGN KEY (page_id) REFERENCES hmi_page (page_id) ON DELETE RESTRICT,
    CONSTRAINT fk_hmi_page_version_publisher FOREIGN KEY (published_by) REFERENCES sys_user (id) ON DELETE RESTRICT,
    CONSTRAINT uq_hmi_page_version_no UNIQUE (page_id, version_no),
    CONSTRAINT uq_hmi_page_version_page_id UNIQUE (page_id, version_id),
    CONSTRAINT uq_hmi_page_version_source_revision UNIQUE (page_id, source_draft_revision),
    CONSTRAINT ck_hmi_page_version_no CHECK (version_no > 0),
    CONSTRAINT ck_hmi_page_version_source_revision CHECK (source_draft_revision > 0)
);
CREATE INDEX idx_hmi_page_updated_at ON hmi_page (updated_at DESC, page_id ASC);

-- +goose Down
DROP INDEX IF EXISTS idx_hmi_page_updated_at;
DROP TABLE IF EXISTS hmi_page_version;
DROP TABLE IF EXISTS hmi_page;
