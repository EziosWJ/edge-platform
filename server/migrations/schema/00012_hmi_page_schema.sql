-- +goose Up
CREATE TABLE hmi_page (
    page_id UUID NOT NULL,
    name VARCHAR(128) NOT NULL,
    description VARCHAR(2048) NOT NULL DEFAULT '',
    draft_document JSONB NOT NULL,
    draft_revision BIGINT NOT NULL DEFAULT 1,
    published_version_id UUID,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
    CONSTRAINT pk_hmi_page PRIMARY KEY (page_id),
    CONSTRAINT ck_hmi_page_name_not_blank CHECK (length(trim(name)) > 0),
    CONSTRAINT ck_hmi_page_draft_revision CHECK (draft_revision > 0)
);

CREATE TABLE hmi_page_version (
    version_id UUID NOT NULL,
    page_id UUID NOT NULL,
    version_no INTEGER NOT NULL,
    source_draft_revision BIGINT NOT NULL,
    document JSONB NOT NULL,
    published_by BIGINT NOT NULL,
    published_at TIMESTAMP WITH TIME ZONE NOT NULL,
    CONSTRAINT pk_hmi_page_version PRIMARY KEY (version_id),
    CONSTRAINT fk_hmi_page_version_page FOREIGN KEY (page_id) REFERENCES hmi_page (page_id) ON DELETE RESTRICT,
    CONSTRAINT fk_hmi_page_version_publisher FOREIGN KEY (published_by) REFERENCES sys_user (id) ON DELETE RESTRICT,
    CONSTRAINT uq_hmi_page_version_no UNIQUE (page_id, version_no),
    CONSTRAINT uq_hmi_page_version_page_id UNIQUE (page_id, version_id),
    CONSTRAINT uq_hmi_page_version_source_revision UNIQUE (page_id, source_draft_revision),
    CONSTRAINT ck_hmi_page_version_no CHECK (version_no > 0),
    CONSTRAINT ck_hmi_page_version_source_revision CHECK (source_draft_revision > 0)
);

ALTER TABLE hmi_page ADD CONSTRAINT fk_hmi_page_published_version
    FOREIGN KEY (page_id, published_version_id) REFERENCES hmi_page_version (page_id, version_id) ON DELETE RESTRICT;
CREATE INDEX idx_hmi_page_updated_at ON hmi_page (updated_at DESC, page_id ASC);

-- +goose Down
DROP INDEX IF EXISTS idx_hmi_page_updated_at;
ALTER TABLE hmi_page DROP CONSTRAINT IF EXISTS fk_hmi_page_published_version;
DROP TABLE IF EXISTS hmi_page_version;
DROP TABLE IF EXISTS hmi_page;
